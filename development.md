# WillClaw

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

## 开发

```bash
wails3 dev
```

## Windows 多架构打包

```bash
# amd64
wails3 task windows:build ARCH=amd64 DEV=true  # DEV=false 表示打包生产环境二进制，需要做代码签名
wails3 task windows:package ARCH=amd64 DEV=true # DEV=false 表示打包生产环境安装包，需要再做一次代码签名

# arm64
wails3 task windows:build ARCH=arm64 DEV=true # DEV=false 表示打包生产环境二进制，需要做代码签名
wails3 task windows:package ARCH=arm64 DEV=true # DEV=false 表示打包生产环境安装包，需要再做一次代码签名
```

### Windows 安装包依赖（makensis）

Windows 打包（生成安装包）需要安装 **makensis（NSIS）**。

- 参考文档：`https://wails.io/zh-Hans/docs/next/guides/windows-installer/`
- 安装后将 makensis 安装目录添加到 **Path** 环境变量中（确保命令行可直接执行 `makensis`）

## macos 多架构构建、签名、打包

```bash
# arm64
wails3 task darwin:sign ARCH=arm64 DEV=true # wails3 task darwin:sign:notarize ARCH=arm64 DEV=false 表示生产环境打包

# amd64
wails3 task darwin:sign ARCH=amd64 DEV=true  # wails3 task darwin:sign:notarize ARCH=amd64 DEV=false 表示生产环境打包

# arm64+amd64
wails3 task darwin:sign UNIVERSAL=true DEV=true  # wails3 task darwin:sign:notarize UNIVERSAL=true DEV=false 表示生产环境打包
```

## 自动更新（Release 资产命名规范）

应用内置了基于 [go-selfupdate](https://github.com/creativeprojects/go-selfupdate) 的自动更新功能，优先从 GitHub Releases 检查更新，GitHub 不可达时自动回退到 Gitee。

发布新版本时，除了常规安装包（`.dmg` / `-installer.exe`），还需要上传**自动更新资产**到 GitHub Release（以及同步到 Gitee Release）。

### 资产命名格式

`go-selfupdate` 默认按以下格式匹配资产文件名：

```
{cmd}_{os}_{arch}.{ext}
```

其中 `{cmd}` = 可执行文件名（`WillClaw`），`{os}` / `{arch}` 为 Go 标准命名。

| 平台            | 文件名                              |
|----------------|-------------------------------------|
| macOS arm64    | `WillClaw_darwin_arm64.tar.gz`      |
| macOS amd64    | `WillClaw_darwin_amd64.tar.gz`      |
| Windows amd64  | `WillClaw_windows_amd64.zip`        |
| Windows arm64  | `WillClaw_windows_arm64.zip`        |
| Linux amd64    | `WillClaw_linux_amd64.tar.gz`       |
| Linux arm64    | `WillClaw_linux_arm64.tar.gz`       |

### 制作更新资产示例（macOS arm64）

```bash
# 1. 构建
wails3 task darwin:sign:notarize ARCH=arm64 DEV=false

# 2. 打包为 tar.gz（仅包含二进制）
cd bin
tar -czf WillClaw_darwin_arm64.tar.gz -C WillClaw.app/Contents/MacOS WillClaw
cd ..
```

### 制作更新资产示例（Windows amd64）

```bash
# 1. 构建
wails3 task windows:build ARCH=amd64 DEV=false

# 2. 打包为 zip（仅包含二进制）
cd bin
zip WillClaw_windows_amd64.zip WillClaw.exe
cd ..
```

### 发布步骤

1. 在 GitHub 创建 Release（tag 格式如 `v1.0.0`）
2. 各平台分别构建并打包更新资产（因为 CGO 无法交叉编译，需在对应平台上打包）
3. 上传所有平台的安装包 + 更新资产到 Release
4. 同步 Release 到 Gitee（包括上传相同的文件）
