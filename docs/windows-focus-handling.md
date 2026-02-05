# Windows 平台窗口焦点处理方案

## 背景

在 Windows 平台上，Wails v3 使用 WebView2 作为渲染引擎。当调用 `window.Focus()` 方法时，Wails 内部会调用 WebView2 的 `Focus()` 方法。但在某些场景下（如弹窗、工具窗口），这会导致 WebView2 报错：

```
[WebView2 Error] The parameter is incorrect.
```

这是因为 WebView2 的 `Focus()` 方法对窗口状态有特定要求，不是所有窗口都能正常获取焦点。

## 核心原则

> **在 Windows 平台上，避免直接调用 Wails 的 `window.Focus()` 方法，改用原生 Windows API。**

## 问题场景

以下场景容易触发 WebView2 Focus 错误：

1. **弹窗窗口 (Popup)** - 如划词搜索弹窗
2. **工具窗口 (Tool Window)** - 如吸附窗口
3. **无边框窗口 (Frameless)** - 透明背景窗口
4. **隐藏在任务栏的窗口** - `HiddenOnTaskbar: true`

## 解决方案

### 方案一：使用原生 API 激活窗口（推荐）

对于需要激活/聚焦的窗口，使用封装好的 `forceActivateWindow` 函数：

**位置**: `internal/services/textselection/activate_windows.go`

```go
// forceActivateWindow uses Windows API to activate window 
// (doesn't call Wails Focus to avoid WebView2 error).
func forceActivateWindow(w *application.WebviewWindow) {
    // 使用 SetForegroundWindow + BringWindowToTop 等原生 API
    // 不调用 w.Focus()
}
```

**使用示例**:

```go
// ✅ 正确：使用封装的原生 API
forceActivateWindow(mainWindow)

// ❌ 错误：直接调用 Wails Focus
mainWindow.Focus()
```

### 方案二：Hook WndProc 拦截激活消息

对于不需要获取焦点的窗口（如弹窗），通过 Hook WndProc 拦截激活相关消息，阻止 Wails 内部调用 `Focus()`。

**位置**: `internal/services/textselection/popup_noactivate_windows.go`

**需要拦截的消息**:

| 消息 | 值 | 说明 |
|------|-----|------|
| `WM_MOUSEACTIVATE` | 0x0021 | 鼠标点击激活 |
| `WM_ACTIVATE` | 0x0006 | 窗口激活 |
| `WM_NCACTIVATE` | 0x0086 | 非客户区激活 |
| `WM_SETFOCUS` | 0x0007 | 键盘焦点获取 |

**实现示例**:

```go
func popupWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
    switch msg {
    case wmMouseActivate:
        return maNoActivate // MA_NOACTIVATE = 3
    case wmActivate:
        if wParam&0xFFFF != waInactive {
            return 0 // Block activation
        }
    case wmNCActivate:
        if wParam != 0 {
            return 0 // Block activation
        }
    case wmSetFocus:
        return 0 // Block focus
    }
    // Pass other messages to original WndProc
    return callWindowProc(originalWndProc, hwnd, msg, wParam, lParam)
}
```

### 方案三：使用 SWP_NOACTIVATE 标志

在调用 `SetWindowPos` 时，始终添加 `SWP_NOACTIVATE` 标志，避免窗口被意外激活。

**位置**: `pkg/winsnap/winsnap_windows.go`

```go
const swpNoActivate = 0x0010

// 移动窗口但不激活
func setWindowPosNoActivate(hwnd windows.HWND, x, y int32) error {
    flags := uintptr(swpNoSize | swpNoZOrder | swpNoActivate)
    _, _, _ = procSetWindowPos.Call(uintptr(hwnd), 0, uintptr(x), uintptr(y), 0, 0, flags)
    return nil
}
```

## 封装函数清单

### 激活窗口

| 函数 | 位置 | 说明 |
|------|------|------|
| `forceActivateWindow` | `textselection/activate_windows.go` | 使用原生 API 激活窗口，不调用 Focus |

### 阻止激活

| 函数 | 位置 | 说明 |
|------|------|------|
| `tryConfigurePopupNoActivate` | `textselection/popup_noactivate_windows.go` | Hook WndProc 阻止弹窗被激活 |
| `removePopupSubclass` | `textselection/popup_noactivate_windows.go` | 移除 WndProc Hook |

### 窗口定位（不激活）

| 函数 | 位置 | 说明 |
|------|------|------|
| `forcePopupTopMostNoActivate` | `textselection/popup_zorder_windows.go` | 置顶窗口但不激活 |
| `setWindowTopMostNoActivate` | `winsnap/winsnap_windows.go` | 设置 TopMost 但不激活 |
| `setWindowNoTopMostNoActivate` | `winsnap/winsnap_windows.go` | 取消 TopMost 但不激活 |
| `setWindowPosWithSizeAfter` | `winsnap/winsnap_windows.go` | 设置位置和大小但不激活 |

## 开发规范

### ✅ 应该这样做

1. **激活主窗口**：使用 `forceActivateWindow(mainWindow)`
2. **显示弹窗**：调用 `tryConfigurePopupNoActivate(w)` 后再 `w.Show()`
3. **移动窗口**：使用带 `SWP_NOACTIVATE` 的 `SetWindowPos`
4. **隐藏弹窗**：使用 `w.SetPosition(-9999, -9999)` 而非 `w.Hide()`

### ❌ 不应该这样做

1. **直接调用 `w.Focus()`** - 可能导致 WebView2 崩溃
2. **直接调用 `w.Hide()`** - Wails 内部可能调用 Focus
3. **不带 `SWP_NOACTIVATE` 的 `SetWindowPos`** - 可能意外激活窗口

## 跨平台兼容

| 平台 | Focus 处理 |
|------|-----------|
| Windows | 使用原生 API，避免 `w.Focus()` |
| macOS | 可以使用 `w.Focus()`，配合 `NSRunningApplication.activateWithOptions` |
| Linux | 可以使用 `w.Focus()` |

**条件编译示例**:

```go
// activate_windows.go
//go:build windows

func forceActivateWindow(w *application.WebviewWindow) {
    // Windows: 使用原生 API
}

// activate_darwin.go
//go:build darwin && cgo

func forceActivateWindow(w *application.WebviewWindow) {
    // macOS: 使用 NSRunningApplication + w.Focus()
    C.textselection_activate_current_app()
    w.Focus()
}

// activate_other.go
//go:build !windows && !darwin

func forceActivateWindow(w *application.WebviewWindow) {
    // 其他平台: 直接使用 w.Focus()
    w.Focus()
}
```

## 调试技巧

### 识别 Focus 相关错误

错误堆栈中包含以下关键字时，通常是 Focus 问题：

```
github.com/wailsapp/go-webview2/pkg/edge.(*Chromium).Focus
github.com/wailsapp/wails/v3/pkg/application.(*windowsWebviewWindow).focus
```

### 排查步骤

1. 检查是否直接调用了 `w.Focus()`
2. 检查是否调用了 `w.Hide()`（内部可能调用 Focus）
3. 检查窗口是否配置了 NoActivate Hook
4. 检查 `SetWindowPos` 是否带 `SWP_NOACTIVATE` 标志

## 相关文件

```
internal/services/textselection/
├── activate_windows.go      # Windows 激活窗口实现
├── activate_darwin.go       # macOS 激活窗口实现
├── activate_other.go        # 其他平台激活窗口实现
├── popup_noactivate_windows.go  # Windows WndProc Hook
├── popup_noactivate_other.go    # 其他平台空实现
├── popup_zorder_windows.go      # Windows 窗口层级控制
└── popup_zorder_other.go        # 其他平台空实现

pkg/winsnap/
├── winsnap_windows.go       # Windows 吸附窗口实现
├── wake_windows.go          # Windows 唤醒窗口实现
└── zorder_windows.go        # Windows Z-Order 控制
```

## 更新日志

| 日期 | 更新内容 |
|------|---------|
| 2026-02-05 | 添加 `WM_SETFOCUS` 消息拦截，修复点击弹窗崩溃问题 |
| 2026-02-04 | 初始版本，实现 NoActivate Hook 方案 |
