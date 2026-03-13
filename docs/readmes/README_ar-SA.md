<p align="center">
<img src="./frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>احصل على وكيل AI شخصي يشبه OpenClaw في 5 دقائق. أمان Sandbox، صغير وسريع</strong>
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

احصل على وكيل AI شخصي يشبه OpenClaw في 5 دقائق. مؤمّن بـ Sandbox، مع برنامج تثبيت صغير للغاية 30 ميجابايت لـ macOS و Windows (تثبيت في دقيقة واحدة). يتصل بـ WhatsApp وTelegram وSlack وDiscord وGmail وDingTalk وWeChat Work وQQ وFeishu وتطبيقات المراسلة الأخرى. يحتوي على سوق المهارات وقاعدة المعرفة والذاكرة وMCP والمهام المجدولة. مطوّر بـ Go: سريع واستهلاك منخفض للموارد.

## المعاينة

### مساعد الدردشة AI

اسأل مساعد AI أي سؤال، وسيبحث ذكيًا في قاعدة معرفتك لإنشاء إجابة ذات صلة.

![](../../images/1.png)

### إنشاء PPT السريع

أرسل أمرًا من جملة واحدة إلى المساعد الذكي لإنشاء عرض تقديمي PowerPoint تلقائيًا.

![](../../images/2.png)

### مدير المهارات

استخدم أمرًا ليقوم الروبوت بمساعدتك في البحث عن الميزات المثبتة على جهاز الكمبيوتر أو تثبيت ملحقات جديدة.

![](../../images/3.png)

### قاعدة المعرفة | تخزين متجه المستندات

قم بتحميل مستنداتك (مثل TXT وPDF وWord وExcel وCSV وHTML وMarkdown). سيقوم النظام تلقائيًا بتحليلها وتقسيمها وتحويلها إلى تضمينات متجهة، وتخزينها في قاعدة المعرفة الشخصية الخاصة بك للاسترجاع الدقيق والاستخدام بواسطة نموذج الذكاء الاصطناعي.

![](../../images/4.png)

### تحديد النص للسؤال والجواب الفوري

حدد أي نص على شاشتك، سيتم نسخه تلقائيًا وملؤه في مربع السؤال العائم. بنقرة واحدة، أرسله إلى مساعد الذكاء الاصطناعي للحصول على إجابة فورية.

![](../../images/5.png)

![](../../images/6.png)

### نافذة Snap الذكية

مساعد ذكي يمكنه الانزلاق بجانب نوافذ التطبيقات الأخرى.قم بالتبديل السريع بين مساعدي AI المُعدّين بشكل مختلف لطرح الأسئلة. يولّد الروبوت إجابات بناءً على قاعدة المعرفة المرتبطة بك ويدعم إرسال الردود بنقرة واحدة إلى محادثتك.

![](../../images/7.png)

### سؤال واحد، إجابات متعددة: قارن بسهولة

لا تحتاج إلى تكرار سؤالك. استشر عدة "خبراء AI" في نفس الوقت واعرض إجاباتهم جنبًا في واجهة واحدة. يسهل المقارنة ويساعدك على الوصول إلى أفضل استنتاج.

![](../../images/8.png)

### كرة الإطلاق بلمسة واحدة

انقر على الكرّة العائمة على سطح المكتب لإيقاظ نافذة تطبيق ChatClaw الرئيسية أو فتحها.

![](../../images/9.png)

## نشر وضع الخادم

يمكن لـ ChatClaw العمل كخادم (لا حاجة لواجهة رسومية للسطح المكتب)، ويمكن الوصول إليه عبر المتصفح.

### الثنائي المباشر

قم بتنزيل الثنائي لمنصتك من [GitHub Releases](https://github.com/chatwiki/chatclaw/releases):

|| المنصة | الملف |
||----------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd4
```

افتح http://localhost:8080 في متصفحك.

الخادم يستمع على `0.0.0.0:8080` افتراضيًا. يمكنك تخصيص المضيف والمنفذ عبر متغيرات البيئة:

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

افتح http://localhost:8080 في متصفحك.

### Docker Compose

أنشئ ملف `docker-compose.yml`:

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

ثم شغّل:

```bash
docker compose up -d
```

افتح http://localhost:8080 في متصفحك. للإيقاف: `docker compose down`. البيانات موجودة بشكل مستمر في حجم `chatclaw-data`.

## تقنية المكدس

|| الطبقة | التقنية |
||-------|-----------|
|| إطار سطح المكتب | [Wails v3](https://wails.io/) (Go + WebView) |
|| لغة الواجهة الخلفية | [Go 1.26](https://go.dev/) |
|| إطار الواجهة الأمامية | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| مكونات UI | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| التصميم | [Tailwind CSS v4](https://tailwindcss.com/) |
|| إدارة الحالة | [Pinia](https://pinia.vuejs.org/) |
|| أداة البناء | [Vite](https://vite.dev/) |
|| إطار الذكاء الاصطناعي | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| موفري نماذج AI | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| قاعدة البيانات | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (بحث المتجهات) |
|| التدويل | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| مُشغّل المهام | [Task](https://taskfile.dev/) |
|| الأيقونات | [Lucide](https://lucide.dev/) |

## هيكل المشروع

```
ChatClaw_D2/
├── main.go                     # نقطة دخول التطبيق
├── go.mod / go.sum             # تبعيات وحدة Go
├── Taskfile.yml                # تكوين مُشغّل المهام
├── build/                      # تكوينات البناء وأصول المنصة
│   ├── config.yml              # تكوين بناء Wails
│   ├── darwin/                 # إعدادات بناء macOS والصلاحيات
│   ├── windows/                # مُثبّت Windows (NSIS/MSIX) والبيانات الوصفية
│   ├── linux/                  # تجميع Linux (AppImage, nfpm)
│   ├── ios/                    # إعدادات بناء iOS
│   └── android:                # إعدادات بناء Android
├── frontend:                   # تطبيق Vue 3 للواجهة الأمامية
│   ├── package.json            # تبعيات Node.js
│   ├── vite.config.ts          # تكوين حزمة Vite
│   ├── components.json         # تكوين shadcn-vue
│   ├── index.html              # نقطة دخول النافذة الرئيسية
│   ├── floatingball.html       # نقطة دخول نافذة الكرّة العائمة
│   ├── selection.html          # نقطة دخول نافذة تحديد النص
│   ├── winsnap.html            # نقطة دخول نافذة Snap
│   └── src/
│       ├── assets/             # الأيقونات (SVG)، الصور وCSS العام
│       ├── components/         # المكونات المشتركة
│       │   ├── layout/         # تخطيط التطبيق، الشريط الجانبي، شريط العنوان
│       │   └── ui:             # عناصر shadcn-vue الأساسية (button, dialog, toast…)
│       ├── composables:        # Vue composables (منطق قابل لإعادة الاستخدام)
│       ├── i18n:               # إعداد التدويل للواجهة الأمامية
│       ├── locales:            # ملفات الترجمة (zh-CN, en-US…)
│       ├── lib:                # وظائف الأدوات
│       ├── pages:              # عروض على مستوى الصفحة
│       │   ├── assistant:      # صفحة مساعد الدردشة AI والمكونات
│       │   ├── knowledge:      # صفحة إدارة قاعدة المعرفة
│       │   ├── multiask:       # صفحة مقارنة النماذج المتعددة
│       │   └── settings:       # صفحة الإعدادات (المزودين، النماذج، الأدوات…)
│       ├── stores:             # مخازن حالة Pinia
│       ├── floatingball:       # تطبيق مصغّر للكرّة العائمة
│       ├── selection:          # تطبيق مصغّر لتحديد النص
│       └── winsnap:            # تطبيق مصغّر لنافذة Snap
├── internal:                   # حزم Go الخاصة
│   ├── bootstrap:              # تهيئة التطبيق والتوصيل
│   ├── define:                 # الثوابت، المزودين المدمجين، علامات البيئة
│   ├── device:                 # تعريف الجهاز
│   ├── eino:                   # طبقة تكامل AI/LLM
│   │   ├── agent:              # تنسيق الوكيل
│   │   ├── chatmodel:          # مصنع نموذج الدردشة (متعدد المزودين)
│   │   ├── embedding:          # مصنع نموذج التضمين
│   │   ├── filesystem:         # أدوات نظام ملفات وكيل AI
│   │   ├── parser:             # محللو المستندات (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # خط معالجة المستندات
│   │   ├── raptor:             # الملخص التكراري RAPTOR
│   │   ├── splitter:           # مصنع مقسم النصوص
│   │   └── tools:              # تكامل أدوات AI (المتصفح، البحث، الآلة الحاسبة…)
│   ├── errs:                   # معالجة الأخطاء المدركة للتدويل
│   ├── fts:                    # محرك بحث النص الكامل
│   ├── logger:                 # التسجيل المُهيكَل
│   ├── services:               # خدمات المنطق التجاري
│   │   ├── agents:             # CRUD للوكلاء
│   │   ├── app:                # دورة حياة التطبيق
│   │   ├── browser:            # أتمتة المتصفح (chromedp)
│   │   ├── chat:               # الدردشة والبث
│   │   ├── conversations:      # إدارة المحادثات
│   │   ├── document:           # تحميل المستندات وتوجيهها
│   │   ├── floatingball:       # نافذة الكرّة العائمة (عبر المنصات)
│   │   ├── i18n:               # التدويل للواجهة الخلفية
│   │   ├── library:            # CRUD لمكتبة المعرفة
│   │   ├── multiask:           # سؤال وجواب نماذج متعددة
│   │   ├── providers:          # تكوين مزود AI
│   │   ├── retrieval:          # خدمة استرجاع RAG
│   │   ├── settings:           # إعدادات المستخدم مع التخزين المؤقت
│   │   ├── textselection:      # تحديد نص الشاشة (عبر المنصات)
│   │   ├── thumbnail:          # التقاط الصورة المصغرة للنافذة
│   │   ├── tray:               # علبة النظام
│   │   ├── updater:            # التحديث التلقائي (GitHub/Gitee)
│   │   ├── windows:            # إدارة النوافذ وخدمة Snap
│   │   └── winsnapchat:        # خدمة جلسة الدردشة Snap
│   ├── sqlite:                 # طبقة قاعدة البيانات (Bun ORM + الترحيلات)
│   └── taskmanager:            # جدولة المهام الخلفية
├── pkg:                         # حزم Go العامة/القابلة لإعادة الاستخدام
│   ├── webviewpanel:           # مدير لوحة WebView عبر المنصات
│   ├── winsnap:                # محرك نافذة Snap (macOS/Windows/Linux)
│   └── winutil:                # أدوات تنشيط النافذة
├── docs:                       # وثائق التطوير
└── images:                      # لقطات شاشة README
```
