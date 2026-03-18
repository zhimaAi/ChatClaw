<p align="center">
<img src="../../frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>5 minutah dobite osebnega agenta AI, kot je OpenClaw. Varnost Sandbox, majhen in hiter</strong>
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

5 minutah dobite osebnega agenta AI, kot je OpenClaw. Z varnostjo Sandbox, z ultra majhnim 30MB namestitvenim programom za macOS in Windows (namestitev v 1 minuti). Poveže se s WhatsApp, Telegram, Slack, Discord, Gmail, DingTalk, WeChat Work, QQ, Feishu in drugimi aplikacijami za sporočanje. Vgrajeno tržnico spretnosti, baza znanja, spomin, MCP, načrtovane naloge. Razvito v Go: hitro in majhna poraba virov.

## Predogledi

### AI Chatbot Asistent

Zastavite svojemu AI asistentu katerokoli vprašanje; pametno bo preiskal vašo bazo znanja in ustvaril ustrezni odgovor.

![](../../images/previews/en/image1.png)

### Hitro ustvarjanje PPT

Pošljite enostavni ukaz pametnemu asistentu, da samodejno ustvari in pripravi predstavitev PowerPoint.

![](../../images/previews/en/image3.png)

### Upravitelj spretnosti

Z ukazom naj vam asistent pomaga najti nameščene funkcije na računalniku ali namestiti nove vtičnike razširitev.

![](../../images/previews/en/image5.png)

### MCP: neomejene razširitve zmožnosti

Dodajte zunanje MCP strežnike za varno in učinkovito povezavo z različnimi viri podatkov in orodji; asistent lahko presega vsakodnevne naloge v profesionalne delovne tokove.

![](../../images/previews/en/image6.png)

### Način peskovnika: dvojna zaščita

Izberite med izvajanjem v peskovniku (izolacija na ravni OS, omejen obseg ukazov) in izvajanjem v domačem okolju (bolj prilagodljivo). Prosto preklapljajte med varnostjo in udobjem.

![](../../images/previews/en/image8.png)

### Spomin: bolj naravna in pametna interakcija

Omogočite kontekstualne pogovore in personalizirano pomoč. Asistent se lahko nenehno uči in razvija kot rastoči partner.

![](../../images/previews/en/image9.png)

### Skupna timska baza znanja

Odobrite dostop z enim klikom do ChatWiki za sinhronizacijo robotov in baz znanja, skupno rabo nastavitev in nadzor dovoljenj članov.

![](../../images/previews/en/image10.png)

### Baza znanja | vektorska shramba dokumentov

Naložite dokumente (TXT, PDF, Word, Excel, CSV, HTML, Markdown). Sistem jih samodejno razčleni, razdeli in pretvori v vektorske vdelave za natančno iskanje.

![](../../images/previews/en/image11.png)

### Bogate integracije IM kanalov

Integrirajte ponudnike IM (Feishu, WeCom, QQ, DingTalk, LINE, Discord, WhatsApp, X/Twitter, Telegram itd.) prek SDK za hitro omogočanje ustvarjanja kanalov, upravljanja uporabnikov in sporočanja.

![](../../images/previews/en/image12.png)

### Načrtovane naloge

Naj asistent samodejno izvaja dejanja ob vnaprej določenih časih ali intervalih: opomniki, ponavljajoče delo in vzdrževanje na ravni sistema.

![](../../images/previews/en/image13.png)

### Izbira besedila za takojšnje vprašanje in odgovor

Izberite poljubno besedilo na zaslonu; samodejno se kopira v lebdeče polje za hitro vprašanje. En klik za vprašanje, takojšnji odgovori.

![](../../images/previews/en/image14.png)

![](../../images/previews/en/image15.png)

### Pametna stranska vrstica

Pripnite asistenta ob druga okna, hitro preklapljajte med različno konfiguriranimi asistenti in z enim klikom pošljite ustvarjene odgovore v pogovore.

![](../../images/previews/en/image16.png)

### Eno vprašanje, več odgovorov: enostavna primerjava

Posvetujte se z več "AI strokovnjaki" hkrati in si oglejte njihove odgovore drug ob drugem za lažjo primerjavo.

![](../../images/previews/en/image17.png)

### Žoga zagnanja z enim klikom

Kliknite lebdečo žogo na namizju, da takoj odprete ali aktivirate glavno okno ChatClaw.

![](../../images/previews/en/image18.png)

## Namestitev v načinu strežnika

ChatClaw lahko deluje kot strežnik (ni potrebna namizna GUI), dostopen prek brskalnika.

### Neposredna binarna datoteka

Prenesite binarno datoteko za svojo platformo z [GitHub Releases](https://github.com/chatwiki/chatclaw/releases):

|| Platforma | Datoteka |
||----------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

Odprite http://localhost:8080 v brskalniku.

Strežnik privzeto posluša na `0.0.0.0:8080`. Host in vrata lahko prilagodite prek okolijskih spremenljivk:

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

Odprite http://localhost:8080 v brskalniku.

### Docker Compose

Ustvarite datoteko `docker-compose.yml`:

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

Nato zaženite:

```bash
docker compose up -d
```

Odprite http://localhost:8080 v brskalniku. Za zaustavitev: `docker compose down`. Podatki so shranjeni v volumnu `chatclaw-data`.

## Tehnološki sklad

|| Plast | Tehnologija |
||-------|-----------|
|| Namizno ogrodje | [Wails v3](https://wails.io/) (Go + WebView) |
|| Back-end jezik | [Go 1.26](https://go.dev/) |
|| Front-end ogrodje | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| UI komponente | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| Oblikovanje | [Tailwind CSS v4](https://tailwindcss.com/) |
|| Upravljanje stanja | [Pinia](https://pinia.vuejs.org/) |
|| Orodje za gradnjo | [Vite](https://vite.dev/) |
|| AI ogrodje | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| Ponudniki AI modelov | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| Podatkovna baza | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (vektorsko iskanje) |
|| Mednarodizacija | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| Izvajalec nalog | [Task](https://taskfile.dev/) |
|| Ikone | [Lucide](https://lucide.dev/) |

## Struktura projekta

```
ChatClaw_D2/
├── main.go                     # Vstopna točka aplikacije
├── go.mod / go.sum             # Odvisnosti Go modula
├── Taskfile.yml                # Konfiguracija izvajalca nalog
├── build:                      # Konfiguracije gradnje in platformi sredstva
│   ├── config.yml              # Konfiguracija gradnje Wails
│   ├── darwin:                 # Nastavitve gradnje macOS in pooblastila
│   ├── windows:                # Namestitveni program Windows (NSIS/MSIX) in manifesti
│   ├── linux:                  # Pakiranje Linux (AppImage, nfpm)
│   ├── ios:                    # Nastavitve gradnje iOS
│   └── android:                # Nastavitve gradnje Android
├── frontend:                   # Vue 3 front-end aplikacija
│   ├── package.json            # Odvisnosti Node.js
│   ├── vite.config.ts          # Konfiguracija Vite bundlerja
│   ├── components.json         # Konfiguracija shadcn-vue
│   ├── index.html              # Vstop v glavno okno
│   ├── floatingball.html       # Vstop v okno plavajoče krogle
│   ├── selection.html          # Vstop v pojavno okno za izbiro besedila
│   ├── winsnap.html            # Vstop v okno Snap
│   └── src/
│       ├── assets:             # Ikone (SVG), slike in globalni CSS
│       ├── components:         # Skupne komponente
│       │   ├── layout:         # Postavitev aplikacije, stranski meni, naslovna vrstica
│       │   └── ui:             # Primitivi shadcn-vue (button, dialog, toast…)
│       ├── composables:        # Vue kompozicije (ponovno uporabna logika)
│       ├── i18n:               # Namestitev i18n za front-end
│       ├── locales:            # Prevajalne datoteke (zh-CN, en-US…)
│       ├── lib:                # Pomožne funkcije
│       ├── pages:              # Pogledi na ravni strani
│       │   ├── assistant:      # Stran AI pomočnika za klepet in komponente
│       │   ├── knowledge:      # Stran za upravljanje baze znanja
│       │   ├── multiask:       # Stran za primerjavo več modelov
│       │   └── settings:       # Stran z nastavitvami (ponudniki, modeli, orodja…)
│       ├── stores:             # Trgovine stanja Pinia
│       ├── floatingball:       # Mini aplikacija plavajoče krogle
│       ├── selection:          # Mini aplikacija za izbiro besedila
│       └── winsnap:            # Mini aplikacija za okno Snap
├── internal:                   # Zasebni Go paketi
│   ├── bootstrap:              # Inicializacija aplikacije in povezovanje
│   ├── define:                 # Konstant, vgrajeni ponudniki, okolijski zastavice
│   ├── device:                 # Identifikacija naprave
│   ├── eino:                   # Plast integracije AI/LLM
│   │   ├── agent:              # Orchesteracija agenta
│   │   ├── chatmodel:          # Tvornica modelov klepeta (več ponudnikov)
│   │   ├── embedding:          # Tvornica modelov vložkov
│   │   ├── filesystem:         # Orodja datotečnega sistema za AI agenta
│   │   ├── parser:             # Analizatorji dokumentov (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # Cevovod za obdelavo dokumentov
│   │   ├── raptor:             # RAPTOR rekurzivno povzemanje
│   │   ├── splitter:           # Tvornica razdelilnikov besedila
│   │   └── tools:              # Integracije AI orodij (brskalnik, iskalnik, kalkulator…)
│   ├── errs:                   # Obravnava napak z zavedanjem i18n
│   ├── fts:                    # Žetonizator za iskanje polnega besedila
│   ├── logger:                 # Strukturirano beleženje
│   ├── services:               # Storitve poslovne logike
│   │   ├── agents:             # CRUD agenta
│   │   ├── app:                # Življenjski cikel aplikacije
│   │   ├── browser:            # Avtomatizacija brskalnika (chromedp)
│   │   ├── chat:               # Klepet in pretakanje
│   │   ├── conversations:      # Upravljanje pogovorov
│   │   ├── document:           # Nalaganje dokumentov in vektorizacija
│   │   ├── floatingball:       # Okno plavajoče krogle (več platform)
│   │   ├── i18n:               # i18n za back-end
│   │   ├── library:            # CRUD knjižnice znanja
│   │   ├── multiask:           # Q&A z več modeli
│   │   ├── providers:          # Konfiguracija ponudnika AI
│   │   ├── retrieval:          # Storitev pridobivanja RAG
│   │   ├── settings:          # Uporabniške nastavitve s predpomnilnikom
│   │   ├── textselection:      # Izbira besedila na zaslonu (več platform)
│   │   ├── thumbnail:          # Zajemanje sličic oken
│   │   ├── tray:               # Sistemska vrstica
│   │   ├── updater:            # Samodejna posodobitev (GitHub/Gitee)
│   │   ├── windows:            # Upravljanje oken in storitev Snap
│   │   └── winsnapchat:        # Storitev klepetalne seje Snap
│   ├── sqlite:                 # Plast podatkovne baze (Bun ORM + migracije)
│   └── taskmanager:            # Načrtovalnik nalog v ozadju
├── pkg:                         # Javni/ponovno uporabni Go paketi
│   ├── webviewpanel:           # Upravljalnik spletnega pogleda za več platform
│   ├── winsnap:                # Pogon za pripenjanje oken (macOS/Windows/Linux)
│   └── winutil:                # Pripomočki za aktivacijo oken
├── docs:                       # Dokumentacija za razvoj
└── images:                      # Posnetki zaslona README
```




