# Windows 文本选中检测技术方案研究

## 概述

本文档汇总了 Windows 平台上检测文本选中和获取选中文本的各种技术方案，包括现有划词翻译工具的实现方式、Windows API 方法、以及通过截图/像素检测的替代方案。

---

## 一、现有划词翻译工具的实现方式

### 1. Pot-Desktop（跨平台划词翻译工具）

**技术栈**：Tauri + Rust + JavaScript (React)

**实现原理**：
- **Windows 平台**：使用 `Selection` 库（Rust crate），结合 **Automation API** 和 **剪贴板** 两种方式
- **核心库**：`pot-app/Selection` - 专门用于跨平台捕获选中文本的 Rust 库
- **API 方法**：`get_text()` - 返回当前选中文本，失败时返回空字符串

**实现细节**：
- 通过全局快捷键触发（如 Ctrl+Shift+E）
- 后端使用 Rust 的 `GlobalShortcutManager` 注册快捷键
- 前端通过 Tauri 命令与后端交互
- 支持 Windows、macOS、Linux（X11 和 Wayland）

**优点**：
- 跨平台统一 API
- 使用官方 Automation API，兼容性好
- 开源，代码可参考

**缺点**：
- 依赖 Automation API，某些应用可能不支持
- 需要用户手动触发快捷键

**参考资源**：
- GitHub: https://github.com/pot-app/pot-desktop
- Selection 库: https://github.com/pot-app/Selection

---

### 2. 沙拉查词（Saladict）

**实现方式**：

**浏览器内**：
- 通过浏览器扩展 API 监听文本选择事件
- 提供多种触发模式：显示图标、直接搜索、双击搜索、按住 Ctrl/⌘、鼠标悬浮取词

**浏览器外（Windows）**：
- **方案 1**：剪贴板中转 + 全局快捷键
  - 用户复制文本 → 按下快捷键 → 扩展读取剪贴板内容
  - 需要扩展权限：读取剪贴板
- **方案 2**：扩展 API 与本地程序通讯（尚未实现）

**配合辅助工具**：
- **Quicker**：支持 OCR、PDF 标注、鼠标手势
- **PantherBar**：类似 macOS 的 PopClip 功能

**优点**：
- 浏览器内体验流畅
- 支持多种触发方式

**缺点**：
- 浏览器外功能依赖剪贴板，需要用户手动复制
- 本地程序通讯方案尚未实现

**参考资源**：
- 官网：https://saladict.crimx.com/
- 文档：https://saladict.aichatone.com/

---

### 3. 有道词典

**核心技术**：

**智能文本定位**：
- 混合式文本检测算法（传统 CV + 深度学习）
- 边缘检测、连通域分析
- YOLOv4 改进网络进行文本区域识别
- 定位准确率：98.7%

**OCR 识别引擎**：
- 支持 72 种语言
- 动态二值化预处理
- 中英文混合识别准确率：96.4%
- 处理速度：单字 3ms

**Windows API 实现**：
- **DirectWrite API**：捕获屏幕文本
- DPI 感知的矢量渲染
- 全局快捷键唤醒时延：< 80ms
- 系统资源占用：< 15MB 内存

**多平台取词方式**：
- **鼠标取词**：监控鼠标指针位置的 Windows 系统接口
- **划词翻译**：监控鼠标选中文本的状态
- **OCR 屏幕取词**：使用光学字符识别处理图片和 PDF

**快捷键**：
- Windows：Ctrl+Alt+D 触发 OCR 取词
- 支持自定义快捷键组合
- 通过 COM 组件实现与 Office 套件的深度集成

**优点**：
- OCR 准确率高
- 支持多种取词方式
- 与 Office 集成良好

**缺点**：
- 闭源，实现细节不透明
- 可能使用私有 API 或特殊技术

---

### 4. Bob（macOS 专用）

**核心功能**：
- 划词翻译、截图翻译、输入翻译

**Windows 替代方案**：
- **TTime**：功能相似度 95%
- **pot-desktop**：跨平台方案
- **STranslate**：Windows 上最接近 Bob 的工具
- **DeepL、Quicker**：也支持划词翻译

**AutoHotKey 自定义实现**：
- 使用 AHK 2.0 检测全局热键
- 通过剪贴板获取选中文本
- 调用大模型 API（DeepSeek、Gemini）进行翻译
- 需要引入 JSON 库处理 API 请求

**参考资源**：
- GitHub: https://github.com/ripperhe/Bob

---

## 二、Windows API 方法

### 1. UI Automation API（推荐方案）

**核心接口**：
- `IUIAutomationTextPattern::GetSelection()` - 获取选中文本范围数组
- `IUIAutomationTextRange::GetText()` - 从文本范围提取文本内容

**实现步骤**：
1. 获取文本控件元素
2. 调用 `GetSelection()` 获取文本范围对象数组
3. 使用 `GetText()` 提取实际文本内容

**优点**：
- **官方推荐方案**，文档完善
- 跨应用通用，支持大多数现代应用
- **不需要剪贴板**，直接获取选中文本
- 支持多选（非连续选择）

**缺点**：
- 某些老旧应用可能不支持
- 性能开销相对较大（跨进程调用）
- 需要应用实现 UI Automation Provider

**性能优化**：
- 使用 `GetText()` 一次性检索中等大小的文本块
- 避免逐个字符检索（会导致多次跨进程调用）

**参考文档**：
- https://learn.microsoft.com/zh-cn/windows/win32/winauto/uiauto-usingtextrangeobjects
- https://learn.microsoft.com/zh-cn/windows/win32/api/uiautomationclient/nf-uiautomationclient-iuiautomationtextpattern-getselection

---

### 2. Win32 API（特定控件）

#### Edit 控件（如 Notepad）

**方法**：
- `EM_GETSEL` - 获取选择起始和结束位置
- `WM_GETTEXT` - 获取完整文本
- 提取选中部分

**实现**：
```cpp
// 获取选择范围
DWORD selStart, selEnd;
SendMessage(hwndEdit, EM_GETSEL, (WPARAM)&selStart, (LPARAM)&selEnd);

// 获取完整文本
int len = GetWindowTextLength(hwndEdit);
char* text = new char[len + 1];
GetWindowText(hwndEdit, text, len + 1);

// 提取选中部分
string selected = text.substr(selStart, selEnd - selStart);
```

#### RichEdit 控件

**方法**：
- `EM_GETSELTEXT` - 直接获取选中文本（更高效）

**优点**：
- 直接高效，无需获取完整文本
- 适合大文档

**缺点**：
- 仅适用于特定控件类型
- 不跨进程（`GetWindowText()` 不跨进程，需用 `SendMessage` + `WM_GETTEXT`）

---

### 3. WM_COPY / WM_GETTEXT（不推荐）

**方法**：
- `WM_COPY` - 发送复制消息到窗口
- `WM_GETTEXT` - 获取窗口文本

**限制**：
- **不可靠**：依赖应用实现消息处理
- 不是所有窗口类型都支持
- `WM_COPY` 可能将文本放入剪贴板（违背"不使用剪贴板"的要求）

**结论**：
- **不推荐**作为通用方案
- 仅适用于特定已知应用

---

### 4. 剪贴板方案（当前项目使用）

**实现流程**：
1. 检测鼠标拖拽（全局鼠标 Hook）
2. 模拟 `Ctrl+C`（`SendInput`）
3. 读取剪贴板内容

**优点**：
- **通用性强**，几乎所有应用都支持复制
- 实现简单
- 不需要应用特殊支持

**缺点**：
- **会覆盖剪贴板**，影响用户体验
- 需要模拟键盘输入，可能触发其他快捷键
- 某些应用可能拦截 `Ctrl+C`（如终端）

**当前项目实现**：
- 文件：`mouse_hook_windows.go`
- 使用 `WH_MOUSE_LL` 全局鼠标 Hook
- 检测左键按下/释放，判断拖拽
- `simulateCtrlC()` 发送 `Ctrl+C`
- `getClipboardText()` 读取剪贴板

---

## 三、截图/像素检测方案

### 1. 文本选中高亮颜色检测

**原理**：
- Windows 文本选中时会有高亮背景色
- 通过检测鼠标位置附近的像素颜色变化判断是否有选中

**Windows API**：
- `GetSysColor(COLOR_HIGHLIGHT)` - 获取系统高亮颜色
- `GetPixel(HDC, x, y)` - 获取指定像素的 RGB 值

**实现思路**：
1. 获取系统高亮颜色：`GetSysColor(COLOR_HIGHLIGHT)`
2. 捕获鼠标位置附近的屏幕截图
3. 检测像素颜色是否匹配高亮颜色
4. 如果匹配，说明该位置有文本选中

**优点**：
- 不需要应用支持
- 不依赖剪贴板

**缺点**：
- **不可靠**：不同应用可能使用不同的高亮颜色
- 某些应用使用自定义主题，不遵循系统颜色
- 需要持续截图，性能开销大
- 无法直接获取选中文本内容（仍需 OCR 或其他方法）

**参考**：
- `GetSysColor`: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getsyscolor
- `GetPixel`: https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-getpixel

---

### 2. 截图对比检测选中

**原理**：
- 定期截取屏幕
- 对比前后截图，检测像素颜色变化
- 如果检测到高亮区域，判断为文本选中

**工具**：
- **PixelDiff**：桌面和命令行工具，可视化图像差异
- **Microsoft Screenshots Diff Toolkit**：自动化截图对比工具集
- **Pixelmatch**：轻量级 JavaScript 库（150 行代码），像素级图像对比

**实现步骤**：
1. 定期截取鼠标位置附近的屏幕区域
2. 对比前后截图
3. 检测像素颜色变化（特别是高亮颜色）
4. 如果检测到变化，触发后续处理

**优点**：
- 不依赖应用 API
- 可以检测任何应用的选中

**缺点**：
- **性能开销大**：需要持续截图和对比
- **准确率低**：容易误判（窗口移动、动画等也会导致像素变化）
- **无法获取文本内容**：只能检测是否有选中，不能获取文本
- 需要 OCR 才能提取文本

**参考**：
- Pixelmatch: https://www.npmjs.com/package/pixelmatch
- GitHub: https://github.com/screenshotbot/pixel-diff

---

### 3. OCR 方案（截图后识别）

**原理**：
- 检测到文本选中后，截取选中区域
- 使用 OCR 识别文本内容

**Windows 内置方案**：
- **PowerToys Text Extractor**：Win+Shift+T，框选区域，自动 OCR 并复制到剪贴板
- 使用 Windows 系统语言设置进行 OCR
- 支持多语言包

**第三方库**：

**screen-ocr (Python)**：
- **WinRT**（推荐，Windows 专用）：快速且准确
- **Tesseract**：跨平台但较慢，准确率较低
- **EasyOCR**：非常准确但很慢，仅支持 64 位 Python

**Tesseract 实现**：
- `pyautogui` + `pytesseract` + `win32gui`
- 捕获特定窗口并运行 OCR
- `tesseract_window_scanner`：Windows 专用 Python 包

**优点**：
- 可以处理图片、PDF 等非文本内容
- 支持截图翻译

**缺点**：
- **性能慢**：OCR 需要时间
- **准确率问题**：复杂布局、小字体可能识别错误
- **资源占用**：OCR 引擎需要加载模型

**参考**：
- PowerToys Text Extractor: https://learn.microsoft.com/en-us/windows/powertoys/text-extractor
- screen-ocr: https://github.com/wolfmanstout/screen-ocr

---

## 四、技术方案对比总结

| 方案 | 优点 | 缺点 | 适用场景 |
|------|------|------|----------|
| **UI Automation API** | 官方推荐，跨应用通用，不需要剪贴板 | 某些应用不支持，性能开销 | 现代应用，需要可靠获取文本 |
| **剪贴板方案** | 通用性强，实现简单 | 覆盖剪贴板，影响用户体验 | 当前项目使用，兼容性优先 |
| **Win32 API (特定控件)** | 直接高效 | 仅适用于特定控件 | Edit/RichEdit 控件 |
| **截图像素检测** | 不依赖应用 | 不可靠，性能开销大，无法获取文本 | 不推荐 |
| **截图对比检测** | 可检测任何应用 | 性能开销大，准确率低，需 OCR | 不推荐 |
| **OCR 方案** | 可处理图片/PDF | 性能慢，准确率问题 | 截图翻译场景 |

---

## 五、推荐方案

### 方案 1：UI Automation API（首选）

**适用场景**：需要可靠获取选中文本，且目标应用支持 UI Automation

**实现步骤**：
1. 获取前景窗口
2. 获取窗口的 UI Automation 元素
3. 查找支持 `TextPattern` 的元素
4. 调用 `GetSelection()` 获取选中文本

**代码示例（C++）**：
```cpp
// 伪代码
IUIAutomation* automation;
CoCreateInstance(CLSID_CUIAutomation, NULL, CLSCTX_INPROC_SERVER, 
                 IID_IUIAutomation, (void**)&automation);

HWND hwnd = GetForegroundWindow();
IUIAutomationElement* element;
automation->ElementFromHandle(hwnd, &element);

IUIAutomationTextPattern* textPattern;
element->GetCurrentPatternAs(UIA_TextPatternId, IID_IUIAutomationTextPattern, 
                              (void**)&textPattern);

IUIAutomationTextRangeArray* ranges;
textPattern->GetSelection(&ranges);

// 提取文本
BSTR text;
ranges->GetElement(0)->GetText(-1, &text);
```

---

### 方案 2：剪贴板方案（当前项目）

**适用场景**：需要最大兼容性，可以接受覆盖剪贴板

**优化建议**：
1. **延迟复制**：仅在用户明确操作（如点击按钮）时复制
2. **保存原剪贴板**：复制前保存，使用后恢复
3. **检测剪贴板变化**：如果剪贴板内容未变化，说明可能没有选中文本

**当前项目实现**：
- 文件：`mouse_hook_windows.go`
- 模式：检测拖拽 → 模拟 Ctrl+C → 读取剪贴板
- 新模式（macOS）：显示弹窗 → 用户点击按钮 → 复制并获取文本

---

### 方案 3：混合方案

**策略**：
1. **优先使用 UI Automation**：检测应用是否支持
2. **回退到剪贴板**：不支持时使用剪贴板方案
3. **缓存应用支持情况**：避免重复检测

**实现**：
```go
func getSelectedText() string {
    // 尝试 UI Automation
    if text := getSelectedTextViaUIA(); text != "" {
        return text
    }
    
    // 回退到剪贴板
    return getSelectedTextViaClipboard()
}
```

---

## 六、特殊场景处理

### 1. 终端应用（CMD、PowerShell、WSL）

**问题**：
- 终端可能拦截 `Ctrl+C`（用于中断进程）
- UI Automation 可能不支持

**解决方案**：
- 检测应用类型，使用特殊处理
- 对于终端，可能需要使用其他快捷键（如 `Ctrl+Insert`）
- 或使用终端特定的 API（如 Windows Terminal API）

---

### 2. 浏览器

**问题**：
- 浏览器内文本选择由 JavaScript 处理
- 跨进程获取困难

**解决方案**：
- **浏览器扩展**：通过扩展 API 获取选中文本（如沙拉查词）
- **剪贴板方案**：通用但会覆盖剪贴板
- **UI Automation**：现代浏览器通常支持

---

### 3. PDF 阅读器

**问题**：
- PDF 文本可能是图片或矢量图形
- 某些阅读器不支持文本选择

**解决方案**：
- **OCR**：截图后使用 OCR 识别
- **PDF 库**：如果 PDF 支持文本层，使用 PDF 库提取

---

## 七、性能考虑

### UI Automation API

**优化**：
- 缓存 Automation 对象，避免重复创建
- 使用 `GetText(-1)` 一次性获取全部文本
- 避免频繁调用 `GetSelection()`

**性能指标**：
- 单次调用：通常 < 50ms
- 跨进程调用：可能达到 100-200ms

---

### 剪贴板方案

**优化**：
- 延迟复制：仅在需要时复制
- 检测剪贴板变化：避免无效操作
- 异步处理：不阻塞 UI

**性能指标**：
- 模拟 Ctrl+C：< 10ms
- 读取剪贴板：< 5ms
- 总耗时：通常 < 100ms

---

### 截图/OCR 方案

**性能开销**：
- 截图：50-200ms（取决于区域大小）
- OCR：500ms - 5s（取决于引擎和文本长度）
- **总耗时**：通常 > 1s

**优化**：
- 仅截取必要区域
- 使用快速 OCR 引擎（如 WinRT）
- 缓存 OCR 结果

---

## 八、参考资料

### 官方文档

1. **UI Automation**：
   - https://learn.microsoft.com/zh-cn/windows/win32/winauto/uiauto-usingtextrangeobjects
   - https://learn.microsoft.com/zh-cn/windows/win32/api/uiautomationclient/nf-uiautomationclient-iuiautomationtextpattern-getselection

2. **Win32 API**：
   - https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getsyscolor
   - https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-getpixel

3. **PowerToys Text Extractor**：
   - https://learn.microsoft.com/en-us/windows/powertoys/text-extractor

---

### 开源项目

1. **pot-desktop**：
   - GitHub: https://github.com/pot-app/pot-desktop
   - Selection 库: https://github.com/pot-app/Selection

2. **Bob**：
   - GitHub: https://github.com/ripperhe/Bob

3. **screen-ocr**：
   - GitHub: https://github.com/wolfmanstout/screen-ocr

---

### Stack Overflow 讨论

1. **获取选中文本（不使用剪贴板）**：
   - https://stackoverflow.com/questions/36083784/winapi-getting-text-selection-of-active-window-without-using-the-clipboard

2. **UI Automation 性能问题**：
   - https://learn.microsoft.com/zh-cn/windows/win32/winauto/uiauto-understandingperformanceissues

---

## 九、结论与建议

### 当前项目（ChatClaw）

**现状**：
- 使用剪贴板方案（鼠标 Hook + 模拟 Ctrl+C）
- 优点：兼容性好，实现简单
- 缺点：覆盖剪贴板，影响用户体验

**优化方向**：

1. **短期优化**：
   - 实现"延迟复制"模式：先显示弹窗，用户点击按钮时再复制
   - 保存并恢复原剪贴板内容
   - 检测剪贴板变化，避免无效操作

2. **中期优化**：
   - 实现 UI Automation API 作为首选方案
   - 检测应用支持情况，智能选择方案
   - 缓存应用支持情况

3. **长期优化**：
   - 支持浏览器扩展（浏览器内场景）
   - 集成 OCR 功能（截图翻译场景）
   - 支持终端特殊处理

---

### 通用建议

1. **优先使用 UI Automation API**：官方推荐，兼容性好
2. **剪贴板作为回退方案**：确保最大兼容性
3. **避免截图/像素检测**：不可靠，性能开销大
4. **OCR 仅用于特殊场景**：图片、PDF 等非文本内容

---

## 附录：相关 API 速查

### UI Automation

```cpp
// 获取选中文本
IUIAutomationTextPattern::GetSelection()
IUIAutomationTextRange::GetText()

// 检查是否支持文本选择
IUIAutomationTextPattern::get_SupportedTextSelection()
```

### Win32 API

```cpp
// 获取系统颜色
GetSysColor(COLOR_HIGHLIGHT)
GetSysColor(COLOR_HIGHLIGHTTEXT)

// 获取像素颜色
GetPixel(HDC hdc, int x, int y)

// Edit 控件
EM_GETSEL        // 获取选择范围
WM_GETTEXT       // 获取文本

// RichEdit 控件
EM_GETSELTEXT    // 直接获取选中文本
```

### 剪贴板

```cpp
// 打开剪贴板
OpenClipboard(HWND hwnd)

// 读取文本
GetClipboardData(CF_TEXT)

// 关闭剪贴板
CloseClipboard()
```

---

**文档创建时间**：2026-02-11  
**最后更新**：2026-02-11
