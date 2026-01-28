# WebviewPanel

WebviewPanel 是一个独立的 Go 包，允许在 Wails v3 应用程序的单个窗口内嵌入多个独立的 WebView 实例。这个实现基于 [wails PR #4880](https://github.com/wailsapp/wails/pull/4880) 的设计，保持 API 兼容性，以便在官方合并后无缝切换。

## 功能特性

- ✅ 创建多个独立的 WebView 面板
- ✅ 支持加载任意 URL（包括外部网站）
- ✅ 绝对定位（X, Y, Width, Height）
- ✅ Z-Index 层级控制
- ✅ 显示/隐藏控制
- ✅ JavaScript 执行
- ✅ 缩放控制
- ✅ 开发者工具
- ✅ 焦点管理

## 平台支持

| 平台 | 状态 |
|------|------|
| Windows | ✅ 完整支持 |
| macOS | ✅ 基础可用（cgo，WKWebView，仍属实验性质） |
| Linux | ✅ 基础可用（cgo，GTK3 + WebKit2GTK，仍属实验性质） |

> 说明：macOS/Linux 的实现需要 cgo 与系统原生依赖；由于无法像 PR #4880 那样直接接入 Wails 内核，这里通过“按窗口标题查找原生窗口 + 在其 contentView/GTK 层级中插入 overlay”的方式实现，兼容性取决于 Wails 版本与各平台环境。

### Linux 依赖（开发包）

- `gtk+-3.0`
- `webkit2gtk-4.1`
- `gdk-3.0`

## 使用方法

### 1. 获取窗口句柄

```go
import "chatwiki/pkg/webviewpanel"

// 通过窗口标题查找
hwnd := webviewpanel.FindWindowByTitle("My Window Title")

// 或通过部分标题匹配
hwnd := webviewpanel.FindWindowByTitleContains("My Window")
```

### 2. 创建面板管理器

```go
// 创建管理器，debugMode 启用开发者工具
manager := webviewpanel.NewPanelManager(hwnd, true)
```

### 3. 创建面板

```go
visible := true
panel := manager.NewPanel(webviewpanel.WebviewPanelOptions{
    Name:    "my-panel",
    X:       0,
    Y:       60,  // 例如：在标签栏下方
    Width:   800,
    Height:  500,
    URL:     "https://www.example.com",
    Visible: &visible,
    ZIndex:  1,
})
```

### 4. 控制面板

```go
// 导航
panel.SetURL("https://www.google.com")
panel.SetHTML("<h1>Hello World</h1>")

// 位置和大小
panel.SetBounds(webviewpanel.Rect{X: 0, Y: 60, Width: 800, Height: 500})
panel.SetPosition(100, 100)
panel.SetSize(600, 400)

// 可见性
panel.Show()
panel.Hide()
visible := panel.IsVisible()

// JavaScript
panel.ExecJS("console.log('Hello from Go!')")

// 缩放
panel.SetZoom(1.5)
zoom := panel.GetZoom()

// 焦点
panel.Focus()
focused := panel.IsFocused()

// 开发者工具
panel.OpenDevTools()

// 重新加载
panel.Reload()
panel.ForceReload()

// 销毁
panel.Destroy()
```

### 5. 管理面板

```go
// 获取面板
panel := manager.GetPanel("my-panel")
panel := manager.GetPanelByID(1)
panels := manager.GetPanels()

// 移除面板
manager.RemovePanel("my-panel")
manager.RemovePanelByID(1)

// 销毁所有面板
manager.DestroyAll()
```

## API 参考

### WebviewPanelOptions

| 字段 | 类型 | 说明 |
|------|------|------|
| Name | string | 面板唯一标识符 |
| X, Y | int | 相对于父窗口的位置（CSS 像素） |
| Width, Height | int | 面板大小（CSS 像素） |
| URL | string | 初始加载的 URL |
| HTML | string | 初始 HTML 内容 |
| JS | string | 页面加载后执行的 JavaScript |
| CSS | string | 注入的 CSS 样式 |
| Visible | *bool | 是否初始可见（默认 true） |
| DevToolsEnabled | *bool | 是否启用开发者工具 |
| Zoom | float64 | 缩放级别（默认 1.0） |
| BackgroundColour | RGBA | 背景颜色 |
| Transparent | bool | 是否透明背景 |
| Anchor | AnchorType | 贴边/锚定（与 PR #4880 对齐；本仓库 PanelManager 暂不自动应用） |

### 与官方 API 的兼容性

当 wails PR #4880 合并后，迁移步骤（建议做法）：

#### 迁移目标
把自定义实现 `chatwiki/pkg/webviewpanel` 迁移到官方 `github.com/wailsapp/wails/v3/pkg/application`（PR #4880 新增的 `WebviewPanel`）。

#### 需要改哪些点
1. **import 替换**
   - 从 `chatwiki/pkg/webviewpanel`
   - 改为 `github.com/wailsapp/wails/v3/pkg/application`

2. **创建方式替换（核心差异）**
   - **现在（本仓库）**：你需要先拿到原生窗口句柄，再 `NewPanelManager(hwnd, ...)`，再 `manager.NewPanel(...)`
   - **官方（PR #4880）**：直接对 `mainWindow.NewPanel(...)`（由 `WebviewWindow` 管理 panels，不需要自己找 hwnd）

3. **类型名替换**
   - `webviewpanel.WebviewPanelOptions` → `application.WebviewPanelOptions`
   - `webviewpanel.Rect` → `application.Rect`

4. **字段差异**
   - 官方 `WebviewPanelOptions` 额外包含 `Anchor/AnchorType`（本仓库已提前对齐该字段，便于后续迁移）

#### 代码对照（示意）

本仓库：

```go
import "chatwiki/pkg/webviewpanel"

hwnd := webviewpanel.FindWindowByTitle("Main")
manager := webviewpanel.NewPanelManager(hwnd, true)
panel := manager.NewPanel(webviewpanel.WebviewPanelOptions{
    Name: "my-panel",
    X: 0, Y: 60, Width: 800, Height: 500,
    URL: "https://example.com",
})
```

PR #4880 合并后（官方）：

```go
import "github.com/wailsapp/wails/v3/pkg/application"

// 假设你已经拿到了 mainWindow（*application.WebviewWindow）
panel := mainWindow.NewPanel(application.WebviewPanelOptions{
    Name: "my-panel",
    X: 0, Y: 60, Width: 800, Height: 500,
    URL: "https://example.com",
})
```

#### 关于“输入框需要先点空白处才能聚焦”的修复
本仓库在 macOS 下额外做了一个焦点修复（把 host WKWebView 强制设为 first-responder，避免出现“要先点空白处再点输入框”的问题）。

如果后续迁移到官方实现后又出现同类问题：
- 可以把这个思路迁移为**官方侧的补丁/PR**（推荐）
- 或在你自己的项目里对官方实现做一层轻量封装再调用（不推荐长期维护）

## 高级封装：TabBrowser

如果你需要实现多标签浏览器效果，可以使用更高级的 `TabBrowser` 封装，它内置了标签管理、切换、新增、关闭等逻辑。

### 基本用法

```go
import "chatwiki/pkg/webviewpanel"

// 创建多标签浏览器
browser := webviewpanel.NewTabBrowser(hwnd, webviewpanel.TabBrowserConfig{
    DebugMode:   true,
    DefaultURL:  "https://www.bing.com",
    InitialTabs: []string{"https://www.baidu.com", "https://www.google.com"},
    OnTabsChanged: func(tabs []webviewpanel.TabInfo, activeIdx int) {
        // 标签变化时回调，可用于更新 UI
    },
})

// 在 Wails 应用中，必须设置主线程调度器（Windows WebView2/COM 线程亲和性要求）
browser.SetDispatchSync(application.InvokeSync)

// 激活浏览器（传入布局区域）
browser.Activate(webviewpanel.Rect{X: 0, Y: 100, Width: 800, Height: 600})
```

### TabBrowser API

```go
// 标签操作
browser.NewTab(url string) int      // 新建标签，返回索引
browser.SwitchTab(idx int) bool     // 切换到指定标签
browser.CloseTab(idx int) bool      // 关闭指定标签
browser.Navigate(url string) bool   // 导航当前标签
browser.Refresh() bool              // 刷新当前标签

// 布局控制
browser.SetLayout(rect Rect)        // 更新面板区域
browser.Activate(rect Rect)         // 激活并显示
browser.Deactivate()                // 隐藏所有标签

// 查询
browser.GetTabs() []TabInfo         // 获取所有标签信息
browser.GetActiveTabIndex() int     // 获取当前标签索引
browser.TabCount() int              // 获取标签数量

// 清理
browser.Destroy()                   // 销毁所有标签
```

### TabBrowserConfig 配置

| 字段 | 类型 | 说明 |
|------|------|------|
| DebugMode | bool | 启用开发者工具 |
| DefaultURL | string | 新建标签的默认 URL |
| InitialTabs | []string | 首次激活时自动打开的标签 |
| OnTabsChanged | func | 标签变化时的回调函数 |

### 与前端配合

前端组件位于 `frontend/src/components/multitabs/`，提供了可复用的 Vue 组件：

```vue
<script setup>
import { BrowserChrome, darkTheme, lightTheme } from "@/components/multitabs";
</script>

<template>
  <BrowserChrome
    :theme="darkTheme"
    :quick-links="[{ label: '百度', url: 'https://www.baidu.com' }]"
    default-new-tab-url="https://www.bing.com"
    @tabs-changed="onTabsChanged"
  />
</template>
```

前端组件支持：
- **主题配置**：内置 `darkTheme` 和 `lightTheme`，支持完全自定义
- **快捷链接**：可配置常用网站快捷按钮
- **事件通信**：自动与 Go 后端通过 Wails Events 通信

## 示例

查看以下文件获取完整实现：
- `internal/features/multitabsdemo/demo.go` - Go 后端多标签浏览器演示
- `frontend/src/pages/PageMultiTabs.vue` - 前端页面示例
- `frontend/src/components/multitabs/` - 可复用的前端组件

## 参考资料

- [wails PR #4880](https://github.com/wailsapp/wails/pull/4880) - WebviewPanel 官方实现
- [wails issue #1997](https://github.com/wailsapp/wails/issues/1997) - 功能需求讨论
