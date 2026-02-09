# Windows syscall.NewCallback 回调槽位限制

## 背景

Go 运行时在 Windows 上对 `syscall.NewCallback` / `syscall.NewCallbackCDecl` / `windows.NewCallback` 有一个**硬性限制**：最多约 **2000 个回调槽位**，且**分配后永远不会释放**。

这是 Go 运行时的实现限制（`runtime/syscall_windows.go` 中的固定数组），不是 Windows 系统限制。一旦超过上限，程序会立即 `fatal error: too many callback functions` 崩溃，无法 recover。

## 问题场景

WillChat 大量使用 Windows API（`EnumWindows`、`SetWinEventHook`、`SetWindowsHookEx` 等），这些 API 需要传入函数回调。如果在**循环或频繁调用的函数**中每次都调用 `syscall.NewCallback()`，回调槽位会快速耗尽。

### 典型错误示例

```go
// ❌ 错误：每次调用都分配新的回调槽位
func findWindowByProcess(processName string) windows.HWND {
    var result windows.HWND
    cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
        // ... 枚举逻辑 ...
        result = windows.HWND(hwnd)
        return 0
    })
    procEnumWindows.Call(cb, 0)
    return result
}
```

如果 `findWindowByProcess` 被定时器每 400ms 调用一次，运行约 **13 分钟**后就会耗尽全部 2000 个槽位，程序崩溃。

## 正确做法：sync.Once + 包级变量

核心原则：**每个回调函数只创建一次**，通过包级变量在调用者和回调之间传递数据。

### 模式一：EnumWindows 类同步回调

适用于 `EnumWindows`、`EnumChildWindows` 等同步枚举 API——回调在 `Call()` 期间同步执行。

```go
// ✅ 正确：回调只创建一次，通过包级变量传参

var (
    fwpCBOnce    sync.Once
    fwpCB        uintptr
    fwpMu        sync.Mutex   // serialise concurrent callers
    fwpTarget    string       // per-call input
    fwpResult    windows.HWND // per-call output
)

// Package-level callback function (never a closure)
func fwpEnumProc(hwnd uintptr, _ uintptr) uintptr {
    h := windows.HWND(hwnd)
    pid, _ := getWindowProcessID(h)
    exe, _ := getProcessImageBaseName(pid)
    if strings.ToLower(exe) == fwpTarget {
        fwpResult = h
        return 0 // stop enumeration
    }
    return 1
}

func findWindowByProcess(processName string) windows.HWND {
    fwpMu.Lock()
    defer fwpMu.Unlock()

    fwpTarget = strings.ToLower(processName)
    fwpResult = 0

    fwpCBOnce.Do(func() {
        fwpCB = syscall.NewCallback(fwpEnumProc)
    })
    procEnumWindows.Call(fwpCB, 0)
    return fwpResult
}
```

**要点**：
- `sync.Once` 保证回调只分配一次
- `sync.Mutex` 防止并发调用互踩共享变量
- 回调函数是**顶级函数**，不是闭包（闭包每次会创建新函数值）
- EnumWindows 在 `Call()` 内同步调用回调，所以 mutex 持有期间数据是安全的

### 模式二：回调已是包级函数

如果回调函数已经是独立的顶级函数（不捕获局部变量），只需用 `sync.Once` 包装 `NewCallback` 调用：

```go
var (
    enumChildCBOnce sync.Once
    enumChildCB     uintptr
)

// enumChildCallback is already a top-level function
func enumChildCallback(hwnd uintptr, lParam uintptr) uintptr {
    // ... uses package-level variables ...
    return 1
}

func enumerateChildWindows(parentHwnd uintptr) []childWindowInfo {
    enumChildCBOnce.Do(func() {
        enumChildCB = syscall.NewCallback(enumChildCallback)
    })
    enumChildResults = nil
    procEnumChildWindows.Call(parentHwnd, enumChildCB, 0)
    return enumChildResults
}
```

### 模式三：SetWinEventHook 异步回调 + 多实例分发

适用于 `SetWinEventHook` 等事件钩子 API——回调从 OS 消息循环异步到达。如果有多个实例（如 follower），用 `sync.Map` 按线程 ID 分发。

```go
var (
    hookCBOnce sync.Once
    hookCB     uintptr
    hookMap    sync.Map // uint32 (thread ID) -> *follower
)

// Package-level shim dispatches to the correct instance
func hookShim(hHook uintptr, event uint32, hwnd windows.HWND,
    idObject, idChild int32, eventThread, eventTime uint32) uintptr {
    tid := getCurrentThreadId()
    if v, ok := hookMap.Load(tid); ok {
        return v.(*follower).winEventProc(hHook, event, hwnd, idObject, idChild, eventThread, eventTime)
    }
    return 0
}

func (f *follower) start() {
    go func() {
        runtime.LockOSThread()
        defer runtime.UnlockOSThread()

        f.tid = getCurrentThreadId()
        hookMap.Store(f.tid, f)
        defer hookMap.Delete(f.tid)

        hookCBOnce.Do(func() {
            hookCB = syscall.NewCallback(hookShim)
        })
        procSetWinEventHook.Call(/* ... */, hookCB, /* ... */)
        // ... message loop ...
    }()
}
```

### 模式四：包级 var 初始化（仅执行一次的场景）

对于窗口类注册等只执行一次的场景，直接用包级 `var` 初始化即可：

```go
// ✅ OK — package init runs exactly once
var wndProcCB = syscall.NewCallback(myWndProc)
```

或者用 `bool` 守卫：

```go
var panelClassRegistered bool

func registerPanelClass() {
    if panelClassRegistered {
        return
    }
    wc := WNDCLASSEX{
        WndProc: syscall.NewCallback(defWindowProc), // only once
    }
    procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
    panelClassRegistered = true
}
```

## 自查清单

在代码审查中，遇到 `syscall.NewCallback` / `windows.NewCallback` 时，检查：

| 检查项 | 通过条件 |
|--------|---------|
| 是否在函数体内调用 | 如果是，必须有 `sync.Once` 保护 |
| 外层函数是否可能被多次调用 | 如果是，必须有 `sync.Once` 保护 |
| 回调是否是闭包（捕获局部变量） | 如果是，重构为顶级函数 + 包级共享变量 |
| 多 goroutine 是否并发调用 | 如果是，共享变量需要 `sync.Mutex` 保护 |

## 修复历史

| 日期 | 文件 | 函数 | 说明 |
|------|------|------|------|
| 2026-02-09 | `pkg/winsnap/zorder_windows.go` | `TopMostVisibleProcessName` | 每 400ms 调用一次，直接触发崩溃 |
| 2026-02-09 | `pkg/winsnap/winsnap_windows.go` | `getProcessWindowsBounds` | WinEvent 回调中频繁调用 |
| 2026-02-09 | `pkg/winsnap/winsnap_windows.go` | `findMainWindowByProcessNameEx` | snap 流程中多次调用 |
| 2026-02-09 | `pkg/winsnap/winsnap_windows.go` | `follower.start()` | 每次 follower 重建时泄漏 |
| 2026-02-09 | `pkg/winsnap/input_windows.go` | `enumerateChildWindows` | 输入操作时调用 |
| 2026-02-09 | `pkg/winsnap/input_windows.go` | `getProcessWindowsBoundsForClick` | 点击计算时调用 |
| 2026-02-09 | `pkg/webviewpanel/hwnd_windows.go` | `FindWindowByTitleContains` | 窗口查找时调用 |
| 2026-02-09 | `pkg/webviewpanel/hwnd_windows.go` | `FindChildWindowByClassContains` | 子窗口查找时调用 |
| 2026-02-09 | `internal/services/textselection/mouse_hook_windows.go` | `MouseHookWatcher.run` | Start/Stop 周期泄漏 |
| 2026-02-09 | `internal/services/textselection/click_outside_windows.go` | `ClickOutsideWatcher.run` | Start/Stop 周期泄漏 |
| 2026-02-09 | `internal/services/textselection/clipboard_windows.go` | `ClipboardWatcher.run` | Start/Stop 周期泄漏 |

## 参考

- [Go 源码 `runtime/syscall_windows.go`](https://github.com/golang/go/blob/master/src/runtime/syscall_windows.go) — `compileCallback` 的固定数组实现
- [Go Issue #45649](https://github.com/golang/go/issues/45649) — 关于回调限制的讨论
