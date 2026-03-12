<p align="center">
<img src="./frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  5分鐘擁有類 OpenClaw 的小龍蝦個人AI智慧體，沙箱安全，佔資源少，運行快.
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
  <a href="README_tlh.md">tlhIngan</a> |
  <a href="README_tr-TR.md">Türkçe</a> |
  <a href="README_vi-VN.md">Tiếng Việt</a>
</p>

5分鐘擁有類 OpenClaw 的小龍蝦個人AI智慧體，沙箱安全防護，macOS/Windows 30M 極小安裝包，1 分鐘安裝。連接 WhatsApp、Telegram、Slack、Discord、Gmail、釘釘、企業微信、QQ、飛書等主流通訊應用，內建技能市場、知識庫、記憶、MCP、計劃任務等核心功能。Go語言開發，運行快、佔資源少。

## 功能預覽

### AI 聊天助手

向 AI 助手提出任何問題，它會智慧搜尋您的知識庫並產生相關回答。

![](../../images/1.png)

### PPT 快速生成

向智慧助手發送一句話指令，即可自動建立和產生 PowerPoint 簡報。

![](../../images/2.png)

### 技能管理器

使用指令讓機器人幫您查詢電腦上已安裝的功能或安裝新的擴充套件。

![](../../images/3.png)

### 知識庫 |  文件向量化儲存

上傳您的文件（如 TXT、PDF、Word、Excel、CSV、HTML、Markdown），系統會自動解析、分割並轉換為向量嵌入，儲存到您的私有知識庫中，供 AI 模型進行精準檢索和利用。

![](../../images/4.png)

### 劃詞即時問答

選取螢幕上的任意文字，它會被自動複製並填入懸浮快問框。一鍵傳送給 AI 助手，即刻獲得回答。

![](../../images/5.png)

![](../../images/6.png)

### 智慧貼靠視窗

可以貼靠在其他應用視窗旁的智慧助手。在其中快速切換不同設定的 AI 助手進行提問。機器人根據您關聯的知識庫產生回答，並支援一鍵將回覆傳送到您的對話中。

![](../../images/7.png)

### 一問多答：輕鬆比較

無需重複提問。同時諮詢多個"AI 專家"，在同一介面中並排檢視它們的回覆。方便比較，幫助您得出最佳結論。

![](../../images/8.png)

### 一鍵啟動球

點擊桌面上的懸浮球，即可喚醒或開啟 ChatClaw 主應用視窗。

![](../../images/9.png)

## 伺服器模式部署

ChatClaw 支援以伺服器模式執行（無需桌面 GUI），透過瀏覽器存取。

### 二進位直接執行

從 [GitHub Releases](https://github.com/chatwiki/chatclaw/releases) 下載對應平台的二進位檔案：

|| 平台 | 檔案 |
||------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

瀏覽器開啟 http://localhost:8080 即可使用。

服務預設監聽 `0.0.0.0:8080`。可透過環境變數自訂監聽位址和連接埠：

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

瀏覽器開啟 http://localhost:8080 即可使用。

### Docker Compose

建立 `docker-compose.yml` 檔案：

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

然後執行：

```bash
docker compose up -d
```

瀏覽器開啟 http://localhost:8080 即可使用。停止服務：`docker compose down`。資料持久化在 `chatclaw-data` 卷中。

## 技術堆疊

|| 層級 | 技術 |
||------|------|
|| 桌面框架 | [Wails v3](https://wails.io/) (Go + WebView) |
|| 後端語言 | [Go 1.26](https://go.dev/) |
|| 前端框架 | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| UI 元件 | [shadcn-vue](https://www.shadcn-vue.com/) (New York 風格) + [Reka UI](https://reka-ui.com/) |
|| 樣式方案 | [Tailwind CSS v4](https://tailwindcss.com/) |
|| 狀態管理 | [Pinia](https://pinia.vuejs.org/) |
|| 建構工具 | [Vite](https://vite.dev/) |
|| AI 框架 | [Eino](https://github.com/cloudwego/eino) (位元組跳動 CloudWeGo) |
|| AI 模型供應商 | OpenAI / Claude / Gemini / Ollama / DeepSeek / 豆包 / 通義千問 / 智譜 / Grok |
|| 資料庫 | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (向量檢索) |
|| 國際化 | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| 任務執行 | [Task](https://taskfile.dev/) |
|| 圖示 | [Lucide](https://lucide.dev/) |

## 專案結構

```
ChatClaw_D2/
├── main.go                     # 應用入口
├── go.mod / go.sum             # Go 模組依賴
├── Taskfile.yml                # 任務執行器設定
├── build/                      # 建構設定與平台資源
│   ├── config.yml              # Wails 建構設定
│   ├── darwin/                 # macOS 建構設定與授權
│   ├── windows/                # Windows 安裝器 (NSIS/MSIX) 與清單
│   ├── linux/                 # Linux 打包 (AppImage, nfpm)
│   ├── ios/                   # iOS 建構設定
│   └── android/               # Android 建構設定
├── frontend/                   # Vue 3 前端應用
│   ├── package.json            # Node.js 依賴
│   ├── vite.config.ts          # Vite 打包設定
│   ├── components.json         # shadcn-vue 設定
│   ├── index.html              # 主視窗入口
│   ├── floatingball.html       # 懸浮球視窗入口
│   ├── selection.html          # 劃詞彈窗入口
│   ├── winsnap.html            # 貼靠視窗入口
│   └── src/
│       ├── assets/             # 圖示 (SVG)、圖片與全域 CSS
│       ├── components/         # 共用元件
│       │   ├── layout/         # 應用版面配置、側邊欄、標題欄
│       │   └── ui/             # shadcn-vue 基礎元件 (button, dialog, toast…)
│       ├── composables/        # Vue 組合式函數（可複用邏輯）
│       ├── i18n/               # 前端國際化設定
│       ├── locales/            # 翻譯檔案 (zh-CN, en-US…)
│       ├── lib/                # 工具函數
│       ├── pages/              # 頁面級視圖
│       │   ├── assistant/      # AI 聊天助手頁面及元件
│       │   ├── knowledge/      # 知識庫管理頁面
│       │   ├── multiask/       # 多模型對比頁面
│       │   └── settings/       # 設定頁面（供應商、模型、工具…）
│       ├── stores/             # Pinia 狀態倉庫
│       ├── floatingball/       # 懸浮球迷你應用
│       ├── selection/          # 劃詞迷你應用
│       └── winsnap/            # 貼靠視窗迷你應用
├── internal/                   # 私有 Go 套件
│   ├── bootstrap/              # 應用初始化與依賴注入
│   ├── define/                 # 常量、內建供應商、環境標誌
│   ├── device/                 # 設備識別
│   ├── eino/                   # AI/LLM 整合層
│   │   ├── agent/              # Agent 編排
│   │   ├── chatmodel/          # 聊天模型工廠（多供應商）
│   │   ├── embedding/          # 嵌入模型工廠
│   │   ├── filesystem/         # AI Agent 檔案系統工具
│   │   ├── parser/             # 文件解析器 (PDF, DOCX, XLSX, CSV)
│   │   ├── processor/          # 文件處理流水線
│   │   ├── raptor:             # RAPTOR 遞迴摘要
│   │   ├── splitter:           # 文字分割器工廠
│   │   └── tools:              # AI 工具整合（瀏覽器、搜尋、計算機…）
│   ├── errs/                   # 國際化錯誤處理
│   ├── fts:                    # 全文檢索分詞器
│   ├── logger:                 # 結構化日誌
│   ├── services:               # 商業邏輯服務
│   │   ├── agents:             # Agent 增刪改查
│   │   ├── app:                # 應用生命週期
│   │   ├── browser:            # 瀏覽器自動化 (chromedp)
│   │   ├── chat:               # 聊天與串流傳輸
│   │   ├── conversations:      # 對話管理
│   │   ├── document:           # 文件上傳與向量化
│   │   ├── floatingball:       # 懸浮球視窗（跨平台）
│   │   ├── i18n:               # 後端國際化
│   │   ├── library:            # 知識庫增刪改查
│   │   ├── multiask:           # 多模型問答
│   │   ├── providers:          # AI 供應商設定
│   │   ├── retrieval:          # RAG 檢索服務
│   │   ├── settings:           # 使用者設定與快取
│   │   ├── textselection:      # 螢幕劃詞（跨平台）
│   │   ├── thumbnail:          # 視窗縮圖擷取
│   │   ├── tray:               # 系統匣
│   │   ├── updater:             # 自動更新 (GitHub/Gitee)
│   │   ├── windows:             # 視窗管理與貼靠服務
│   │   └── winsnapchat:        # 貼靠聊天對話服務
│   ├── sqlite:                  # 資料庫層 (Bun ORM + 遷移)
│   └── taskmanager:             # 背景任務排程器
├── pkg:                         # 公共/可複用 Go 套件
│   ├── webviewpanel:           # 跨平台 WebView 面板管理器
│   ├── winsnap:                # 視窗貼靠引擎 (macOS/Windows/Linux)
│   └── winutil:                # 視窗啟動工具
├── docs:                       # 開發文件
└── images:                      # README 截圖
```
