# Scheduled Task Expiration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an optional scheduled-task expiration date that auto-disables expired enabled tasks, blocks re-enabling expired tasks, and exposes expiration info to the task UI without adding a new task status.

**Architecture:** Extend scheduled task persistence with an optional `expires_at` timestamp stored as end-of-day local time, then centralize expiration checks inside the scheduled-task service so create, update, startup reload, manual enable, scheduled execution, and a minute-level sweeper all follow the same rules. Surface the field and expired flag through DTOs so the frontend can render an optional date picker and prevent toggling expired tasks back on while still showing the task as disabled.

**Tech Stack:** Go, Bun, SQLite migrations, Vue 3, Wails bindings, Vitest

---

### Task 1: Add backend expiration coverage first

**Files:**
- Modify: `internal/services/scheduledtasks/service_notification_test.go`
- Create: `internal/services/scheduledtasks/service_expiration_test.go`

**Step 1: Write the failing test**

Add tests for:
- creating a task with `expires_at` persists it
- enabling an expired task returns an expiration error and keeps it disabled
- minute sweeper disables expired enabled tasks
- expired tasks are not re-registered on startup reload

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: FAIL because expiration fields and sweeper logic do not exist yet

**Step 3: Write minimal implementation**

Add the smallest service-side code needed for expiration parsing, validation, and automatic disabling.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: PASS for scheduled task service tests

### Task 2: Add backend model, DTO, and migration support

**Files:**
- Modify: `internal/services/scheduledtasks/dto.go`
- Modify: `internal/services/scheduledtasks/model.go`
- Modify: `internal/services/scheduledtasks/service.go`
- Create: `internal/sqlite/migrations/202603191830_add_expires_at_to_scheduled_tasks.go`
- Modify: `internal/services/i18n/locales/zh-CN.json`
- Modify: `internal/services/i18n/locales/en-US.json`

**Step 1: Write the failing test**

Extend the backend tests to assert:
- DTOs include `expires_at` and `is_expired`
- operation snapshots keep expiration values when relevant
- expired enable attempts return a dedicated localized error

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: FAIL because schema and DTOs do not expose expiration yet

**Step 3: Write minimal implementation**

Add the optional field, migrate the table, compute `is_expired`, and centralize expiration checks in create/update/reload/enable/run paths plus the once-per-minute sweeper.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: PASS

### Task 3: Add frontend form and toggle behavior coverage first

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/utils.test.ts`
- Create: `frontend/src/pages/scheduled-tasks/components/CreateTaskDialog.test.ts`
- Create: `frontend/src/pages/scheduled-tasks/composables/useScheduledTasks.test.ts`

**Step 1: Write the failing test**

Add tests for:
- form state round-trips the optional expiration date
- payload builder sends end-of-day expiration timestamp
- expired tasks still render as disabled but block toggle-on with the expired toast

**Step 2: Run test to verify it fails**

Run: `pnpm test -- scheduled-tasks`
Expected: FAIL because the form and composable do not know about expiration yet

**Step 3: Write minimal implementation**

Update the form model, payload builder, and toggle guard to support expiration.

**Step 4: Run test to verify it passes**

Run: `pnpm test -- scheduled-tasks`
Expected: PASS for scheduled task frontend tests

### Task 4: Add frontend expiration UI

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/types.ts`
- Modify: `frontend/src/pages/scheduled-tasks/utils.ts`
- Modify: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Modify: `frontend/src/pages/scheduled-tasks/composables/useScheduledTasks.ts`
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`
- Modify: `frontend/src/locales/zh-CN.ts`
- Modify: `frontend/src/locales/en-US.ts`

**Step 1: Write the failing test**

Add assertions for:
- the create/edit dialog shows an optional expiration date field
- expired tasks show expiration hints in the form/table
- toggling an expired task on shows the “已过期” message and does not call the backend

**Step 2: Run test to verify it fails**

Run: `pnpm test -- scheduled-tasks`
Expected: FAIL until the UI is updated

**Step 3: Write minimal implementation**

Render the optional expiration date picker, show expired hints, and keep the table status display aligned with disabled state.

**Step 4: Run test to verify it passes**

Run: `pnpm test -- scheduled-tasks`
Expected: PASS

### Task 5: Verify end to end

**Files:**
- Modify: `docs/plans/2026-03-19-scheduled-task-expiration.md`

**Step 1: Run backend verification**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: PASS

**Step 2: Run frontend verification**

Run: `pnpm test -- scheduled-tasks`
Expected: PASS

**Step 3: Summarize any remaining risks**

Document any residual gaps, especially around timezone assumptions and existing records with null expiration.
