# ARCHITECTURE — ブラウザ自動操作 & ローカル監査プロキシ基盤

## 1. システム構成

```text
 ┌──────────────────────────── Chrome / Chromium ─────────────────────────────┐
 │                                                                             │
 │  content.js (各タブ, isolated world)        background.js (MV3 service worker)
 │  ・Vimium C 方式の可視要素収集              ・chrome.runtime.connectNative  │
 │  ・data-webcli-hint 付与                    ・chrome.tabs / scripting       │
 │  ・クリック/フォーカス合成イベント           ・chrome.declarativeNetRequest  │
 │        ▲ chrome.tabs.sendMessage            ・chrome.cookies                │
 │        └──────────────────────────────────────┘      │ stdio                │
 │                                                      │ [uint32 長 (native endian)][JSON]
 │   ページのHTTP(S)通信 ── X-Audit-Action-Id (DNR) ──┐ │                      │
 └────────────────────────────────────────────────────┼─┼──────────────────────┘
                                                      │ ▼
                      --proxy-server=127.0.0.1:8080   │ nm-host (Go, Chromeが起動)
                                                      │  ・フレーム ⇄ NDJSON の透過ブリッジ
                                                      │ │ Unix Domain Socket
                                                      │ │ /tmp/my_browser_ipc.sock (0600)
                                                      ▼ ▼  1行 = 1 JSON メッセージ
 ┌──────────────────────────── cmd/app (Go, TTY 常駐) ───────────────────────────┐
 │ internal/ipc   Hub: セッション管理 / id 相関の Request-Reply / ping 自動応答    │
 │ internal/proxy goproxy MITM + 動的CA + キャプチャ + ルール(遮断/改変)          │
 │                + X-Audit-Action-Id 検出 → 紐付け → 除去                         │
 │ internal/audit Store(JSONL) / Action / .http 生成 / Cookie JSON                 │
 │ REPL           hints → os/exec fzf (stdin に候補, UI は /dev/tty) → click       │
 └────────────────────────────────────┬──────────────────────────────────────────┘
                                      │ 上流へ転送 (X-Audit-Action-Id は除去済み)
                                      ▼
                               インターネット / 対象サーバ
```

### 言語スタック

| 領域 | 言語 | 実体 |
|---|---|---|
| CLI / IPC / Native Messaging Host / プロキシ / ログ / ファイル出力 | **Go 100%** | `cmd/app`, `cmd/nm-host`, `internal/*` |
| ブラウザ内（chrome.* API が必要な最小限） | JavaScript (MV3) | `extension/background.js`, `extension/content.js` |
| 自動化スクリプト | bash + Go | `scripts/*.sh`, `scripts/e2e`（Go） |

バックエンドに Node.js / Python は一切使用していない。

### 既存資産の活用（車輪の再発明をしない）

| 機能 | 採用したもの | 自前実装しなかったもの |
|---|---|---|
| HTTP/HTTPS プロキシ、CONNECT、TLS 終端、ホスト別証明書の署名 | `github.com/elazarl/goproxy` v1.9.1 | プロキシプロトコル全般 |
| 選択 UI / あいまい検索 | システムの `fzf`（`os/exec` + パイプ） | TUI |
| 可視判定 | Vimium C の手法（`getClientRects`/`getBoundingClientRect` をビューポートでクリップ → `getComputedStyle` → `elementFromPoint` による遮蔽判定）、クリック合成イベント列（over→down→focus→up→click） | 独自のヒューリスティクス |
| リクエストへのヘッダ付与 | `chrome.declarativeNetRequest` のセッションルール（MV3 標準） | webRequest でのブロッキング書き換え |
| E2E でのブラウザ操作 | `github.com/chromedp/chromedp`（テスト専用） | CDP クライアント |

## 2. ディレクトリ

```text
cmd/app/            常駐 CLI（main.go: 起動/REPL, commands.go: コマンド, fzf.go: fzf 連携）
cmd/nm-host/        Native Messaging Host（Chrome ⇄ UDS ブリッジ）
internal/ipc/       nativemsg.go: 4バイト長フレーミング / conn.go: NDJSON 接続 / server.go: Hub
internal/proxy/     ca.go: 動的CA / proxy.go: goproxy 統合・キャプチャ / rules.go: 遮断・改変ルール
internal/audit/     store.go: 記録・JSONL / httpfile.go: .http 生成 / cookies.go: Cookie 形式
extension/          manifest.json（固定 key → 固定拡張 ID）, background.js, content.js
scripts/            build.sh, install-host.sh, extension-id.sh, proxy-demo.sh, test-tty.sh, test-all.sh, e2e/
docs/               本書, PROGRESS.md, evidence/（テスト実行ログ）
```

## 3. IPC プロトコル仕様

### 3.1 レイヤ1: Chrome Native Messaging（拡張 ⇄ nm-host, stdio）

```
+----------------------+---------------------------+
| length: uint32       | UTF-8 JSON (length bytes) |
| (ネイティブエンディアン) |                           |
+----------------------+---------------------------+
```

- 実装: `ipc.ReadNativeMessage` / `ipc.WriteNativeMessage`（`encoding/binary.NativeEndian`）。
- 上限: Chrome → ホスト 64 MiB、ホスト → Chrome 1 MB（超過時は `error` を返し送信しない）。
- ヘッダと本文は 1 回の `Write` で出力（ミューテックス下）し、フレームの混在を防ぐ。
- stdin の EOF = Chrome がポートを切断 → nm-host は正常終了。
- stdout はプロトコル専用。診断ログは stderr（Chrome のログに転送）と任意で `$WEBCLI_NMHOST_LOG`。
- Chrome は `argv[1]` に `chrome-extension://<id>/` を渡す。nm-host はこれを `hello` で app に通知する。

### 3.2 レイヤ2: nm-host ⇄ app（Unix Domain Socket, NDJSON）

- パス: `/tmp/my_browser_ipc.sock`（`-socket` フラグ / `WEBCLI_SOCKET` 環境変数で変更可）、パーミッション `0600`。
- 1 行 = 1 JSON 値（改行区切り）。nm-host は **中身を解釈せず** フレーム ⇄ 行を相互変換する。
- app 起動時、生きているソケットがあればエラー（二重起動防止）、死んだソケットファイルは削除して再作成。
- nm-host は app に最大 5 秒リトライで接続。接続できなければ拡張に `error`（"web-cli app is not running"）を返して終了し、background.js は指数バックオフ（1s→30s）で再接続する。

### 3.3 メッセージエンベロープ（全ホップ共通）

```json
{ "id": "5f0c…", "type": "click", "payload": { "hint": 2, "tag": true }, "error": "" }
```

| フィールド | 説明 |
|---|---|
| `id` | リクエスト相関 ID（128bit hex / UUID）。返信は同じ `id` を持つ。通知は省略。 |
| `type` | メッセージ種別（下表） |
| `payload` | 種別ごとの JSON |
| `error` | `type:"error"` のときのエラーメッセージ |

### 3.4 メッセージ種別

| type | 方向 | payload | 返信 |
|---|---|---|---|
| `hello` | nm-host → app | `{"origin":"chrome-extension://…/","pid":123}` | なし |
| `ping` | 拡張 → app | `{"from":"extension","version":"0.1.0"}` | `pong` `{"from":"app"}`（Hub が即時応答） |
| `ping` | app → 拡張 | `{"from":"app"}` | `pong` `{"from":"extension"}` |
| `log` | 拡張 → app | 任意 JSON（例 `{"event":"pong_received"}`） | なし（app が表示） |
| `collect` | app → 拡張 | なし | `elements` `{"ok":true,"url","title","elements":[{hint,tag,type?,role?,text,href?,name?,id?,rect:{x,y,w,h}}]}` |
| `click` | app → 拡張 | `{"hint":2,"tag":true}`（`tag`: プロキシ稼働中ならアクション ID ヘッダを付与） | `click_result` `{"action_id":"<UUID>","kind":"click"\|"focus","hint","tag","text","url"}` |
| `cookies_get` | app → 拡張 | `{"url"?: "https://…"}`（省略時アクティブタブ） | `cookies` `{"url","cookies":[chrome.cookies.Cookie…]}` |
| `cookies_set` | app → 拡張 | `{"cookies":[chrome.cookies.SetDetails…]}` | `cookies_set_result` `{"set":1,"errors":[]}` |
| `dom` | app → 拡張 | `{"op", "target"?: {"hint":n}\|{"css":"…"}, "tag", …}`。op: `click` `focus` `type{text}` `clear` `select{value}` `check{on}` `press{key}` `submit` `scroll{to}` `text` `html` `exists{text}` `info` `storage_get` `storage_set{local,session}` | `dom_result` `{op, action_id?（変更系のみ）, kind, tag, type, text, selector, url, title, value, html, local, session, …}` |
| `collect` | app → 拡張 | `{"scope":"page"}` でビューポート外も対象（名前による要素指定の解決用） | `elements`（同上） |
| `navigate` | app → 拡張 | `{"action":"open"\|"back"\|"forward"\|"reload","url"?,"new_tab"?,"tag"}` | `nav_result` `{action, action_id, tab_id, url, title}`（読み込み完了後） |
| `tabs` | app → 拡張 | `{"op":"list"\|"select"\|"close","id"?}` | `tabs_result` `{"tabs":[{id,window_id,active,url,title}]}` |
| `eval` | app → 拡張 | `{"code":"…"}` | `eval_result` `{type, value, via:"main-world"\|"debugger"}` |
| `screenshot` | app → 拡張 | なし | `screenshot_result` `{url, title, png_base64}` |
| `record` | app → 拡張 | `{"on":bool,"secrets":bool}` | `record_result` `{on, secrets, url, title}` |
| `record_event` | 拡張 → app | `{op:"click"\|"type"\|"select"\|"check"\|"uncheck"\|"press", selector, tag, type, text, url, value?, secret, key?}` | なし（app が記録ファイルへ書く） |
| `error` | どちらでも | — | `{"id":"<要求 id>","type":"error","error":"…"}` |

### 3.4.1 ページ操作の実装メモ

- **入力**: `type` / `select` は、要素の prototype が持つ value セッターで値を設定し、`input` / `change` イベントを発火する。React / Vue などのフレームワークが監視している値の変更として扱われる。content script は隔離された実行環境（isolated world）で動くため、ページ側の JS に上書きされない。
- **キー**: 合成したキーイベントでは、ブラウザ本来の動作（Enter でフォーム送信など）が起きない。そのため `press` はイベント発火に加えて、本来の動作を明示的に実行する。
  - Enter: 入力欄なら `form.requestSubmit(既定の送信ボタン)`、ボタンやリンクならクリック
  - Tab: フォーカスを次の要素へ移す
  - Space: チェックボックスやボタンをクリック
- **要素指定**:
  - 名前で指定した場合: `collect{scope:"page"}` の一覧から `fzf --filter` で最良一致を選ぶ。ラベルは `<label for>`、要素を囲む `<label>`、`aria-labelledby` から取得する。
  - 見つからない場合: 要素が現れるまで最大 10 秒待つ（`timeout` で変更可）。
- **記録**: `isTrusted` のイベント（人の操作）だけを記録する。web-cli 自身の合成イベントは二重に記録しない。
  - セレクタ: id → name / data-testid / aria-label / placeholder → nth-of-type のパス、の順で一意になるものを使う。
  - パスワード: 値は保存せず `$WEBCLI_PASSWORD` と書く。
- **back / forward**:
  - `chrome.tabs.goBack` はブラウザの戻るボタンと同じく、ユーザー操作のなかったページを飛ばす（Chrome の history manipulation intervention）。自動操作で開いたページはすべて該当するため、数ページ前まで戻ってしまう。
  - そのため、ページ内の `history.back()` / `history.forward()` で 1 つずつ移動する。
- **eval**: まずページの MAIN world で間接 eval を実行する。CSP で eval が禁止されているページでは、`chrome.debugger` の `Runtime.evaluate` にフォールバックする。
- **本文**: プロキシが記録した本文は gzip / deflate / br / zstd を展開して表示する（`show` / `body`）。

### 3.5 シーケンス

**疎通（Phase 1）**
```
background.js ─connectNative→ Chrome ─spawn(argv[1]=origin)→ nm-host ─dial UDS→ app
nm-host → app : {"type":"hello",…}
background.js → nm-host → app : {"id":A,"type":"ping"}
app(Hub) → nm-host → background.js : {"id":A,"type":"pong","payload":{"from":"app"}}
background.js → app : {"type":"log","payload":{"event":"pong_received"}}
```

**要素選択とクリック（Phase 2 / 4）**
```
user: hints [query]
app → ext   : collect                     → content.js: 可視要素を収集・data-webcli-hint 付与
ext → app   : elements
app         : fzf（stdin に "hint\t[kind]\ttext\thref"、UI は /dev/tty、--with-nth=2..）
app → ext   : click {hint, tag}
background  : actionId = crypto.randomUUID()               ← クリック直前に生成
background  : DNR セッションルール(id=1, tabIds=[tab], 全 resourceType) で
              X-Audit-Action-Id: actionId を 3 秒間付与
content.js  : 応答を返してから次タスクで over→down→focus→up→click
ext → app   : click_result {action_id,…} → audit.Store.AddAction
page → proxy: X-Audit-Action-Id 付きリクエスト → Exchange.ActionID に記録 → ヘッダ除去 → 上流へ
```

## 4. content.js 可視判定（Vimium C 方式）

対象セレクタ: `a, button, input, select, textarea, [role=button], [onclick]`

1. `disabled` 要素、`input[type=hidden]` を除外。
2. `getComputedStyle`: `visibility` が `hidden`/`collapse`、`display:none` を除外。`opacity` は継承されないため **祖先まで** `opacity === "0"` を確認。
3. `getBoundingClientRect()` が幅・高さ > 0 でビューポートと交差すること。さらに Vimium C と同様に `getClientRects()` の各矩形をビューポートでクリップし、1px 以上残る最初の矩形を採用（折り返しリンク対策）。
4. `document.elementFromPoint()`（open shadow root は掘り下げ）を矩形の中心 → 内側 4 隅の順に試し、ヒット要素が対象自身・子孫・対象を `control` とする `<label>` であれば可視。すべて失敗なら「モーダル等の背後」として除外。

クリック: テキスト入力系・`select`・`textarea`・contenteditable は `focus`、それ以外は Vimium C と同じ合成イベント列（`pointerover/mouseover → pointerdown/mousedown → focus → pointerup/mouseup → click`）。ナビゲーションでメッセージチャネルが破棄されないよう、**応答を先に返してから** `setTimeout(0)` で実行する。

多重注入防止（manifest の content_scripts と `chrome.scripting.executeScript` フォールバック）に `globalThis.__webcliContentLoaded` ガードを使う。

## 5. プロキシ（internal/proxy）

- **goproxy** の `OnRequest().HandleConnect` で全 CONNECT を `ConnectMitm`、`TLSConfigFromCA` でホスト別証明書を動的署名、`CertStore` で署名結果をキャッシュ。
- **動的 CA**: 初回起動時に ECDSA P-256 のルート CA を `-ca-dir`（既定 `~/.config/web-cli`）に生成（`ca.pem` 0644 / `ca-key.pem` 0600、5 年、`MaxPathLenZero`）。以降は再利用。`http://<proxy>/ca.pem` で配布、SPKI ハッシュを起動ログと `ca` コマンドで表示（Chrome の `--ignore-certificate-errors-spki-list` 用）。
- **上流 TLS 検証**: goproxy の既定（検証なし）を上書きし、**システムルートで検証**する。`-upstream-ca` で追加 CA、`-insecure-upstream` はテスト専用。
- **キャプチャ**: リクエストボディは最大 `MaxBody`（4 MiB）を保持し、それ以上も上流へは全量転送（`Truncated` を記録）。レスポンスボディはクライアントへストリーミングしながら tee で保持し、EOF/Close 時に記録を確定（SSE 等でも遅延しない）。
- **アクション紐付け**: `X-Audit-Action-Id` を検出 → `Exchange.ActionID` に記録 → `Header.Del` してから転送。記録されるヘッダは「実際に上流へ送ったもの」（ルール適用後・監査ヘッダなし）。
- **ルール**（`RuleSet`、実行時に REPL から追加/削除、`-rules file.json` で起動時ロード）:

| action | 効果 | 必須 |
|---|---|---|
| `block` | ローカルで `status`（既定 403）を返し上流へ送らない | — |
| `set-req-header` / `del-req-header` | 上流へ送る前にリクエストヘッダを設定/削除 | `name`（set は `value`） |
| `set-resp-header` / `del-resp-header` | クライアントへ返す前にレスポンスヘッダを設定/削除 | `name` |
| `replace-body` | レスポンスボディを `value` に置換 | — |

マッチ条件（すべて AND、空は無条件）: `host`（ホスト名への正規表現）、`url`（完全 URL への正規表現）、`method`（完全一致）。

REPL 構文: `rule add block host=^ads\. status=451` / `rule add set-req-header url=/api/ name=X-Debug value="1"`

JSON（`-rules` / `rule load`）:
```json
[
  {"action": "block", "host": "^ads\\.example\\.com$", "status": 451},
  {"action": "set-req-header", "url": "/api/", "name": "X-Debug", "value": "1"}
]
```

## 6. 出力フォーマット

### 6.1 `.http`（kulala.nvim / VS Code REST Client）

RFC 7230 のメッセージ形式（request-line / header-field / 空行 / message-body）で、リクエスト間は `###` で区切る。

```http
### #2 POST https://app.example/api/login
# @name req_2
# action: d6c02512-… click hint=2 <button> "Login" page=https://app.example/
# response: 200 OK, application/json, 27 bytes
POST https://app.example/api/login HTTP/1.1
Content-Type: application/json
Cookie: session=abc123

{"user":"alice"}
```

- hop-by-hop ヘッダ（`Connection`, `Keep-Alive`, `Proxy-*`, `TE`, `Trailer`, `Transfer-Encoding`, `Upgrade`）、再計算される `Content-Length`、絶対 URI で自明な `Host`、`Accept-Encoding`、`X-Audit-Action-Id` は出力しない。
- gzip/deflate のリクエストボディは展開して出力。バイナリはコメントで省略を明記。本文中の行頭 `###` は区切りと誤認されないようエスケープ。
- `export [file] [action=<id>] [host=<regexp>]` でアクション単位・ホスト単位に絞り込み可能。

### 6.2 監査ログ `audit.jsonl`

1 行 1 レコード。`{"record":"exchange", …, "request_body":{encoding,data,content_decoded?}, "response_body":{…}}` と `{"record":"action","id","kind","hint","tag","text","page_url",…}`。ボディは UTF-8 ならそのまま、それ以外は base64、gzip/deflate は可読化のため展開。

### 6.3 Cookie JSON

`chrome.cookies.Cookie` と同じフィールド。ファイルは 0600 で保存。インポートは `{url?, cookies:[…]}` 形式と、一般的な Cookie エクスポート拡張の「素の配列」形式の両方を受け付ける。Go 側で `chrome.cookies.SetDetails` に変換（host-only Cookie は `domain` を付けない、セッション Cookie は `expirationDate` を付けない、`secure` に応じた `url` を合成）してから拡張へ送る。

## 7. セキュリティ上の考慮

- UDS は 0600。CA 秘密鍵・Cookie ダンプ・.http・JSONL はすべて 0600。
- `X-Audit-Action-Id` は **app がプロキシを起動しているときだけ** 付与を指示（`click.tag`）。プロキシは必ず除去する。ブラウザがこのプロキシを経由していない状態で `tag` が有効だと外部に漏れるため、Chrome は必ず `--proxy-server` 付きで起動すること。
- ヘッダ付与ルールは対象タブに限定し、3 秒または次のアクションで解除する。
- 生成 CA は信頼ストアへ自動登録しない（利用者が明示的に登録 or SPKI 指定）。
- nm-host は `allowed_origins` で固定拡張 ID のみに限定（manifest の `key` から ID を導出: `scripts/extension-id.sh`）。

## 8. 利用手順（実機）

```bash
scripts/build.sh                       # bin/app, bin/nm-host
scripts/install-host.sh                # NativeMessagingHosts/com.share026.webcli.json を登録
# chrome://extensions → デベロッパーモード → 「パッケージ化されていない拡張機能を読み込む」→ extension/
bin/app                                # TTY で起動（プロキシ 127.0.0.1:8080）
google-chrome --proxy-server=127.0.0.1:8080 \
  --ignore-certificate-errors-spki-list=<app の起動ログに出る SPKI>   # もしくは ca.pem を信頼ストアへ
web-cli> hints            # fzf で選んでクリック
web-cli> export           # audit-out/requests.http
web-cli> cookies dump     # audit-out/cookies.json
```

補足（実ブラウザで確認済みの注意点）:

- `--user-data-dir` を指定した場合、Linux の Chromium はホスト定義を `<user-data-dir>/NativeMessagingHosts/` から探す（`scripts/install-host.sh <user-data-dir>`）。
- Chromium 137 以降でコマンドラインから拡張機能を読み込む場合は `--disable-features=DisableLoadExtensionCommandLineSwitch` が必要。
- ブラウザは自分の stderr を nm-host に引き継ぐ。stderr が読み手のいないパイプでも nm-host が落ちないよう、SIGPIPE を無視している。ログは `WEBCLI_NMHOST_LOG` に確実に残る。
- 動作確認済み: CloakBrowser 146.0.7680.177（Chromium 146）、real E2E 57/57（画面あり・`--headless` とも、`docs/evidence/ci/`）。
