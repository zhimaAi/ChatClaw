<p align="center">
<img src="../../frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>Get OpenClaw-like personal AI agent in 5 mins. with Sandbox security,small and fast</strong>
</p>

<p align="center">
  <a href="./docs/readmes/README.md" target="_blank">English</a> |
  <a href="./docs/readmes/README_zh-CN.md" target="_blank">简体中文</a>
</p>

Get OpenClaw-like personal AI agent in 5 mins. Sandbox-secured, with an ultra-small 30MB installer for macOS & Windows (install in 1 min). Connects to WhatsApp, Telegram, Slack, Discord, Gmail, DingTalk, WeChat Work, QQ, Feishu & other messaging apps. Built-in Skill Market, Knowledge Base, Memory, MCP, Scheduled Tasks. Developed in Go: fast & low resource usage.

 5分钟拥有类 OpenClaw 的小龙虾个人AI智能体,沙箱安全防护,支持macOS/Windows 30M 极小安装包,1 分钟安装。连接WhatsApp、Telegram、Slack、Discord、Gmail、钉钉、企业微信、QQ、飞书等主流通讯应用，内置技能市场、IMA开源版本地知识库平替、记忆、MCP、计划任务等核心功能。Go语言开发，运行快、占资源少。
 
 
<p align="center">
<a href="https://github.com/zhimaAi/ChatClaw/releases" target="_blank" >Windows/Mac/Linux (Releases)</a>  
</p>

## Previews

### AI Chatbot Assistant

Ask your AI assistant any question, and it will intelligently search your knowledge base to generate a relevant answer.

![](../../images/previews/en/image1.png)

### PPT Quick Generate

Send a one-sentence command to the smart assistant to automatically create and generate a PowerPoint presentation.

![](../../images/previews/en/image3.png)

### Skill Manager

Use a command to have the assistant help you find installed features on your computer or install new extension plugins.

![](../../images/previews/en/image5.png)

### MCP: Unlimited Capability Extensions

Add external MCP servers to securely and efficiently connect to diverse data sources and tools, enabling your assistant to go beyond daily tasks into professional workflows.

![](../../images/previews/en/image6.png)

### Sandbox Mode: Double Protection

Choose between sandbox-isolated execution (OS-level isolation, restricted command scope) and native execution (more flexible). Switch freely to balance safety and convenience.

![](../../images/previews/en/image8.png)

### Memory: More Natural, Smarter Interactions

Enable contextual conversations and personalized assistance. The assistant can continuously learn and evolve so it feels like a growing partner.

![](../../images/previews/en/image9.png)

### Shared Team Knowledge Base

Authorize one-click access to ChatWiki to sync robots and knowledge bases, share configurations, and control member permissions.

![](../../images/previews/en/image10.png)

### Knowledge Base | Document Vectorization Storage

Upload documents (TXT, PDF, Word, Excel, CSV, HTML, Markdown). The system automatically parses, splits, and converts them into vector embeddings for precise retrieval.

![](../../images/previews/en/image11.png)

### Rich IM Channel Integrations

Integrate IM providers (Feishu, WeCom, QQ, DingTalk, LINE, Discord, WhatsApp, X/Twitter, Telegram, etc.) via SDKs to quickly enable channel creation, user management, and messaging.

![](../../images/previews/en/image12.png)

### Scheduled Tasks

Let your assistant automatically execute actions at preset times or intervals: reminders, recurring work, and system-level maintenance.

![](../../images/previews/en/image13.png)

### Text Selection for Instant Q&A

Select any text on your screen. It is automatically copied into a floating quick-ask box. One click to ask, instant answers.

![](../../images/previews/en/image14.png)

![](../../images/previews/en/image15.png)

### Smart Sidebar

Snap the assistant alongside other windows, quickly switch between differently configured assistants, and one-click send generated replies into your conversations.

![](../../images/previews/en/image16.png)

### One Question, Multiple Answers: Compare with Ease

Consult multiple "AI experts" simultaneously and view their responses side-by-side for easy comparison.

![](../../images/previews/en/image17.png)

### One-Click Launcher Ball

Click the floating ball on your desktop to instantly wake up or open the main ChatClaw window.

![](../../images/previews/en/image18.png)

## Server Mode Deployment

ChatClaw can run as a server (no desktop GUI required), accessible via a browser.

### Binary

Download the binary for your platform from [GitHub Releases](https://github.com/chatwiki/chatclaw/releases):

| Platform | File |
|----------|------|
| Linux x86_64 | `ChatClaw-server-linux-amd64` |
| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

Open http://localhost:8080 in your browser.

The server listens on `0.0.0.0:8080` by default. You can customize host and port via environment variables:

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

Open http://localhost:8080 in your browser.

### Docker Compose

Create a `docker-compose.yml` file:

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

Then run:

```bash
docker compose up -d
```

Open http://localhost:8080 in your browser. To stop: `docker compose down`. Data is persisted in the `chatclaw-data` volume.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Desktop Framework | [Wails v3](https://wails.io/) (Go + WebView) |
| Backend Language | [Go 1.26](https://go.dev/) |
| Frontend Framework | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
| UI Components | [shadcn-vue](https://www.shadcn-vue.com/) (New York style) + [Reka UI](https://reka-ui.com/) |
| Styling | [Tailwind CSS v4](https://tailwindcss.com/) |
| State Management | [Pinia](https://pinia.vuejs.org/) |
| Build Tool | [Vite](https://vite.dev/) |
| AI Framework | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
| AI Model Providers | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
| Database | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (vector search) |
| Internationalization | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
| Task Runner | [Task](https://taskfile.dev/) |
| Icons | [Lucide](https://lucide.dev/) |

## Project Structure

```
ChatClaw_D2/
├── main.go                     # Application entry point
├── go.mod / go.sum             # Go module dependencies
├── Taskfile.yml                # Task runner configuration
├── build/                      # Build configurations & platform assets
│   ├── config.yml              # Wails build config
│   ├── darwin/                 # macOS build settings & entitlements
│   ├── windows/                # Windows installer (NSIS/MSIX) & manifests
│   ├── linux/                  # Linux packaging (AppImage, nfpm)
│   ├── ios/                    # iOS build settings
│   └── android/                # Android build settings
├── frontend/                   # Vue 3 frontend application
│   ├── package.json            # Node.js dependencies
│   ├── vite.config.ts          # Vite bundler config
│   ├── components.json         # shadcn-vue config
│   ├── index.html              # Main window entry
│   ├── floatingball.html       # Floating ball window entry
│   ├── selection.html          # Text selection popup entry
│   ├── winsnap.html            # Snap window entry
│   └── src/
│       ├── assets/             # Icons (SVG), images & global CSS
│       ├── components/         # Shared components
│       │   ├── layout/         # App layout, sidebar, title bar
│       │   └── ui/             # shadcn-vue primitives (button, dialog, toast…)
│       ├── composables/        # Vue composables (reusable logic)
│       ├── i18n/               # Frontend i18n setup
│       ├── locales/            # Translation files (zh-CN, en-US…)
│       ├── lib/                # Utility functions
│       ├── pages/              # Page-level views
│       │   ├── assistant/      # AI chat assistant page & components
│       │   ├── knowledge/      # Knowledge base management page
│       │   ├── multiask/       # Multi-model comparison page
│       │   └── settings/       # Settings page (providers, models, tools…)
│       ├── stores/             # Pinia state stores
│       ├── floatingball/       # Floating ball mini-app
│       ├── selection/          # Text selection mini-app
│       └── winsnap/            # Snap window mini-app
├── internal/                   # Private Go packages
│   ├── bootstrap/              # Application initialization & wiring
│   ├── define/                 # Constants, built-in providers, env flags
│   ├── device/                 # Device identification
│   ├── eino/                   # AI/LLM integration layer
│   │   ├── agent/              # Agent orchestration
│   │   ├── chatmodel/          # Chat model factory (multi-provider)
│   │   ├── embedding/          # Embedding model factory
│   │   ├── filesystem/         # File-system tools for AI agents
│   │   ├── parser/             # Document parsers (PDF, DOCX, XLSX, CSV)
│   │   ├── processor/          # Document processing pipeline
│   │   ├── raptor/             # RAPTOR recursive summarization
│   │   ├── splitter/           # Text splitter factory
│   │   └── tools/              # AI tool integrations (browser, search, calculator…)
│   ├── errs/                   # i18n-aware error handling
│   ├── fts/                    # Full-text search tokenizer
│   ├── logger/                 # Structured logging
│   ├── services/               # Business logic services
│   │   ├── agents/             # Agent CRUD
│   │   ├── app/                # Application lifecycle
│   │   ├── browser/            # Browser automation (via chromedp)
│   │   ├── chat/               # Chat & streaming
│   │   ├── conversations/      # Conversation management
│   │   ├── document/           # Document upload & vectorization
│   │   ├── floatingball/       # Floating ball window (cross-platform)
│   │   ├── i18n/               # Backend internationalization
│   │   ├── library/            # Knowledge library CRUD
│   │   ├── multiask/           # Multi-model Q&A
│   │   ├── providers/          # AI provider configuration
│   │   ├── retrieval/          # RAG retrieval service
│   │   ├── settings/           # User settings with cache
│   │   ├── textselection/      # Screen text selection (cross-platform)
│   │   ├── thumbnail/          # Window thumbnail capture
│   │   ├── tray/               # System tray
│   │   ├── updater/            # Auto-update (GitHub/Gitee)
│   │   ├── windows/            # Window management & snap service
│   │   └── winsnapchat/        # Snap chat session service
│   ├── sqlite/                 # Database layer (Bun ORM + migrations)
│   └── taskmanager/            # Background task scheduler
├── pkg/                        # Public/reusable Go packages
│   ├── webviewpanel/           # Cross-platform webview panel manager
│   ├── winsnap/                # Window snapping engine (macOS/Windows/Linux)
│   └── winutil/                # Window activation utilities
├── docs/                       # Development documentation
└── images/                     # README screenshots
```

### License
This project is licensed under the MIT License. See [LICENSE](../../LICENSE).

### Changelog
To view the complete update log, please click👉️👉️[UpdateLog.md](./UpdateLog.md)

### 2026/03/10
1. **ChatWiki Account Binding**: Added ChatWiki account binding flow in settings (cloud/open-source selection, browser auth, deep-link callback, countdown + re-auth/unbind).<br/>
2. **ChatWiki Backend Service**: Introduced `ChatWikiService` with binding persistence, robot/library management APIs, and auth-expired handling.<br/>
3. **Team Chat Streaming**: Implemented team-mode SSE streaming with `dialogue_id` continuation support and conversation/message persistence for team sessions.<br/>
4. **DB Migrations**: Added SQLite migrations for ChatWiki binding storage and team conversation fields (`team_type`, `dialogue_id`).<br/>
5. **UI & i18n Updates**: Updated assistant/settings/knowledge pages and added new locales to support ChatWiki integration and related UI states.<br/>

### 2026/03/09
1. **Branding Assets Refresh**: Updated app icons and frontend logo assets across Windows/macOS builds and UI images.<br/>

### 2026/03/06
1. **Multimodal Support**: Added image input capability to assistant and knowledge pages, with model capability checks to detect multimodal support.<br/>
2. **Model Configuration Updates**: Updated OpenAI, Anthropic, Zhipu (GLM), and Qwen model configurations with refined capabilities and new model additions.<br/>
3. **Thinking Mode Control**: Added `DisableThinking` option to `ProviderConfig` and streamlined enable-thinking logic; added toast notifications for thinking mode changes.<br/>
4. **Sandbox Security**: Implemented sensitive path protection in sandbox mode to prevent unauthorized file access.<br/>
5. **Build System Improvements**: Replaced shell commands with Go tools for directory creation, file existence checks, and platform detection in Taskfile.<br/>
6. **License Update**: Switched from GNU Affero GPL to GNU General Public License.<br/>



