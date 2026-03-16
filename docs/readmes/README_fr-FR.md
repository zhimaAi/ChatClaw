<p align="center">
<img src="../../frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>Obtenez un agent AI personnel comme OpenClaw en 5 minutes. Sécurité Sandbox, petit et rapide</strong>
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

Obtenez un agent AI personnel comme OpenClaw en 5 minutes. Sécurisé par Sandbox, avec un installateur ultra-petit de 30 Mo pour macOS et Windows (installation en 1 minute). Se connecte à WhatsApp, Telegram, Slack, Discord, Gmail, DingTalk, WeChat Work, QQ, Feishu et autres applications de messagerie. Marché de compétences intégré, Base de Connaissances, Mémoire, MCP, Tâches Planifiées. Développé en Go : rapide et faible utilisation des ressources.

## Aperçus

### Assistant de Chat AI

Posez n'importe quelle question à votre assistant AI, et il cherchera intelligemment dans votre base de connaissances pour générer une réponse pertinente.

![](../../images/1.png)

### Génération Rapide de PPT

Envoyez une commande d'une phrase à l'assistant intelligent pour créer et générer automatiquement une présentation PowerPoint.

![](../../images/2.png)

### Gestionnaire de Compétences

Utilisez une commande pour que le robot vous aide à trouver les fonctionnalités installées sur votre ordinateur ou installer de nouvelles extensions.

![](../../images/3.png)

### Base de Connaissances | Stockage Vectoriel de Documents

Téléchargez vos documents (ex: TXT, PDF, Word, Excel, CSV, HTML, Markdown). Le système les analysera, divisera et convertira automatiquement en embeddings vectoriels, les stockant dans votre base de connaissances privée pour une récupération précise par le modèle AI.

![](../../images/4.png)

### Sélection de Texte pour Q&R Instantanée

Sélectionnez n'importe quel texte sur votre écran, il sera automatiquement copié et rempli dans une boîte de question flottante. En un clic, envoyez-le à l'assistant AI pour une réponse immédiate.

![](../../images/5.png)

![](../../images/6.png)

### Fenêtre Snap Intelligente

Un assistant intelligent qui peut s'aligner aux côtés d'autres fenêtres d'application. Basculez rapidement entre les assistants AI configurés différemment pour poser des questions. Le robot génère des réponses basées sur votre base de connaissances connectée et prend en charge l'envoi de réponses en un clic directement dans votre conversation.

![](../../images/7.png)

### Une Question, Réponses Multiples : Comparez Facilement

Vous n'avez pas besoin de répéter votre question. Consultez plusieurs "experts AI" simultanément et voyez leurs réponses côte à côte dans une seule interface. Cela permet une comparaison facile et vous aide à arriver à la meilleure conclusion.

![](../../images/8.png)

### Balle de Lancement en Un Clic

Cliquez sur la balle flottante sur votre bureau pour réveiller ou ouvrir la fenêtre de l'application principale ChatClaw.

![](../../images/9.png)

## Déploiement en Mode Serveur

ChatClaw peut fonctionner en mode serveur (pas besoin de GUI bureau), accessible via un navigateur.

### Binaire Direct

Téléchargez le binaire pour votre plateforme depuis [GitHub Releases](https://github.com/chatwiki/chatclaw/releases):

|| Plateforme | Fichier |
||----------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

Ouvrez http://localhost:8080 dans votre navigateur.

Le serveur écoute sur `0.0.0.0:8080` par défaut. Vous pouvez personnaliser l'hôte et le port via des variables d'environnement :

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

Ouvrez http://localhost:8080 dans votre navigateur.

### Docker Compose

Créez un fichier `docker-compose.yml` :

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

Puis exécutez :

```bash
docker compose up -d
```

Ouvrez http://localhost:8080 dans votre navigateur. Pour arrêter : `docker compose down`. Les données persistent dans le volume `chatclaw-data`.

## Stack Technologique

|| Couche | Technologie |
||-------|-----------|
|| Framework Bureau | [Wails v3](https://wails.io/) (Go + WebView) |
|| Langage Backend | [Go 1.26](https://go.dev/) |
|| Framework Frontend | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| Composants UI | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| Style | [Tailwind CSS v4](https://tailwindcss.com/) |
|| Gestion d'État | [Pinia](https://pinia.vuejs.org/) |
|| Outil de Build | [Vite](https://vite.dev/) |
|| Framework AI | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| Fournisseurs de Modèles AI | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| Base de Données | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (recherche vectorielle) |
|| Internationalisation | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| Exécuteur de Tâches | [Task](https://taskfile.dev/) |
|| Icônes | [Lucide](https://lucide.dev/) |

## Structure du Projet

```
ChatClaw_D2/
├── main.go                     # Point d'entrée de l'application
├── go.mod / go.sum             # Dépendances du module Go
├── Taskfile.yml                # Configuration de l'exécuteur de tâches
├── build/                      # Configurations de build et ressources plateforme
│   ├── config.yml              # Configuration de build Wails
│   ├── darwin/                 # Paramètres de build macOS et entitlements
│   ├── windows/                # Installateur Windows (NSIS/MSIX) et manifestes
│   ├── linux/                  # Packaging Linux (AppImage, nfpm)
│   ├── ios/                    # Paramètres de build iOS
│   └── android:                # Paramètres de build Android
├── frontend:                   # Application frontend Vue 3
│   ├── package.json            # Dépendances Node.js
│   ├── vite.config.ts          # Configuration du bundleur Vite
│   ├── components.json         # Configuration shadcn-vue
│   ├── index.html              # Entrée de la fenêtre principale
│   ├── floatingball.html       # Entrée de la fenêtre balle flottante
│   ├── selection.html          # Entrée du popup de sélection de texte
│   ├── winsnap.html            # Entrée de la fenêtre Snap
│   └── src/
│       ├── assets/             # Icônes (SVG), images et CSS global
│       ├── components:         # Composants partagés
│       │   ├── layout:         # Layout de l'app, barre latérale, barre de titre
│       │   └── ui:             # Primitifs shadcn-vue (button, dialog, toast…)
│       ├── composables:        # Composables Vue (logique réutilisable)
│       ├── i18n:               # Configuration i18n du frontend
│       ├── locales:            # Fichiers de traduction (zh-CN, en-US…)
│       ├── lib:                # Fonctions utilitaires
│       ├── pages:              # Vues au niveau page
│       │   ├── assistant:      # Page assistant chat AI et composants
│       │   ├── knowledge:      # Page gestion base de connaissances
│       │   ├── multiask:       # Page comparaison multi-modèles
│       │   └── settings:       # Page paramètres (fournisseurs, modèles, outils…)
│       ├── stores:             # Magasins d'état Pinia
│       ├── floatingball:       # Mini-app balle flottante
│       ├── selection:          # Mini-app sélection de texte
│       └── winsnap:            # Mini-app fenêtre Snap
├── internal:                   # Paquets Go privés
│   ├── bootstrap:              # Initialisation de l'application et câblage
│   ├── define:                 # Constantes, fournisseurs intégrés, drapeaux env
│   ├── device:                 # Identification de l'appareil
│   ├── eino:                   # Couche d'intégration AI/LLM
│   │   ├── agent:              # Orchestration d'Agent
│   │   ├── chatmodel:          # Usine de modèles de chat (multi-fournisseur)
│   │   ├── embedding:          # Usine de modèles d'embedding
│   │   ├── filesystem:         # Outils système de fichiers pour Agent AI
│   │   ├── parser:             # Parseurs de documents (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # Pipeline de traitement de documents
│   │   ├── raptor:             # Résumé récursif RAPTOR
│   │   ├── splitter:           # Usine de diviseurs de texte
│   │   └── tools:              # Intégrations d'outils AI (navigateur, recherche, calculatrice…)
│   ├── errs:                   # Gestion d'erreurs compatible i18n
│   ├── fts:                    # Tokeniseur de recherche plein texte
│   ├── logger:                 # Journalisation structurée
│   ├── services:               # Services de logique métier
│   │   ├── agents:             # CRUD d'Agent
│   │   ├── app:                # Cycle de vie de l'application
│   │   ├── browser:            # Automatisation du navigateur (chromedp)
│   │   ├── chat:               # Chat et streaming
│   │   ├── conversations:      # Gestion des conversations
│   │   ├── document:           # Upload de documents et vectorisation
│   │   ├── floatingball:       # Fenêtre balle flottante (cross-platform)
│   │   ├── i18n:               # i18n backend
│   │   ├── library:            # CRUD de bibliothèque de connaissances
│   │   ├── multiask:           # Q&R multi-modèles
│   │   ├── providers:          # Configuration du fournisseur AI
│   │   ├── retrieval:          # Service de récupération RAG
│   │   ├── settings:           # Paramètres utilisateur avec cache
│   │   ├── textselection:      # Sélection de texte à l'écran (cross-platform)
│   │   ├── thumbnail:          # Capture de miniature de fenêtre
│   │   ├── tray:               # Barre système
│   │   ├── updater:            # Mise à jour automatique (GitHub/Gitee)
│   │   ├── windows:            # Gestion de fenêtres et service Snap
│   │   └── winsnapchat:        # Service de session de chat Snap
│   ├── sqlite:                 # Couche base de données (Bun ORM + migrations)
│   └── taskmanager:            # Planificateur de tâches en arrière-plan
├── pkg:                         # Paquets Go publics/réutilisables
│   ├── webviewpanel:           # Gestionnaire de panel WebView multiplateforme
│   ├── winsnap:                # Moteur de snap de fenêtres (macOS/Windows/Linux)
│   └── winutil:                # Utilitaires d'activation de fenêtre
├── docs:                       # Documentation de développement
└── images:                      # Captures d'écran du README
```
