# Scheduled Task Operation Log Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add scheduled task operation logging with manual versus AI source tracking, multi-field change summaries, and a read-only historical task detail view backed by stored snapshots.

**Architecture:** Add a dedicated operation-log persistence layer in SQLite, write logs from the scheduled-task service for every create/update/delete/enable/disable action, and expose list/detail APIs to the frontend. Reuse the scheduled-task form UI in a read-only mode for historical snapshots so the detail view matches the live editor without duplicating rendering logic.

**Tech Stack:** Go, Bun ORM, SQLite migrations, Wails bindings, Vue 3, TypeScript

---

### Task 1: Add failing backend coverage for operation logs

**Files:**
- Modify: `internal/services/scheduledtasks/service_notification_test.go`
- Create: `internal/services/scheduledtasks/service_operation_log_test.go`

**Step 1: Write the failing tests**

Cover:

- creating a task writes one operation log with source and snapshot
- updating multiple auditable fields writes one log with ordered changed-field items
- enabling/disabling writes one update log with a `状态` diff
- deleting a task writes one delete log with the pre-delete snapshot
- list/detail APIs return display-ready payloads

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: FAIL because operation-log storage and APIs do not exist yet.

**Step 3: Write minimal implementation**

Add only the backend code needed to support the new tests.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: PASS

### Task 2: Add backend storage and DTO support

**Files:**
- Modify: `internal/services/scheduledtasks/dto.go`
- Modify: `internal/services/scheduledtasks/model.go`
- Create: `internal/sqlite/migrations/202603191500_create_scheduled_task_operation_logs_table.go`

**Step 1: Add DTOs**

Define:

- operation type constants
- operation source constants
- changed-field item DTO
- operation log list/detail DTOs
- task snapshot DTO if needed

**Step 2: Add Bun model**

Create the model for `scheduled_task_operation_logs` with JSON-backed fields for changed items and snapshots.

**Step 3: Add migration**

Create the new table with indexes suitable for task and time ordered reads.

**Step 4: Run focused backend tests**

Run: `go test ./internal/services/scheduledtasks/...`

### Task 3: Implement service-layer logging and APIs

**Files:**
- Modify: `internal/services/scheduledtasks/service.go`
- Modify: `internal/bootstrap/scheduled_task_tools.go`

**Step 1: Add operation source plumbing**

Extend scheduled-task service entrypoints so callers can pass `manual` or `ai`.

**Step 2: Implement diff helpers**

Create helpers that:

- compare auditable fields
- format display-ready before/after values
- build ordered `changed_fields_json`
- build full `task_snapshot_json`

**Step 3: Log create/update/delete/enable-disable operations**

Write exactly one log record per user-visible operation.

**Step 4: Add list/detail service methods**

Implement backend reads for operation-log list and detail.

**Step 5: Update tool path**

Pass source `ai` through the assistant scheduled-task tool integration.

**Step 6: Run focused backend tests**

Run: `go test ./internal/services/scheduledtasks/...`

### Task 4: Add failing frontend coverage for operation log presentation

**Files:**
- Create: `frontend/src/pages/scheduled-tasks/components/OperationLogListDialog.test.ts`
- Modify: `frontend/src/pages/scheduled-tasks/utils.test.ts`

**Step 1: Write the failing tests**

Cover:

- operation log rows render multi-line changed fields, before, and after values in order
- task snapshots can hydrate read-only form state
- top action bar includes the operation log button

**Step 2: Run test to verify it fails**

Run: `pnpm test -- scheduled-tasks`
Expected: FAIL because operation-log UI and snapshot hydration do not exist yet.

**Step 3: Write minimal implementation**

Add only the code needed to satisfy the expected UI behavior.

**Step 4: Run test to verify it passes**

Run: `pnpm test -- scheduled-tasks`
Expected: PASS

### Task 5: Implement frontend operation-log list and read-only detail

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Modify: `frontend/src/pages/scheduled-tasks/composables/useScheduledTasks.ts`
- Modify: `frontend/src/pages/scheduled-tasks/types.ts`
- Modify: `frontend/src/pages/scheduled-tasks/utils.ts`
- Modify: `frontend/src/pages/scheduled-tasks/components/CreateTaskDialog.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/OperationLogListDialog.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/OperationLogDetailDialog.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`

**Step 1: Extract reusable form content**

Move the scheduled-task form fields into a shared component that supports editable and read-only modes.

**Step 2: Load operation-log data**

Add composable support for opening the log list and fetching detail payloads.

**Step 3: Add operation-log entry point**

Insert the new `Operation Log` button to the right of `Refresh` with matching button styling.

**Step 4: Build the log list UI**

Render one row per operation with multi-line cells for changed fields, before, and after values.

**Step 5: Build the read-only detail UI**

Open a dialog that renders the historical task snapshot through the shared form component with all controls disabled.

**Step 6: Pass manual source on UI writes**

Ensure create/update/delete/enable-disable requests from the page identify themselves as manual operations.

**Step 7: Run focused frontend tests**

Run: `pnpm test -- scheduled-tasks`

### Task 6: Verify end-to-end behavior

**Files:**
- Modify: any touched files above as needed

**Step 1: Run backend verification**

Run: `go test ./internal/services/scheduledtasks/...`

**Step 2: Run frontend verification**

Run: `pnpm test -- scheduled-tasks`

**Step 3: Run type verification**

Run: `pnpm exec vue-tsc --noEmit`

**Step 4: Run targeted app smoke check if available**

Run the scheduled-task page and verify:

- the operation log button appears
- a manual edit creates a visible log row
- viewing a deleted-task log still shows the historical snapshot

**Step 5: Summarize any residual gaps**

Document any unverified UI flows or follow-up cleanup.
