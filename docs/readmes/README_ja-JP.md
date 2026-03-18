<p align="center">
<img src="../../frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>OpenClawのような個人AIエージェントを5分で入手。サンドボックスセキュリティ、小型で高速</strong>
</p>

<p align="center">
  <a href="../../README.md">English</a> |
  <a href="README_zh-CN.md">简体中文</a> |
  <a href="README_zh-TW.md">繁體中文</a> |
  <a href="README_ja-JP.md">日本語</a> |
  <a href="README_ko-KR.md">한국어</a> |
  <a href="README_ar-SA.md">العربية</a> |
  <a href="README_bn-BD.md">বাংলা</a> |
  <a href="README_de-DE.md">Deutsch</a> |
  <a href="README_es-ES.md">Español</a> |
  <a href="README_fr-FR.md">Français</a> |
  <a href="README_hi-IN.md">हिन्दी</a> |
  <a href="README_it-IT.md">Italiano</a> |
  <a href="README_pt-BR.md">Português</a> |
  <a href="README_sl-SI.md">Slovenščina</a> |
  <a href="README_tr-TR.md">Türkçe</a> |
  <a href="README_vi-VN.md">Tiếng Việt</a>
</p>

OpenClawのような個人AIエージェントを5分で入手。サンドボックスセキュリティ付き、macOS＆Windows用の超小型30MBインストーラー（1分でインストール）。WhatsApp、Telegram、Slack、Discord、Gmail、DingTalk、WeChat Work、QQ、Feishuなどのメッセージングアプリに接続。組み込みスキルマーケット、ナレッジベース、メモリ、MCP、スケジュールタスク。Goで開発：高速でリソース使用量が少ない。

## プレビュー

### AIチャットアシスタント

AIアシスタントに何でも質問してください。ナレッジベースをインテリジェントに検索し、関連する回答を生成します。

![](../../images/previews/en/image1.png)

### PPT即座に生成

スマートアシスタントに一文のコマンドを送信すると、PowerPointプレゼンテーションを自動作成・生成します。

![](../../images/previews/en/image3.png)

### スキルマネージャー

コマンドでアシスタントに、コンピュータにインストールされた機能の検索や新しい拡張プラグインのインストールを依頼できます。

![](../../images/previews/en/image5.png)

### MCP: 無制限の能力拡張

外部MCPサーバーを追加し、多様なデータソースやツールに安全かつ効率的に接続。アシスタントが日常タスクを超えてプロのワークフローで活躍できます。

![](../../images/previews/en/image6.png)

### サンドボックスモード: 二重保護

サンドボックス隔離実行（OSレベル隔離・コマンド範囲制限）とネイティブ実行（より柔軟）を選択可能。安全と利便のバランスで自由に切り替え。

![](../../images/previews/en/image8.png)

### メモリ: より自然でスマートな対話

コンテキスト会話とパーソナライズ支援を有効化。アシスタントは継続的に学習・進化し、成長するパートナーのように働きます。

![](../../images/previews/en/image9.png)

### 共有チームナレッジベース

ChatWikiへのワンクリックアクセスを許可し、ロボットとナレッジベースの同期、設定の共有、メンバー権限の管理ができます。

![](../../images/previews/en/image10.png)

### ナレッジベース | ドキュメントベクトル化ストレージ

ドキュメント（TXT, PDF, Word, Excel, CSV, HTML, Markdown）をアップロード。システムが自動で解析・分割し、精密検索用のベクトル埋め込みに変換します。

![](../../images/previews/en/image11.png)

### 豊富なIMチャネル連携

IMプロバイダー（Feishu, WeCom, QQ, DingTalk, LINE, Discord, WhatsApp, X/Twitter, Telegram等）をSDKで統合し、チャネル作成・ユーザー管理・メッセージングを迅速に実現。

![](../../images/previews/en/image12.png)

### スケジュールタスク

アシスタントに事前設定した時刻や間隔でリマインダー・定期作業・システム保守などを自動実行させられます。

![](../../images/previews/en/image13.png)

### テキスト選択で即座にQ&A

画面上の任意のテキストを選択すると、自動でフローティングのクイック質問ボックスにコピーされます。ワンクリックで質問、即答。

![](../../images/previews/en/image14.png)

![](../../images/previews/en/image15.png)

### スマートサイドバー

アシスタントを他のウィンドウにスナップし、異なる設定のアシスタント間を素早く切り替え、生成された返答をワンクリックで会話に送信。

![](../../images/previews/en/image16.png)

### 一問多答: 簡単に比較

複数の「AI専門家」に同時に相談し、回答を横並びで表示して簡単に比較できます。

![](../../images/previews/en/image17.png)

### ワンクリックランチャーボール

デスクトップの浮き球をクリックすると、ChatClawメインウィンドウを即座に起動または開きます。

![](../../images/previews/en/image18.png)

## サーバーモードデプロイ

ChatClawはサーバーモードで実行可能（デスクトップGUI不要）、ブラウザでアクセス可能。

### バイナリ直接実行

[GitHub Releases](https://github.com/chatwiki/chatclaw/releases)から該当プラットフォームのバイナリをダウンロード：

|| プラットフォーム | ファイル |
||------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

ブラウザで http://localhost:8080 を開く。

デフォルトで `0.0.0.0:8080` をリッスン。環境変数でホストとポートをカスタマイズ可能：

```bash
WAILS_SERVER_HOST=127.0.0.1 WAILS_SERVER_PORT=3000 ./ChatClaw-server-linux-amd64
```

### Docker

```bash
docker run -d \
  --name chatclaw-server \
  -p 8080:8080 \
  -v chatclaw-data:/root/.config/chatclaw \
  registry.cn-hangzhou.aliyuncs.com/chatwiki/chatclaw:latest
```

ブラウザで http://localhost:8080 を開く。

### Docker Compose

`docker-compose.yml` ファイルを作成：

```yaml
services:
  chatclaw:
    image: registry.cn-hangzhou.aliyuncs.com/chatwiki/chatclaw:latest
    container_name: chatclaw-server
    volumes:
      - chatclaw-data:/root/.config/chatclaw
    ports:
      - "8080:8080"
    restart: unless-stopped

volumes:
  chatclaw-data:
```

その後実行：

```bash
docker compose up -d
```

ブラウザで http://localhost:8080 を開く。停止：`docker compose down`。データは `chatclaw-data` ボリュームに永続化。

## 技術スタック

|| レイヤー | テクノロジー |
||------|------|
|| デスクトップフレームワーク | [Wails v3](https://wails.io/) (Go + WebView) |
|| バックエンド言語 | [Go 1.26](https://go.dev/) |
|| フロントエンドフレームワーク | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| UIコンポーネント | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| スタイリング | [Tailwind CSS v4](https://tailwindcss.com/) |
|| 状態管理 | [Pinia](https://pinia.vuejs.org/) |
|| ビルドツール | [Vite](https://vite.dev/) |
|| AIフレームワーク | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| AIモデルプロバイダー | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| データベース | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (ベクトル検索) |
|| 国際化 | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| タスクランナー | [Task](https://taskfile.dev/) |
|| アイコン | [Lucide](https://lucide.dev/) |

## プロジェクト構造

```
ChatClaw_D2/
├── main.go                     # アプリケーションエントリポイント
├── go.mod / go.sum             # Goモジュール依存関係
├── Taskfile.yml                # タスクランナー設定
├── build/                      # ビルド設定とプラットフォームアセット
│   ├── config.yml              # Wailsビルド設定
│   ├── darwin/                 # macOSビルド設定と資格情報
│   ├── windows/                # Windowsインストーラー (NSIS/MSIX) とマニフェスト
│   ├── linux/                  # Linuxパッケージ (AppImage, nfpm)
│   ├── ios/                    # iOSビルド設定
│   └── android/                # Androidビルド設定
├── frontend/                   # Vue 3フロントエンドアプリケーション
│   ├── package.json            # Node.js依存関係
│   ├── vite.config.ts          # Viteバンドラー設定
│   ├── components.json         # shadcn-vue設定
│   ├── index.html              # メインウィンドウエントリ
│   ├── floatingball.html       # 浮き球ウィンドウエントリ
│   ├── selection.html          # テキスト選択ポップアップエントリ
│   ├── winsnap.html            # スナップウィンドウエントリ
│   └── src/
│       ├── assets/             # アイコン(SVG)、画像、グローバルCSS
│       ├── components/         # 共有コンポーネント
│       │   ├── layout/         # アプリレイアウト、サイドバー、タイトルバー
│       │   └── ui/             # shadcn-vueプリミティブ (button, dialog, toast…)
│       ├── composables/        # Vueコンポーザブル（再利用可能なロジック）
│       ├── i18n/               # フロントエンド国際化セットアップ
│       ├── locales/            # 翻訳ファイル (zh-CN, en-US…)
│       ├── lib/                # ユーティリティ関数
│       ├── pages/              # ページレベルビュー
│       │   ├── assistant/      # AIチャットアシентентページとコンポーネント
│       │   ├── knowledge/      # ナレッジベース管理ページ
│       │   ├── multiask/       # マルチモデル比較ページ
│       │   └── settings/       # 設定ページ（プロバイダー、モデル、ツール…）
│       ├── stores/             # Pinia状態ストア
│       ├── floatingball/       # 浮き球ミニアプリ
│       ├── selection/          # テキスト選択ミニアプリ
│       └── winsnap/            # スナップウィンドウミニアプリ
├── internal/                   # プライベートGoパッケージ
│   ├── bootstrap/              # アプリケーション初期化とワイヤリング
│   ├── define/                 # 定数組み込みプロバイダー、環境フラグ
│   ├── device/                 # デバイス識別
│   ├── eino/                   # AI/LLM統合レイヤー
│   │   ├── agent/              # Agentオーケストレーション
│   │   ├── chatmodel/          # チャットモデルファクトリ（マルチプロバイダー）
│   │   ├── embedding/          # 埋め込みモデルファクトリ
│   │   ├── filesystem:         # AI Agentファイルシステムツール
│   │   ├── parser:             # ドキュメントパーサー (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # ドキュメント処理パイプライン
│   │   ├── raptor:             # RAPTOR再帰的要約
│   │   ├── splitter:           # テキスト分割器ファクトリ
│   │   └── tools:              # AIツール統合（ブラウザ、検索、電卓…）
│   ├── errs/                   # 国際化エラー処理
│   ├── fts:                    # 全文検索トークナイザー
│   ├── logger:                 # 構造化ログ
│   ├── services:               # ビジネスロジックサービス
│   │   ├── agents:             # Agent CRUD
│   │   ├── app:                # アプリケーションライフサイクル
│   │   ├── browser:            # ブラウザ自動化 (chromedp)
│   │   ├── chat:               # チャットとストリーミング
│   │   ├── conversations:      # 会話管理
│   │   ├── document:           # ドキュメントアップロードとベクトル化
│   │   ├── floatingball:       # 浮き球ウィンドウ（クロスプラットフォーム）
│   │   ├── i18n:               # バックエンド国際化
│   │   ├── library:            # ナレッジライブラリCRUD
│   │   ├── multiask:           # マルチモデルQ&A
│   │   ├── providers:          # AIプロバイダー設定
│   │   ├── retrieval:          # RAG検索サービス
│   │   ├── settings:           # ユーザー設定とキャッシュ
│   │   ├── textselection:      # 画面テキスト選択（クロスプラットフォーム）
│   │   ├── thumbnail:          # ウィンドウサムネイルキャプチャ
│   │   ├── tray:               # システムトレイ
│   │   ├── updater:            # 自動更新 (GitHub/Gitee)
│   │   ├── windows:            # ウィンドウ管理とスナップサービス
│   │   └── winsnapchat:        # スナップチャットセッションサービス
│   ├── sqlite:                 # データベースレイヤー (Bun ORM + マイグレーション)
│   └── taskmanager:            # バックグラウンドタスクスケジューラー
├── pkg:                         # パブリック/再利用可能なGoパッケージ
│   ├── webviewpanel:           # クロスプラットフォームWebViewパネルマネージャー
│   ├── winsnap:                # ウィンドウスナップエンジン (macOS/Windows/Linux)
│   └── winutil:                # ウィンドウアクティブ化ユーティリティ
├── docs:                       # 開発ドキュメント
└── images:                      # READMEスクリーンショット
```




