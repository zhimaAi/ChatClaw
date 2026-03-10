# 定时任务 Agent Tools 设计文档

## 目标

在当前项目内，为聊天 Agent 增加一组可直接调用的定时任务管理 tools，使大模型可以在对话中完成以下操作：

- 查询计划任务列表
- 基于用户自然语言描述创建计划任务
- 删除计划任务
- 停用计划任务
- 启用计划任务

约束如下：

- 仅限本项目内部 Agent 使用，不对外提供 MCP 或 HTTP API
- 时间语义由大模型先理解并转换为结构化调度字段
- 创建、删除、启用、停用都必须先向用户确认
- 创建任务时必须按 AI 助手名称匹配，并在执行前向用户确认

## 背景与现状

当前项目已经具备以下基础能力：

- 技能系统已存在，`SKILL.md` 可被加载，但技能本身更偏向提示词和操作规范
- Agent 真正可执行的能力以 Eino tool 的形式注入
- 已有完整的定时任务服务与数据库表
- 定时任务服务已经支持：
  - 列表查询
  - 创建
  - 删除
  - 启用/停用
  - 立即执行
  - 运行记录

相关代码位置：

- `internal/services/scheduledtasks/service.go`
- `internal/services/scheduledtasks/dto.go`
- `internal/eino/agent/agent.go`
- `internal/services/chat/service.go`

## 结论

本需求采用“方案 1：内置工具为主，skill 为辅”。

第一期只新增任务管理 tools，不依赖新增 skill 作为功能承载体。原因如下：

- 真正能被 Agent 执行的是 tool，不是 `SKILL.md`
- 定时任务属于明确的业务动作，适合通过 tool 调服务
- 现有 `ScheduledTasksService` 已经提供核心 CRUD 和启停逻辑
- 将来若模型在工具使用上不稳定，可以再补一个 `task-manager` skill 作为约束层

## 架构分层

### 1. 定时任务业务层

继续复用现有 `ScheduledTasksService` 作为唯一业务落点，负责：

- 查询定时任务
- 创建定时任务
- 删除定时任务
- 启用/停用定时任务
- 调度注册与取消

这一层不负责理解用户自然语言，只负责接受结构化参数并完成校验、入库和调度。

### 2. 助手查询层

在 agents service 中补充按名称查询/匹配助手的能力，用于把用户说的“销售助手”“默认助手”解析成唯一的 `agent_id`。

这一层负责：

- 列出可选助手
- 按名称匹配助手
- 返回唯一命中、多重命中、未命中等状态

### 3. Agent Tools 层

新增一组 task management tools，供主 Agent 在聊天过程中直接调用。

这一层负责：

- 接收模型整理后的结构化输入
- 做助手名称匹配
- 做任务名称匹配
- 做调度字段合法性校验
- 在执行前生成确认摘要
- 在用户确认后调用业务 service

### 4. 可选 skill 层

第一期不强制实现。若后续发现模型经常跳过确认或误用 tool，再补一个 `task-manager` skill，约束模型：

- 任务创建前必须确认
- 删除/启停前必须确认
- 名称匹配不唯一时必须追问用户

## 为什么这里应该做 tool，而不是只做 skill

本项目中，tool 是大模型可实际调用的函数能力。其运行方式是：

- tool 在 Go 侧实现 `tool.BaseTool`
- 在创建 Agent 时注入到 `ToolsConfig`
- 模型在 ReAct 过程中自动选择调用

而 skill 更像：

- 一段操作说明
- 一组行为规范
- 对模型如何决策的补充提示

因此：

- 查询任务列表、创建任务、删除任务、启停任务，应该落在 tool
- skill 只适合做使用说明和行为约束，不适合单独承载任务 CRUD

## Tools 注入时机

这些 task tools 不建议注册进全局 `ToolRegistry`，而应在每轮 Agent 创建时通过 `extraTools` 注入。

原因：

- 这些 tools 依赖业务 service，如 `ScheduledTasksService`、`AgentsService`
- `ToolRegistry` 当前更适合无状态或通用工具
- `NewChatModelAgent(...)` 已支持 `extraTools`

因此本期设计采用：

- 通用工具继续走 `ToolRegistry`
- 任务管理 tools 在聊天生成时通过 `extraTools` 动态注入

## 最终 tool 集合

建议实现以下 7 个 tools：

- `scheduled_task_list`
- `agent_match_by_name`
- `scheduled_task_create_preview`
- `scheduled_task_create_confirm`
- `scheduled_task_delete`
- `scheduled_task_enable`
- `scheduled_task_disable`

其中创建拆成 `preview / confirm` 两个 tool，而不是一个混合 `confirm=true/false` 的 tool。

原因：

- 对模型更清晰
- 参数语义更稳定
- 更容易限制“未预览不可确认”
- 可降低误创建风险

## 各 tool 设计

### 1. `scheduled_task_list`

用途：

- 查询任务列表
- 供模型检索、定位任务

建议参数：

- `keyword?: string`
- `status?: "enabled" | "disabled" | "all"`
- `limit?: number`

建议返回：

- `tasks`
  - `id`
  - `name`
  - `prompt`
  - `agent_id`
  - `agent_name`
  - `enabled`
  - `schedule_type`
  - `schedule_value`
  - `cron_expr`
  - `next_run_at`
  - `last_status`

特点：

- 只读操作
- 不需要确认

### 2. `agent_match_by_name`

用途：

- 按助手名称匹配 `agent_id`

建议参数：

- `query: string`

建议返回：

- `matches`
  - `id`
  - `name`
  - `score`
- `match_status: "exact" | "single" | "multiple" | "none"`
- `recommended_agent_id?: number`

规则：

- 名称完全一致优先
- 忽略大小写和前后空格后再匹配
- 允许包含式与简单相似度排序
- 返回唯一命中或候选列表

### 3. `scheduled_task_create_preview`

用途：

- 对模型整理出的结构化任务草案做校验
- 不真正创建任务
- 返回待确认摘要

建议参数：

- `name: string`
- `prompt: string`
- `agent_name: string`
- `schedule_type: string`
- `schedule_value: string`
- `cron_expr: string`
- `enabled?: boolean`

说明：

- 时间自然语言由模型自行理解后转换成上述调度字段
- 该 tool 不做自然语言时间理解，只做校验

建议返回：

- `needs_confirmation: true`
- `parsed_task`
  - `name`
  - `prompt`
  - `agent_id`
  - `agent_name`
  - `schedule_type`
  - `schedule_value`
  - `cron_expr`
  - `enabled`
- `issues`
- `confirmation_message`

校验内容：

- 助手名称是否能唯一匹配
- 调度参数是否能通过现有 `parseSchedule(...)`
- 任务名称和 prompt 是否为空

若助手匹配失败或匹配到多个候选，不允许进入确认创建阶段。

### 4. `scheduled_task_create_confirm`

用途：

- 在用户确认后真正创建任务

建议参数：

- `name: string`
- `prompt: string`
- `agent_id: number`
- `schedule_type: string`
- `schedule_value: string`
- `cron_expr: string`
- `enabled?: boolean`

建议返回：

- `action: "created"`
- `task`
  - 创建后的完整任务信息

校验要求：

- `agent_id` 必须存在
- 调度参数必须再次通过校验
- 若参数不完整，拒绝执行

### 5. `scheduled_task_delete`

用途：

- 删除计划任务

建议参数：

- `task_id?: number`
- `task_name?: string`
- `confirm: boolean`

建议返回：

- 未确认时：
  - `action: "preview_delete"`
  - `needs_confirmation: true`
  - `matched_tasks`
  - `confirmation_message`
- 确认后成功时：
  - `action: "deleted"`
  - `deleted_task_id`
  - `deleted_task_name`

规则：

- 允许按 `id` 或名称定位
- 多任务重名时禁止直接删除
- `confirm=false` 仅做预检查
- `confirm=true` 才真正执行

### 6. `scheduled_task_enable`

用途：

- 启用已停用任务

建议参数：

- `task_id?: number`
- `task_name?: string`
- `confirm: boolean`

建议返回：

- `preview_enable` 或 `enabled`

规则：

- 若目标已启用，直接返回说明
- 名称匹配多条时必须让用户确认具体目标

### 7. `scheduled_task_disable`

用途：

- 停用已启用任务

建议参数：

- `task_id?: number`
- `task_name?: string`
- `confirm: boolean`

建议返回：

- `preview_disable` 或 `disabled`

规则：

- 若目标已停用，直接返回说明
- 名称匹配多条时必须让用户确认具体目标

## 时间解析职责划分

时间自然语言理解由大模型负责，而不是后端手写固定短语匹配。

职责划分如下：

- 用户：输入自然语言，例如“每个工作日上午九点”
- 大模型：将其理解并转换为结构化字段
  - `schedule_type`
  - `schedule_value`
  - `cron_expr`
- task tool：对结构化字段做合法性校验
- `ScheduledTasksService`：通过现有 `parseSchedule(...)` 完成最终验证和入库

这样可以充分利用大模型的自然语言理解能力，同时保留后端的稳定校验边界。

## 匹配规则

### 助手匹配规则

匹配顺序建议如下：

1. 完全相等
2. 忽略大小写相等
3. 去掉前后空格后相等
4. 包含式匹配
5. 简单相似度排序

返回结果分为：

- `exact`
- `single`
- `multiple`
- `none`

处理策略：

- `exact` / `single`：可进入确认态，但仍不能直接创建
- `multiple`：必须让用户选具体助手
- `none`：必须提示用户重新指定助手名称

### 任务匹配规则

删除、启用、停用都需要先定位任务。

建议支持：

- 按 `task_id`
- 按 `task_name`

规则：

- `id` 命中则视为唯一目标
- 名称完全匹配优先
- 名称模糊匹配若多条，则不执行，只返回候选项

## 交互流程

### 1. 查询任务

- 用户请求查看任务
- Agent 调用 `scheduled_task_list`
- tool 返回任务列表
- Agent 直接组织结果回复用户

不需要确认。

### 2. 创建任务

第一阶段：预览

- 用户用自然语言描述任务
- 模型先解析出结构化调度字段和助手名称
- Agent 调 `scheduled_task_create_preview`
- tool 返回任务草案与确认摘要

第二阶段：确认创建

- Agent 向用户复述任务草案
- 用户确认
- Agent 调 `scheduled_task_create_confirm`
- tool 调用 `CreateScheduledTask(...)`

### 3. 删除任务

- Agent 先调用 `scheduled_task_delete(confirm=false, ...)`
- tool 返回待确认摘要或候选列表
- 用户确认后再调用 `scheduled_task_delete(confirm=true, ...)`

### 4. 停用任务

- Agent 先调用 `scheduled_task_disable(confirm=false, ...)`
- 用户确认后再调用 `scheduled_task_disable(confirm=true, ...)`

### 5. 启用任务

- Agent 先调用 `scheduled_task_enable(confirm=false, ...)`
- 用户确认后再调用 `scheduled_task_enable(confirm=true, ...)`

## 确认机制

所有会修改状态的动作都必须走“预检查 -> 用户确认 -> 执行”。

适用范围：

- 创建
- 删除
- 启用
- 停用

确认摘要建议统一包含：

- 操作类型
- 任务名称
- AI 助手
- 执行时间
- 关键 prompt
- 当前状态或目标状态

设计原则：

- 宁可多确认一次，也不要误操作
- tool 返回 `needs_confirmation=true` 时，Agent 不得跳过确认直接执行

## 代码落点

### 新增文件

- `internal/eino/tools/scheduled_task_management.go`

建议在该文件中实现全部 task management tools。

### 需要补充的 service 能力

#### `internal/services/scheduledtasks/service.go`

建议补充：

- 按 ID 读取任务
- 按名称查找任务
- 输出适合 tool 层消费的轻量辅助方法

#### `internal/services/agents/`

建议补充：

- 列出助手
- 按名称匹配助手

### Agent 注入位置

在聊天生成阶段创建这些 tools，并通过 `extraTools` 注入当前 Agent。

预期涉及：

- `internal/services/chat/generation.go`
- `internal/eino/agent/agent.go`

其中：

- `agent.go` 无需改变总体结构
- `generation.go` 中增加 task tools 的创建与注入逻辑

## 第一版不做的内容

以下内容不纳入本次实现范围：

- 对外 MCP 或 HTTP API
- 新建 task sub-agent
- 新建 task-manager skill
- 草案持久化缓存
- 批量删除/批量启停
- 超复杂自然语言时间规则的后端手工解析

## 风险与边界

### 1. 时间解析误判

最大风险不在 CRUD，而在大模型将自然语言时间转换为结构化字段时出现误解。

控制方式：

- 创建前必须 preview
- preview 返回明确的时间摘要
- 用户确认后才创建
- confirm 阶段再次做后端校验

### 2. 助手名称匹配误判

控制方式：

- 严格区分唯一命中、多重命中、未命中
- 多重命中时禁止直接创建

### 3. 模型跳过确认

控制方式：

- tool 描述中写明确认要求
- confirm tool 必须要求完整参数
- 缺失参数时拒绝执行

## 实施顺序建议

建议按以下顺序开发：

1. 补 agents service 的名称匹配能力
2. 补 scheduledtasks service 的任务查找辅助方法
3. 新增 `scheduled_task_management.go`
4. 在 chat generation 中注入 task tools
5. 补测试
6. 视效果再决定是否增加 `task-manager` skill

## 验证建议

第一期至少覆盖以下场景：

- 查询任务列表成功
- 助手名唯一匹配时可生成 preview
- 助手名多重匹配时拒绝确认创建
- 非法调度字段 preview 失败
- 确认后成功创建任务
- 任务名称匹配多条时删除/启停被阻止
- 启用已启用任务、停用已停用任务时返回合理提示

## 结论

本方案以 task management tools 为核心，通过复用现有 `ScheduledTasksService` 和新增助手匹配能力，在不改变整体架构的前提下，为项目内聊天 Agent 增加计划任务管理能力。

该方案的关键点是：

- 用 tool 承载执行能力
- 用大模型承载自然语言时间理解
- 用后端 service 承载合法性校验和真正执行
- 用统一确认机制控制误操作风险

第一期按本设计实现后，已经能够满足“通过大模型查询任务列表、创建任务、删除任务、启用任务、停用任务”的核心需求。
