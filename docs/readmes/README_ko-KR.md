<p align="center">
<img src="./frontend/src/assets/images/logo-floatingball.png" width="150" height="150">
</p>

<h1 align="center">ChatClaw</h1>

<p align="center">
  <strong>5분 만에 OpenClaw 같은 개인 AI 에이전트를 확보하세요. 샌드박스 보안, 작고 빠름</strong>
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

5분 만에 OpenClaw 같은 개인 AI 에이전트를 확보하세요. 샌드박스 보안, macOS 및 Windows용 초소형 30MB 설치 프로그램(1분 설치). WhatsApp, Telegram, Slack, Discord, Gmail, DingTalk, WeChat Work, QQ, Feishu 및 기타 메시징 앱에 연결됩니다. 기본 제공 스킬 마켓, 지식 베이스, 메모리, MCP, 예약 작업. Go로 개발: 빠르고 리소스 사용량 적음.

## 미리보기

### AI 채팅 어시스턴트

AI 어시스턴트에 질문을 하면 지식 베이스를 지능적으로 검색하여 관련 답변을 생성합니다.

![](../../images/1.png)

### PPT 빠른 생성

스마트 어시스턴트에 한 문장 명령을 보내면 PowerPoint 프레젠테이션을 자동으로 생성합니다.

![](../../images/2.png)

### 스킬 관리자

명령어를 사용하여 로봇이 컴퓨터에 설치된 기능을 찾거나 새 확장 프로그램을 설치하도록 합니다.

![](../../images/3.png)

### 지식 베이스 | 문서 벡터화 저장

문서(TXT, PDF, Word, Excel, CSV, HTML, Markdown 등)를 업로드하면 시스템이 자동으로 구문 분석, 분할 및 벡터 임베딩으로 변환하여 AI 모델이 정확한 검색과 활용을 할 수 있도록 개인 지식 베이스에 저장합니다.

![](../../images/4.png)

### 텍스트 선택 즉시 Q&A

화면上の任意 टेक्स्ट를 선택하면 자동으로 복사되어浮动 질문 상자에 채워집니다. 한 번의 클릭으로 AI 어시스턴트에 보내면 즉시 답변을 얻습니다.

![](../../images/5.png)

![](../../images/6.png)

### 스마트 스냅 창

다른 애플리케이션 창에 스냅할 수 있는 지능형 어시스턴트. 其中快速切换不同配置的 AI 助手进行提问。 연결된 지식 베이스に基づいて 답변을 생성하고 원클릭으로 회화를 통해 응답을 보낼 수 있습니다.

![](../../images/7.png)

### 한 질문, 여러 답변: 쉽게 비교

질문을 반복할 필요 없이 여러 "AI 전문가"에게 동시에 상담하고同一 인터페이스에서 나란히 답변을 확인하세요. 비교가 용이하고 최고의 결론에 도달하는 데 도움이 됩니다.

![](../../images/8.png)

### 원클릭 런처 볼

데스크톱의浮动 볼을 클릭하면 ChatClaw 기본 애플리케이션 창을 깨우거나 열 수 있습니다.

![](../../images/9.png)

## 서버 모드 배포

ChatClaw은 서버 모드로 실행可能(데스크톱 GUI 불필요), 브라우저를 통해 액세스 가능.

### 바이너리 직접 실행

[GitHub Releases](https://github.com/chatwiki/chatclaw/releases)에서 해당 플랫폼의 바이너리를 다운로드하세요:

|| 플랫폼 | 파일 |
||------|------|
|| Linux x86_64 | `ChatClaw-server-linux-amd64` |
|| Linux ARM64 | `ChatClaw-server-linux-arm64` |

```bash
chmod +x ChatClaw-server-linux-amd64
./ChatClaw-server-linux-amd64
```

브라우저에서 http://localhost:8080을 여세요.

서버는 기본적으로 `0.0.0.0:8080`에서 수신합니다. 환경 변수를 통해 호스트와 포트를 사용자 지정할 수 있습니다:

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

브라우저에서 http://localhost:8080을 여세요.

### Docker Compose

`docker-compose.yml` 파일 생성:

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

그 후 실행:

```bash
docker compose up -d
```

브라우저에서 http://localhost:8080을 여세요. 중지: `docker compose down`. 데이터는 `chatclaw-data` 볼륨에 지속됩니다.

## 기술 스택

|| 레이어 | 기술 |
||------|------|
|| 데스크톱 프레임워크 | [Wails v3](https://wails.io/) (Go + WebView) |
|| 백엔드 언어 | [Go 1.26](https://go.dev/) |
|| 프론트엔드 프레임워크 | [Vue 3](https://vuejs.org/) + [TypeScript](https://www.typescriptlang.org/) |
|| UI 컴포넌트 | [shadcn-vue](https://www.shadcn-vue.com/) + [Reka UI](https://reka-ui.com/) |
|| 스타일링 | [Tailwind CSS v4](https://tailwindcss.com/) |
|| 상태 관리 | [Pinia](https://pinia.vuejs.org/) |
|| 빌드 도구 | [Vite](https://vite.dev/) |
|| AI 프레임워크 | [Eino](https://github.com/cloudwego/eino) (ByteDance CloudWeGo) |
|| AI 모델 제공자 | OpenAI / Claude / Gemini / Ollama / DeepSeek / Doubao / Qwen / Zhipu / Grok |
|| 데이터베이스 | [SQLite](https://www.sqlite.org/) + [sqlite-vec](https://github.com/asg017/sqlite-vec) (벡터 검색) |
|| 국제화 | [go-i18n](https://github.com/nicksnyder/go-i18n) + [vue-i18n](https://vue-i18n.intlify.dev/) |
|| 작업 실행기 | [Task](https://taskfile.dev/) |
|| 아이콘 | [Lucide](https://lucide.dev/) |

## 프로젝트 구조

```
ChatClaw_D2/
├── main.go                     # 애플리케이션 진입점
├── go.mod / go.sum             # Go 모듈 종속성
├── Taskfile.yml                # 작업 실행기 구성
├── build/                      # 빌드 구성 및 플랫폼 자산
│   ├── config.yml              # Wails 빌드 구성
│   ├── darwin/                 # macOS 빌드 설정 및 자격 증명
│   ├── windows/                # Windows 설치 관리자 (NSIS/MSIX) 및 매니페스트
│   ├── linux/                  # Linux 패키징 (AppImage, nfpm)
│   ├── ios/                    # iOS 빌드 설정
│   └── android/                # Android 빌드 설정
├── frontend/                   # Vue 3 프론트엔드 애플리케이션
│   ├── package.json            # Node.js 종속성
│   ├── vite.config.ts          # Vite 번들러 구성
│   ├── components.json         # shadcn-vue 구성
│   ├── index.html              # 기본 창 진입
│   ├── floatingball.html       #浮动 볼 창 진입
│   ├── selection.html          # 텍스트 선택 팝업 진입
│   ├── winsnap.html            # 스냅 창 진입
│   └── src/
│       ├── assets/             # 아이콘(SVG), 이미지 및 전역 CSS
│       ├── components/         # 공유 컴포넌트
│       │   ├── layout/         # 앱 레이아웃, 사이드바, 타이틀바
│       │   └── ui:             # shadcn-vue 기본 요소 (button, dialog, toast…)
│       ├── composables/        # Vue 컴포저블(재사용 가능한 로직)
│       ├── i18n/               # 프론트엔드 i18n 설정
│       ├── locales/            # 번역 파일 (zh-CN, en-US…)
│       ├── lib/                # 유틸리티 함수
│       ├── pages/              # 페이지 수준 뷰
│       │   ├── assistant/      # AI 채팅 어시스턴트 페이지 및 컴포넌트
│       │   ├── knowledge/      # 지식 베이스 관리 페이지
│       │   ├── multiask/       # 멀티 모델 비교 페이지
│       │   └── settings/       # 설정 페이지(제공자, 모델, 도구…)
│       ├── stores:             # Pinia 상태 저장소
│       ├── floatingball:       #浮动 볼 미니 앱
│       ├── selection:          # 텍스트 선택 미니 앱
│       └── winsnap:            # 스냅 창 미니 앱
├── internal:                   # 비공개 Go 패키지
│   ├── bootstrap:              # 애플리케이션 초기화 및 와이어링
│   ├── define:                 # 상수, 기본 제공자, 환경 플래그
│   ├── device:                 # 장치 식별
│   ├── eino:                   # AI/LLM 통합 레이어
│   │   ├── agent:              # Agent 오케스트레이션
│   │   ├── chatmodel:          # 채팅 모델 팩토리(멀티 제공자)
│   │   ├── embedding:          # 임베딩 모델 팩토리
│   │   ├── filesystem:         # AI Agent 파일 시스템 도구
│   │   ├── parser:             # 문서 파서 (PDF, DOCX, XLSX, CSV)
│   │   ├── processor:          # 문서 처리 파이프라인
│   │   ├── raptor:             # RAPTOR 재귀 요약
│   │   ├── splitter:           # 텍스트 분할기 팩토리
│   │   └── tools:              # AI 도구 통합(브라우저, 검색, 계산기…)
│   ├── errs:                   # i18n 인식 오류 처리
│   ├── fts:                    # 전체 텍스트 검색 토크나이저
│   ├── logger:                 # 구조화된 로깅
│   ├── services:               # 비즈니스 로직 서비스
│   │   ├── agents:             # Agent CRUD
│   │   ├── app:                # 애플리케이션 수명 주기
│   │   ├── browser:            # 브라우저 자동화 (chromedp)
│   │   ├── chat:               # 채팅 및 스트리밍
│   │   ├── conversations:      # 대화 관리
│   │   ├── document:           # 문서 업로드 및 벡터화
│   │   ├── floatingball:       #浮动 볼 창(크로스 플랫폼)
│   │   ├── i18n:               # 백엔드 i18n
│   │   ├── library:            # 지식 라이브러리 CRUD
│   │   ├── multiask:           # 멀티 모델 Q&A
│   │   ├── providers:          # AI 제공자 구성
│   │   ├── retrieval:          # RAG 검색 서비스
│   │   ├── settings:           # 사용자 설정 및 캐시
│   │   ├── textselection:      # 화면 텍스트 선택(크로스 플랫폼)
│   │   ├── thumbnail:          # 창 축소판 캡처
│   │   ├── tray:               # 시스템 트레이
│   │   ├── updater:            # 자동 업데이트 (GitHub/Gitee)
│   │   ├── windows:            # 창 관리 및 스냅 서비스
│   │   └── winsnapchat:        # 스냅 채팅 세션 서비스
│   ├── sqlite:                 # 데이터베이스 레이어 (Bun ORM + 마이그레이션)
│   └── taskmanager:            # 백그라운드 작업 스케줄러
├── pkg:                         # 공개/재사용 가능한 Go 패키지
│   ├── webviewpanel:           # 크로스 플랫폼 웹뷰 패널 관리자
│   ├── winsnap:                # 창 스냅 엔진 (macOS/Windows/Linux)
│   └── winutil:                # 창 활성화 유틸리티
├── docs:                       # 개발 문서
└── images:                      # README 스크린샷
```
