# 钉钉输入模拟问题修复日志

## 问题描述

在吸附窗口中点击"发送并触发发送键"或"发送到编辑框"按钮时，钉钉应用无法正确接收输入。

### 现象
- 点击按钮后出现白色弹窗（可能是 WebView2 Focus 错误的副作用）
- 文本未被粘贴到钉钉的输入框
- 发送按键（Enter/Ctrl+Enter）未被触发

### 对比
- 微信、QQ 等其他应用：功能正常
- 钉钉：功能失效

## 技术背景

### 当前实现方式

文件：`pkg/winsnap/input_windows.go`

```go
func SendTextToTarget(targetProcess string, text string, triggerSend bool, sendKeyStrategy string) error {
    // 1. 查找目标窗口
    // 2. 复制文本到剪贴板
    // 3. 激活目标窗口 (activateHwndInput)
    // 4. 使用 SendInput API 发送 Ctrl+V
    // 5. 可选：发送 Enter 或 Ctrl+Enter
}
```

### 窗口激活方式

使用与 `wake_windows.go` 相同的方法：
1. `AttachThreadInput` - 附加到前台窗口线程
2. `ShowWindow(SW_RESTORE)` - 恢复窗口
3. `SetForegroundWindow` - 设置前台窗口
4. `BringWindowToTop` - 带到顶层

### 键盘模拟方式

使用 `SendInput` API：
```go
type inputUnion struct {
    inputType uint32      // INPUT_KEYBOARD = 1
    ki        keyboardInput
    padding   [8]byte
}

type keyboardInput struct {
    wVk         uint16    // 虚拟键码
    wScan       uint16    // 扫描码
    dwFlags     uint32    // 标志
    time        uint32
    dwExtraInfo uintptr
}
```

## 钉钉特殊性分析

### 钉钉技术栈
- 钉钉桌面版使用 Electron 框架
- Electron 基于 Chromium，有自己的输入处理机制
- 可能使用了 Web 技术实现的输入框（contenteditable 或 input 元素）

### 可能的原因

1. **窗口层级问题**
   - 钉钉可能有多个子窗口
   - 输入框可能在独立的子窗口中
   - `findMainWindowByProcessName` 可能找到的不是正确的窗口

2. **焦点问题**
   - Electron 应用的焦点处理可能与原生应用不同
   - 窗口激活后，输入焦点可能不在输入框上

3. **输入模拟方式问题**
   - `SendInput` 发送的是全局键盘事件
   - Electron 应用可能需要特定的输入方式
   - 可能需要使用 `PostMessage` 直接发送到目标窗口

4. **时序问题**
   - 窗口激活和键盘事件之间的延迟可能不够
   - Electron 应用的事件处理可能有延迟

5. **安全限制**
   - 钉钉可能有输入保护机制
   - 可能屏蔽了模拟的键盘事件

## 尝试方案记录

### 方案 1：keybd_event API（已尝试 - 失败）

**日期**：2026-02-05

**修改**：使用传统的 `keybd_event` API 替代 `SendInput`

**结果**：失败 - 钉钉仍然无法接收输入

**分析**：`keybd_event` 是更老的 API，SendInput 是其替代品，理论上 SendInput 应该更可靠

---

### 方案 2：SendInput API

**日期**：2026-02-05

**修改**：使用 `SendInput` API，复用 wake_windows.go 的窗口激活逻辑

**结果**：失败 - 钉钉仍然无法接收输入

---

### 方案 3：增加扫描码 + 分步发送 + 详细日志（当前）

**日期**：2026-02-05

**修改**：
1. 添加扫描码支持（使用 `MapVirtualKeyW` 获取）
2. 分步发送按键事件（每个按键之间添加 20ms 延迟）
3. 增加激活窗口后的延迟（250ms）
4. 增加 Ctrl+V 后的延迟（150ms）
5. 添加详细的日志输出用于诊断

**代码**：
```go
// 获取扫描码
func getScanCode(vk uint16) uint16 {
    scan, _, _ := procMapVirtualKeyW.Call(uintptr(vk), MAPVK_VK_TO_VSC)
    return uint16(scan)
}

// 创建按键输入（包含虚拟键码和扫描码）
func makeKeyDown(vk uint16) inputUnion {
    scan := getScanCode(vk)
    return inputUnion{
        inputType: INPUT_KEYBOARD,
        ki: keyboardInput{
            wVk:     vk,
            wScan:   scan,
            dwFlags: 0,
        },
    }
}

// 分步发送 Ctrl+V
func sendCtrlV() uintptr {
    var total uintptr
    
    // Ctrl down
    inputs := []inputUnion{makeKeyDown(VK_CONTROL)}
    total += sendKeyboardInput(inputs)
    time.Sleep(20 * time.Millisecond)
    
    // V down
    inputs = []inputUnion{makeKeyDown(VK_V)}
    total += sendKeyboardInput(inputs)
    time.Sleep(20 * time.Millisecond)
    
    // V up
    inputs = []inputUnion{makeKeyUp(VK_V)}
    total += sendKeyboardInput(inputs)
    time.Sleep(20 * time.Millisecond)
    
    // Ctrl up
    inputs = []inputUnion{makeKeyUp(VK_CONTROL)}
    total += sendKeyboardInput(inputs)
    
    return total
}
```

**日志输出示例**：
```
[winsnap/input] SendTextToTarget: process=DingTalk, textLen=10, triggerSend=true, sendKeyStrategy=enter
[winsnap/input] Expanded target names: [DingTalk.exe dingtalk.exe]
[winsnap/input] Found window for DingTalk.exe: hwnd=0x12345678
[winsnap/input] Clipboard set successfully
[winsnap/input] Activating target window...
[winsnap/input] After activation: foreground=0x12345678, target=0x12345678, match=true
[winsnap/input] Sending Ctrl+V...
[winsnap/input] SendInput returned: 4 events sent
[winsnap/input] Sending Enter...
[winsnap/input] SendTextToTarget completed
```

**结果**：待测试

---

### 方案 4：（待尝试）PostMessage/SendMessage

**思路**：某些应用可能需要扫描码而不仅是虚拟键码

**修改方向**：
```go
type keyboardInput struct {
    wVk         uint16
    wScan       uint16    // 添加扫描码
    dwFlags     uint32    // 添加 KEYEVENTF_SCANCODE 标志
    ...
}
```

**参考**：
- V 键扫描码：0x2F
- Ctrl 键扫描码：0x1D
- Enter 键扫描码：0x1C

---

### 方案 4：（待尝试）PostMessage/SendMessage

**思路**：直接向目标窗口发送 WM_PASTE 或 WM_KEYDOWN 消息

**修改方向**：
```go
// 发送粘贴消息
procSendMessage.Call(hwnd, WM_PASTE, 0, 0)

// 或发送按键消息
procPostMessage.Call(hwnd, WM_KEYDOWN, VK_CONTROL, 0)
procPostMessage.Call(hwnd, WM_KEYDOWN, VK_V, 0)
procPostMessage.Call(hwnd, WM_KEYUP, VK_V, 0)
procPostMessage.Call(hwnd, WM_KEYUP, VK_CONTROL, 0)
```

**注意**：需要找到正确的子窗口（输入框窗口）

---

### 方案 5：（待尝试）查找并定位输入框子窗口

**思路**：钉钉可能有多个子窗口，需要找到真正的输入框窗口

**修改方向**：
```go
// 枚举子窗口
procEnumChildWindows.Call(mainHwnd, callback, 0)

// 在回调中查找 Edit 或 RichEdit 类名的窗口
// 或者查找具有焦点的子窗口
```

---

### 方案 6：（待尝试）使用 UI Automation

**思路**：使用 Windows UI Automation API 来操作钉钉的输入框

**修改方向**：
- 使用 `IUIAutomation` 接口
- 查找输入框元素
- 使用 `ValuePattern` 或 `TextPattern` 设置值

**优点**：更可靠，是 Windows 推荐的自动化方式
**缺点**：实现复杂度高

---

### 方案 7：（待尝试）调整时序

**思路**：增加各步骤之间的延迟

**修改方向**：
```go
activateHwndInput(targetHWND)
time.Sleep(300 * time.Millisecond)  // 增加到 300ms

sendCtrlV()
time.Sleep(200 * time.Millisecond)  // 增加到 200ms
```

---

## 调试建议

### 1. 添加日志

在 `input_windows.go` 中添加详细日志：

```go
import "log"

func SendTextToTarget(...) error {
    log.Printf("[winsnap] SendTextToTarget: process=%s, text=%s, triggerSend=%v", 
        targetProcess, text, triggerSend)
    
    // 查找窗口后
    log.Printf("[winsnap] Found target window: hwnd=%v", targetHWND)
    
    // 激活窗口后
    log.Printf("[winsnap] Window activated, waiting...")
    
    // 发送按键后
    log.Printf("[winsnap] Sent Ctrl+V")
}
```

### 2. 使用 Spy++ 分析

使用 Windows SDK 的 Spy++ 工具：
1. 分析钉钉的窗口层级结构
2. 确定输入框所在的窗口
3. 观察输入事件是否被接收

### 3. 使用 API Monitor

监控 API 调用：
1. 观察 SendInput 是否成功调用
2. 观察钉钉是否接收到键盘事件

## 相关文件

- `pkg/winsnap/input_windows.go` - 输入模拟实现
- `pkg/winsnap/wake_windows.go` - 窗口激活逻辑
- `pkg/winsnap/winsnap_windows.go` - 窗口查找逻辑
- `internal/services/windows/snap_service.go` - 服务层调用

## 更新日志

| 日期 | 更新内容 | 结果 |
|------|---------|------|
| 2026-02-05 | 创建文档，记录问题和初步分析 | - |
| 2026-02-05 | 尝试 keybd_event API | 失败 |
| 2026-02-05 | 切换到 SendInput API | 待测试 |

## 参考资料

- [SendInput function](https://learn.microsoft.com/windows/win32/api/winuser/nf-winuser-sendinput)
- [Virtual-Key Codes](https://learn.microsoft.com/windows/win32/inputdev/virtual-key-codes)
- [Keyboard Input](https://learn.microsoft.com/windows/win32/inputdev/keyboard-input)
- [UI Automation Overview](https://learn.microsoft.com/windows/win32/winauto/uiauto-uiautomationoverview)
