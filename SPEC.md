# 完全自律開発仕様書：ブラウザ自動操作＆ローカル監査プロキシ基盤

## 0. メタ指示・絶対自律ルール（Agent Rules）

1. **人間の介入を要求するな（完全自律自走）:**
   - 実装途中で「次はどうしますか？」「確認してください」と人間に聞くことを禁止する。
   - エラーが発生した場合は、ログを解析し自律的に修正・リトライを繰り返して完了させよ。
2. **車輪の再発明の完全禁止（既存資産の徹底活用）:**
   - 枯れた実績のあるOSSライブラリ・ツールを最優先で使用せよ。
   - プロキシコア: ゼロから自作せず `github.com/elazarl/goproxy` 等の実績あるGoライブラリを利用せよ。
   - TUI/選択UI: 自作せず、システムの `fzf` コマンドを `os/exec` でパイプ接続せよ。
   - DOM解析: 車輪の再発明をせず、Vimium C の可視判定アルゴリズム（`getBoundingClientRect`, `elementFromPoint`）をそのまま組み込め。
3. **言語スタックの厳格な分離（JS逃げの禁止）:**
   - **Go 100%:** バックエンド、CLI、Native Messaging Host、プロキシ、IPC、ログ管理、ファイル出力。
   - **JavaScript (MV3):** Chrome拡張機能（Content Script / Background）に必要な最小限のコードのみ。
   - バックエンド側にNode.jsやPythonを混入させることは一切認めない。
4. **ドキュメント駆動・検証駆動（自作自演のエビデンス記録）:**
   - `docs/` ディレクトリ配下に、以下のドキュメントを自律的に作成・更新しながら進めよ。
     - `docs/ARCHITECTURE.md`: 設計とIPCプロトコル仕様
     - `docs/PROGRESS.md`: 各フェーズの進捗、実行したテスト、エビデンス（実行ログ）
   - モックや「後で実装する」などの手抜き・ハードコーディングを禁止する。各フェーズで自律的にテストスクリプトを実行し、正常動作をログに記録せよ。

---

## 1. プロジェクト構成

以下のディレクトリ構成を厳格に維持して構築せよ。

```text
.
├── docs/                      # AIが自律的に記録するドキュメント類
│   ├── ARCHITECTURE.md
│   └── PROGRESS.md
├── cmd/
│   ├── app/                   # メイン常駐CLI（TTY直結、fzf実行、プロキシ統括、kulala出力）
│   └── nm-host/               # Chromeから呼ばれるNative Messaging Host（Go製極小ブリッジ）
├── internal/
│   ├── proxy/                 # Go製 MITMプロキシ（goproxy活用）
│   ├── ipc/                   # app と nm-host 間の Unix Domain Socket 通信
│   └── audit/                 # リクエスト記録、Cookie管理、.httpファイル生成
├── extension/                 # Chrome拡張機能 (Manifest V3)
│   ├── manifest.json
│   ├── background.js          # Native Messaging との中継
│   └── content.js             # Vimium C方式のDOM可視要素収集 & クリック実行
├── scripts/                   # ホスト登録やビルド・テスト用自動化スクリプト
└── go.mod
```

---

## 2. 実装フェーズ（自律実行順序）

### Phase 1: Native Messaging (Go) ＆ IPC基盤の構築
- **nm-host (Go):**
  - Chrome Native Messaging 仕様（メッセージ長を表す先頭4バイトのNative Endian uint32 + JSON）の送受信パーサーを実装せよ。
  - 受信したJSONを Unix Domain Socket (`/tmp/my_browser_ipc.sock`) 経由で `cmd/app` へ中継するブリッジとして機能させよ。
- **cmd/app (Go):**
  - `/tmp/my_browser_ipc.sock` をリッスンし、双方向通信を確立せよ。
- **Chrome拡張 (MV3):**
  - `nativeMessaging` 権限を設定し、起動時に `{"type": "ping"}` を送信、Go側から `{"type": "pong"}` を受領する疎通を完了せよ。
- **検証 & 記録:**
  - ホスト定義用JSONを配置し、拡張機能とGoプロセス間の疎通テストを自動実行し、結果を `docs/PROGRESS.md` に記録せよ。

### Phase 2: Vimium C方式 DOM要素収集 ＆ fzf（TTY）連携
- **content.js (DOM可視判定):**
  - 車輪の再発明を避け、Vimium C のロジックを採用せよ。
  - `a, button, input, select, textarea, [role=button], [onclick]` を対象とし、以下を判定：
    1. `getBoundingClientRect()` で画面内かつ幅・高さ > 0
    2. `getComputedStyle` で `visibility !== 'hidden'`、`display !== 'none'`、`opacity !== '0'`
    3. `document.elementFromPoint()` で他の要素（モーダル等）の背後に隠れていないか判定
  - 抽出要素に一意のHint属性を付与し、Background -> nm-host -> `cmd/app` へ送信せよ。
- **cmd/app (fzf起動):**
  - 本プロセスは実際のTTY上で起動している。`os/exec` でシステム既存の `fzf` を起動し、標準入力に要素リストを流し込め。
  - ユーザー（または自動テスト入力）によって選択された要素のHint番号を拡張機能へ送り返し、`content.js` 側でクリックイベントを発火させよ。
- **検証 & 記録:**
  - ダミーHTMLに対する抽出・選択・クリックのシミュレーションテストを実行し、エビデンスを `docs/PROGRESS.md` に記録せよ。

### Phase 3: Go 100% ローカルMITMプロキシの実装
- **proxy (Go):**
  - `github.com/elazarl/goproxy` などの既存ライブラリを活用し、ゼロからプロトコルを書く無駄（車輪の再発明）を排除せよ。
  - 動的CA証明書発行機能を組み込み、HTTPS通信（リクエスト/レスポンスのURL、ヘッダ、ボディ）を完全キャプチャせよ。
  - `cmd/app` と連携し、特定条件にマッチする通信の遮断・改変（インターセプト）を可能にせよ。
- **検証 & 記録:**
  - `curl -x` でローカルプロキシを経由させ、HTTPS通信のログ取得およびヘッダ改変が機能していることをテストコードで証明し、ログを記録せよ。

### Phase 4: コンテキスト紐付け ＆ kulala.nvim / Cookie連携
- **コンテキスト紐付け:**
  - DOM要素クリック直前に拡張側でUUID（アクションID）を生成し、直後のリクエストに `X-Audit-Action-Id: <UUID>` ヘッダを付与せよ。
  - Goプロキシ側でこのヘッダを検知して「どのDOM操作で発生した通信か」を紐付けて記録し、外部へ転送する直前にヘッダを除去せよ。
- **kulala.nvim連携 (.http出力):**
  - キャプチャした通信を Neovim の `kulala.nvim` や VS Code `REST Client` でそのまま実行可能な `.http` (RFC 7230準拠) 形式でファイル出力する機能を実装せよ。
- **Cookie連携:**
  - `chrome.cookies` API を叩いて現ドメインのCookieをJSONでダンプ / 外部JSONからCookieをインポートするコマンドを Go -> 拡張 経由で実行可能にせよ。
- **検証 & 記録:**
  - サンプルのHTTPファイルおよびCookieダンプが正常に出力されることを確認し、全機能の完了サマリーを `docs/PROGRESS.md` に記載せよ。

---

## 3. 完了条件
1. `go build ./...` がエラー・警告ゼロで完了すること。
2. 外部ライブラリが `go.mod` に適切に記述されていること。
3. `docs/ARCHITECTURE.md` にシステム構成図・IPCプロトコル仕様がまとめられていること。
4. `docs/PROGRESS.md` に全フェーズの実行ログ・テストエビデンスが記録されていること。
