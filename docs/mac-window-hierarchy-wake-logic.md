## macOS：吸附窗体 / 被吸附窗体 / 主窗体 / 划词弹窗 的层级与唤醒逻辑

本文档梳理 WillClaw 在 **macOS** 上与窗口相关的 4 个对象之间的层级（z-order / window level）与唤醒（activate / focus）策略：

- **主窗体**：`main`（WillClaw 主界面）
- **吸附窗体**：`winsnap`（WillClaw 侧边吸附聊天窗）
- **被吸附窗体**：外部目标应用的主窗口（如 微信/企业微信/飞书 等）
- **划词搜索弹窗**：`textselection`（透明小弹窗按钮）

目标是回答三个问题：

- **层级**：谁在谁上面？什么时候会“只在目标窗体上方”而不是全局置顶？
- **互动**：哪些用户动作会触发窗口之间的联动？
- **唤醒**：哪些场景会激活目标应用、激活 WillClaw、以及把键盘焦点交给哪个窗口？

---

## 一、窗口与关键文件映射

- **主窗体 `main`**
  - 创建：`internal/services/windows/main_window.go`
  - 唤醒（第二实例/托盘/Dock 重开）：`internal/bootstrap/app.go`（`mainWindowManager.safeWake/safeShow/safeUnMinimiseAndShow`）

- **吸附窗体 `winsnap`**
  - Window 定义：`internal/services/windows/definitions.go`（`WindowWinsnap`）
  - 运行/状态机：`internal/services/windows/snap_service.go`（`SnapService`）
  - macOS 跟随与 z-order：`pkg/winsnap/winsnap_darwin.go`（AXObserver + NSWorkspace 激活监听）
  - macOS 唤醒与“回焦”：`pkg/winsnap/wake_darwin.go`
  - macOS 顶层可见目标选择（并排除 self frontmost）：`pkg/winsnap/zorder_darwin.go`

- **划词弹窗 `textselection`**
  - 服务主逻辑：`internal/services/textselection/service.go`
  - macOS 不激活/置顶：`internal/services/textselection/popup_noactivate_darwin.go`、`popup_zorder_darwin.go`
  - macOS 激活应用（更强力唤醒）：`internal/services/textselection/activate_darwin.go`
  - 前端 UI：`frontend/src/selection/App.vue`

- **前端事件路由**
  - 主窗体路由（决定发给 winsnap 还是 assistant）：`frontend/src/App.vue`
  - winsnap 接收并自动发送：`frontend/src/winsnap/App.vue`

---

## 二、窗口层级（从高到低）

### 1) 划词弹窗 `textselection`（最高层、但不抢焦点）

**目的**：无论当前前台是谁（外部应用 / winsnap / main），弹窗都必须可见，并且点击时尽量不改变用户的键盘焦点（避免焦点/激活导致的副作用）。

**实现要点（macOS）**：

- Window level 强制提升到 **`NSPopUpMenuWindowLevel`**（比 `NSFloatingWindowLevel` 更高）
  - `popup_noactivate_darwin.go`: `setLevel:NSPopUpMenuWindowLevel`
  - `popup_zorder_darwin.go`: show 后再 `setLevel` + `orderFrontRegardless`
- 使用 **`orderFrontRegardless`** 保证在最前，但不会像 `makeKeyAndOrderFront` 那样强行抢 key window
- 通过 `CollectionBehavior` 让其可跨 Space、并忽略窗口切换循环（Mission Control / Cmd+Tab）

结论：**`textselection` 永远在最上层显示，但“显示≠激活”，它尽量不改变谁是 key window。**

### 2) 吸附窗体 `winsnap`（只在被吸附窗体之上，而不是全局置顶）

**目的**：winsnap 应“贴在目标应用的窗口旁边”，并且只在目标应用为前台时位于其上方；当用户切到其它应用时，不应覆盖其它应用（避免全局 AlwaysOnTop 的干扰）。

**实现要点（macOS）**：

- winsnap 自身保持 **Normal window level**（`MacWindowLevelNormal`）
  - `internal/services/windows/definitions.go`: `WindowLevel: Normal`
- 通过窗口号 `windowNumber` 做相对层级：`orderWindow:NSWindowAbove relativeTo:targetWinNo`
  - `pkg/winsnap/winsnap_darwin.go`: `winsnap_order_above_target`
- 只有在 **目标应用 frontmost** 时才重申相对层级
  - `winsnap_is_frontmost_pid(f->pid)` 为真才执行 `orderWindow`

结论：**winsnap 不做全局置顶，而是“条件性置顶”：仅当目标应用在前台时，winsnap 才位于目标窗口之上。**

### 3) 被吸附窗体（外部应用的主窗口）

**目的**：作为 winsnap 的“参考系”，winsnap 的位置（右侧对齐）和层级（其上方）都以该窗口为基准。

关键点：macOS 没有公开 AX API 直接拿“窗口号”，因此：

- 先用 AX 拿到窗口 frame（坐标系：左上为原点，Y 向下）
- 再用 `CGWindowListCopyWindowInfo` 结合 PID + bounds 近似匹配，拿到 `kCGWindowNumber`
- 最终用该 windowNumber 进行 `orderWindow:relativeTo:`

对应实现：`pkg/winsnap/winsnap_darwin.go` 的 `winsnap_get_ax_window_number(...)`。

### 4) 主窗体 `main`

主窗体是正常应用窗口，默认不参与与外部 app 的相对 z-order 维护；只有在需要承接“划词文本路由到主窗体”时才被强制激活/聚焦。

---

## 三、吸附（winsnap）跟随与 z-order 维护逻辑

### 1) 目标选择：找到“应该吸附的那个应用”

后台循环（约 400ms 一次）在 `SnapService.step()` 里调用：

- `winsnap.TopMostVisibleProcessName(enabledTargets)`

macOS 下该函数策略为：

- **如果 frontmost 是我们自己**（用户在点 main/winsnap），返回 `ErrSelfIsFrontmost`
  - `snap_service.go` 会直接 `return`，**保持当前吸附状态不变**（避免“点 winsnap 反而触发隐藏”）
- 否则优先用 PID 对比判断 frontmost 是否属于目标列表；若不是，则 fallback 遍历目标列表，找任意“有可见窗口”的 app

对应实现：`pkg/winsnap/zorder_darwin.go`。

### 2) 跟随：winsnap 永远贴在目标窗口右侧

macOS 跟随器由 `AttachRightOfProcess` 启动（返回一个 `Controller`）：

- `pkg/winsnap/winsnap_darwin.go` 的 `darwinFollower`
- 底层通过 Accessibility：
  - `AXObserver` 监听目标 app 的 `FocusedWindow/MainWindow` 变化
  - 对具体窗口监听 `Moved/Resized`
  - 每次触发都计算目标 frame，并把 winsnap 的 frame 设为“目标右侧对齐 + 高度一致”

注意：坐标系会在 AX（上左原点）与 Cocoa（下左原点）之间转换。

### 3) z-order：只在目标应用为前台时“压住目标窗口”

z-order 的两条维护路径：

- **实时跟随时顺手维护**：每次 setFrame 后，如果目标 app frontmost，执行 `orderWindow:NSWindowAbove relativeTo:targetWinNo`
- **目标应用重新激活时重申**：监听 `NSWorkspaceDidActivateApplicationNotification`，当目标 app 重新成为前台时，再执行一次 `orderWindow`

对应实现：`pkg/winsnap/winsnap_darwin.go`：

- `winsnap_register_activation_observer(...)`
- `winsnap_order_above_target(...)`

补充：建立新的吸附关系后，后端会做一次“**不激活目标应用**”的 z-order 同步，以避免后台 attach 时抢焦点：

- `internal/services/windows/snap_service.go`: `winsnap.SyncAttachedZOrderNoActivate(...)`

这里的“同步方向”始终是 **winsnap → above target（低→高）**：即只调整 winsnap 使其压在目标窗口之上，不会做反向（把目标压到 winsnap 之上）的同步。

---

## 四、唤醒/聚焦逻辑（谁被 activate，谁拿键盘焦点）

这里的“唤醒”分成两层：

- **activate app**：把某个应用提到前台（macOS 用 `NSRunningApplication.activateWithOptions(NSApplicationActivateIgnoringOtherApps)`）
- **focus window**：让某个具体窗口拿到键盘焦点（Wails `w.Focus()` / Cocoa `makeKeyAndOrderFront`）

### 场景 A：用户点击 winsnap 窗口（想继续在 winsnap 输入）

触发点：

- `frontend/src/winsnap/App.vue`：`pointerdown` 调用 `SnapService.WakeAttached()`

后台策略（`SnapService.WakeAttached`）：

- 调用 `winsnap.WakeAttachedWindowWithRefocus(selfWindow, targetProcessName)`

macOS 下实现（`pkg/winsnap/wake_darwin.go`）：

- 先 **activate 目标应用**（保证目标窗口不被其它 app 遮挡）
- 再把 winsnap **order 到目标窗口之上**
- 最后 **回焦到 winsnap**（重新 activate 当前 app + `makeKeyAndOrderFront`）
- 并带一个“防抖/防闪烁”的优化：若检测到目标窗口已经与 winsnap 相邻（z-order 紧挨，没有其它 app 窗口夹在中间），则 **不再 activate 目标**，只 focus winsnap，避免焦点来回跳

结论：**点击 winsnap = 目标应用可见 + winsnap 在其上方 + 键盘焦点回到 winsnap。**

### 场景 B：用户点击划词弹窗按钮（把文本送到 winsnap 或 main）

触发点：

- `frontend/src/selection/App.vue`：`mousedown` 触发 `Events.Emit('text-selection:button-click')`
  - 使用 mousedown + `preventDefault()`，减少焦点副作用

后台处理（`TextSelectionService.handleButtonClick`）：

- 先发 `text-selection:action` 事件给前端（只携带 text）
- 再根据 snap 状态决定唤醒哪个窗口：
  - 若 snap 状态是 `attached/standalone`：调用注入的 `wakeSnapWindow()` → `SnapService.WakeWindow()`
  - 否则：`forceActivateWindow(mainWindow)`
- 150ms 后 hide 弹窗

前端路由（`frontend/src/App.vue`）：

- 收到 `text-selection:action` 后再次调用 `SnapService.GetStatus()`
- 若 `attached/standalone`：Emit `text-selection:send-to-snap`
- 否则：切到 assistant tab 并 Emit `text-selection:send-to-assistant`

补充：winsnap 侧（`frontend/src/winsnap/App.vue`）收到 `send-to-snap` 后会把文本填入输入框并自动发送。

结论：**点击划词弹窗 = 事件路由由前端决定，但“唤醒哪个窗口”由后端先做一层兜底。**

### 场景 C：用户切回目标应用（目标应用变前台）

触发点：

- `NSWorkspaceDidActivateApplicationNotification`

策略：

- 不抢焦点、不激活 WillClaw
- 只做 `orderWindow:NSWindowAbove relativeTo:targetWinNo`，保证 winsnap 仍浮在目标窗口之上

结论：**目标 app 激活时，winsnap 仅调整可见层级，不抢键盘焦点。**

### 场景 D：用户在 WillClaw 自己窗口内划词（main/winsnap）

问题背景：

- macOS 的系统级 mouse hook 会跳过“我们自己是 frontmost”的情况（避免递归/权限/事件干扰）
- 因此在 WillClaw 内部划词需要前端兜底监听 selection

实现：

- `frontend/src/App.vue` 与 `frontend/src/winsnap/App.vue` 都注册了 capture `mouseup`：
  - `window.getSelection()` 拿 text
  - `TextSelectionService.ShowAtScreenPos(text, e.screenX * devicePixelRatio, e.screenY * devicePixelRatio)`（mac 用物理像素）

结论：**在自家窗口内划词：前端负责触发 ShowAtScreenPos，后端负责显示 popup。**

---

## 五、关键“不会互相打架”的设计点（经验总结）

- **textselection 永远最顶层，但不激活**：用 `NSPopUpMenuWindowLevel` + `orderFrontRegardless`，避免被外部 app 或 winsnap 覆盖，同时尽量不抢焦点
- **winsnap 不是全局置顶**：只在目标 app frontmost 时才压住目标窗口，避免覆盖其它 app
- **排除 self frontmost**：`TopMostVisibleProcessName` 返回 `ErrSelfIsFrontmost` 时 snap loop 不做任何隐藏/切换，避免“点击 winsnap 反而隐藏 winsnap”
- **点击 winsnap 才强制回焦**：`WakeAttachedWindowWithRefocus` 只在用户明确点击 winsnap 时触发，减少不必要的焦点跳转
- **坐标系统一**：macOS 下后端 hook/弹窗定位以“物理像素”为主，前端通过 `devicePixelRatio` 做转换，点击区域判断也用同一套坐标

---

## 六、快速排查清单（出现“层级/唤醒不对”时）

- **winsnap 点击后消失/被隐藏**
  - 检查 `ErrSelfIsFrontmost` 是否被正确处理（`snap_service.go` 应直接 return）
- **目标 app 切到前台但 winsnap 被盖住**
  - 检查 `NSWorkspaceDidActivateApplicationNotification` 监听是否仍在（activationObserver 是否被释放/未注册）
  - 检查 targetWinNo 是否能取到（AX frame ↔ CGWindowList 的匹配是否失败）
- **划词弹窗显示但点击无反应**
  - 检查是否走 `text-selection:button-click`（前端用 mousedown）
  - 检查后端是否 `Emit('text-selection:action')`
- **划词弹窗位置偏移**
  - 检查前端是否在 mac 下使用 `e.screenX * devicePixelRatio`
  - 检查后端 `showAtScreenPosInternal` 是否按“像素坐标一致”处理

