# Mac 划词搜索与吸附功能修复/测试日志

本文档记录在 macOS 上运行划词搜索（text selection）和吸附（snap）功能时的报错原因、修复方案及测试检查清单，便于后续不再犯相同错误，或调试时不重复尝试已证明无效的方案。

---

## 一、Mac 构建报错原因分析（2026-02-04）

### 1. 实际导致编译失败的错误（必须修复）

**报错：**

```text
# willchat/pkg/winsnap
pkg/winsnap/winsnap_darwin.go:230:41: error: use of undeclared identifier 'kAXWindowNumberAttribute'
        if (AXUIElementCopyAttributeValue(win, kAXWindowNumberAttribute, &numVal) != kAXErrorSuccess || !numVal) {
                                               ^
1 error generated.
```

**原因：**

- `kAXWindowNumberAttribute` **在 macOS 公开的 Accessibility API 中不存在**。
- Apple 公开头文件（ApplicationServices/Accessibility）里没有声明该常量；通过 AXUIElement 获取「窗口号」没有公开 API。
- 社区常见做法：用 **CGWindowListCopyWindowInfo** 拿到带 `kCGWindowNumber` 的窗口列表，再通过 **位置/大小/标题** 与 AX 窗口匹配，从而得到窗口号（用于 `orderWindow:relativeTo:` 等）。

**结论：** 不能依赖 `kAXWindowNumberAttribute`，需改用「AX 窗口 frame + PID + CGWindowList 按 frame 匹配」获取窗口号。

---

### 2. CGO 结构体大小计算错误（id 类型）

**报错：**

```text
cgo: pkg/winsnap/winsnap_darwin.go:611:10: struct size calculation error off=8 bytesize=0
```

**原因：**

- 结构体 `WinsnapFollower` 中有字段 `id activationObserver;`。
- `id` 是 **Objective-C 特有类型**，CGO 无法识别其大小，因此报 `bytesize=0` 错误。

**解决方案：**

- 将 `id activationObserver;` 改为 `void *activationObserver;`。
- 赋值时使用 `(__bridge_retained void *)` 桥接（ARC 下保留引用计数）。
- 释放时使用 `(__bridge_transfer id)` 桥接（将所有权交还 ARC）。
- 初始化和清空时用 `NULL` 代替 `nil`。

---

### 3. 仅警告、不阻止编译的部分（可后续优化）

**sqlite-vec-go-bindings / CGO：**

- `sqlite3_auto_extension` / `sqlite3_cancel_auto_extension`：在 macOS 上被标记为 deprecated（进程级 auto extension 在 Apple 平台不受支持），仅为警告。
- `sqlite3_vtab_in` / `sqlite3_vtab_in_first` / `sqlite3_vtab_in_next`：仅在 macOS 13.0+ 可用，当前部署目标为 10.15，产生 `-Wunguarded-availability-new` 警告；建议在 C 代码里用 `__builtin_available` 包裹，或提升部署目标，或联系 sqlite-vec 上游。

**处理建议：** 先保证 Mac 能编译通过（修复 winsnap 的 `kAXWindowNumberAttribute`）；上述 CGO 警告可单独排期处理。

---

## 二、Mac 划词与吸附运行时问题修复（2026-02-04 第二轮）

### 1. 吸附窗体（winsnap）点击就隐藏

**问题描述：** Mac 上点击吸附窗体后，吸附窗体立即隐藏/消失。

**原因：**
- `TopMostVisibleProcessName` 使用 `frontmostApplication` 检测当前最前的应用
- 当用户点击吸附窗体时，WillChat 应用变成了 `frontmostApplication`
- 检测到前台应用不是目标应用（如微信），返回 `found=false`
- `step()` 调用 `hideOffscreen()` 隐藏吸附窗体

**修复：**
- 在 `zorder_darwin.go` 中添加 `winsnap_is_self_frontmost()` 检测当前应用是否是我们自己
- 当检测到前台应用是我们自己时，返回 `ErrSelfIsFrontmost` 错误
- 在 `snap_service.go` 的 `step()` 中处理此错误，保持当前吸附状态不变

### 1.5 划词弹窗位置不准确 & 点击无响应

**问题描述：** 
1. Mac 上划词弹窗位置不准确
2. 点击弹窗区域没有触发消息给吸附窗体或主窗体

**原因分析：**
1. **坐标系统问题**：尝试将像素坐标转换为点坐标传给 `SetPosition`，但效果不对
2. **点击处理问题**：当点击在弹窗区域内时，willchat 只是 `return`，而 demo 项目会**主动调用** `handleButtonClick()`

**修复方案（与 demo 项目保持一致）：**
1. `showAtScreenPosInternal` 中：
   - 直接使用像素坐标作为 `finalX, finalY`（不做点坐标转换）
   - 与 demo 项目行为一致
2. `showPopupAt` 中：
   - 统一使用传入的坐标（Mac 上是像素坐标）
   - click outside rect 使用相同坐标系统
3. `onDragStart` 回调中：
   - 当点击在弹窗区域内时，**主动调用** `go s.handleButtonClick()`
   - 这是 demo 项目的关键区别，确保点击弹窗能触发文本发送

### 2. 点击划词弹窗没触发程序唤醒

**问题描述：** Mac 上点击划词弹窗按钮后，主程序/吸附窗口/被吸附窗口都没有被唤醒到最前。

**原因：**
- `forceActivateWindow` 在非 Windows 平台只是调用 `w.Focus()`
- Mac 上 `Focus()` 可能不够强力，无法将应用从后台唤醒到前台

**修复：**
- 新增 `activate_darwin.go`，使用 `NSRunningApplication.activateWithOptions(NSApplicationActivateIgnoringOtherApps)` 激活应用
- 同时保留 `w.Focus()` 确保特定窗口获得焦点

### 3. 主窗体/吸附窗体内划词没弹窗

**问题描述：** 在自己应用内划词（主窗口或 winsnap 窗口），没有弹出划词弹窗。

**原因分析：**
- Mac 上 mouse hook 会跳过自己的应用（检测 frontmostApplication == currentApplication）
- 依赖前端 `onMouseUp` 监听 `window.getSelection()` 并调用 `ShowAtScreenPos`
- 前端使用 `e.screenX * devicePixelRatio` 转换为像素坐标

**相关修复：**
- 上述坐标转换修复应该同时解决此问题
- 如果仍有问题，可能需要检查 Wails 在 Mac 上的窗口坐标系统（Cocoa 使用左下角原点）

---

## 三、吸附功能（winsnap）CGO 编译修复（已实施）

**文件：** `pkg/winsnap/winsnap_darwin.go`

**思路：** 不再使用不存在的 `kAXWindowNumberAttribute`，改为：

1. 用已有 `winsnap_get_ax_frame(win, &axFrame)` 得到 AX 窗口的 frame（与 CG 同坐标系：左上角为原点，Y 向下）。
2. 用 `CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly | kCGWindowListExcludeDesktopElements, kCGNullWindowID)` 获取所有在屏窗口。
3. 在列表中按 **PID**（`kCGWindowOwnerPID`）过滤出目标进程的窗口，再用 **bounds**（`kCGWindowBounds`，用 `CGRectMakeWithDictionaryRepresentation` 解析）与 AX frame 做匹配（允许约 2 像素误差）。
4. 匹配到则取该条目的 `kCGWindowNumber`，作为 `orderWindow:NSWindowAbove relativeTo:winNo` 的 `winNo`。

**代码层面：**

- 增加 `#import <CoreGraphics/CGWindow.h>` 和 `-framework CoreGraphics`。
- `winsnap_get_ax_window_number(AXUIElementRef win)` 改为 `winsnap_get_ax_window_number(AXUIElementRef win, pid_t pid)`，在实现内用上述 CGWindowList + frame 匹配逻辑。
- 调用处（如 `winsnap_sync_to_target`）传入 `f->pid`。

**注意：** 若同一进程存在位置/大小完全相同的多窗口，理论上可能匹配到错误窗口；实际场景中极少见，可与「按标题再匹配」等作为后续增强。

---

## 三、Mac 划词搜索与吸附功能测试检查清单

在 Mac 上验证划词搜索和吸附时，建议按下面清单逐项打勾，避免重复测试无效方案或遗漏环境差异。

### 3.1 环境与构建

- [ ] **系统版本**：macOS _____（如 13.x / 14.x / 15.x）
- [ ] **架构**：arm64 / x86_64
- [ ] **构建命令**：`wails3 build DEV=true` 或 `wails3 task darwin:build` 等 _____
- [ ] **构建结果**：通过 / 失败（若失败，错误信息记录在下方「本次测试备注」）

### 3.2 划词搜索（Text Selection）

- [ ] **外部应用划词**：在目标应用（如浏览器、文本编辑器）中选中一段文字，能否正常弹出划词搜索 UI
- [ ] **主窗体内划词**：在 WillChat 主窗口内选中文字，能否正常弹出划词搜索 UI
- [ ] **吸附窗体内划词**：在 winsnap 窗口内选中文字，能否正常弹出划词搜索 UI
- [ ] 弹窗位置、大小是否合理，是否被遮挡或错位
- [ ] **点击弹窗按钮**：点击划词弹窗按钮后，文字是否正确发送到目标位置
- [ ] **点击弹窗后唤醒**：点击弹窗按钮后，主程序/吸附窗口是否被唤醒到前台
- [ ] 再次划词时，前一次弹窗是否正确关闭或更新
- [ ] 点击弹窗外部时，弹窗是否正确关闭

**已知平台差异（可在此补充）：**

- Mac 上 mouse hook 检测到自己应用是前台时会跳过，依赖前端 `onMouseUp` 监听
- Mac 上坐标系统：mouse hook 使用物理像素，Wails 窗口使用点坐标，需要转换
- Mac 上剪贴板操作：Cmd+C 模拟可能不会发送到原始应用，需前端配合

### 3.3 吸附功能（Snap）

- [ ] 选择目标应用并开启吸附后，侧边小窗是否稳定出现在目标窗口右侧并随其移动
- [ ] 目标窗口置前时，小窗是否保持在目标窗口之上（z-order），不被其它窗口盖住
- [ ] 多显示器、窗口全屏、窗口移动/缩放时，吸附位置是否仍正确
- [ ] 关闭目标应用或取消吸附后，小窗是否正常消失或恢复

**已知平台差异（可在此补充）：**

- macOS 无公开的「AX 窗口号」API，需通过 CGWindowList + frame 匹配获取窗口号；若匹配失败，z-order 可能异常，需在此注明现象。

### 3.4 本次测试备注（自由填写）

```text
日期：
测试人：
问题描述：
已尝试方案（避免重复）：
结论：
```

---

## 四、已尝试且无效/不推荐的方案（避免重复踩坑）

| 方案 | 结果 | 说明 |
|------|------|------|
| 在 macOS 上直接使用 `kAXWindowNumberAttribute` | 编译失败 | 该常量在公开 SDK 中未声明，不要使用。 |
| 仅靠 AX API 获取窗口号 | 不可行 | 公开 AX 无「窗口号」属性，需配合 CGWindowList。 |
| 不传 PID 仅用 AX frame 在全局窗口列表中匹配 | 易错 | 多进程时可能匹配到其它进程同位置窗口，必须用 PID 过滤。 |
| 在 C 结构体中直接使用 `id` 类型 | CGO 编译失败 | CGO 无法识别 Objective-C 的 `id` 类型大小，需改用 `void*` 并桥接。 |
| click outside rect 使用点坐标而检测用像素坐标 | 检测失败 | Mac 上 click outside 检测使用物理像素，rect 必须也用像素坐标。 |
| 非 Windows 平台用 `w.Focus()` 唤醒窗口 | 效果不佳 | Mac 上需用 `NSRunningApplication.activateWithOptions` 激活应用。 |
| 仅用 `frontmostApplication` 判断目标可见性 | 点击即隐藏 | Mac 上点击自己的窗口会让自己成为 frontmost，需排除自己。 |

---

## 五、后续可优化项（非本次必须）

- 处理 sqlite-vec 在 Mac 上的 CGO 弃用/可用性警告（部署目标或 `__builtin_available`）。
- 吸附匹配：若遇多窗口同 frame，可增加标题或其它属性二次匹配。
- 将本日志中的「已尝试且无效方案」与「测试检查清单」随问题迭代更新，便于新同事或后续调试复用。

---

*文档版本：2026-02-04；随 Mac 修复与测试进展更新。*
