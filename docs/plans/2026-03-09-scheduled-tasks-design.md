# ChatClaw 定时任务运行记录设计文档

**日期**: 2026-03-09

**目标**: 在左侧导航“知识库”下方新增“定时任务”模块，支持创建定时任务、查看任务列表、查看每次执行的运行记录，并将每次定时触发与一个独立的新会话关联，便于在右侧查看对话明细。

## 1. 需求结论

本次需求范围已经确认如下：

- 在左侧导航中，位于“知识库”下面新增一级菜单“定时任务”
- 定时任务页面为独立模块，不放入 AI 助手内部子页面
- 每次定时任务触发时，都创建一个新的会话
- 该会话名称增加前缀 `(定时)`
- 运行记录需要保留，删除任务后仍然能看到历史运行记录
- 需要展示每个任务的执行记录，包括：
  - 执行时间
  - 执行状态
  - 执行耗时
  - 失败时的错误原因（鼠标移入显示）
- 运行记录弹窗采用左右结构：
  - 左侧为运行记录列表
  - 右侧为该次运行关联的对话明细
- 后端与桌面端当前是一体部署，时区一致
- 暂不处理“同一任务执行时间过长时的并发冲突”
- 不额外评估“后台无人值守调用是否支持”，执行链路直接按“读取当前 AI 助手配置、创建新对话并发送提示词”设计

## 2. 整体方案

采用“独立定时任务模块 + 独立运行记录表 + 每次执行创建独立会话”的方案。

该方案包含三层：

1. 定时任务定义层
   - 管理任务名称、提示词、关联助手、时间配置、启用状态
   - 不在任务上保存模型供应商、模型、知识库、思考模式、对话模式等派生配置

2. 调度执行层
   - 负责根据 cron/自定义时间触发任务
   - 每次触发生成一条运行记录

3. 会话关联层
   - 每次触发都创建一个新的 conversation
   - 执行时先读取当前 AI 助手配置，再创建 conversation
   - 发送一条任务提示词作为该会话的首条用户消息
   - 将本次运行记录与该 conversation 关联

这样设计的优点：

- 运行记录与对话明细天然一一对应，最符合设计图
- 不污染现有 AI 助手页面结构
- 后续可以继续扩展“手动执行”“编辑任务”“失败重试”“跳转到原会话”等能力
- 删除任务时不影响运行历史，满足审计与追溯需求

## 3. 前端模块设计

### 3.1 模块位置

在左侧导航新增一级模块 `scheduled-tasks`，放在“知识库”下面。

涉及接入位置：

- `frontend/src/stores/navigation.ts`
- `frontend/src/components/layout/SideNav.vue`
- `frontend/src/App.vue`
- `frontend/src/locales/zh-CN.ts`
- `frontend/src/locales/en-US.ts`

### 3.2 页面目录结构

前端组件收敛到一个独立目录，不拆散到其他页面目录：

```text
frontend/src/pages/scheduled-tasks/
├── ScheduledTasksPage.vue
├── components/
│   ├── TaskSummaryCards.vue
│   ├── TaskTable.vue
│   ├── CreateTaskDialog.vue
│   ├── TaskRunHistoryDialog.vue
│   ├── TaskRunConversationPreview.vue
│   └── TaskRunStatusBadge.vue
├── composables/
│   └── useScheduledTasks.ts
├── types.ts
├── constants.ts
└── utils.ts
```

这样组织的原因：

- 定时任务是独立一级模块，页面和弹窗高度内聚
- 后续继续增加编辑任务、手动执行、分页、筛选时，不会污染 `assistant` 或公共组件目录
- 页面级逻辑、类型、工具函数都能在同目录维护，结构清晰

### 3.3 页面结构

#### 顶部统计区

展示四张统计卡：

- 任务总数
- 运行中
- 已暂停
- 失败

统计来源建议以后端聚合接口返回为主，不在前端自行扫描大列表。

#### 中部任务列表区

表格列建议如下：

- 任务标题
- 执行时间
- 上次运行
- 下次运行
- 状态
- 操作

操作项建议包括：

- 立即运行
- 历史记录
- 编辑
- 删除

#### 运行记录弹窗

弹窗结构采用左右分栏：

- 左侧：运行记录列表
- 右侧：当前选中运行记录的会话详情

左侧每条记录展示：

- 状态图标
- 执行时间
- 触发方式（手动/定时）
- 耗时

右侧展示：

- 关联会话基本信息
- 会话消息列表
- 只读展示，不允许直接在弹窗内继续发送

右侧建议单独实现 `TaskRunConversationPreview.vue`，不要直接复用整个 `AssistantPage.vue`，原因是完整助手页包含会话切换、输入框、工具栏、知识库选择等无关能力，复用成本高且容易引入额外耦合。

#### 创建/编辑任务弹窗字段

本次确认创建和编辑定时任务时只保留以下字段：

- 任务名称
- 提示词
- AI 助手
- 调度方式与时间配置
- 是否启用

以下字段从弹窗中移除，不再由计划任务自行配置：

- 对话模式
- 模型供应商
- 模型
- 知识库
- 思考模式

这些能力统一由选中的 AI 助手配置决定。

## 4. 后端模块设计

建议新增独立服务模块：

```text
internal/services/scheduledtasks/
├── service.go
├── model.go
├── dto.go
├── scheduler.go
├── cron_parser.go
├── conversation_runner.go
└── migrations/
```

### 4.1 文件职责

- `service.go`
  - 对外暴露 CRUD、启停、立即执行、查询运行记录、查询运行详情等接口

- `model.go`
  - 定义数据库模型

- `dto.go`
  - 定义前端交互 DTO

- `scheduler.go`
  - 负责定时器注册、启动恢复、下次执行时间计算

- `cron_parser.go`
  - 负责将快捷时间、自定义时间、Linux Crontab 转换为统一 cron 表达式

- `conversation_runner.go`
  - 负责执行一次具体任务：
    - 创建运行记录
    - 创建新会话
    - 发送提示词
    - 更新运行状态和错误信息

### 4.2 为什么独立成模块

原因如下：

- “任务定义”“调度触发”“会话发送”“运行记录”是四种不同职责
- 如果全部塞入现有聊天或会话服务，会让边界混乱
- 后续扩展编辑任务、手动执行、历史记录分页时维护成本会快速上升

因此独立成 `scheduledtasks` 模块是合理的。

## 5. 数据库设计

本次至少新增两张主表，满足第一期需求。

### 5.1 `scheduled_tasks`

用于存储任务定义。

建议字段：

- `id`
- `created_at`
- `updated_at`
- `name`
- `prompt`
- `agent_id`
- `schedule_type`
  - 取值建议：`preset | custom | cron`
- `schedule_value`
  - 保存用户原始配置，例如快捷项、自定义 JSON、cron 原文
- `cron_expr`
  - 标准化后的 cron 表达式，调度器只认这个字段
- `timezone`
- `enabled`
- `last_run_at`
- `next_run_at`
- `last_status`
  - 取值建议：`pending | running | success | failed`
- `last_error`
- `last_run_id`
- `deleted_at`
  - 建议支持软删除，方便删除后仍能从历史记录反查任务信息

### 5.2 `scheduled_task_runs`

用于存储每次执行记录。

建议字段：

- `id`
- `task_id`
- `trigger_type`
  - 取值建议：`schedule | manual`
- `status`
  - 取值建议：`queued | running | success | failed`
- `started_at`
- `finished_at`
- `duration_ms`
- `error_message`
- `conversation_id`
- `user_message_id`
- `assistant_message_id`
- `snapshot_task_name`
- `snapshot_prompt`
- `snapshot_agent_id`

增加 snapshot 字段的原因：

- 定时任务后续可能被编辑
- 删除任务后仍保留运行记录，需要保留基础可读信息
- 本次已明确历史详情始终按当前助手配置展示，因此不保留模型侧配置快照

### 5.3 删除策略

任务删除后，运行记录仍需保留。

建议策略：

- `scheduled_tasks` 采用软删除
- `scheduled_task_runs` 不删除

这样可以满足：

- 页面任务列表中不再展示已删除任务
- 历史运行记录仍然可查询
- 后续如需“已删除任务历史归档”也有空间

## 6. 与会话系统的关联方式

本次设计要求每次任务执行都创建一个新的会话。

执行链路建议如下：

1. 调度器触发某个任务
2. 创建一条 `scheduled_task_runs` 记录，状态为 `running`
3. 读取当前 `agent_id` 对应的 AI 助手配置
4. 调用现有 `conversations` 服务创建新的 conversation，并把当前助手配置写入 conversation
5. 会话标题建议格式：

```text
(定时) 任务名称 - 2026-03-09 09:00
```

6. 将生成的 `conversation_id` 回填到本次 run 记录
7. 在该 conversation 中创建一条用户消息，内容为任务提示词
8. 走现有聊天发送链路，生成 AI 回复
9. 成功则更新 run 状态为 `success`
10. 失败则更新 run 状态为 `failed` 并写入错误信息
11. 同步更新 `scheduled_tasks.last_run_at / last_status / last_error / next_run_at / last_run_id`

这样设计的优点：

- 每次运行记录都能稳定关联一个独立会话
- 会话实际运行配置天然与当前 AI 助手保持一致
- 弹窗右侧直接展示现有消息结构即可
- 在 AI 助手页面里也能看到这些定时生成的会话
- `(定时)` 前缀可以让普通对话和自动任务对话一眼区分

## 7. 调度与时间配置设计

### 7.1 时间配置方式

创建任务弹窗保留三种方式：

- 快捷设置
- 自定义时间
- Linux Crontab 代码

建议前端提交统一结构：

```ts
type ScheduledTaskScheduleInput = {
  scheduleType: 'preset' | 'custom' | 'cron'
  scheduleValue: string
  cronExpr: string
}
```

### 7.2 时区

当前产品后端与桌面端是一体的，时区一致，因此第一期可按统一本地时区处理，不需要额外做多时区设计。

数据库仍建议保留 `timezone` 字段，原因：

- 不影响当前实现
- 为未来服务端部署或跨时区同步预留扩展点

### 7.3 执行过久的并发问题

第一期暂不处理“同一任务执行时间过长时又被下一次调度触发”的并发控制。

文档中明确：

- 不加锁
- 不做跳过策略
- 不做排队策略

后续若要扩展，可再增加并发策略字段，例如：

- `parallel`
- `skip_if_running`
- `queue_if_running`

但本期不纳入开发范围。

## 8. 状态与错误展示设计

### 8.1 任务级状态

任务列表状态建议由 `enabled + last_status` 共同决定：

- `enabled = false` 显示 `已暂停`
- `enabled = true && last_status = running` 显示 `进行中`
- `enabled = true && last_status = failed` 显示 `失败`
- 其他情况显示 `已启用`

### 8.2 运行记录级状态

运行记录状态建议统一为：

- `queued`
- `running`
- `success`
- `failed`

### 8.3 错误展示

列表中不直接展开错误正文。

建议交互：

- 若任务最近一次执行失败，则在任务列表状态区域显示失败标识
- 鼠标移入时通过 tooltip 展示 `last_error`
- 在运行记录列表中，失败项鼠标移入显示该 run 的 `error_message`

这样设计的原因：

- 列表区域保持紧凑
- 长错误不会破坏布局
- 与设计图中的 hover 查看错误原因一致

## 9. 前后端接口建议

建议后端对前端暴露以下接口：

- `ListScheduledTasks()`
- `GetScheduledTaskSummary()`
- `CreateScheduledTask(input)`
- `UpdateScheduledTask(id, input)`
- `DeleteScheduledTask(id)`
- `SetScheduledTaskEnabled(id, enabled)`
- `RunScheduledTaskNow(id)`
- `ListScheduledTaskRuns(taskID, page, pageSize)`
- `GetScheduledTaskRunDetail(runID)`

### 9.1 运行详情接口返回结构

建议 `GetScheduledTaskRunDetail(runID)` 直接返回聚合数据：

```ts
{
  run: ScheduledTaskRun
  conversation: Conversation | null
  messages: Message[]
}
```

这样设计的好处：

- 前端弹窗只需一次请求
- 不需要在前端串联多个服务自行拼装
- 运行记录与消息明细的边界由后端统一控制

## 10. 与现有代码的接入点

### 10.1 前端

需要修改的现有文件：

- `frontend/src/stores/navigation.ts`
  - 新增 `scheduled-tasks` 模块定义

- `frontend/src/components/layout/SideNav.vue`
  - 在“知识库”下方增加“定时任务”菜单

- `frontend/src/App.vue`
  - 注册 `ScheduledTasksPage.vue`

- `frontend/src/locales/zh-CN.ts`
- `frontend/src/locales/en-US.ts`
  - 增加菜单、列表、弹窗、状态等文案

### 10.2 后端

需要新增或修改的现有位置：

- `internal/services/scheduledtasks/`
  - 新增整个模块

- `internal/bootstrap/app.go`
  - 注入并初始化 `scheduledtasks` 服务
  - 应用启动时恢复启用中的定时任务

- `internal/sqlite/migrations/`
  - 增加两张表的迁移文件

- 现有 `conversations` 与 `chat` 服务
  - 复用创建会话和发送消息能力
  - 不建议直接修改数据结构，只通过服务调用接入

## 11. 开发顺序建议

建议开发顺序如下：

1. 数据库迁移
   - 创建 `scheduled_tasks`
   - 创建 `scheduled_task_runs`

2. 后端基础服务
   - 完成 DTO、Model、CRUD
   - 完成任务启停和列表查询

3. 调度器
   - 支持应用启动恢复已启用任务
   - 支持计算 `next_run_at`

4. 执行链路
   - 触发任务
   - 创建 run
   - 创建 conversation
   - 发送提示词
   - 更新执行状态

5. 前端页面
   - 导航接入
   - 列表页
   - 创建任务弹窗
   - 运行记录弹窗

6. 文案与样式收尾
   - tooltip
   - 状态色
   - 空状态

这个顺序可以减少前后端反复返工。

## 12. 风险与边界说明

本次已明确的处理边界如下：

### 12.1 不纳入本期

- 不额外评估后台无人值守调用能力
- 不处理同一任务执行过久导致的并发冲突
- 不实现运行过程中的中间日志流
- 不做多时区支持

### 12.2 本期必须保证

- 每次执行都新建独立会话
- 会话前缀为 `(定时)`
- 删除任务后，历史运行记录仍然保留可查
- 失败错误可通过 hover 查看
- 左侧运行记录、右侧会话明细结构可正常联动

## 13. 方案合理性结论

本方案是合理的，原因如下：

- 前端独立一级模块，符合产品入口要求
- 页面组件全部收敛在 `frontend/src/pages/scheduled-tasks/`，结构清晰
- 后端独立服务模块职责明确，便于维护
- 每次执行创建新会话，并在执行时读取当前 AI 助手配置，最符合“运行记录 + 对话明细”交互模型
- 删除任务仍保留记录，满足历史追溯需求
- 对现有会话与消息系统改动可控，复用度高

## 14. 后续可选扩展

以下内容不属于本期，但结构上已经预留空间：

- 编辑任务
- 失败重试
- 手动重跑最近一次配置
- 运行记录分页与筛选
- 从运行记录跳转到 AI 助手中的原会话
- 并发策略配置
- 任务执行过程日志

---

**结论**: 按本设计实施，可以较低风险地完成“定时任务 + 运行记录 + 对话明细关联”的需求，并且后续扩展空间充足。
