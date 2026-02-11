# 多屏幕划词弹窗定位方案（Windows & macOS）

## 背景

ChatClaw 的划词搜索弹窗（Text Selection Popup）需要在用户选中文本后，在鼠标附近弹出。在多屏幕环境下（尤其是不同 DPI/缩放比的屏幕），弹窗需要正确识别鼠标所在的屏幕，并在该屏幕上定位弹窗。本文档涵盖 Windows 和 macOS 两个平台的方案。

### 典型场景

- 屏幕 1（主屏幕）：2560×1600，150% 缩放（DPI = 144）
- 屏幕 2（副屏幕）：1920×1080，100% 缩放（DPI = 96），位于主屏幕右侧

用户在屏幕 2 上划词时，弹窗应出现在屏幕 2 上（鼠标附近），而非被错误地拉到屏幕 1。

## 遇到的问题链

### 问题 1：鼠标 Hook 坐标系不一致

Windows 低级鼠标 Hook（`WH_MOUSE_LL`）的 `MSLLHOOKSTRUCT.pt` 字段返回的坐标可能受 DPI 感知模式影响，不一定是物理像素坐标。在主屏幕为 150% 缩放时，返回的可能是系统 DPI 逻辑坐标（除以 1.5），导致屏幕 2 上的物理坐标 `3000` 被缩放为 `2000`，落入屏幕 1 的物理范围内（0–2560）。

**解决方案**：使用 `GetPhysicalCursorPos()` API 替代 `MSLLHOOKSTRUCT.pt`。该 API 始终返回物理像素坐标，不受调用线程的 DPI 感知模式影响。

```go
// clipboard_windows.go
var procGetPhysicalCursorPos = modUser32.NewProc("GetPhysicalCursorPos")

// GetPhysicalCursorPos always returns physical screen coordinates,
// regardless of the calling thread's DPI awareness context.
func GetPhysicalCursorPos() (x, y int32) {
    var pt point
    ret, _, _ := procGetPhysicalCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
    if ret == 0 {
        // Fallback to GetCursorPos
        procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
    }
    return pt.X, pt.Y
}
```

涉及文件：
- `clipboard_windows.go` — 添加 `GetPhysicalCursorPos()` 函数
- `mouse_hook_windows.go` — `wmLButtonDown`、`wmMouseMove`、`wmLButtonUp` 改用 `GetPhysicalCursorPos()`
- `click_outside_windows.go` — `clickOutsideMouseProc` 改用 `GetPhysicalCursorPos()`

### 问题 2：`MonitorFromPoint` 在 x64 上 POINT 结构体参数打包错误（**核心 Bug**）

`MonitorFromPoint(POINT pt, DWORD dwFlags)` 的 Windows API 签名中，第一个参数是 **POINT 结构体（8 字节）**，而非两个独立的 int 参数。

在 x64 Windows 调用约定中：

> 大小为 8 字节的结构体"按值传递"时，被视为与相同大小的整数等同 —— 即打包到单个 64 位寄存器 (RCX) 中。

#### 错误的调用方式

```go
// ❌ WRONG: 传递 3 个独立参数
procMonitorFromPoint.Call(
    uintptr(pt.X),    // → RCX = x (高 32 位为 0，POINT.y 永远是 0)
    uintptr(pt.Y),    // → RDX = y（被 API 当作 dwFlags！）
    uintptr(flags),   // → R8 = flags（API 不读取此寄存器）
)
```

**实际效果**：
| 寄存器 | 我们传递的值 | API 读取为 |
|--------|-------------|-----------|
| RCX | x (如 3741) | POINT = {x:3741, y:**0**} |
| RDX | y (如 320) | dwFlags = **320** |
| R8 | 2 (MONITOR_DEFAULTTONEAREST) | （被忽略） |

- `POINT.y` 始终为 `0`（RCX 的高 32 位全为 0）
- `dwFlags` 变成了 y 坐标值（如 320），不是 `MONITOR_DEFAULTTONEAREST`（2）
- `dwFlags = 320` 等效于 `MONITOR_DEFAULTTONULL`（bit0 = 0） → 找不到显示器时返回 **NULL**
- 代码回退到默认值 `WorkArea{0, 0, 1920, 1080}` 和 `dpiScale = 1.5`
- `clampToWorkArea` 将 x=3636 截断到 1710（屏幕 1 范围内）→ 弹窗跑到屏幕 1

#### 正确的调用方式

```go
// ✅ CORRECT: 将 POINT 打包为 64 位值
func monitorFromPointPacked(x, y int32, flags uintptr) uintptr {
    // Pack POINT: low 32 bits = x, high 32 bits = y
    packed := uintptr(uint32(x)) | (uintptr(uint32(y)) << 32)
    hMonitor, _, _ := procMonitorFromPoint.Call(packed, flags)
    return hMonitor
}
```

**实际效果**：
| 寄存器 | 传递的值 | API 读取为 |
|--------|---------|-----------|
| RCX | (y << 32) \| x | POINT = {x:3741, y:320} ✓ |
| RDX | 2 | dwFlags = MONITOR_DEFAULTTONEAREST ✓ |

涉及文件：
- `screen_windows.go` — `getDPIScaleForPoint()` 和 `getWorkAreaAtPoint()` 改用 `monitorFromPointPacked()`

### 问题 3：Wails 的 `WM_DPICHANGED` 处理覆盖原生定位

当弹窗从屏幕 1（150%）移动到屏幕 2（100%）时，Windows 发送 `WM_DPICHANGED` 消息。Wails 的默认 WndProc 会处理此消息并重新定位窗口，覆盖我们通过 `SetWindowPos` 设置的正确位置。

**解决方案**：子类化弹窗的 WndProc，拦截 `WM_DPICHANGED` 消息并直接返回 0。

```go
// popup_noactivate_windows.go
const wmDpiChanged = 0x02E0

func popupWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
    switch msg {
    case wmDpiChanged:
        // Block WM_DPICHANGED to prevent Wails from repositioning the popup.
        return 0
    // ... other message handling
    }
}
```

涉及文件：
- `popup_noactivate_windows.go` — 添加 `wmDpiChanged` 拦截

### 问题 4：Wails 的 `SetPosition` 使用 DIP 坐标转换不准确

Wails 的 `window.SetPosition()` 内部使用 DIP（Device Independent Pixels）坐标系，并通过 `PhysicalToDipPoint` / `DipToPhysicalPoint` 进行转换。在多 DPI 屏幕之间移动窗口时，这些转换可能不准确。

**解决方案**：完全绕过 Wails 的定位系统，直接使用 Win32 原生 API `SetWindowPos` 进行物理像素定位。

```go
// popup_zorder_windows.go
func setPopupPositionPhysical(w *application.WebviewWindow, x, y, width, height int) {
    h := uintptr(w.NativeWindow())
    if h == 0 { return }
    procSetWindowPos.Call(
        h, hwndTopMost,
        uintptr(x), uintptr(y), uintptr(width), uintptr(height),
        uintptr(swpNoActivate),
    )
}
```

涉及文件：
- `popup_zorder_windows.go` — 添加 `setPopupPositionPhysical()` 和 `getPopupWindowRect()`
- `service.go` — 添加 `showPopupPhysical()` 方法，外部选中路径全部使用物理像素坐标

## 整体架构

### 坐标流水线（外部划词路径）

```
鼠标 Hook 回调
    ↓
GetPhysicalCursorPos()          // 始终返回物理像素坐标
    ↓
showPopupOnlyAtScreenPos(physX, physY)
    ↓
getDPIScaleForPoint(physX, physY)   // monitorFromPointPacked → GetDpiForMonitor
    → physW = popWidth * scale       // 将 DIP 弹窗尺寸转为物理像素
    → physH = popHeight * scale
    ↓
clampToWorkArea(...)                 // monitorFromPointPacked → GetMonitorInfoW
    → 确保弹窗不超出目标屏幕边界
    ↓
showPopupPhysical(physX, physY, physW, physH)
    → ensurePopWindowCreate()        // 创建窗口（首次）/ 复用
    → setPopupPositionPhysical()     // 原生 SetWindowPos（绕过 Wails DIP）
    → w.Show()
    → forcePopupTopMostNoActivate()
```

### 关键文件列表

| 文件 | 职责 |
|------|------|
| `screen_windows.go` | `monitorFromPointPacked()`、`getDPIScaleForPoint()`、`getWorkAreaAtPoint()`、`clampToWorkArea()` |
| `clipboard_windows.go` | `GetPhysicalCursorPos()` — 始终返回物理像素坐标 |
| `mouse_hook_windows.go` | 全局鼠标 Hook，使用 `GetPhysicalCursorPos()` 获取坐标 |
| `click_outside_windows.go` | 弹窗外部点击检测，使用 `GetPhysicalCursorPos()` |
| `popup_zorder_windows.go` | `setPopupPositionPhysical()`、`getPopupWindowRect()`、`forcePopupTopMostNoActivate()` |
| `popup_noactivate_windows.go` | WndProc 子类化，拦截 `WM_DPICHANGED` 和激活消息 |
| `service.go` | 核心业务逻辑，`showPopupPhysical()` 编排整个定位流程 |

## 注意事项

### 1. `MonitorFromPoint` 的 POINT 打包是 x64 特有问题

在 x86 (32-bit) 上，`POINT` 结构体（8字节）超过寄存器大小，通过栈传递，Go 的 `proc.Call` 多参数方式恰好匹配。但在 x64 上，8 字节结构体被打包为 64 位寄存器值，必须手动打包。

**检查清单**：代码中任何使用 `procMonitorFromPoint.Call()` 的地方，都必须将 POINT 打包为 `(y << 32) | x`。

### 2. 负坐标的正确处理

当屏幕排列导致负坐标时（例如副屏在主屏左侧或上方），`uint32(int32(-200))` 会正确保留二进制位模式 `0xFFFFFF38`，打包到 64 位寄存器中仍能被 API 正确读取为有符号 `-200`。

### 3. Wails 兼容性

此方案特别针对 Wails v3 + WebView2 环境。Wails 的坐标系统（DIP）在单屏幕下工作正常，但在多 DPI 屏幕间移动窗口时有精度问题。我们的解决方案是：**创建窗口用 Wails API，定位窗口用 Win32 原生 API**。

### 4. 诊断日志

关键位置有诊断日志，在排查问题时可检查：

```
// screen_windows.go
[MONITOR-DEBUG] getDPIScaleForPoint(3741,320) → dpi=96 scale=1.00
[MONITOR-DEBUG] getWorkAreaAtPoint(3741,320) → workArea=(2560,-80,1920,1080)

// service.go
[POPUP-DEBUG] showPopupOnlyAtScreenPos physMouseX=3741 physMouseY=320 dpiScale=1.00
[POPUP-DEBUG] afterClamp finalX=3636 finalY=230
[POPUP-DEBUG] showPopupPhysical FINAL position actualLeft=3636 actualTop=230
```

如果 `workAreaX` 为 0 且 `dpiScale` 为主屏幕的缩放率（而非目标屏幕），说明 `MonitorFromPoint` 仍未正确找到目标屏幕。

---

# macOS 多屏幕划词弹窗定位方案

## 坐标系统

macOS 使用 **Cocoa 全局坐标系统**：

- 原点 `(0, 0)` 在主屏幕左下角
- Y 轴向上增长（与 Windows 相反）
- 所有屏幕共享同一坐标空间（unified coordinate system）
- 坐标单位为 **Cocoa 点**（point），与物理像素关系由 `backingScaleFactor` 决定（Retina = 2.0，普通 = 1.0）
- `[NSEvent mouseLocation]` 和 `[NSWindow setFrame:display:]` 都使用同一坐标系

因此，macOS 上不存在 Windows 那样的"DPI 逻辑坐标 vs 物理像素坐标"转换问题。坐标可以在所有 API 间直接传递。

## 遇到的问题

### 问题 1：Wails 的 `SetPosition` 在跨屏幕时可能不准确

Wails 的 `window.SetPosition()` 内部通过 `[window screen]` 获取当前屏幕并进行坐标转换。当弹窗需要从一个屏幕移动到另一个屏幕时，`[window screen]` 返回的仍是旧屏幕，导致转换结果不正确。

**解决方案**：绕过 Wails 的 `SetPosition`，直接使用 Cocoa 原生 API 定位。

```c
// popup_zorder_darwin.go (CGO)
static void textselection_set_popup_position(void *nsWindow, int x, int y) {
    NSWindow *win = (__bridge NSWindow *)nsWindow;
    NSRect frame = [win frame];
    frame.origin.x = (CGFloat)x;
    frame.origin.y = (CGFloat)y;
    [win setFrame:frame display:YES];
}
```

### 问题 2：`w.Show()` 和 `setFrame` 的操作顺序竞态

macOS 路径的操作流程是：`setPopupPositionCocoa` → `w.Show()` → `forcePopupTopMostNoActivate`。

由于 `forcePopupTopMostNoActivate` 使用 `dispatch_async(dispatch_get_main_queue(), ...)` 异步执行，而 `w.Show()` 内部也可能异步派发到主线程，存在以下竞态风险：

1. 我们设置位置（主线程同步）
2. `w.Show()` 内部异步派发 → 可能重置位置
3. `forcePopupTopMostNoActivate` 异步派发 → 在步骤 2 之后执行

如果 Wails 的 `Show()` 重新定位窗口，我们之前设置的正确位置会被覆盖。

**解决方案**：将屏幕检测、clamp、定位、置顶合并为一个原子操作，在单个 `dispatch_async(dispatch_get_main_queue(), ...)` 块中执行。

### 问题 3：缺少屏幕边界 clamp

原来的 macOS 路径完全没有做屏幕边界检测和 clamp（`clampToWorkArea` 在非 Windows 平台是 no-op），弹窗可能超出屏幕边界。

**解决方案**：在 Objective-C 层直接用 `NSScreen` API 找到正确的屏幕并 clamp。

## 核心实现：`textselection_show_popup_clamped`

```c
// popup_zorder_darwin.go (CGO)
static void textselection_show_popup_clamped(
    void *nsWindow, int mouseX, int mouseY, int popWidth, int popHeight
) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *win = (__bridge NSWindow *)nsWindow;

        // 1. 找到鼠标所在的屏幕
        NSPoint mousePt = NSMakePoint((CGFloat)mouseX, (CGFloat)mouseY);
        NSScreen *screen = nil;
        for (NSScreen *s in [NSScreen screens]) {
            if (NSPointInRect(mousePt, s.frame)) {
                screen = s;
                break;
            }
        }
        if (screen == nil) screen = [NSScreen mainScreen];

        // 2. 获取该屏幕的可见区域（排除 menu bar 和 dock）
        NSRect visibleFrame = [screen visibleFrame];

        // 3. 计算位置：鼠标上方、水平居中
        CGFloat popX = mouseX - popWidth / 2.0;
        CGFloat popY = mouseY + 10;  // Cocoa Y 向上，+10 = 鼠标上方

        // 4. Clamp 到可见区域
        if (popX < NSMinX(visibleFrame))
            popX = NSMinX(visibleFrame);
        if (popX + popWidth > NSMaxX(visibleFrame))
            popX = NSMaxX(visibleFrame) - popWidth;
        if (popY + popHeight > NSMaxY(visibleFrame))
            popY = NSMaxY(visibleFrame) - popHeight;
        if (popY < NSMinY(visibleFrame))
            popY = mouseY - popHeight - 10;  // 改为鼠标下方

        // 5. 设置窗口帧（位置 + 大小）
        [win setFrame:NSMakeRect(popX, popY, popWidth, popHeight) display:YES];

        // 6. 置顶但不激活
        [win setLevel:NSPopUpMenuWindowLevel];
        [win orderFrontRegardless];
    });
}
```

### 为什么在 Objective-C 层做而不是 Go 层

1. **`NSScreen.screens` 只能在 Cocoa 层高效遍历** — 通过 CGO 调用避免多次 CGO 跨界开销
2. **`dispatch_async(dispatch_get_main_queue())` 保证原子性** — 所有操作在同一个主线程回调中完成，不会与 Wails 的内部派发交错
3. **`visibleFrame` 天然排除 menu bar 和 dock** — 无需手动计算任务栏偏移

## 整体架构（macOS）

### 坐标流水线（外部划词路径）

```
CGEventTap 回调 (mouse hook)
    ↓
[NSEvent mouseLocation]              // Cocoa 全局坐标（点）
    ↓
mouseHookDarwinShowPopup(x, y, pid)  // Go 回调
    ↓
showPopupOnlyAtScreenPos(screenX, screenY)
    ↓
ensurePopWindowDarwinClamped(mouseX, mouseY, popW, popH)
    ↓
ensurePopWindowCreateDarwin()         // 创建窗口（首次）/ 复用
    ↓
w.Show()                              // 让 Wails 初始化窗口状态
    ↓
showPopupClampedCocoa(w, mouseX, mouseY, popW, popH)
    → dispatch_async(main_queue) {
        1. NSPointInRect 遍历 NSScreen.screens 找目标屏幕
        2. screen.visibleFrame 获取可见区域
        3. 计算 + clamp 弹窗位置
        4. setFrame:display: 设置窗口
        5. setLevel: + orderFrontRegardless 置顶
    }
```

### 关键文件列表

| 文件 | 职责 |
|------|------|
| `clipboard_darwin.go` | `getCursorPosition()` — `[NSEvent mouseLocation]` Cocoa 全局坐标 |
| `mouse_hook_darwin.go` | CGEventTap 全局鼠标 Hook，使用 Cocoa 坐标 |
| `click_outside_darwin.go` | 弹窗外部点击检测，使用 Cocoa 坐标 |
| `popup_zorder_darwin.go` | `showPopupClampedCocoa()` — 屏幕检测 + clamp + 定位 + 置顶 |
| `popup_noactivate_darwin.go` | 弹窗窗口配置（level、behavior） |
| `service.go` | `ensurePopWindowDarwinClamped()` — 编排创建 + 显示 + 定位 |
| `screen_other.go` | macOS/其他平台的 `getDPIScaleForPoint()` stub（返回 1.0） |

## Windows vs macOS 对比

| 维度 | Windows | macOS |
|------|---------|-------|
| 坐标系统 | 物理像素（virtual screen） | Cocoa 点（global, Y from bottom） |
| Y 轴方向 | 向下增长 | 向上增长 |
| DPI 处理 | Per-Monitor DPI，需要 `GetDpiForMonitor` | `backingScaleFactor`，Cocoa 自动处理 |
| 屏幕检测 | `MonitorFromPoint`（需 POINT 打包） | `NSPointInRect` + `NSScreen.screens` |
| 工作区获取 | `GetMonitorInfoW` → `rcWork` | `NSScreen.visibleFrame` |
| 窗口定位 | `SetWindowPos`（物理像素） | `setFrame:display:`（Cocoa 点） |
| 绕过 Wails | `SetWindowPos` + 拦截 `WM_DPICHANGED` | `dispatch_async` 原子操作 |
| 弹窗尺寸 | `popWidth * dpiScale`（物理像素） | `popWidth`（Cocoa 点 = DIP） |

## 诊断日志

### macOS

macOS 的诊断通过 `NSLog` 输出，可在 Console.app 中查看：

```
[POPUP-DEBUG-MAC] screen=LG UltraFine visibleFrame=(2560,0,1920,1055)
    mouse=(3200,500) final=(3130,510) size=(140,50)
```

关键检查：
- `screen` 名称应匹配鼠标所在的物理屏幕
- `visibleFrame` 的 origin 应为该屏幕在全局坐标系中的实际位置（非主屏幕时 X ≠ 0）
- `final` 坐标应在 `visibleFrame` 范围内

如果 `screen` 始终显示主屏幕名称，说明 `NSPointInRect` 没有匹配到正确的屏幕——可能是传入的坐标有问题（检查 `[NSEvent mouseLocation]` 的输出）。
