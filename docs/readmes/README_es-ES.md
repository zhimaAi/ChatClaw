<p align="center">
<img src="./frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>Obtén un agente AI personal como OpenClaw en 5 minutos. Seguridad Sandbox, pequeño y rápido</strong>
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

Obtén un agente AI personal como OpenClaw en 5 minutos. Seguro con Sandbox, con un instalador ultracompacto de 30MB para macOS y Windows (instala en 1 minuto). Se conecta a WhatsApp, Telegram, Slack, Discord, Gmail, DingTalk, WeChat Work, QQ, Feishu y otras apps de mensajería. Mercado de habilidades integrado, Base de Conocimiento, Memoria, MCP, Tareas Programadas. Desarrollado en Go: rápido y bajo consumo de recursos.

## Previsualizaciones

### Asistente de Chat AI

Haz cualquier pregunta a tu asistente AI, y buscará inteligentemente en tu base de conocimiento para generar una respuesta relevante.

![](../../images/1.png)

### Generación Rápida de PPT

Envía un comando de una frase al asistente inteligente para crear y generar automáticamente una presentación de PowerPoint.

![](../../images/2.png)

### Gestor de Habilidades

Usa un comando para que el robot te ayude a buscar funciones instaladas en tu computadora o instalar nuevos plugins de extensión.

![](../../images/3.png)

### Base de Conocimiento | Almacenamiento Vectorial de Documentos

Sube tus documentos (ej. TXT, PDF, Word, Excel, CSV, HTML, Markdown). El sistema analizará, dividirá y convertirá automáticamente a embeddings vectoriales, almacenándolos en tu base de conocimiento privada para recuperación y uso preciso por el modelo AI.

![](../../images/4.png)

### Selección de Texto para Q&A Instantáneo

Selecciona cualquier texto en tu pantalla, se copiará automáticamente y llenará una caja de pregunta rápida flotante. Con un clic, envíalo al asistente AI para una respuesta inmediata.

![](../../images/5.png)

![](../../images/6.png)

### Ventana de Snap Inteligente

Un asistente inteligente que puede adherirse junto a otras ventanas de aplicaciones. Cambia rápidamente entre diferentes asistentes AI configurados para hacer preguntas. El robot genera respuestas basadas en tu base de conocimiento conectada y soporta enviar respuestas con un clic directamente a tu conversación.

![](../../images/7.png)

### Una Pregunta, Múltiples Respuestas: Compara con Facilidad

No necesitas repetir tu consulta. Consulta múltiples "expertos AI" simultáneamente y ve sus respuestas una al lado de la otra en una sola interfaz. Esto permite fácil comparación y te ayuda a llegar a la mejor conclusión.

![](../../images/8.png)

### Bola Lanzadora de Un Clic

Haz clic en la bola flotante de tu escritorio para despertar o abrir instantáneamente la ventana de la aplicación principal de ChatClaw.

![](../../images/9.png)

## Despliegue en Modo Servidor

ChatClaw puede ejecutarse como servidor (sin necesidad de GUI de escritorio), accesible a través del navegador.

### Binario Directo

Descarga el binario para tu plataforma desde [GitHub Releases](https://github.com/chatwiki/chatclaw/releases):

|| Plataforma | Archivo |
||----------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

Abre http://localhost:8080 en tu navegador.

El servidor escucha en `0.0.0.0:8080` por defecto. Puedes personalizar host y puerto mediante variables de entorno:

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

Abre http://localhost:8080 en tu navegador.

### Docker Compose

Crea un archivo `docker-compose.yml`:

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

Luego ejecuta:

```bash
docker compose up -d
```

Abre http://localhost:8080 en tu navegador. Para detener: `docker compose down`. Los datos persisten en el volumen `chatclaw-data`.

## Stack Tecnológico

|| Capa | Tecnología |
||-------|-----------|
|| Framework de Escritorio | [Wails v3](https://wails.io/) (Go + WebView) |
|| Lenguaje Backend | [Go 1.26](https://go.dev/) |
|| Framework Frontend | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| Componentes UI | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| Estilizado | [Tailwind CSS v4](https://tailwindcss.com/) |
|| Gestión de Estado | [Pinia](https://pinia.vuejs.org/) |
|| Herramienta de Build | [Vite](https://vite.dev/) |
|| Framework AI | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| Proveedores de Modelos AI | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| Base de Datos | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (búsqueda vectorial) |
|| Internacionalización | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| Ejecutor de Tareas | [Task](https://taskfile.dev/) |
|| Iconos | [Lucide](https://lucide.dev/) |

## Estructura del Proyecto

```
ChatClaw_D2/
├── main.go                     # Punto de entrada de la aplicación
├── go.mod / go.sum             # Dependencias del módulo Go
├── Taskfile.yml                # Configuración del ejecutor de tareas
├── build/                      # Configuraciones de build y activos de plataforma
│   ├── config.yml              # Configuración de build de Wails
│   ├── darwin/                 # Configuración de build de macOS y permisos
│   ├── windows/                # Instalador de Windows (NSIS/MSIX) y manifiestos
│   ├── linux/                  # Empaquetado de Linux (AppImage, nfpm)
│   ├── ios/                    # Configuración de build de iOS
│   └── android:                # Configuración de build de Android
├── frontend:                   # Aplicación frontend Vue 3
│   ├── package.json            # Dependencias de Node.js
│   ├── vite.config.ts          # Configuración del bundle Vite
│   ├── components.json         # Configuración de shadcn-vue
│   ├── index.html              # Entrada de ventana principal
│   ├── floatingball.html       # Entrada de ventana de bola flotante
│   ├── selection.html          # Entrada de popup de selección de texto
│   ├── winsnap.html            # Entrada de ventana Snap
│   └── src/
│       ├── assets/             # Iconos (SVG), imágenes y CSS global
│       ├── components/         # Componentes compartidos
│       │   ├── layout/         # Layout de app, barra lateral, barra de título
│       │   └── ui:             # Primitivos de shadcn-vue (button, dialog, toast…)
│       ├── composables:        # Composables de Vue (lógica reutilizable)
│       ├── i18n:               # Configuración de i18n del frontend
│       ├── locales:            # Archivos de traducción (zh-CN, en-US…)
│       ├── lib:                # Funciones de utilidad
│       ├── pages:              # Vistas a nivel de página
│       │   ├── assistant:      # Página de asistente de chat AI y componentes
│       │   ├── knowledge:      # Página de gestión de base de conocimiento
│       │   ├── multiask:       # Página de comparación de múltiples modelos
│       │   └── settings:       # Página de configuración (proveedores, modelos, herramientas…)
│       ├── stores:             # Almacenes de estado Pinia
│       ├── floatingball:       # Mini-app de bola flotante
│       ├── selection:          # Mini-app de selección de texto
│       └── winsnap:            # Mini-app de ventana Snap
├── internal:                   # Paquetes Go privados
│   ├── bootstrap:              # Inicialización de aplicación y cableado
│   ├── define:                 # Constantes, proveedores integrados, flags de entorno
│   ├── device:                 # Identificación de dispositivo
│   ├── eino:                   # Capa de integración AI/LLM
│   │   ├── agent:              # Orquestación de Agent
│   │   ├── chatmodel:          # Fábrica de modelos de chat (multi-proveedor)
│   │   ├── embedding:          # Fábrica de modelos de embedding
│   │   ├── filesystem:         # Herramientas de sistema de archivos para AI Agent
│   │   ├── parser:             # Parsers de documentos (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # Pipeline de procesamiento de documentos
│   │   ├── raptor:             # Resumen recursivo RAPTOR
│   │   ├── splitter:          # Fábrica de divisores de texto
│   │   └── tools:              # Integraciones de herramientas AI (buscador, búsqueda, calculadora…)
│   ├── errs:                   # Manejo de errores con soporte i18n
│   ├── fts:                    # Tokenizador de búsqueda de texto completo
│   ├── logger:                 # Logging estructurado
│   ├── services:               # Servicios de lógica de negocio
│   │   ├── agents:             # CRUD de Agent
│   │   ├── app:                # Ciclo de vida de aplicación
│   │   ├── browser:            # Automatización de navegador (chromedp)
│   │   ├── chat:               # Chat y streaming
│   │   ├── conversations:      # Gestión de conversaciones
│   │   ├── document:           # Carga de documentos y vectorización
│   │   ├── floatingball:       # Ventana de bola flotante (multi-plataforma)
│   │   ├── i18n:               # i18n del backend
│   │   ├── library:            # CRUD de biblioteca de conocimiento
│   │   ├── multiask:           # Q&A multi-modelo
│   │   ├── providers:          # Configuración de proveedor AI
│   │   ├── retrieval:          # Servicio de retrieval RAG
│   │   ├── settings:           # Configuración de usuario con caché
│   │   ├── textselection:      # Selección de texto en pantalla (multi-plataforma)
│   │   ├── thumbnail:          # Captura de miniaturas de ventana
│   │   ├── tray:               # Bandeja del sistema
│   │   ├── updater:            # Actualización automática (GitHub/Gitee)
│   │   ├── windows:            # Gestión de ventanas y servicio de snap
│   │   └── winsnapchat:        # Servicio de sesión de chat snap
│   ├── sqlite:                 # Capa de base de datos (Bun ORM + migraciones)
│   └── taskmanager:            # Programador de tareas en segundo plano
├── pkg:                         # Paquetes Go públicos/reutilizables
│   ├── webviewpanel:           # Gestor de panel WebView multiplataforma
│   ├── winsnap:                # Motor de snap de ventanas (macOS/Windows/Linux)
│   └── winutil:                # Utilidades de activación de ventanas
├── docs:                       # Documentación de desarrollo
└── images:                      # Capturas de pantalla del README
```
