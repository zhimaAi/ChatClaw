# ChatClaw

## 前置依赖

### npm

```
nvm install --lts
```

### Wails3 cli

```shell
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

### Windows CGO 环境配置（UCRT64）

本项目使用 CGO 版本的 sqlite-vec 扩展，需要配置 C 编译环境。

#### 1. 安装 MSYS2

从 [https://www.msys2.org/](https://www.msys2.org/) 下载并安装 MSYS2。

#### 2. 安装 GCC 和 SQLite3 开发库

打开 **MSYS2 UCRT64** 终端，执行：

```bash
# 安装 GCC 编译器
pacman -S mingw-w64-ucrt-x86_64-gcc

# 安装 SQLite3 开发库（包含 sqlite3.h 头文件）
pacman -S mingw-w64-ucrt-x86_64-sqlite3
```

#### 3. 配置 PATH 环境变量

将 MSYS2 UCRT64 的 bin 目录添加到系统 PATH：

```
C:\msys64\ucrt64\bin
```

#### 4. 验证安装

```bash
gcc --version
# 应输出类似: gcc.exe (Rev8, Built by MSYS2 project) 15.x.x
```

#### 5. 构建项目

CGO 已在 `build/windows/Taskfile.yml` 中默认启用（`CGO_ENABLED=1`）。

---

#### Windows 安装包依赖（makensis）

Windows 打包（生成安装包）需要安装 **makensis（NSIS）**。

- 参考文档：`https://wails.io/zh-Hans/docs/next/guides/windows-installer/`
- 安装后将 makensis 安装目录添加到 **Path** 环境变量中（确保命令行可直接执行 `makensis`）

## 开发

```bash
# gui模式
wails3 dev

# server模式 (only linux)
wails3 task build:server
wails3 task run:server
```

## Windows 打包

```bash
# amd64
wails3 task windows:build ARCH=amd64 DEV=false
cd bin
zip ChatClaw_windows_amd64.zip ChatClaw.exe
cd ..
wails3 task windows:package ARCH=amd64 DEV=false
```

## macos 多架构打包

```bash
# arm64
wails3 task darwin:sign:notarize ARCH=arm64 DEV=false  # wails3 task darwin:sign ARCH=arm64 DEV=true
cd bin
tar -czf ChatClaw_darwin_arm64.tar.gz -C ChatClaw.app/Contents/MacOS ChatClaw
cd ..

# amd64
wails3 task darwin:sign:notarize ARCH=amd64 DEV=false # wails3 task darwin:sign ARCH=amd64 DEV=true
cd bin
tar -czf ChatClaw_darwin_amd64.tar.gz -C ChatClaw.app/Contents/MacOS ChatClaw
cd ..

# arm64+amd64
wails3 task darwin:sign:notarize UNIVERSAL=true DEV=false
```

## Linux Server 模式构建、打包

```bash
cd frontend && npm run build && cd ..
wails3 task build:docker PLATFORM=multi
docker push registry.cn-hangzhou.aliyuncs.com/chatwiki/chatclaw:latest
```