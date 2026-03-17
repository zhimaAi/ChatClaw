# ChatWiki Provider Login Reuse Design

## Goal

让服务商设置页 `ChatwikiProviderDetail.vue` 中的“立即登录”与 `ChatwikiSettings.vue` 中的“登录 ChatWiki Cloud”复用同一段浏览器登录逻辑，点击后都直接打开系统默认浏览器进入 ChatWiki Cloud 登录页。

## Scope

- 提取共享的 ChatWiki Cloud 登录 helper。
- 复用现有登录参数拼装规则。
- 保持原有开源版、自定义地址登录流程不变。

## Data Flow

1. 前端从 `ChatWikiService.GetCloudURL()` 获取 Cloud 基础地址。
2. 共享 helper 调用 `BrowserService.GetLoginParams()` 组装 `os_type`、`os_version` 查询参数。
3. helper 统一拼出 `/#/chatclaw/login` 地址。
4. helper 调用 `BrowserService.OpenURL()` 打开系统默认浏览器。

## Risks

- 如果两个页面继续各自维护登录 URL，后续再改参数规则时容易发生行为漂移。
- Cloud URL 为空时需要保留原有错误提示，避免静默失败。

## Validation

- `node --test src/lib/chatwikiAuth.test.ts` 验证共享 helper 的 URL 组装与浏览器打开行为。
- `npm run build` 验证前端类型检查和构建通过。
