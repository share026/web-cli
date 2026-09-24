# PROGRESS — 実行記録とテストエビデンス

最終更新: 2026-09-24 (UTC) / ブランチ `arena/01a0d0b5-web-cli`

| Phase | 内容 | 状態 | 主な検証 |
|---|---|---|---|
| 0 | ツールチェーン整備・プロジェクト雛形 | ✅ 完了 | `go build ./...`, `go vet ./...` |
| 1 | Native Messaging (Go) & IPC 基盤 | ✅ 完了 | ユニット 5件 + nm-host 実バイナリ 2件 + E2E 5項目 |
| 2 | Vimium C 方式 DOM 収集 & fzf (TTY) 連携 | ✅ 完了 | fzf 実バイナリ 2件（filter / PTY 対話）+ E2E 10項目 |
| 3 | Go 100% ローカル MITM プロキシ | ✅ 完了 | httptest+実 curl 4件 + github.com 実通信デモ + E2E 8項目 |
| 4 | コンテキスト紐付け・.http・Cookie | ✅ 完了 | ユニット 3件 + E2E 11項目（curl で .http 再生を含む） |

**総合結果: `scripts/test-all.sh` → `ALL CHECKS PASSED`（ローカル + GitHub Actions 上の実ブラウザ CloakBrowser）**

### 実ブラウザ検証（CloakBrowser / GitHub Actions）

| 項目 | 内容 |
|---|---|
| ブラウザ | **CloakBrowser** `chromium-v146.0.7680.177.5`（Chromium 146.0.7680.177、公式 Release の `cloakbrowser-linux-x64.tar.gz`、GitHub API の digest `sha256 4a12bcde…670e` と `sha256sum -c` で一致確認） |
| 実行場所 | `.github/workflows/e2e-cloakbrowser.yml`（ubuntu-latest + Xvfb、headed）。サンドボックスからは配布元（cloakbrowser.dev / GitHub のアセット CDN）に到達できないため、GitHub のランナーでダウンロード・検証・テストし、エビデンスをブランチへ自動コミットする方式にした |
| 方式 | `-mode real`: Chromium 自身が `--load-extension=extension/` で拡張機能を読み込み、MV3 service worker が `chrome.runtime.connectNative` で `install-host.sh` 登録済みの **本物の `bin/nm-host` をブラウザが起動**。chrome.* の代替・CDP エミュレーションは一切なし |
| 結果 | **E2E real 32/32 PASS**、emulated 31/31 PASS、Go テスト全 PASS（`-race`）、PTY fzf PASS、curl プロキシデモ PASS |
| エビデンス | [`evidence/ci/`](evidence/ci/)（`environment.txt` に run URL・コミット・ブラウザ版・digest、`e2e-report-real.md` 判定表、`e2e-sw-console-real.log` = service worker の console、`e2e-nm-host-real.log`、`sample-requests-real.http`、`sample-cookies-real.json` ほか） |

実ブラウザで確認できたもの（`evidence/ci/e2e-report-real.md` より）: ping→pong（app→拡張の往復 約1.2ms）、Vimium C 方式の可視判定で期待 7 要素ちょうど、fzf 選択→クリック、クリック前に生成した UUID が declarativeNetRequest で `X-Audit-Action-Id` として付与され、プロキシで紐付け後に除去されて上流には届かないこと、`chrome.cookies.getAll/set` による HttpOnly/Secure Cookie のダンプ・インポート、ルールによるヘッダ注入・451 遮断、main_frame 遷移への action 付与、遷移後の content script 再注入、`.http` の出力と curl 再生。
（Go テスト 15 PASS / 1 SKIP※ を `-race` 付きで実行、PTY 上の fzf 対話 PASS、curl プロキシデモ PASS、E2E 31/31 PASS）
※ SKIP は PTY 専用テストで、通常の `go test` ではスキップし `scripts/test-tty.sh` が PTY 上で実行して PASS している。

エビデンス（すべて `scripts/test-all.sh` で再生成可能）:

| ファイル | 内容 |
|---|---|
| [`evidence/ci/`](evidence/ci/) | **実ブラウザ CloakBrowser** での同一スクリプト実行結果（GitHub Actions、emulated + real） |
| [`evidence/build.log`](evidence/build.log) | Go バージョン、`go vet` + `go build ./...` + バイナリビルド |
| [`evidence/go-test.log`](evidence/go-test.log) | `go test -race -count=1 -v ./...` の全出力 |
| [`evidence/tty-fzf.log`](evidence/tty-fzf.log) | 擬似端末上の対話型 fzf（"check" を入力 + Enter → hint 7 を選択） |
| [`evidence/proxy-curl.log`](evidence/proxy-curl.log) | `curl -x` で実サイト (github.com) を MITM：証明書チェーン、ルール、遮断、.http 出力 |
| [`evidence/e2e.log`](evidence/e2e.log) | E2E ハーネスの全ログ（nm-host の stderr、background.js の console を含む） |
| [`evidence/e2e-report.md`](evidence/e2e-report.md) | E2E 31 項目の判定表 |
| [`evidence/e2e-app-transcript.log`](evidence/e2e-app-transcript.log) | E2E 中の `app` の標準出力/標準エラー全文 |
| [`evidence/sample-requests.http`](evidence/sample-requests.http) | E2E で出力された .http（kulala.nvim / REST Client 用） |
| [`evidence/sample-cookies.json`](evidence/sample-cookies.json) | E2E で出力された Cookie ダンプ |

---

## Phase 0: 環境整備

サンドボックスのネットワーク制約（`go.dev` / `proxy.golang.org` / `golang.org` / GitHub Releases / apt が不通、GitHub の git と npm / PyPI は可）を自律的に回避した。

| 必要物 | 入手方法 | 備考 |
|---|---|---|
| Go 1.27.1 | PyPI の `go-bin` wheel（公式バイナリ同梱）を展開 | `~/.local`（リポジトリ外） |
| Go モジュール | `GOPROXY=direct` で GitHub から取得。`golang.org/x/*` と `gopkg.in/yaml.v3` は GitHub ミラーから **GOPROXY 形式の file:// ミラー**をローカル構築 | `go.mod` に `replace` は入れていない。生成された `go.sum` の h1 ハッシュが goproxy / chromedp 本家の `go.sum` と **完全一致**することを確認済み（x/net, x/text, x/sys, yaml.v3） |
| fzf 0.74 | `go install github.com/junegunn/fzf@latest`（ソースからビルド） | システムの `fzf` として `PATH` に配置 |
| Chromium 153 | npm `@sparticuz/chromium` の同梱バイナリ（headless shell） | ローカルの emulated E2E 用（拡張機能を読み込めないビルド、§E2E 方式） |
| CloakBrowser 146 | 公式 GitHub Release を **GitHub Actions ランナー上で**取得し digest 検証 | 実ブラウザ E2E（real モード）用。再配布禁止ライセンスのためリポジトリには含めない |

出所不明の非公式 Chrome バイナリ（npm の匿名パッケージ）は安全上の理由で採用しなかった。

## Phase 1: Native Messaging & IPC

### 実装
- `internal/ipc/nativemsg.go`: 4 バイト長（`binary.NativeEndian`）+ JSON の読み書き。上限（→Chrome 1MB / ←Chrome 64MiB）、切り詰めフレーム、不正 JSON を検出。
- `internal/ipc/conn.go` / `server.go`: UDS 上の NDJSON。`Hub` がセッション管理、`id` 相関の Request/Reply、拡張からの `ping` への即時 `pong`、`hello` 記録、通知イベント、二重起動防止、死んだソケットの掃除、ソケット 0600。
- `cmd/nm-host`: Chrome ⇄ UDS 透過ブリッジ。`argv[1]` の origin を `hello` で通知。app 不在時は `error` を返して終了。stdin EOF で正常終了。
- `extension/background.js`: `connectNative` → 起動時に `{"type":"ping"}`、`pong` 受領でログ + app へ `pong_received` 通知、切断時は指数バックオフ再接続。
- `scripts/install-host.sh`: ホスト定義 JSON を配置（`allowed_origins` は manifest の `key` から導出した固定 ID）。

### テスト（抜粋: `evidence/go-test.log`）
```
--- PASS: TestNativeMessageRoundTrip
--- PASS: TestNativeMessageErrors
--- PASS: TestHubPingPongAndRequest
--- PASS: TestHubDisconnectFailsPending
--- PASS: TestListenRejectsLiveSocket
=== RUN   TestBridgeLikeChrome        ← 実 nm-host バイナリを Chrome と同じく子プロセス+stdio で起動
    main_test.go:62: browser received: {"id":"x1","type":"pong","payload":{"from":"app"}}
    [nm-host] nm-host start pid=… origin="chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/"
    [nm-host] connected to app at …/ipc.sock
    [nm-host] bridge stopped: browser closed stdin (port disconnected)
--- PASS: TestBridgeLikeChrome
--- PASS: TestAppNotRunning           ← app 不在時に拡張へ error を返す
```

### E2E（`evidence/e2e-app-transcript.log`）
```
[ipc] session #1 connected
[ipc] session #1 hello origin=chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/ pid=14935
[ipc] session #1 ping -> pong
[ext#1] {"event":"pong_received","reply_to":"c2cedeeeec664d80be23f3e03f64369c"}
pong from extension in 845µs: {"from":"extension"}
```
ハーネス側ログ: `native host manifest OK: …/chrome-profile/NativeMessagingHosts/com.share026.webcli.json -> …/bin/nm-host (allowed_origins=[chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/])`、`[background.js console] [web-cli] pong received from app {"from":"app"}`

## Phase 2: Vimium C 方式 DOM 収集 & fzf

### 実装
- `extension/content.js`: 対象 `a, button, input, select, textarea, [role=button], [onclick]`。① `getBoundingClientRect`/`getClientRects` をビューポートでクリップ（幅・高さ > 0）、② `getComputedStyle`（visibility / display / opacity、opacity は祖先まで）、③ `elementFromPoint`（中心 + 内側 4 隅、shadow DOM 対応、label→control 許容）。`data-webcli-hint` を付与。クリックは Vimium C の合成イベント列、入力系は focus。応答を先に返してから実行（遷移で応答が失われない）。
- `cmd/app/fzf.go`: `os/exec` でシステム `fzf` を起動し stdin に `hint\t[kind]\ttext\thref` を流す。`--with-nth=2..` で hint 番号を隠し、選択行から番号を復元。対話時は fzf が `/dev/tty` に描画（REPL は同期実行で端末を奪い合わない）。`hints <query>` / 自動テストでは `fzf --filter`（同じマッチャ）で最良一致を選択。
- 選択 hint → `click` → background.js → content.js でクリック発火。

### テスト
```
--- PASS: TestSelectWithRealFzf          (fzf --filter: login→2, search→3, home→1, [button]→2, 不一致→エラー)
INTERACTIVE_FZF_SELECTED=7               (evidence/tty-fzf.log: PTY 上で "check"+Enter)
--- PASS: TestInteractiveFzfOnTTY
```

### E2E（ダミー HTML に 7 個の可視要素と 9 個の不可視要素を配置）
```
collected 7 visible element(s) on https://127.0.0.1:41467/
  1  [a]  Next page  https://127.0.0.1:41467/next
  2  [button]  Login
  3  [button]  Who am I
  4  [input:text]  Search products
  5  [select]  English
  6  [div@button]  Role Button
  7  [span]  Onclick span
fzf selected hint 2
click hint=2 <button> "Login" action_id=d6c02512-d255-4cb5-8b18-898a1eed730b
```
除外を確認した要素: `display:none` / `visibility:hidden` / `opacity:0` / 親が `opacity:0` / 幅高さ 0 / 画面外（上）/ `disabled` / モーダルの背後 / スクロール範囲外（下）/ `input[type=hidden]`。
クリック後にページの `fetch('/api/login')` が実行され `#status = "logged in: alice"`。リンククリックで `/next` へ遷移し、遷移先で content.js が再注入され `Press me` のクリックも成功。

## Phase 3: Go 100% MITM プロキシ

### 実装
- `internal/proxy/proxy.go`: goproxy に全 CONNECT を MITM させ、`TLSConfigFromCA` + `CertStore` キャッシュでホスト別証明書を動的署名。OnRequest/OnResponse でリクエスト/レスポンスの URL・ヘッダ・ボディを記録（レスポンスはストリーミング tee）。上流 TLS はシステムルートで検証（goproxy 既定の検証なしを上書き）。`/ca.pem` で CA 配布。
- `internal/proxy/ca.go`: ECDSA P-256 ルート CA を初回生成・再利用、SPKI ハッシュ算出。
- `internal/proxy/rules.go`: `block` / `set-req-header` / `del-req-header` / `set-resp-header` / `del-resp-header` / `replace-body`、host/url 正規表現 + method。REPL（`rule add|list|del|load`）と `-rules` JSON。

### テスト（`evidence/go-test.log`）
```
--- PASS: TestHTTPSCaptureAndActionHeaderStripped   ← クライアントは生成 CA だけを信頼 → TLS 成功 = 動的発行を証明
    captured: POST https://127.0.0.1:32979/api/login?x=1 action=11111111-2222-3333-4444-555555555555 status=200 resp={"action_hdr":"",…}
--- PASS: TestInterceptRules
    #1 GET …/modify -> 200 intercepts=[set-req-header X-Injected, del-req-header X-Secret, set-resp-header X-Audited]
    #2 GET …/blocked -> 451 blocked=true
    #3 GET …/replace -> 200 intercepts=[replace-body]
--- PASS: TestCAIsReusedAndServed
=== RUN   TestCurlThroughProxy                        ← 実 curl: curl -x http://proxy --cacert ca.pem
        HTTP/1.1 200 OK            (CONNECT)
        HTTP/1.1 200 OK
        X-Audited: web-cli         (レスポンスヘッダ改変)
        {"action_hdr":"", … "X-Curl-Modified":["1"] …}   (上流は監査ヘッダを受け取らず、注入ヘッダを受け取った)
--- PASS: TestCurlThroughProxy
```

### 実インターネットでのデモ（`evidence/proxy-curl.log`, `scripts/proxy-demo.sh`）
```
*  subject: O=GoProxy untrusted MITM proxy Inc; CN=github.com
*  issuer: O=web-cli; CN=web-cli local audit CA (e2b.local)
*  SSL certificate verify ok.
< X-Audited: web-cli
HTTP status: 451 / body: blocked by web-cli rule #3
[proxy] #1 GET https://github.com/robots.txt -> 200 (6397 bytes, 159ms) action=demo-action-0001 intercepts=2
action_id: demo-action-0001
forwarded headers: ['Accept', 'User-Agent', 'X-Web-Cli-Test']      ← X-Audit-Action-Id は転送されていない
```

### E2E（ブラウザ経由）
Chromium を `--proxy-server` + `--ignore-certificate-errors-spki-list=<生成 CA の SPKI>`（全エラー無視ではなく、この CA だけを信頼）で起動。ブラウザの HTTPS 通信がすべてキャプチャされ、REPL から追加したルールでヘッダ注入（上流が `X-Injected: e2e-rule` を受信）、遮断（ページには 451、上流には 0 件）、レスポンスヘッダ改変が動作。

## Phase 4: コンテキスト紐付け・.http・Cookie

### 実装
- background.js: クリック直前に `crypto.randomUUID()` → `declarativeNetRequest.updateSessionRules` で対象タブの全リソース種別（main_frame 含む）に `X-Audit-Action-Id` を 3 秒間付与 → content.js でクリック。app がプロキシ稼働中の時のみ付与（`tag`）。
- プロキシ: ヘッダ検出 → `Exchange.ActionID` → 除去して転送。app は `click_result` から `Action`（要素情報・ページ URL）を登録。
- `internal/audit/httpfile.go`: RFC 7230 形式の `.http`（`###` 区切り、`# @name`、アクション/レスポンス/インターセプトのコメント、hop-by-hop 等を除外）。`export [file] [action=<id>] [host=<re>]`。
- Cookie: `cookies dump [file] [url]`（chrome.cookies.getAll）/ `cookies import <file>`（Go で SetDetails に変換 → chrome.cookies.set）。

### テスト
```
--- PASS: TestWriteHTTPFile       (アクション注記、レスポンス注記、ブロック注記、除外ヘッダ)
--- PASS: TestJSONLAndDecompress  (exchange / action レコード)
--- PASS: TestCookieRoundTrip     (host-only / domain / session / 期限付き / 素の配列形式)
```

### E2E
```
[proxy] #2 POST https://127.0.0.1:41467/api/login -> 200 (27 bytes, 1ms) action=d6c02512-…      ← クリックと紐付け
origin saw 1 POST /api/login, action header present=false                                 ← 上流へは除去済み
dumped 1 cookie(s) … session=abc123; domain=127.0.0.1; path=/; secure=true; httpOnly=true  ← HttpOnly も取得
imported 1/1 cookie(s) from …/cookies-import.json
origin received Cookie: "session=abc123; imported=from-json"                              ← インポートした Cookie をブラウザが送信
[proxy] #5 GET https://127.0.0.1:41467/next -> 200 … action=f85e4b84-…                     ← リンク遷移 (main_frame) も紐付け
wrote 5 request(s) to …/requests.http
wrote 1 request(s) to …/login-action.http        (action= で絞り込み)
curl replay of login-action.http: POST https://127.0.0.1:41467/api/login -> {"ok":true,"user":"alice"}
```
アクション一覧（`actions` コマンド）:
```
ACTION ID                             KIND   HINT  ELEMENT              REQUESTS  PAGE
d6c02512-d255-4cb5-8b18-898a1eed730b  click  2     <button> "Login"     1         https://127.0.0.1:41467/
6bcb1533-3144-4e81-8791-9a92b74b533b  click  3     <button> "Who am I"  2         https://127.0.0.1:41467/
```
出力サンプル: [`evidence/sample-requests.http`](evidence/sample-requests.http), [`evidence/sample-cookies.json`](evidence/sample-cookies.json)

---

## E2E 方式について（透明性のための記載）

`scripts/e2e` は 2 モードを持つ。

- **real**（`-mode real`、実ブラウザ）: 通常の Chromium ビルド（CI では CloakBrowser 146）が `--load-extension` で `extension/` を読み込み、`--user-data-dir/NativeMessagingHosts/com.share026.webcli.json` から `bin/nm-host` を**ブラウザ自身が起動**する。本番と同一経路で、エミュレーションなし。結果 32/32（`evidence/ci/e2e-report-real.md`）。
- **emulated**（`-mode emulated`、既定）: headless shell（拡張機能非対応）向け。
  - **本物のまま使うもの**: Blink（DOM・レイアウト・`elementFromPoint`・イベント）、HTTPS 通信とプロキシ、出荷するファイルそのままの `background.js` / `content.js`（content.js は CDP の isolated world で実行）、ホスト定義 JSON の検証、Chrome と同じ方式での `bin/nm-host` 起動、`bin/app`、`fzf`、goproxy。
  - **CDP で代替したもの**: chrome.* API のみ（`scripts/e2e/exthost.go`）。
  - 結果: 31/31。

どちらのモードも `scripts/test-all.sh` の `E2E_MODES="emulated real"` で実行され、CI は両方を実行している。

## 自律的に検出・修正した問題

| # | 事象 | 原因 | 対応 |
|---|---|---|---|
| 1 | `go get` が `golang.org/x/net` で失敗 | `golang.org` のメタ取得が不通 | GitHub ミラーから GOPROXY 形式の file:// ミラーを構築（`go.mod` を汚さず、go.sum ハッシュは本家と一致） |
| 2 | 拡張機能が読み込まれない | 入手可能な Chromium が headless shell | 上記 E2E 方式を設計（JS は実ファイルのまま） |
| 3 | E2E: `crypto.randomUUID` が使えない | `about:blank` は非セキュアコンテキスト | CDP Fetch で `https://` 文書を返しその上で background.js を実行 |
| 4 | `audit.jsonl` の action レコードで `kind:"click"` が消える | ラッパーの `kind` が埋め込み構造体の `kind` を隠していた | ラッパーのキーを `record` に変更し、テストを追加 |
| 5 | `-race` 下でテスト終了後ログによる panic（稀） | `Listen` の生存確認接続を後から処理する goroutine が `Close` 後も動いていた | `Hub.Close` がハンドラ終了を待つように修正（closed フラグ + WaitGroup）、`-count=20` で安定を確認 |
| 6 | proxy-demo のブロック確認が `200` と表示 | `curl -i` が CONNECT 応答行を先に出力 | `-w '%{http_code}'` で最終ステータスを表示 |
| 7 | CI（Go 1.26）で `scripts/e2e` がビルド不可 | cdproto の `RemoteObject.Value` が `jsontext.Value` 型で、Go 1.26 では `json.RawMessage` への代入ができない（1.27 では通る） | 明示的に型変換 `json.RawMessage(ro.Value)` |
| 8 | **実ブラウザで nm-host が起動直後に消える**（real 7/32） | ブラウザは自分の stderr をネイティブホストに引き継ぐ。chromedp 起動時は読み手のいないパイプとなり、Go は fd 2 への書き込みで SIGPIPE を受けると即終了する（ローカルで exit -13 を再現） | `nm-host` で `signal.Ignore(SIGPIPE)`。回帰テスト `TestSurvivesClosedStderr`（修正前 FAIL / 修正後 PASS）。stderr を取り込まない条件の CI で real 32/32 を確認 |
| 9 | 上記条件で `WEBCLI_NMHOST_LOG` が空 | `io.MultiWriter(os.Stderr, f)` は最初の失敗で後続に書かない | ファイルを先頭に並べ替え、テストでログ内容も検証 |
| 10 | CI の emulated モードが起動失敗 | `E2E_HEADED=1` が emulated にも効き、X サーバがなかった | emulated は常に headless、real だけ `xvfb-run` |
| 11 | CI で PTY fzf テストが FAIL | ディストリ版 fzf（旧版）と、冷えたランナーでの入力タイミング | CI も `go install fzf@latest`（ローカルと同一）、入力待ちを延長、失敗時はテスト出力を表示 |

## 完了条件チェック

| 条件 | 結果 |
|---|---|
| `go build ./...` がエラー・警告ゼロ | ✅ `evidence/build.log`（`go vet ./...` もクリーン、`gofmt -l` 差分なし） |
| 外部ライブラリが `go.mod` に記述 | ✅ `github.com/elazarl/goproxy v1.9.1`（本体）、`github.com/chromedp/chromedp v0.16.0` / `cdproto`（E2E ハーネスのみ）、間接依存は `go mod tidy` 済み |
| `docs/ARCHITECTURE.md` に構成図・IPC 仕様 | ✅ |
| `docs/PROGRESS.md` に全フェーズの実行ログ・エビデンス | ✅ 本書 + `docs/evidence/` |

## 再現手順

```bash
scripts/build.sh                                   # vet + build + bin/
go test -race ./...                                # ユニット/統合
scripts/test-tty.sh                                # PTY 上の対話 fzf
scripts/proxy-demo.sh [https://example.com/]       # curl -x による実サイト MITM
CHROME_PATH=/path/to/chromium go run ./scripts/e2e # E2E emulated（31 項目）
CHROME_PATH=/path/to/chrome xvfb-run go run ./scripts/e2e -mode real -headed  # 実ブラウザ（32 項目）
gh workflow run e2e-cloakbrowser --ref <branch>    # CloakBrowser で全検証 → docs/evidence/ci/
CHROME_PATH=/path/to/chromium scripts/test-all.sh  # 上記すべて + docs/evidence/ 更新
```
