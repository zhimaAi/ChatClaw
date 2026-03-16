<p align="center">
<img src="../../frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>5 dakikada OpenClaw benzeri kişisel AI ajanı edinin. Sandbox güvenliği, küçük ve hızlı</strong>
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

5 dakikada OpenClaw benzeri kişisel AI ajanı edinin. Sandbox güvenliği ile, macOS ve Windows için ultra küçük 30MB yükleyici (1 dakikada kurulum). WhatsApp, Telegram, Slack, Discord, Gmail, DingTalk, WeChat Work, QQ, Feishu ve diğer mesajlaşma uygulamalarına bağlanır. Yerleşik Yetenek Pazarı, Bilgi Tabanı, Hafıza, MCP, Planlı Görevler. Go ile geliştirildi: hızlı ve düşük kaynak kullanımı.

## Önizlemeler

### AI Sohbet Asistanı

AI asistanınıza herhangi bir soru sorun, ilgili bir yanıt oluşturmak için bilgi tabanınızı akıllıca arayacaktır.

![](../../images/1.png)

### PPT Hızlı Oluşturma

Akıllı asistan'a tek cümlelik bir komut gönderin ve otomatik olarak bir PowerPoint sunumu oluşturun.

![](../../images/2.png)

### Yetenek Yöneticisi

Bilgisayarınızda yüklü özellikleri bulmanıza veya yeni uzantı eklentileri yüklemenize yardımcı olması için bir komut kullanın.

![](../../images/3.png)

### Bilgi Tabanı | Belge Vektörleştirme Deposu

Belgelerinizi yükleyin (örn. TXT, PDF, Word, Excel, CSV, HTML, Markdown). Sistem otomatik olarak ayrıştıracak, bölecek ve vektör gömülerine dönüştürecek ve AI modeli tarafından hassas alma ve kullanım için özel bilgi tabanınızda saklayacaktır.

![](../../images/4.png)

### Anında S&C için Metin Seçimi

Ekrandaki herhangi bir metni seçin, otomatik olarak kopyalanır ve havuzlu hızlı soru kutusuna doldurulur. Tek tıklamayla, anında yanıt için AI asistanına gönderin.

![](../../images/5.png)

![](../../images/6.png)

### Akıllı Pencere Yakalama

Diğer uygulama pencerelerinin yanına yaklaşabilen akıllı bir asistan. Sorular sormak için içinde farklı yapılandırılmış AI asistanları arasında hızla geçiş yapın. Robot, bağlı bilgi tabanınıza göre yanıtlar oluşturur ve tek tıklamayla yanıtları doğrudan görüşmenize göndermeyi destekler.

![](../../images/7.png)

### Bir Soru, Birden Fazla Yanıt: Kolayca Karşılaştırın

Sorunuzu tekrarlamanıza gerek yok. Aynı anda birden fazla "AI uzmanı"na danışın ve yanıtlarını tek bir arayüzde yan yana görüntüleyin. Bu kolay karşılaştırma sağlar ve en iyi sonuca ulaşmanıza yardımcı olur.

![](../../images/8.png)

### Tek Tıklama Başlatıcı Topu

ChatClaw ana uygulama penceresini uyandırmak veya açmak için masaüstünüzdeki havuzlu topa tıklayın.

![](../../images/9.png)

## Sunucu Modu Dağıtımı

ChatClaw sunucu modunda çalıştırılabilir (masaüstü GUI gerekmez), tarayıcı üzerinden erişilebilir.

### İkili Dosyayı Doğrudan Çalıştır

Platformunuz için ikili dosyayı [GitHub Releases](https://github.com/chatwiki/chatclaw/releases)'den indirin:

|| Platform | Dosya |
||----------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

Tarayıcınızda http://localhost:8080'i açın.

Sunucu varsayılan olarak `0.0.0.0:8080` dinler. Ana bilgisayar ve bağlantı noktasını ortam değişkenleri aracılığıyla özelleştirebilirsiniz:

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

Tarayıcınızda http://localhost:8080'i açın.

### Docker Compose

Bir `docker-compose.yml` dosyası oluşturun:

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

Ardından çalıştırın:

```bash
docker compose up -d
```

Tarayıcınızda http://localhost:8080'i açın. Durdurmak için: `docker compose down`. Veriler `chatclaw-data` hacminde kalıcıdır.

## Teknoloji Yığını

|| Katman | Teknoloji |
||-------|-----------|
|| Masaüstü Çerçevesi | [Wails v3](https://wails.io/) (Go + WebView) |
|| Arka Uç Dili | [Go 1.26](https://go.dev/) |
|| Ön Uç Çerçevesi | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| UI Bileşenleri | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| Stil | [Tailwind CSS v4](https://tailwindcss.com/) |
|| Durum Yönetimi | [Pinia](https://pinia.vuejs.org/) |
|| Yapı Aracı | [Vite](https://vite.dev/) |
|| AI Çerçevesi | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| AI Model Sağlayıcıları | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| Veritabanı | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (vektör arama) |
|| Uluslararasılaştırma | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| Görev Çalıştırıcı | [Task](https://taskfile.dev/) |
|| İkonlar | [Lucide](https://lucide.dev/) |

## Proje Yapısı

```
ChatClaw_D2/
├── main.go                     # Uygulama giriş noktası
├── go.mod / go.sum             # Go modülü bağımlılıkları
├── Taskfile.yml                # Görev çalıştırıcı yapılandırması
├── build/                      # Yapı yapılandırmaları ve platform varlıkları
│   ├── config.yml              # Wails yapı yapılandırması
│   ├── darwin/                 # macOS yapı ayarları ve yetkilendirmeler
│   ├── windows/                # Windows yükleyici (NSIS/MSIX) ve bildirimler
│   ├── linux/                  # Linux paketleme (AppImage, nfpm)
│   ├── ios/                    # iOS yapı ayarları
│   └── android:                # Android yapı ayarları
├── frontend:                   # Vue 3 ön uç uygulaması
│   ├── package.json            # Node.js bağımlılıkları
│   ├── vite.config.ts          # Vite paketleyici yapılandırması
│   ├── components.json         # shadcn-vue yapılandırması
│   ├── index.html              # Ana pencere girişi
│   ├── floatingball.html       # Top pencere girişi
│   ├── selection.html          # Metin seçimi açılır penceresi girişi
│   ├── winsnap.html            # Yakalama penceresi girişi
│   └── src/
│       ├── assets:             # İkonlar (SVG), resimler ve genel CSS
│       ├── components:         # Paylaşılan bileşenler
│       │   ├── layout:         # Uygulama düzeni, kenar çubuğu, başlık çubuğu
│       │   └── ui:             # shadcn-vue ilkelleri (düğme, iletişim kutusu, bildirim…)
│       ├── composables:        # Vue bileşenleri (yeniden kullanılabilir mantık)
│       ├── i18n:               # Ön uç i18n kurulumu
│       ├── locales:            # Çeviri dosyaları (zh-CN, en-US…)
│       ├── lib:                # Yardımcı işlevler
│       ├── pages:              # Sayfa düzeyinde görünümler
│       │   ├── assistant:      # AI sohbet asistanı sayfası ve bileşenleri
│       │   ├── knowledge:      # Bilgi tabanı yönetimi sayfası
│       │   ├── multiask:       # Çoklu model karşılaştırma sayfası
│       │   └── settings:       # Ayarlar sayfası (sağlayıcılar, modeller, araçlar…)
│       ├── stores:             # Pinia durum depoları
│       ├── floatingball:       # Toplu mini uygulama
│       ├── selection:          # Metin seçimi mini uygulaması
│       └── winsnap:            # Yakalama penceresi mini uygulaması
├── internal:                   # Özel Go paketleri
│   ├── bootstrap:              # Uygulama başlatma ve kablolama
│   ├── define:                 # Sabitler, yerleşik sağlayıcılar, ortam bayrakları
│   ├── device:                 # Cihaz tanımlama
│   ├── eino:                   # AI/LLM entegrasyon katmanı
│   │   ├── agent:              # Ajan orkestrasyonu
│   │   ├── chatmodel:          # Sohbet modeli fabrikası (çoklu sağlayıcı)
│   │   ├── embedding:          # Gömme modeli fabrikası
│   │   ├── filesystem:         # AI Ajanı dosya sistemi araçları
│   │   ├── parser:             # Belge ayrıştırıcıları (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # Belge işleme hattı
│   │   ├── raptor:             # RAPTOR özyinelemeli özetleme
│   │   ├── splitter:           # Metin bölücü fabrikası
│   │   └── tools:              # AI araç entegrasyonları (tarayıcı, arama, hesap makinesi…)
│   ├── errs:                   # i18n farkındalıklı hata işleme
│   ├── fts:                    # Tam metin arama belirteci
│   ├── logger:                 # Yapılandırılmış günlük
│   ├── services:               # İş mantığı hizmetleri
│   │   ├── agents:             # Ajan CRUD
│   │   ├── app:                # Uygulama yaşam döngüsü
│   │   ├── browser:            # Tarayıcı otomasyonu (chromedp)
│   │   ├── chat:               # Sohbet ve akış
│   │   ├── conversations:      # Konuşma yönetimi
│   │   ├── document:           # Belge yükleme ve vektörleştirme
│   │   ├── floatingball:       # Toplu pencere (çapraz platform)
│   │   ├── i18n:               # Arka uç i18n
│   │   ├── library:            # Bilgi kütüphanesi CRUD
│   │   ├── multiask:           # Çoklu model S&C
│   │   ├── providers:          # AI sağlayıcı yapılandırması
│   │   ├── retrieval:          # RAG alma hizmeti
│   │   ├── settings:           # Önbellekli kullanıcı ayarları
│   │   ├── textselection:      # Ekran metin seçimi (çapraz platform)
│   │   ├── thumbnail:          # Pencere küçük resim yakalama
│   │   ├── tray:               # Sistem tepsisi
│   │   ├── updater:            # Otomatik güncelleme (GitHub/Gitee)
│   │   ├── windows:            # Pencere yönetimi ve yakalama hizmeti
│   │   └── winsnapchat:        # Yakalama sohbet oturumu hizmeti
│   ├── sqlite:                 # Veritabanı katmanı (Bun ORM + geçişler)
│   └── taskmanager:            # Arka plan görev zamanlayıcı
├── pkg:                         # Genel/yeniden kullanılabilir Go paketleri
│   ├── webviewpanel:           # Çapraz platform WebView panel yöneticisi
│   ├── winsnap:                # Pencere yakalama motoru (macOS/Windows/Linux)
│   └── winutil:                # Pencere etkinleştirme yardımcı programları
├── docs:                       # Geliştirme belgeleri
└── images:                      # README ekran görüntüleri
```
