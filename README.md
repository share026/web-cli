# web-cli

ブラウザ自動操作 & ローカル監査プロキシ基盤。Go 製の常駐 CLI が、Chrome 拡張（MV3）と
Native Messaging で連携して **ページを操作**し（fzf で要素を選んでクリック、フォーム入力、
ページ遷移、ソース・テキスト取得、JavaScript 実行、スクリーンショット、操作の記録と再生）、同時に
**goproxy ベースの MITM プロキシ**でその操作が発生させた HTTPS 通信を記録・遮断・改変し、
**kulala.nvim / REST Client 用の `.http`** や Cookie JSON として書き出します。

- 仕様: [SPEC.md](SPEC.md)
- 設計・IPC プロトコル: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- 進捗・テストエビデンス: [docs/PROGRESS.md](docs/PROGRESS.md)

## 必要なもの

- Go 1.26 以上（開発は 1.27.1。chromedp v0.16 の要件）
- `fzf`
- Chrome / Chromium（拡張機能を読み込める通常版）

## クイックスタート

```bash
scripts/build.sh              # bin/app, bin/nm-host
scripts/install-host.sh       # Native Messaging ホスト "com.share026.webcli" を登録
```

1. `chrome://extensions` → デベロッパーモード → 「パッケージ化されていない拡張機能を読み込む」→ `extension/`
2. 端末で `bin/app` を起動（UDS `/tmp/my_browser_ipc.sock`、プロキシ `127.0.0.1:8080`）
3. プロキシ経由で Chrome を起動（生成された CA を信頼させる）:
   ```bash
   google-chrome --proxy-server=127.0.0.1:8080 \
     --ignore-certificate-errors-spki-list=<app の起動ログに表示される SPKI>
   ```

### 使い方の例: ログインしてページとリクエストを調べる

操作対象（`<target>`）は 3 通りで指定できます。
- `list` で表示される番号
- `css=<セレクタ>`
- 要素の名前（ラベル・placeholder・ボタンの文字など。fzf であいまい検索します）

```text
web-cli> open example.com/login           # 読み込み完了まで待つ
web-cli> type Email alice@example.com     # <label>Email</label> の欄に入力
web-cli> type Password $MY_PASSWORD       # 環境変数から（"-" なら画面に出さずに入力）
web-cli> select Country Japan
web-cli> check "Remember me"
web-cli> press Enter Password             # フォーム送信（click "Sign in" でも可）
web-cli> waitfor text=Welcome             # 遷移後の表示を待つ
web-cli> text                             # ページの文字
web-cli> source page.html                 # JavaScript 実行後の DOM
web-cli> eval document.title              # DevTools の Console と同じ
web-cli> screenshot shot.png
web-cli> log                              # 通信一覧（どの操作で起きたかの action ID 付き）
web-cli> show 12                          # 12 番のリクエスト/レスポンスのヘッダと本文
web-cli> body 13 served.html              # サーバが返した HTML そのもの
web-cli> export                           # .http（kulala.nvim / REST Client で再送できる）
web-cli> cookies dump | storage dump      # Cookie / localStorage・sessionStorage
```

### 記録と再生（DevTools の Recorder 相当）

```text
web-cli> record start login.webcli        # ここからブラウザで普通に操作する
  … 入力・クリック・チェック・選択・Enter が記録される …
web-cli> record stop
```
- 記録結果は 1 行 1 コマンドのスクリプトです（`type css=#email bob@example.com` など）。
- パスワード欄は値を保存せず `$WEBCLI_PASSWORD` と書き出します。
- 再生: `WEBCLI_PASSWORD=... bin/app -f login.webcli`（または REPL で `run login.webcli`）。

その他:
- ナビゲーション: `back` / `forward` / `reload` / `tabs` / `newtab` / `closetab`
- 入力: `clear` / `uncheck` / `submit` / `focus` / `scroll`
- プロキシのルール: `rule add block host=^ads\. status=451`
- 全コマンド: `help`

非対話実行: `bin/app -c "wait 30s; open example.com; hints login; export"`、`bin/app -f script.webcli`

## テスト

```bash
go test -race ./...
scripts/test-tty.sh                                # 擬似端末上の対話 fzf
scripts/proxy-demo.sh                              # curl -x による実サイトの MITM デモ
CHROME_PATH=/path/to/chromium go run ./scripts/e2e # E2E emulated（31 項目, headless shell 可）
CHROME_PATH=/path/to/chrome go run ./scripts/e2e -mode real -headed  # 実ブラウザ E2E（57 項目）
CHROME_PATH=/path/to/chromium scripts/test-all.sh  # 全部 + docs/evidence/ を更新
```

実ブラウザ検証は GitHub Actions（`.github/workflows/e2e-cloakbrowser.yml`）が
[CloakBrowser](https://github.com/CloakHQ/CloakBrowser) 146 の公式バイナリを取得・digest 検証して
`scripts/test-all.sh`（emulated + real）を実行し、結果を `docs/evidence/ci/` にコミットします。
画面なし（`--headless`）でも拡張機能込みで動作します（CI で real 57/57 を確認）。
