# Scheduled Task Expired Status Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Stop writing operation logs on scheduled-task creation and persist expired tasks as `last_status = expired` while keeping their enable switch on and preventing future execution.

**Architecture:** Keep the change local to the scheduled-task module by extending the existing `last_status` enum, updating the expiration sweeper and schedule-registration flow to mark expired tasks instead of disabling them, and leaving operation-log behavior unchanged for update/delete/toggle actions. Add a small frontend status-mapping update so the task table renders the new expired state without changing existing dialog behavior.

**Tech Stack:** Go, Bun, SQLite, Vue 3, TypeScript, Vitest

---

### Task 1: Lock backend expiration and operation-log behavior with tests

**Files:**
- Modify: `internal/services/scheduledtasks/service_expiration_test.go`
- Modify: `internal/services/scheduledtasks/service_operation_log_test.go`

**Step 1: Write the failing tests**

Add backend tests that assert:
- creating a task does not create an operation log record
- the expiration sweeper marks enabled expired tasks as `last_status = expired`
- expired tasks keep `enabled = true`
- expired tasks clear `next_run_at`

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: FAIL because create still writes logs and expiration still disables tasks.

**Step 3: Write minimal implementation**

Update the scheduled-task service to stop inserting create logs and to mark expired tasks instead of disabling them.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: PASS

### Task 2: Implement backend expired-status flow

**Files:**
- Modify: `internal/services/scheduledtasks/dto.go`
- Modify: `internal/services/scheduledtasks/service.go`

**Step 1: Add the expired status constant**

Introduce `TaskStatusExpired` and keep all task-status string literals behind constants.

**Step 2: Centralize expiration marking**

Add a helper that:
- sets `last_status = expired`
- clears `next_run_at`
- leaves `enabled` unchanged
- unregisters the task from the in-memory scheduler when a row changes

**Step 3: Apply the helper in all expiration paths**

Use the helper from:
- expiration sweeper
- startup reload
- scheduled trigger guard
- update path when a task is already expired

**Step 4: Keep enable semantics intact**

Continue rejecting attempts to re-enable an expired disabled task, but do not auto-flip enabled to false when expiration is detected.

### Task 3: Render expired state in the task list

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`
- Modify: `frontend/src/pages/scheduled-tasks/types.ts`
- Modify: `frontend/src/locales/zh-CN.ts`

**Step 1: Write the failing frontend test or minimal render assertion**

If scheduled-task component tests are available, add a focused test for expired status label rendering. Otherwise keep the change limited and verify through build.

**Step 2: Update the view model**

Make the task table prefer `last_status === 'expired'` over the enabled flag when rendering the status label and text style.

**Step 3: Add locale text**

Expose the expired label through i18n instead of hardcoding.

**Step 4: Verify frontend compilation**

Run: `pnpm test -- scheduled-tasks` or `pnpm build`
Expected: PASS for the available verification command.

### Task 4: Final verification

**Files:**
- Modify: `.codex/docs/2026-03-24-scheduled-task-expired-status-plan.md`

**Step 1: Run backend verification**

Run: `go test ./internal/services/scheduledtasks/...`
Expected: PASS

**Step 2: Run frontend verification**

Run the narrowest available scheduled-task frontend verification command.

**Step 3: Summarize evidence**

Report exactly which commands passed and call out any remaining gaps.
