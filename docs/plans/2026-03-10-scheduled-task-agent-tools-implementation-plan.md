# 定时任务 Agent Tools Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为本项目聊天 Agent 增加定时任务管理 tools，支持查询、预览创建、确认创建、删除、启用、停用计划任务，并在创建前按助手名称匹配与确认。

**Architecture:** 继续复用 `internal/services/scheduledtasks` 作为唯一业务落点，在 `internal/eino/tools` 新增任务管理 tools，并在 `internal/services/chat/generation.go` 通过 `extraTools` 注入到每轮 Agent。时间自然语言由模型先解析成结构化调度参数，后端仅负责匹配、校验、确认和执行。

**Tech Stack:** Go, Bun, SQLite, CloudWeGo Eino ADK, Wails

---

### Task 1: 补充助手匹配 DTO 与查询能力

**Files:**
- Modify: `internal/services/agents/model.go`
- Modify: `internal/services/agents/service.go`
- Create: `internal/services/agents/service_test.go`

**Step 1: Write the failing test**

在 `internal/services/agents/service_test.go` 新增测试，覆盖：

- `ListAgentsForMatching` 返回最小字段集合
- `MatchAgentsByName("销售助手")` 命中唯一助手
- `MatchAgentsByName("日报")` 命中多个助手
- `MatchAgentsByName("不存在")` 返回空结果

测试示例骨架：

```go
func TestMatchAgentsByName(t *testing.T) {
    db := newTestDB(t)
    seedAgent(t, db, 1, "销售助手")
    seedAgent(t, db, 2, "销售日报助手")

    svc := NewAgentsService(nil)
    // 注入测试 DB 的方式按现有 service 结构补一个 test helper 或独立查询函数

    matches, status, err := svc.MatchAgentsByName("销售助手")
    if err != nil {
        t.Fatalf("MatchAgentsByName returned error: %v", err)
    }
    if status != "exact" {
        t.Fatalf("unexpected status: %s", status)
    }
    if len(matches) != 1 || matches[0].ID != 1 {
        t.Fatalf("unexpected matches: %+v", matches)
    }
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/services/agents -run MatchAgentsByName -v
```

Expected:

- FAIL
- 缺少测试文件、缺少匹配方法或缺少最小 DTO

**Step 3: Write minimal implementation**

在 `internal/services/agents/model.go` 增加最小匹配 DTO，例如：

```go
type AgentMatch struct {
    ID   int64  `json:"id"`
    Name string `json:"name"`
}
```

在 `internal/services/agents/service.go` 增加：

- 只返回 `id + name` 的读取方法
- 按名称匹配并输出 `exact / single / multiple / none` 的方法

第一版最小逻辑：

- 完全相等优先
- 忽略大小写与前后空格
- 包含式匹配作为兜底

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/services/agents -run MatchAgentsByName -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/services/agents/model.go internal/services/agents/service.go internal/services/agents/service_test.go
git commit -m "feat: add agent name matching helpers"
```

### Task 2: 为定时任务 service 增加按 ID 和名称查找能力

**Files:**
- Modify: `internal/services/scheduledtasks/service.go`
- Modify: `internal/services/scheduledtasks/service_test.go`

**Step 1: Write the failing test**

在 `internal/services/scheduledtasks/service_test.go` 增加测试，覆盖：

- `GetScheduledTaskByID` 返回指定任务
- `FindScheduledTasksByName("日报")` 返回唯一或多个候选
- 被软删除的任务不会出现在按名称查找结果里

测试示例骨架：

```go
func TestFindScheduledTasksByName(t *testing.T) {
    db := newTestDB(t)
    seedAgent(t, db, 1, "openai", "gpt-5")
    svc := NewScheduledTasksServiceForTest(nil, db, &stubConversationService{}, stubChatService{})

    _, _ = svc.CreateScheduledTask(CreateScheduledTaskInput{...})
    _, _ = svc.CreateScheduledTask(CreateScheduledTaskInput{...})

    tasks, err := svc.FindScheduledTasksByName("日报")
    if err != nil {
        t.Fatalf("FindScheduledTasksByName returned error: %v", err)
    }
    if len(tasks) != 2 {
        t.Fatalf("unexpected tasks: %+v", tasks)
    }
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/services/scheduledtasks -run 'GetScheduledTaskByID|FindScheduledTasksByName' -v
```

Expected:

- FAIL
- 缺少查找方法

**Step 3: Write minimal implementation**

在 `internal/services/scheduledtasks/service.go` 增加：

- `GetScheduledTaskByID(id int64) (*ScheduledTask, error)`
- `FindScheduledTasksByName(name string) ([]ScheduledTask, error)`

要求：

- 只查询 `deleted_at IS NULL`
- 保持与现有 DTO 一致
- 名称查找支持完全匹配优先，必要时包含式兜底

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/services/scheduledtasks -run 'GetScheduledTaskByID|FindScheduledTasksByName' -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/services/scheduledtasks/service.go internal/services/scheduledtasks/service_test.go
git commit -m "feat: add scheduled task lookup helpers"
```

### Task 3: 搭建任务管理 tools 基础结构

**Files:**
- Create: `internal/eino/tools/scheduled_task_management.go`
- Create: `internal/eino/tools/scheduled_task_management_test.go`

**Step 1: Write the failing test**

在 `internal/eino/tools/scheduled_task_management_test.go` 先写最小 smoke test，覆盖：

- 能成功创建 `scheduled_task_list` tool
- `Info()` 返回正确工具名
- `scheduled_task_create_preview` 的参数 schema 包含 `agent_name / schedule_type / schedule_value / cron_expr`

测试示例骨架：

```go
func TestScheduledTaskCreatePreviewToolInfo(t *testing.T) {
    tool := &scheduledTaskCreatePreviewTool{}
    info, err := tool.Info(context.Background())
    if err != nil {
        t.Fatalf("Info returned error: %v", err)
    }
    if info.Name != "scheduled_task_create_preview" {
        t.Fatalf("unexpected tool name: %s", info.Name)
    }
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/eino/tools -run ScheduledTaskCreatePreviewToolInfo -v
```

Expected:

- FAIL
- 缺少文件或 tool 定义

**Step 3: Write minimal implementation**

在 `internal/eino/tools/scheduled_task_management.go` 建立：

- 依赖配置结构，例如 `ScheduledTaskManagementConfig`
- 7 个 tool 的结构体定义
- 每个 tool 的 `Info()`

此时只需要把：

- tool 名称
- 描述
- 参数 schema

定义完整，`InvokableRun()` 先返回占位错误也可以。

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/eino/tools -run ScheduledTaskCreatePreviewToolInfo -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/eino/tools/scheduled_task_management.go internal/eino/tools/scheduled_task_management_test.go
git commit -m "feat: scaffold scheduled task management tools"
```

### Task 4: 实现只读类 tool

**Files:**
- Modify: `internal/eino/tools/scheduled_task_management.go`
- Modify: `internal/eino/tools/scheduled_task_management_test.go`

**Step 1: Write the failing test**

补充测试，覆盖：

- `scheduled_task_list` 返回任务列表
- `agent_match_by_name` 返回 `exact / multiple / none`

测试示例骨架：

```go
func TestAgentMatchByNameTool(t *testing.T) {
    tool := newTestAgentMatchTool(t)
    result, err := tool.InvokableRun(context.Background(), `{"query":"销售助手"}`)
    if err != nil {
        t.Fatalf("InvokableRun returned error: %v", err)
    }
    if !strings.Contains(result, `"match_status":"exact"`) {
        t.Fatalf("unexpected result: %s", result)
    }
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/eino/tools -run 'ScheduledTaskListTool|AgentMatchByNameTool' -v
```

Expected:

- FAIL
- 返回结果不完整或未接入 service

**Step 3: Write minimal implementation**

在 `internal/eino/tools/scheduled_task_management.go` 中实现：

- `scheduled_task_list`
- `agent_match_by_name`

要求：

- 输入 JSON 解码严格
- 输出统一为结构化 JSON 字符串
- 名称匹配结果包含状态与候选项

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/eino/tools -run 'ScheduledTaskListTool|AgentMatchByNameTool' -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/eino/tools/scheduled_task_management.go internal/eino/tools/scheduled_task_management_test.go
git commit -m "feat: add task list and agent match tools"
```

### Task 5: 实现创建预览 tool

**Files:**
- Modify: `internal/eino/tools/scheduled_task_management.go`
- Modify: `internal/eino/tools/scheduled_task_management_test.go`

**Step 1: Write the failing test**

补充测试，覆盖：

- 助手唯一命中时，`scheduled_task_create_preview` 返回 `needs_confirmation=true`
- 助手多重命中时，返回 `issues` 和候选助手，不允许进入确认
- 非法调度参数时返回校验失败

测试示例骨架：

```go
func TestScheduledTaskCreatePreviewTool(t *testing.T) {
    tool := newTestCreatePreviewTool(t)
    result, err := tool.InvokableRun(context.Background(), `{
        "name":"销售日报",
        "prompt":"总结昨日新增线索",
        "agent_name":"销售助手",
        "schedule_type":"preset",
        "schedule_value":"every_day_0900",
        "cron_expr":"",
        "enabled":true
    }`)
    if err != nil {
        t.Fatalf("InvokableRun returned error: %v", err)
    }
    if !strings.Contains(result, `"needs_confirmation":true`) {
        t.Fatalf("unexpected result: %s", result)
    }
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/eino/tools -run ScheduledTaskCreatePreviewTool -v
```

Expected:

- FAIL

**Step 3: Write minimal implementation**

实现 `scheduled_task_create_preview`：

- 调用助手匹配逻辑
- 通过 `ScheduledTasksService` 的调度校验路径验证 `schedule_type / schedule_value / cron_expr`
- 生成标准化确认摘要
- 不落库

确认摘要至少包含：

- 任务名称
- 助手名称
- 执行时间
- prompt
- 启用状态

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/eino/tools -run ScheduledTaskCreatePreviewTool -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/eino/tools/scheduled_task_management.go internal/eino/tools/scheduled_task_management_test.go
git commit -m "feat: add scheduled task create preview tool"
```

### Task 6: 实现创建确认 tool

**Files:**
- Modify: `internal/eino/tools/scheduled_task_management.go`
- Modify: `internal/eino/tools/scheduled_task_management_test.go`

**Step 1: Write the failing test**

补充测试，覆盖：

- `scheduled_task_create_confirm` 成功创建任务
- 缺少 `agent_id` 时拒绝执行
- 非法调度字段时拒绝执行

测试示例骨架：

```go
func TestScheduledTaskCreateConfirmTool(t *testing.T) {
    tool := newTestCreateConfirmTool(t)
    result, err := tool.InvokableRun(context.Background(), `{
        "name":"销售日报",
        "prompt":"总结昨日新增线索",
        "agent_id":1,
        "schedule_type":"preset",
        "schedule_value":"every_day_0900",
        "cron_expr":"",
        "enabled":true
    }`)
    if err != nil {
        t.Fatalf("InvokableRun returned error: %v", err)
    }
    if !strings.Contains(result, `"action":"created"`) {
        t.Fatalf("unexpected result: %s", result)
    }
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/eino/tools -run ScheduledTaskCreateConfirmTool -v
```

Expected:

- FAIL

**Step 3: Write minimal implementation**

实现 `scheduled_task_create_confirm`：

- 解析 JSON 入参
- 重新校验调度参数
- 调用 `ScheduledTasksService.CreateScheduledTask`
- 输出创建后的任务结果

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/eino/tools -run ScheduledTaskCreateConfirmTool -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/eino/tools/scheduled_task_management.go internal/eino/tools/scheduled_task_management_test.go
git commit -m "feat: add scheduled task create confirm tool"
```

### Task 7: 实现删除与启停 tools

**Files:**
- Modify: `internal/eino/tools/scheduled_task_management.go`
- Modify: `internal/eino/tools/scheduled_task_management_test.go`

**Step 1: Write the failing test**

补充测试，覆盖：

- `scheduled_task_delete(confirm=false)` 返回待确认摘要
- `scheduled_task_delete(confirm=true)` 真正删除任务
- `scheduled_task_enable/disable` 在 `confirm=false` 时只预览
- `scheduled_task_enable/disable` 在 `confirm=true` 时修改状态
- 名称匹配多条时阻止执行

测试示例骨架：

```go
func TestScheduledTaskDisableTool(t *testing.T) {
    tool := newTestDisableTool(t)
    preview, err := tool.InvokableRun(context.Background(), `{"task_name":"销售日报","confirm":false}`)
    if err != nil {
        t.Fatalf("preview returned error: %v", err)
    }
    if !strings.Contains(preview, `"needs_confirmation":true`) {
        t.Fatalf("unexpected preview: %s", preview)
    }
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/eino/tools -run 'ScheduledTaskDeleteTool|ScheduledTaskEnableTool|ScheduledTaskDisableTool' -v
```

Expected:

- FAIL

**Step 3: Write minimal implementation**

实现：

- `scheduled_task_delete`
- `scheduled_task_enable`
- `scheduled_task_disable`

统一规则：

- `confirm=false` 仅返回预览
- `confirm=true` 才执行
- 名称不唯一时输出候选列表

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/eino/tools -run 'ScheduledTaskDeleteTool|ScheduledTaskEnableTool|ScheduledTaskDisableTool' -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/eino/tools/scheduled_task_management.go internal/eino/tools/scheduled_task_management_test.go
git commit -m "feat: add scheduled task mutation tools"
```

### Task 8: 在聊天生成链路注入 task tools

**Files:**
- Modify: `internal/services/chat/generation.go`
- Create: `internal/services/chat/generation_task_tools_test.go`

**Step 1: Write the failing test**

新增测试，覆盖：

- `buildExtras(...)` 会把 task management tools 注入 `extraTools`
- 当依赖 service 可用时，返回至少 7 个任务管理相关 tools

测试示例骨架：

```go
func TestBuildExtrasIncludesScheduledTaskTools(t *testing.T) {
    svc := NewChatService(nil)
    gc := &generationContext{service: svc, db: newTestDB(t)}

    tools, _ := svc.buildExtras(context.Background(), gc)

    names := toolNames(t, tools)
    requireContains(t, names, "scheduled_task_list")
    requireContains(t, names, "scheduled_task_create_preview")
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/services/chat -run BuildExtrasIncludesScheduledTaskTools -v
```

Expected:

- FAIL
- 未注入任务 tools

**Step 3: Write minimal implementation**

在 `internal/services/chat/generation.go` 的 `buildExtras(...)` 中：

- 创建 `ScheduledTasksService`
- 创建 `AgentsService`
- 调用新的 task tool 构造函数
- append 到 `extraTools`

要求：

- 保持现有 library/memory/skills tool 注入逻辑不变
- task tools 默认总是注入

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/services/chat -run BuildExtrasIncludesScheduledTaskTools -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/services/chat/generation.go internal/services/chat/generation_task_tools_test.go
git commit -m "feat: inject scheduled task tools into chat agent"
```

### Task 9: 端到端验证任务工具行为

**Files:**
- Modify: `internal/eino/tools/scheduled_task_management_test.go`
- Modify: `internal/services/chat/generation_task_tools_test.go`

**Step 1: Write the failing test**

补充更贴近真实流程的测试，覆盖：

- 模型先 preview，再 confirm 创建任务
- 随后通过 `scheduled_task_list` 能看到该任务
- 再通过 `scheduled_task_disable(confirm=true)` 停用
- 再通过 `scheduled_task_enable(confirm=true)` 启用
- 最后通过 `scheduled_task_delete(confirm=true)` 删除

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/eino/tools ./internal/services/chat -run 'ScheduledTask.*Flow|BuildExtrasIncludesScheduledTaskTools' -v
```

Expected:

- FAIL
- 某个阶段结果与预期不一致

**Step 3: Write minimal implementation**

根据失败点补齐：

- tool 返回字段一致性
- 预览与确认分支的状态文案
- 启停与删除后的列表同步

保持 YAGNI，不新增额外状态缓存。

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/eino/tools ./internal/services/chat -run 'ScheduledTask.*Flow|BuildExtrasIncludesScheduledTaskTools' -v
```

Expected:

- PASS

**Step 5: Commit**

```bash
git add internal/eino/tools/scheduled_task_management_test.go internal/services/chat/generation_task_tools_test.go
git commit -m "test: cover scheduled task tool flows"
```

### Task 10: 全量回归验证

**Files:**
- Modify: `internal/services/agents/service_test.go`
- Modify: `internal/services/scheduledtasks/service_test.go`
- Modify: `internal/eino/tools/scheduled_task_management_test.go`
- Modify: `internal/services/chat/generation_task_tools_test.go`

**Step 1: Run focused package tests**

Run:

```bash
go test ./internal/services/agents ./internal/services/scheduledtasks ./internal/eino/tools ./internal/services/chat -v
```

Expected:

- 全部 PASS

**Step 2: Fix regressions if any**

如果失败，仅修复：

- 名称匹配逻辑
- tool JSON 返回结构
- task tool 注入逻辑

不要顺手改 unrelated 代码。

**Step 3: Run focused package tests again**

Run:

```bash
go test ./internal/services/agents ./internal/services/scheduledtasks ./internal/eino/tools ./internal/services/chat -v
```

Expected:

- 全部 PASS

**Step 4: Optional broader verification**

Run:

```bash
go test ./... 
```

Expected:

- 若仓库现有用例稳定，则全部 PASS
- 若有历史不稳定测试，记录未通过项，不在本任务中扩散修复

**Step 5: Commit**

```bash
git add internal/services/agents/service_test.go internal/services/scheduledtasks/service_test.go internal/eino/tools/scheduled_task_management_test.go internal/services/chat/generation_task_tools_test.go
git commit -m "test: verify scheduled task agent tool integration"
```
