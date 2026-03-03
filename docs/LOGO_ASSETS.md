## ChatClaw 图标资源与替换说明

> 本文档记录龙虾图标的 PNG 资源位置，以及以后更换图标时需要修改的前端文件，方便统一维护。


### 0. 应用图标

应用图标：
因为在window 上透明图标不明显，所以window上单独一个图标样式（有背景有颜色的），其他的是 无背景有颜色的样式

window在`build\appicon.png`  使用的（有背景有颜色）与`src/assets/images/logo-app-colored.png` 一样的

其他在`build\sysicon.png` 使用的（无背景有颜色）与`src/assets/images/logo-floatingball.png` 一样的

需要都替换，打包时候才能使用对应的图标


### 1. 图标文件路径约定

所有前台图标均放在 `src/assets/images/` 目录下（相对于 `frontend`）。

- **应用主图标（有背景有颜色）**  
  - 路径：`src/assets/images/logo-app-colored.png`  
  - 用途：设置页「关于」中展示的应用图标等需要带圆角背景的场景。

- **悬浮球专用图标（无背景有颜色）**  
  - 路径：`src/assets/images/logo-floatingball.png`  
  - 用途：桌面悬浮球窗口中的龙虾图标，仅用于悬浮球。

- **浅色主题使用图标**  
  - 路径：`src/assets/images/logo-light.png`  
  - 用途：一般 UI 内的占位 / 头像图标（列表、对话头部等），在浅色背景下使用。

- **深色主题使用图标**  
  - 路径：`src/assets/images/logo-dark.png`  
  - 用途：标签页图标等需要根据主题切换明暗的场景（通过 JS 判断 `document.documentElement.classList.contains('dark')` 选择 light/dark 图）。

> 注意：当前代码中，浅色 / 深色主题的 PNG 由 `useLogo.ts` 统一选择；一般组件直接使用 `logo-light.png` 作为占位图，避免过度复杂的主题监听。

### 2. 主题感知 Tab 图标（Assistant 标签页）

- **逻辑入口文件**：`src/composables/useLogo.ts`
- **当前导出函数**：

  - `getLogoDataUrl()`  
    - 返回值：根据主题返回 `logo-light.png` 或 `logo-dark.png` 的 URL 字符串。  
    - 被使用位置：
      - `src/stores/navigation.ts`：`refreshAssistantDefaultIcons()` 中用于刷新所有 assistant 标签页的默认图标。
      - `src/pages/assistant/AssistantPage.vue`：`updateCurrentTab()` 中为当前助手标签页设置默认图标。

- **主题切换触发刷新**：
  - `src/App.vue` 中通过 `MutationObserver` 监听 `document.documentElement.classList` 的 `dark` class 变化，调用：
    - `navigationStore.refreshAssistantDefaultIcons()`  
    → 间接重新调用 `getLogoDataUrl()` 替换所有默认 assistant tab 图标。

> 如果将来要更换 Tab 图标，只需：
> 1. 替换 `logo-light.png` / `logo-dark.png` 文件或调整 `useLogo.ts` 的返回逻辑；
> 2. 保持 `getLogoDataUrl()` 函数签名不变即可。

### 3. 悬浮球图标使用位置

- **文件**：`src/floatingball/App.vue`
- **资源**：`logo-floatingball.png`
- **关键代码**：

```startLine:endLine:frontend/src/floatingball/App.vue
import logoFloatingball from '@/assets/images/logo-floatingball.png'

<!-- 模板中 -->
<img
  :key="collapsed ? 'collapsed' : 'expanded'"
  :src="logoFloatingball"
  :class="collapsed ? 'h-7 w-7' : 'h-11 w-11'"
  class="block"
  alt="ChatClaw floating icon"
/>
```

> 更换悬浮球图标时，只需要替换 `logo-floatingball.png` 文件即可，通常不需要改代码。

### 4. 应用主 Logo（关于页）

- **文件**：`src/pages/settings/components/AboutSettings.vue`
- **资源**：`logo-app-colored.png`
- **用途**：设置页「关于」卡片左侧的大图标。

> 若要换成新的应用大图标，只需替换 `logo-app-colored.png` 文件。

### 5. 一般 UI 占位 Logo 使用位置（使用 `logo-light.png`）

以下场景统一使用浅色版龙虾图标作为占位图标（无自定义头像时）：

- **助手聊天输入区首屏 Logo**
  - 文件：`src/pages/assistant/components/ChatInputArea.vue`

- **助手侧边栏列表头像**
  - 文件：`src/pages/assistant/components/AgentSidebar.vue`

- **助手消息列表中助手头像**
  - 文件：`src/pages/assistant/components/ChatMessageItem.vue`

- **记忆页左侧 Agent 列表头像**
  - 文件：`src/pages/memory/MemoryPage.vue`

- **新建助手对话框默认图标**
  - 文件：`src/pages/assistant/components/CreateAgentDialog.vue`

- **助手设置对话框默认图标**
  - 文件：`src/pages/assistant/components/AgentSettingsDialog.vue`

- **知识库页底部快捷提问区域中的 Agent 选择器图标**
  - 文件：`src/pages/knowledge/components/KnowledgeChatInput.vue`

- **Snap 模式头部 Agent 选择器默认图标**
  - 文件：`src/pages/assistant/components/SnapModeHeader.vue`

> 如果以后想让这些位置也随主题切换图标，可以在对应组件中引入新的 `useThemeLogo` 之类的组合式函数，或直接根据 `document.documentElement.classList.contains('dark')` 在组件内选择 `logo-light.png` / `logo-dark.png`。

### 6. Provider 图标中的 ChatClaw 图标

- **文件**：`src/components/ui/provider-icon/ProviderIcon.vue`
- **用途**：模型 / Provider 选择器中，用于展示各 Provider 的小图标。
- **行为**：
  - 当 `icon="chatclaw"` 时，不再使用 SVG 组件，而是使用 `logo-light.png` 作为 `<img>` 的 `src`。

> 若要调整 ChatClaw Provider 的小图标，只需替换 `logo-light.png`，或者在 `ProviderIcon.vue` 中改为使用其它 PNG 路径。

---

### 7. 未来更换图标的修改清单

如果以后要整体替换龙虾图标，通常只需要：

1. **替换 PNG 资源文件**（推荐优先级从高到低）：
   - `src/assets/images/logo-app-colored.png`
   - `src/assets/images/logo-floatingball.png`
   - `src/assets/images/logo-light.png`
   - `src/assets/images/logo-dark.png`

2. **如需调整逻辑，再考虑修改的代码文件**：
   - 主题相关：
     - `src/composables/useLogo.ts`
     - `src/stores/navigation.ts`（仅在改变默认图标策略时）
     - `src/App.vue`（仅在改动主题监听逻辑时）
   - 悬浮球：
     - `src/floatingball/App.vue`
   - 设置页 / 关于：
     - `src/pages/settings/components/AboutSettings.vue`
   - 助手相关 UI：
     - `src/pages/assistant/components/ChatInputArea.vue`
     - `src/pages/assistant/components/AgentSidebar.vue`
     - `src/pages/assistant/components/ChatMessageItem.vue`
     - `src/pages/assistant/components/CreateAgentDialog.vue`
     - `src/pages/assistant/components/AgentSettingsDialog.vue`
     - `src/pages/assistant/components/SnapModeHeader.vue`
   - 记忆 / 知识库：
     - `src/pages/memory/MemoryPage.vue`
     - `src/pages/knowledge/components/KnowledgeChatInput.vue`
   - Provider 小图标：
     - `src/components/ui/provider-icon/ProviderIcon.vue`

> 一般情况下，只替换 PNG 文件即可满足大部分场景；只有在更换为完全不同的命名或形态（例如拆分更多尺寸 / 主题）时，才需要改上述代码。

