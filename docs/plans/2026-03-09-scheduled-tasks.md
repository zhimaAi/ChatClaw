# Scheduled Tasks Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为桌面端增加可创建、编辑、启停、立即执行、查看运行记录和会话明细的定时任务模块，任务创建时只选择 AI 助手，执行时完全按当前助手配置发起对话。

**Architecture:** 后端新增 `scheduledtasks` 独立服务，负责任务定义、调度注册、运行记录和会话执行链路；任务和运行记录仅保存 `agent_id` 及最小任务快照，执行时实时读取当前助手配置创建会话。前端新增独立页面接入导航，复用现有会话与消息结构做只读预览。实现顺序按数据库、服务、调度、页面逐层推进，并用测试先行约束 cron 解析和核心业务规则。

**Tech Stack:** Go, Bun, SQLite, Wails v3, Vue 3, Pinia, TypeScript

---

### Task 1: 收缩数据模型与迁移脚本

**Files:**
- Create: `internal/services/scheduledtasks/model.go`
- Create: `internal/services/scheduledtasks/dto.go`
- Create: `internal/services/scheduledtasks/service.go`
- Create: `internal/services/scheduledtasks/cron_parser.go`
- Create: `internal/services/scheduledtasks/scheduler.go`
- Create: `internal/services/scheduledtasks/conversation_runner.go`
- Create: `internal/sqlite/migrations/202603091200_create_scheduled_tasks_tables.go`

**Step 1: 写 cron 解析失败测试**

Run: `go test ./internal/services/scheduledtasks -run TestParseSchedule -count=1`
Expected: FAIL，包或测试不存在

**Step 2: 实现最小模型和迁移结构**

删除任务表和运行表中供应商、模型、知识库、思考模式、对话模式相关字段，只保留 `agent_id` 与最小任务快照；同时实现 `preset/custom/cron` 的最小解析逻辑。

**Step 3: 跑测试确认通过**

Run: `go test ./internal/services/scheduledtasks -run TestParseSchedule -count=1`
Expected: PASS

### Task 2: 落地 CRUD 与运行记录查询

**Files:**
- Modify: `internal/services/scheduledtasks/service.go`
- Modify: `internal/services/scheduledtasks/model.go`
- Test: `internal/services/scheduledtasks/service_test.go`

**Step 1: 先写失败测试**

覆盖创建任务、编辑任务、启停、删除软删、列表查询、汇总统计、运行记录列表查询的核心行为，并验证输入只接受 AI 助手而不再接收模型侧配置。

**Step 2: 实现最小服务代码**

补齐数据库读写、输入校验、软删除、状态与时间字段更新逻辑，清理任务 DTO/Model 中已移除字段。

**Step 3: 跑测试确认通过**

Run: `go test ./internal/services/scheduledtasks -count=1`
Expected: PASS

### Task 3: 接入调度器与执行链路

**Files:**
- Modify: `internal/services/scheduledtasks/scheduler.go`
- Modify: `internal/services/scheduledtasks/conversation_runner.go`
- Modify: `internal/services/scheduledtasks/service.go`
- Modify: `internal/bootstrap/app.go`

**Step 1: 先写失败测试**

覆盖应用启动恢复已启用任务、手动立即执行创建 run 记录、成功/失败时任务状态回写。

**Step 2: 实现最小执行链路**

复用 `conversations` 和 `chat` 服务，执行时按 `agent_id` 读取当前助手配置创建新会话、发送首条用户消息、回填运行记录，并注册/取消本地调度。

**Step 3: 跑测试确认通过**

Run: `go test ./internal/services/scheduledtasks -count=1`
Expected: PASS

### Task 4: 接入前端页面与导航

**Files:**
- Modify: `frontend/src/stores/navigation.ts`
- Modify: `frontend/src/components/layout/SideNav.vue`
- Modify: `frontend/src/App.vue`
- Modify: `frontend/src/locales/zh-CN.ts`
- Modify: `frontend/src/locales/en-US.ts`
- Create: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Create: `frontend/src/pages/scheduled-tasks/types.ts`
- Create: `frontend/src/pages/scheduled-tasks/constants.ts`
- Create: `frontend/src/pages/scheduled-tasks/utils.ts`
- Create: `frontend/src/pages/scheduled-tasks/composables/useScheduledTasks.ts`
- Create: `frontend/src/pages/scheduled-tasks/components/TaskSummaryCards.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/CreateTaskDialog.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/TaskRunHistoryDialog.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/TaskRunConversationPreview.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/TaskRunStatusBadge.vue`

**Step 1: 先补前端类型和绑定缺口**

确认 `@bindings` 可直接调用新服务；如需生成类型，走现有前端构建流程。

**Step 2: 实现页面和弹窗**

完成导航入口、任务列表、创建/编辑弹窗、启停与立即运行操作、历史记录弹窗及右侧会话明细只读预览；弹窗中移除对话模式、模型供应商、模型、知识库、思考模式配置项，只保留 AI 助手选择。

**Step 3: 跑前端校验**

Run: `npm run build`
Workdir: `frontend`
Expected: PASS

### Task 5: 完成联调与验收

**Files:**
- Modify: `internal/bootstrap/app.go`
- Modify: `frontend/src/pages/scheduled-tasks/*`
- Modify: `internal/services/scheduledtasks/*`

**Step 1: 运行后端测试**

Run: `go test ./...`
Expected: PASS，或仅出现与本功能无关的既有失败

**Step 2: 运行前端构建**

Run: `npm run build`
Workdir: `frontend`
Expected: PASS

**Step 3: 手工核对关键流程**

核对创建、编辑、启停、立即执行、查看运行记录、查看会话预览、删除后历史仍可查看。
