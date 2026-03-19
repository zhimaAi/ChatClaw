# Scheduled Task Notification Channels Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add notification platform and multi-channel selection to scheduled tasks so completed task results can be delivered through configured channels from the main Channels menu.

**Architecture:** Extend scheduled task persistence with notification fields, reuse the existing channel list as the source of selectable channels, and send the final assistant reply through the selected channels after a successful scheduled run. Frontend form behavior should mirror the assistant channel selection model: choose a platform first, then choose one or more matching channels.

**Tech Stack:** Vue 3, TypeScript, Wails bindings, Go, Bun ORM, SQLite

---

### Task 1: Add failing backend coverage for notification fields and delivery

**Files:**
- Modify: `internal/services/scheduledtasks/service_test.go`
- Modify: `internal/services/scheduledtasks/conversation_runner_test.go`

**Step 1: Write the failing tests**

Cover:
- create/update/list round-trip for `notification_platform` and `notification_channel_ids`
- scheduled task execution sends the final assistant message to each selected channel
- platform mismatch or unavailable channel is surfaced as a task run error

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: FAIL because notification fields and notification sending do not exist yet.

**Step 3: Write minimal implementation**

Add the smallest backend changes needed for the new task fields and delivery flow.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: PASS

### Task 2: Add backend storage and service support

**Files:**
- Modify: `internal/services/scheduledtasks/dto.go`
- Modify: `internal/services/scheduledtasks/model.go`
- Modify: `internal/services/scheduledtasks/service.go`
- Modify: `internal/services/scheduledtasks/conversation_runner.go`
- Modify: `internal/services/scheduledtasks/scheduler.go`
- Create: `internal/sqlite/migrations/202603191200_add_notification_fields_to_scheduled_tasks.go`
- Modify: `internal/bootstrap/scheduled_task_tools.go`

**Step 1: Update DTO and model definitions**

Add:
- `notification_platform`
- `notification_channel_ids`

**Step 2: Add migration**

Persist the new columns with safe defaults for existing rows.

**Step 3: Update create/update/read paths**

Validate and store the new fields; keep empty values valid when notifications are not configured.

**Step 4: Add task-result notification delivery**

After a successful run completes and the final assistant message is available, send it through each selected channel.

**Step 5: Run focused backend tests**

Run: `go test ./internal/services/scheduledtasks/...`

### Task 3: Add failing frontend coverage for the new form behavior

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/utils.test.ts` or nearest existing scheduled-task tests
- Modify: `frontend/src/pages/scheduled-tasks/components/CreateTaskDialog.test.ts` if present, otherwise add a focused test file

**Step 1: Write the failing tests**

Cover:
- form state includes notification platform and selected channel ids
- channel options filter by the chosen notification platform
- payload builder includes notification fields

**Step 2: Run test to verify it fails**

Run: `pnpm test -- scheduled-tasks`
Expected: FAIL because form state and payload do not support notification fields yet.

**Step 3: Write minimal implementation**

Add only the code needed to satisfy the new behavior.

**Step 4: Run test to verify it passes**

Run: `pnpm test -- scheduled-tasks`
Expected: PASS

### Task 4: Implement frontend notification fields and binding payloads

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/types.ts`
- Modify: `frontend/src/pages/scheduled-tasks/utils.ts`
- Modify: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Modify: `frontend/src/pages/scheduled-tasks/components/CreateTaskDialog.vue`

**Step 1: Extend form state**

Add:
- `notificationPlatform`
- `notificationChannelIds`

**Step 2: Load channel choices**

Reuse `ChannelService.ListChannels()` and filter options by the selected platform.

**Step 3: Update dialog UI**

Add the two new fields and keep the behavior aligned with assistant channel selection.

**Step 4: Update payload conversion**

Include notification fields in create/update requests and edit-form hydration.

**Step 5: Run focused frontend tests**

Run: `pnpm test -- scheduled-tasks`

### Task 5: Verify end-to-end behavior

**Files:**
- Modify: any touched files above as needed

**Step 1: Run backend verification**

Run: `go test ./internal/services/scheduledtasks/...`

**Step 2: Run frontend verification**

Run: `pnpm test -- scheduled-tasks`

**Step 3: Run any required type/build verification if test harness requires it**

Run: `pnpm exec vue-tsc --noEmit`

**Step 4: Summarize final behavior and any residual gaps**
