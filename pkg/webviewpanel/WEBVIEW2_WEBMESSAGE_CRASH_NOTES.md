# Windows 下访问 `https://yiyan.baidu.com/` 触发崩溃：根因与解决方案说明

## 现象概述

- **现象**：在 Windows（WebView2）环境中，只要 WebView2 加载 `https://yiyan.baidu.com/`（不管是在“三路对比问答”WebviewPanel 里，还是在多标签浏览器里作为普通 tab），都会在加载到一定阶段后直接 **进程退出**。
- **特征**：
  - UI 往往已经渲染出 1~2 个页面后才退出（不是立刻）。
  - 日志中常见 `[WebView2] Environment created successfully`，随后出现：
    - `panel error: The parameter is incorrect.`
  - 堆栈显示崩溃点落在 `go-webview2` 的 `Chromium.MessageReceived()` 中。

## 根因（结论）

这是一个 **go-webview2 的健壮性问题**：

- WebView2 的宿主会收到 `WebMessageReceived` 事件（网页调用 `chrome.webview.postMessage(...)` 触发）。
- `go-webview2` 在 `Chromium.MessageReceived()` 中 **强制把所有消息当作 string 读取**：
  - 调用 `args.TryGetWebMessageAsString()`
- 但很多网页会发送 **非 string payload**（对象/JSON 等），此时 WebView2 会返回 **E_INVALIDARG**：
  - 表现为错误文本：`The parameter is incorrect.`
- `go-webview2` 把这个错误走到了 `errorCallback()`，而 `errorCallback()` 默认会 `os.Exit(1)`：
  - 结果就是：**任意网页只要发一次非 string 的 postMessage，就能把宿主进程直接杀掉**

> 这不是业务逻辑 bug；业务层最多只能“绕开触发条件”，无法从根上保证所有第三方站点都不触发。

## 为什么 macOS 上更“流畅/不崩”？

macOS 使用的是 **WKWebView**（Wails/macOS 的实现），而 Windows 使用 **WebView2**：

- Windows/WebView2 暴露 `window.chrome.webview`，很多站点会检测到它并使用 `chrome.webview.postMessage(...)` 进行“宿主通信”。
- macOS/WKWebView **没有** `window.chrome.webview` 这一套对象，站点的“WebView2 宿主通信”逻辑通常不会触发，因此不会走到 `WebMessageReceived` → `TryGetWebMessageAsString` 这条崩溃路径。

所以表现为：macOS 上同站点更稳、Windows 上容易触发 go-webview2 的 fatal 分支。

---

## 更优雅的两个解决方案（推荐顺序）

### 方案 A（推荐）：上游修复 go-webview2（根治）

**目标**：让 `Chromium.MessageReceived()` 对“非 string 消息”具备容错能力，绝不应该直接 `os.Exit(1)`。

**建议修复策略**（核心要点）：

1. `TryGetWebMessageAsString()` 失败时：
   - 不要 fatal
   - 尝试 `GetWebMessageAsJSON()`（WebView2 原生提供）
   - 如果仍失败，忽略该条消息并返回 0
2. 对“非 string 消息”（走 JSON fallback）的场景：
   - 不要再执行 `sender.PostWebMessageAsString(message)` 回显（避免改变类型/语义）
3. `errorCallback()` 不应对“业务可忽略/可恢复”的事件走 `os.Exit(1)`。

**优点**：
- 从根上修复：不依赖业务站点、不依赖注入脚本
- 不管你是 Browser tab 还是 WebviewPanel，都彻底稳定

**落地方式**：
- 提 Issue + PR 到 `wailsapp/go-webview2`
- 合并后项目只需升级依赖版本，无需 `vendor`、无需 `replace`

---

### 方案 B：应用侧规避（Workaround，不是根治）

如果短期无法等待上游合并，可以用应用侧手段降低触发概率。

#### B1. Fork + `go.mod replace`（工程上比 vendor 更优雅）

- fork `github.com/wailsapp/go-webview2`
- 在项目 `go.mod` 写：

```text
replace github.com/wailsapp/go-webview2 => github.com/<your-org>/go-webview2 <commit>
```

你仍然是在“修 go-webview2”，但方式比 vendor 更可维护，也更容易回归上游。

#### B2. 业务/注入规避：把网页 postMessage 强制变成 string（有兼容风险）

**思路**：在页面最早期注入脚本，把 `chrome.webview.postMessage(x)` 改写成：
- `postMessage(typeof x === 'string' ? x : JSON.stringify(x))`
或直接 no-op。

**注意**：要做到“最早期”（document-created）才可靠，否则网站脚本可能已提前发过 message。

在当前 `pkg/webviewpanel` 实现里：
- `WebviewPanelOptions.JS` 的执行时机偏晚（NavigationCompleted 后 ExecJS），对“最早期拦截”不够稳。
- 若要业务层走该方案，建议在 `webviewpanel` 增加类似 `PreloadJS`/`InitJS` 选项，并在 `Navigate(url)` 前调用 `chromium.Init(preloadJS)`。

**优点**：
- 不改依赖（理论上）

**缺点**：
- 仍是 workaround，且对第三方站点有潜在副作用
- 无法保证所有站点都不触发（尤其是多 frame/ServiceWorker 场景）

---

## 与“左上角黑块/闪烁”的关系（补充）

你在 Windows 下看到的“左上角黑块/加载过程中黑框闪烁”，多数不是 WebView2 bug，而是 **panel 创建/布局事件**引起：

- 若创建 panel 时未传 `X/Y/Width/Height`，默认会以 `0,0,400,300` 先出现在主窗口左上角，后续 `SetBounds` 再搬走。
- 若前端在启动阶段密集发送 layout（`nextTick + rAF + ResizeObserver`），后端如果没有“只创建一次”的保护，会反复创建 panel，导致大量 `Environment created successfully` 和明显闪烁。

这类问题的建议做法：
- **创建 panel 之前先拿到稳定 layout**
- **创建时就带初始 bounds**
- **layout 事件去重（x/y/w/h 不变则不发）**
- **后端 ensurePanels 增加初始化状态机，防止重复创建**

