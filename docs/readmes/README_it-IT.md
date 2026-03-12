<p align="center">
<img src="./frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>Ottieni un agente AI personale come OpenClaw in 5 minuti. Sicurezza Sandbox, piccolo e veloce</strong>
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

Ottieni un agente AI personale come OpenClaw in 5 minuti. Protetto da Sandbox, con un installatore ultra-compatto di 30MB per macOS e Windows (installa in 1 minuto). Si connette a WhatsApp, Telegram, Slack, Discord, Gmail, DingTalk, WeChat Work, QQ, Feishu e altre app di messaggistica. Marketplace competenze integrato, Base di Conoscenza, Memoria, MCP, Attività Programmate. Sviluppato in Go: veloce e basso utilizzo delle risorse.

## Anteprime

### Assistente Chat AI

Fai qualsiasi domanda al tuo assistente AI, e cercherà intelligentemente nella tua base di conoscenza per generare una risposta pertinente.

![](../../images/1.png)

### Generazione PPT Rapida

Invia un comando di una frase all'assistente intelligente per creare e generare automaticamente una presentazione PowerPoint.

![](../../images/2.png)

### Gestore Competenze

Usa un comando per far sì che il robot ti aiuti a trovare le funzionalità installate sul tuo computer o installare nuovi plugin di estensione.

![](../../images/3.png)

### Base di Conoscenza | Archiviazione Vettoriale Documenti

Carica i tuoi documenti (es. TXT, PDF, Word, Excel, CSV, HTML, Markdown). Il sistema analizzerà automaticamente, dividerà e convertirà in embedding vettoriali, archiviandoli nella tua base di conoscenza privata per il recupero preciso e l'utilizzo da parte del modello AI.

![](../../images/4.png)

### Selezione Testo per Q&A Istantaneo

Seleziona qualsiasi testo sullo schermo, verrà automaticamente copiato e riempito in una casella di domanda rapida fluttuante. Con un clic, invialo all'assistente AI per una risposta immediata.

![](../../images/5.png)

![](../../images/6.png)

### Finestra Snap Intelligente

Un assistente intelligente che può agganciarsi accanto ad altre finestre di applicazioni. Passa rapidamente tra assistenti AI configurati diversamente per fare domande. Il robot genera risposte basate sulla tua base di conoscenza collegata e supporta l'invio di risposte con un clic direttamente nella tua conversazione.

![](../../images/7.png)

### Una Domanda, Risposte Multiple: Confronta Facilmente

Non devi ripetere la tua domanda. Consulta più "esperti AI" contemporaneamente e visualizza le loro risposte affiancate in un'unica interfaccia. Questo consente un facile confronto e ti aiuta a raggiungere la migliore conclusione.

![](../../images/8.png)

### Pallina Launcher a Un Clic

Fai clic sulla pallina fluttuante sul tuo desktop per riattivare o aprire la finestra dell'applicazione principale ChatClaw.

![](../../images/9.png)

## Distribuzione Modalità Server

ChatClaw può funzionare come server (nessuna GUI desktop richiesta), accessibile tramite browser.

### Binario Diretto

Scarica il binario per la tua piattaforma da [GitHub Releases](https://github.com/chatwiki/chatclaw/releases):

|| Piattaforma | File |
||----------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

Apri http://localhost:8080 nel tuo browser.

Il server ascolta su `0.0.0.0:8080` per impostazione predefinita. Puoi personalizzare host e porta tramite variabili di ambiente:

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

Apri http://localhost:8080 nel tuo browser.

### Docker Compose

Crea un file `docker-compose.yml`:

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

Quindi esegui:

```bash
docker compose up -d
```

Apri http://localhost:8080 nel tuo browser. Per fermare: `docker compose down`. I dati persistono nel volume `chatclaw-data`.

## Stack Tecnologico

|| Livello | Tecnologia |
||-------|-----------|
|| Framework Desktop | [Wails v3](https://wails.io/) (Go + WebView) |
|| Linguaggio Backend | [Go 1.26](https://go.dev/) |
|| Framework Frontend | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| Componenti UI | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| Styling | [Tailwind CSS v4](https://tailwindcss.com/) |
|| Gestione Stato | [Pinia](https://pinia.vuejs.org/) |
|| Strumento Build | [Vite](https://vite.dev/) |
|| Framework AI | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| Fornitori Modelli AI | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| Database | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (ricerca vettoriale) |
|| Internazionalizzazione | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| Task Runner | [Task](https://taskfile.dev/) |
|| Icone | [Lucide](https://lucide.dev/) |

## Struttura del Progetto

```
ChatClaw_D2/
├── main.go                     # Punto di ingresso applicazione
├── go.mod / go.sum             # Dipendenze modulo Go
├── Taskfile.yml                # Configurazione task runner
├── build/                      # Configurazioni build e asset piattaforma
│   ├── config.yml              # Configurazione build Wails
│   ├── darwin/                 # Impostazioni build macOS e entitlement
│   ├── windows/                # Installatore Windows (NSIS/MSIX) e manifesti
│   ├── linux/                  # Pacchettizzazione Linux (AppImage, nfpm)
│   ├── ios/                    # Impostazioni build iOS
│   └── android:                # Impostazioni build Android
├── frontend:                   # Applicazione frontend Vue 3
│   ├── package.json            # Dipendenze Node.js
│   ├── vite.config.ts          # Configurazione bundler Vite
│   ├── components.json         # Configurazione shadcn-vue
│   ├── index.html              # Entry finestra principale
│   ├── floatingball.html       # Entry finestra palla fluttuante
│   ├── selection.html          # Entry popup selezione testo
│   ├── winsnap.html            # Entry finestra Snap
│   └── src/
│       ├── assets:             # Icone (SVG), immagini e CSS globale
│       ├── components:         # Componenti condivisi
│       │   ├── layout:         # Layout app, sidebar, barra titolo
│       │   └── ui:             # Primitivi shadcn-vue (button, dialog, toast…)
│       ├── composables:        # Composables Vue (logica riutilizzabile)
│       ├── i18n:               # Setup i18n frontend
│       ├── locales:            # File traduzione (zh-CN, en-US…)
│       ├── lib:                # Funzioni utility
│       ├── pages:              # Viste a livello di pagina
│       │   ├── assistant:      # Pagina assistente chat AI e componenti
│       │   ├── knowledge:      # Pagina gestione base conoscenza
│       │   ├── multiask:       # Pagina confronto multi-modello
│       │   └── settings:       # Pagina impostazioni (fornitori, modelli, strumenti…)
│       ├── stores:             # Store stato Pinia
│       ├── floatingball:        # Mini-app palla fluttuante
│       ├── selection:           # Mini-app selezione testo
│       └── winsnap:             # Mini-app finestra Snap
├── internal:                   # Pacchetti Go privati
│   ├── bootstrap:              # Inizializzazione app e cablaggio
│   ├── define:                 # Costanti, fornitori integrati, flag ambiente
│   ├── device:                 # Identificazione dispositivo
│   ├── eino:                   # Livello integrazione AI/LLM
│   │   ├── agent:              # Orchestrazione Agente
│   │   ├── chatmodel:          # Fabbrica modelli chat (multi-fornitore)
│   │   ├── embedding:          # Fabbrica modelli embedding
│   │   ├── filesystem:         # Strumenti filesystem per Agente AI
│   │   ├── parser:             # Parser documenti (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # Pipeline elaborazione documenti
│   │   ├── raptor:             # Riassunto ricorsivo RAPTOR
│   │   ├── splitter:           # Fabbrica divisori testo
│   │   └── tools:              # Integrazioni strumenti AI (browser, ricerca, calcolatrice…)
│   ├── errs:                   # Gestione errori i18n-aware
│   ├── fts:                    # Tokenizer ricerca testo completo
│   ├── logger:                 # Logging strutturato
│   ├── services:               # Servizi logica di business
│   │   ├── agents:             # CRUD Agente
│   │   ├── app:                # Ciclo vita applicazione
│   │   ├── browser:            # Automazione browser (chromedp)
│   │   ├── chat:               # Chat e streaming
│   │   ├── conversations:      # Gestione conversazioni
│   │   ├── document:           # Upload documenti e vettorizzazione
│   │   ├── floatingball:       # Finestra palla fluttuante (cross-platform)
│   │   ├── i18n:               # i18n backend
│   │   ├── library:            # CRUD libreria conoscenza
│   │   ├── multiask:           # Q&A multi-modello
│   │   ├── providers:          # Configurazione fornitore AI
│   │   ├── retrieval:          # Servizio retrieval RAG
│   │   ├── settings:           # Impostazioni utente con cache
│   │   ├── textselection:      # Selezione testo schermo (cross-platform)
│   │   ├── thumbnail:          # Cattura miniatura finestra
│   │   ├── tray:               # System tray
│   │   ├── updater:            # Aggiornamento automatico (GitHub/Gitee)
│   │   ├── windows:            # Gestione finestre e servizio Snap
│   │   └── winsnapchat:        # Servizio sessione chat Snap
│   ├── sqlite:                 # Livello database (Bun ORM + migrazioni)
│   └── taskmanager:            # Scheduler attività in background
├── pkg:                         # Pacchetti Go pubblici/riutilizzabili
│   ├── webviewpanel:           # Gestore panel WebView cross-platform
│   ├── winsnap:                # Motore snap finestre (macOS/Windows/Linux)
│   └── winutil:                # Utility attivazione finestra
├── docs:                       # Documentazione sviluppo
└── images:                      # Screenshot README
```
