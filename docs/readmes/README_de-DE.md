<p align="center">
<img src="../../frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>Erhalten Sie in 5 Minuten einen persönlichen KI-Agenten wie OpenClaw. Sandbox-Sicherheit, klein und schnell</strong>
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

Erhalten Sie in 5 Minuten einen persönlichen KI-Agenten wie OpenClaw. Sandbox-gesichert, mit einem ultrakleinen 30MB-Installer für macOS & Windows (Installation in 1 Minute). Verbindet sich mit WhatsApp, Telegram, Slack, Discord, Gmail, DingTalk, WeChat Work, QQ, Feishu und anderen Messaging-Apps. Integrierter Skill-Markt, Wissensbasis, Speicher, MCP, geplante Aufgaben. Entwickelt in Go: schnell und ressourcenschonend.

## Vorschau

### KI-Chat-Assistent

Stellen Sie Ihrem KI-Assistenten eine beliebige Frage, und er wird intelligent Ihre Wissensbasis durchsuchen, um eine relevante Antwort zu generieren.

![](../../images/1.png)

### PPT-Schnellgenerierung

Senden Sie einen Ein-Satz-Befehl an den intelligenten Assistenten, um automatisch eine PowerPoint-Präsentation zu erstellen und zu generieren.

![](../../images/2.png)

### Skill-Manager

Verwenden Sie einen Befehl, damit der Roboter Ihnen hilft, installierte Funktionen auf Ihrem Computer zu finden oder neue Erweiterungs-Plugins zu installieren.

![](../../images/3.png)

### Wissensbasis | Dokument-Vektorisierungsspeicher

Laden Sie Ihre Dokumente hoch (z.B. TXT, PDF, Word, Excel, CSV, HTML, Markdown). Das System wird sie automatisch analysieren, aufteilen und in Vektor-Embeddings konvertieren und in Ihrer privaten Wissensbasis speichern, die vom KI-Modell für präzise Abfrage und Nutzung verwendet werden kann.

![](../../images/4.png)

### Textauswahl für sofortige Fragen & Antworten

Wählen Sie einen beliebigen Text auf Ihrem Bildschirm aus, er wird automatisch kopiert und in ein schwebendes Schnellfragefeld gefüllt. Senden Sie ihn mit einem Klick an den KI-Assistenten für eine sofortige Antwort.

![](../../images/5.png)

![](../../images/6.png)

### Intelligentes Snap-Fenster

Ein intelligenter Assistent, der neben anderen Anwendungsfenstern angedockt werden kann. Wechseln Sie schnell zwischen verschieden konfigurierten KI-Assistenten darin, um Fragen zu stellen. Der Roboter generiert Antworten basierend auf Ihrer verbundenen Wissensbasis und unterstützt das Senden von Antworten mit einem Klick direkt in Ihre Konversation.

![](../../images/7.png)

### Eine Frage, mehrere Antworten: Einfach vergleichen

Sie müssen Ihre Frage nicht wiederholen. Konsultieren Sie gleichzeitig mehrere "KI-Experten" und sehen Sie ihre Antworten nebeneinander in einer einzigen Oberfläche. Dies ermöglicht einfachen Vergleich und hilft Ihnen, die beste Schlussfolgerung zu ziehen.

![](../../images/8.png)

### Ein-Klick-Starterball

Klicken Sie auf den schwebenden Ball auf Ihrem Desktop, um das Hauptanwendungsfenster von ChatClaw sofort aufzuwecken oder zu öffnen.

![](../../images/9.png)

## Server-Modus-Bereitstellung

ChatClaw kann im Server-Modus ausgeführt werden (keine Desktop-GUI erforderlich), über einen Browser zugänglich.

### Binärdatei direkt ausführen

Laden Sie die Binärdatei für Ihre Plattform von [GitHub Releases](https://github.com/chatwiki/chatclaw/releases) herunter:

|| Plattform | Datei |
||----------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

Öffnen Sie http://localhost:8080 in Ihrem Browser.

Der Server lauscht standardmäßig auf `0.0.0.0:8080`. Sie können Host und Port über Umgebungsvariablen anpassen:

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

Öffnen Sie http://localhost:8080 in Ihrem Browser.

### Docker Compose

Erstellen Sie eine `docker-compose.yml`-Datei:

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

Dann ausführen:

```bash
docker compose up -d
```

Öffnen Sie http://localhost:8080 in Ihrem Browser. Zum Stoppen: `docker compose down`. Daten werden im Volume `chatclaw-data` persistiert.

## Technologie-Stack

|| Schicht | Technologie |
||-------|-----------|
|| Desktop-Framework | [Wails v3](https://wails.io/) (Go + WebView) |
|| Backend-Sprache | [Go 1.26](https://go.dev/) |
|| Frontend-Framework | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| UI-Komponenten | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| Styling | [Tailwind CSS v4](https://tailwindcss.com/) |
|| Zustandsverwaltung | [Pinia](https://pinia.vuejs.org/) |
|| Build-Tool | [Vite](https://vite.dev/) |
|| KI-Framework | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| KI-Modellanbieter | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| Datenbank | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (Vektorsuche) |
|| Internationalisierung | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| Task-Runner | [Task](https://taskfile.dev/) |
|| Icons | [Lucide](https://lucide.dev/) |

## Projektstruktur

```
ChatClaw_D2/
├── main.go                     # Anwendungseinstiegspunkt
├── go.mod / go.sum             # Go-Modul-Abhängigkeiten
├── Taskfile.yml                # Task-Runner-Konfiguration
├── build/                      # Build-Konfigurationen und Plattform-Assets
│   ├── config.yml              # Wails-Build-Konfiguration
│   ├── darwin/                 # macOS-Build-Einstellungen und Berechtigungen
│   ├── windows/                # Windows-Installer (NSIS/MSIX) und Manifeste
│   ├── linux/                  # Linux-Paketierung (AppImage, nfpm)
│   ├── ios/                    # iOS-Build-Einstellungen
│   └── android:                # Android-Build-Einstellungen
├── frontend:                   # Vue 3-Frontend-Anwendung
│   ├── package.json            # Node.js-Abhängigkeiten
│   ├── vite.config.ts          # Vite-Bundler-Konfiguration
│   ├── components.json         # shadcn-vue-Konfiguration
│   ├── index.html              # Hauptfenster-Einstieg
│   ├── floatingball.html       # Floating-Ball-Fenster-Einstieg
│   ├── selection.html          # Textauswahl-Popup-Einstieg
│   ├── winsnap.html            # Snap-Fenster-Einstieg
│   └── src/
│       ├── assets/             # Icons (SVG), Bilder und globales CSS
│       ├── components/         # Gemeinsame Komponenten
│       │   ├── layout/         # App-Layout, Seitenleiste, Titelleiste
│       │   └── ui:             # shadcn-vue-Primitiven (button, dialog, toast…)
│       ├── composables:        # Vue-Composables (wiederverwendbare Logik)
│       ├── i18n:               # Frontend-i18n-Setup
│       ├── locales:            # Übersetzungsdateien (zh-CN, en-US…)
│       ├── lib:                # Hilfsfunktionen
│       ├── pages:              # Seitenebene-Ansichten
│       │   ├── assistant:      # KI-Chat-Assistent-Seite und Komponenten
│       │   ├── knowledge:      # Wissensbasis-Verwaltungsseite
│       │   ├── multiask:       # Multi-Modell-Vergleichsseite
│       │   └── settings:       # Einstellungsseite (Anbieter, Modelle, Tools…)
│       ├── stores:             # Pinia-Zustandsspeicher
│       ├── floatingball:       # Floating-Ball-Mini-App
│       ├── selection:          # Textauswahl-Mini-App
│       └── winsnap:            # Snap-Fenster-Mini-App
├── internal:                   # Private Go-Pakete
│   ├── bootstrap:              # Anwendungsinitialisierung und Verdrahtung
│   ├── define:                 # Konstanten, integrierte Anbieter, Umgebungs-Flags
│   ├── device:                 # Geräteidentifikation
│   ├── eino:                   # AI/LLM-Integrationsschicht
│   │   ├── agent:              # Agenten-Orchestrierung
│   │   ├── chatmodel:          # Chat-Modell-Fabrik (Multi-Anbieter)
│   │   ├── embedding:          # Embedding-Modell-Fabrik
│   │   ├── filesystem:         # AI-Agent-Dateisystem-Tools
│   │   ├── parser:             # Dokument-Parser (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # Dokument-Verarbeitungspipeline
│   │   ├── raptor:             # RAPTOR rekursive Zusammenfassung
│   │   ├── splitter:           # Text-Splitter-Fabrik
│   │   └── tools:              # AI-Tool-Integrationen (Browser, Suche, Rechner…)
│   ├── errs:                   # i18n-fehlerbewusste Fehlerbehandlung
│   ├── fts:                    # Volltext-Such-Tokenizer
│   ├── logger:                 # Strukturiertes Logging
│   ├── services:               # Geschäftslogik-Services
│   │   ├── agents:             # Agent-CRUD
│   │   ├── app:                # Anwendungslebenszyklus
│   │   ├── browser:            # Browser-Automatisierung (chromedp)
│   │   ├── chat:               # Chat und Streaming
│   │   ├── conversations:      # Gesprächsverwaltung
│   │   ├── document:           # Dokument-Upload und Vektorisierung
│   │   ├── floatingball:       # Floating-Ball-Fenster (plattformübergreifend)
│   │   ├── i18n:               # Backend-i18n
│   │   ├── library:            # Wissensbibliothek-CRUD
│   │   ├── multiask:           # Multi-Modell-Q&A
│   │   ├── providers:          # KI-Anbieter-Konfiguration
│   │   ├── retrieval:          # RAG-Abrufservice
│   │   ├── settings:           # Benutzereinstellungen mit Cache
│   │   ├── textselection:      # Bildschirm-Textauswahl (plattformübergreifend)
│   │   ├── thumbnail:          # Fenster-Miniaturansicht-Erfassung
│   │   ├── tray:               # System-Tray
│   │   ├── updater:            # Auto-Update (GitHub/Gitee)
│   │   ├── windows:            # Fensterverwaltung und Snap-Service
│   │   └── winsnapchat:        # Snap-Chat-Sitzungs-Service
│   ├── sqlite:                 # Datenbankschicht (Bun ORM + Migrationen)
│   └── taskmanager:            # Hintergrundaufgaben-Scheduler
├── pkg:                         # Öffentliche/wiederverwendbare Go-Pakete
│   ├── webviewpanel:           # Plattformübergreifender WebView-Panel-Manager
│   ├── winsnap:                # Fenster-Snap-Engine (macOS/Windows/Linux)
│   └── winutil:                # Fenster-Aktivierungswerkzeuge
├── docs:                       # Entwicklungsdokumentation
└── images:                      # README-Screenshots
```
