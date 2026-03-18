# ChatWiki Sidebar Account Card Design

## Goal

在全局左侧导航底部的“设置”按钮上方增加一个 ChatWiki 账号状态卡片，按绑定状态展示不同文案和点击行为。

## Scope

- 仅处理全局左侧导航中的卡片展示。
- 不改动设置页主体布局。
- 样式参考用户提供的 HTML，但适配现有侧边栏视觉体系。

## UI Behavior

- 仅在左侧导航展开时显示卡片；折叠时隐藏。
- 卡片位于“设置”按钮上方。
- 若已绑定且 `chatwiki_version !== 'dev'`：
  - 第一行显示账号，优先 `user_name`，其次 `user_id`。
  - 第二行显示剩余积分，读取 ChatWiki 模型目录统计中的 `all_surplus`。
  - 点击卡片后跳转到设置页，并打开 ChatWiki 服务商设置页。
- 若未绑定，或 `chatwiki_version === 'dev'`：
  - 显示“立即登录”态。
  - 点击后跳转到设置页 ChatWiki 菜单，并触发云版登录流程。

## Data Flow

1. 侧边栏卡片挂载时通过现有 `chatwikiCache.getBinding()` 获取绑定信息。
2. 当绑定存在且不是 `dev` 时，再通过 `ChatWikiService.GetModelCatalog(false)` 获取积分统计。
3. 卡片内部将原始后端数据归一化为可渲染的视图模型：
   - `mode`: `bound` 或 `login`
   - `accountLabel`
   - `creditsLabel`
   - `action`: `openProviderSettings` 或 `login`

## Navigation Behavior

- `openProviderSettings`:
  - `settingsStore.setActiveMenu('modelService')`
  - `navigationStore.navigateToModule('settings')`
  - 由模型服务页默认选中 ChatWiki 服务商详情
- `login`:
  - `settingsStore.setActiveMenu('chatwiki')`
  - `settingsStore.requestChatwikiCloudLogin()`
  - `navigationStore.navigateToModule('settings')`

## Implementation Notes

- 为避免把状态判断和文案拼装散落在 SFC 中，新增一个轻量 helper，负责把 binding/catalog 映射为卡片视图模型。
- 为保持变更最小，新增独立组件并在 `SideNav.vue` 中引用。
- 复用现有缓存读取 binding，避免每次导航重渲染都直接请求后端。

## Testing

- 新增纯 TypeScript 测试覆盖：
  - 未绑定 -> 立即登录态
  - `dev` 绑定 -> 立即登录态
  - 云版绑定 -> 账号 + 积分态
  - 点击行为映射正确
- 组件集成层只做最小拼装，不新增重型 SFC 测试基础设施。
