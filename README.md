# web-cli

ブラウザ自動操作 & ローカル監査プロキシ基盤。Go 製の常駐 CLI が、Chrome 拡張（MV3）と
Native Messaging で連携して **ページ上の可視要素を fzf で選んでクリック**し、同時に
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

```text
web-cli> hints                 # 可視要素 → fzf → クリック（hints login なら非対話で最良一致）
web-cli> log                   # キャプチャした通信（クリックのアクション ID 付き）
web-cli> actions               # DOM 操作と紐付いたリクエスト数
web-cli> export                # audit-out/requests.http（action=<id> / host=<re> で絞り込み）
web-cli> cookies dump          # audit-out/cookies.json
web-cli> cookies import a.json
web-cli> rule add block host=^ads\. status=451
web-cli> rule add set-req-header url=/api/ name=X-Debug value=1
web-cli> help
```

非対話実行: `bin/app -c "wait 30s; hints login; export"`

## テスト

```bash
go test -race ./...
scripts/test-tty.sh                                # 擬似端末上の対話 fzf
scripts/proxy-demo.sh                              # curl -x による実サイトの MITM デモ
CHROME_PATH=/path/to/chromium go run ./scripts/e2e # E2E（31 項目）
CHROME_PATH=/path/to/chromium scripts/test-all.sh  # 全部 + docs/evidence/ を更新
```
