# Scheduled Task Expiration Timezone Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unify scheduled task expiration storage, display, and expiration checks around the task timezone so the same `expires_at` instant renders and expires consistently.

**Architecture:** Keep `expires_at` stored in UTC in the database, but treat it as the UTC serialization of a task-timezone expiration instant. Frontend date input/output will convert through the task `timezone`, and backend expiration normalization/checks will use the same timezone semantics for persisted timestamps and comparisons.

**Tech Stack:** Vue 3, TypeScript, Go, Bun, node:test, go test

---

### Task 1: Lock frontend timezone behavior with failing tests

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/__tests__/utils.test.mjs`
- Modify: `frontend/src/pages/scheduled-tasks/expirationDate.test.ts`

**Steps:**
1. Add a failing test for `toDateInputValue` so `2026-03-23T23:59:59Z` stays `2026-03-23` when timezone is `UTC`.
2. Add a failing test for `toDateInputValue` so `2026-03-23T15:59:59Z` becomes `2026-03-23` when timezone is `Asia/Shanghai`.
3. Add a failing test for `buildExpirationDateTime` so a selected calendar date serializes to the correct UTC instant for the provided timezone.
4. Run the frontend test files and confirm the new assertions fail for the current implementation.

### Task 2: Implement frontend timezone-aware expiration helpers

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/utils.ts`
- Modify: `frontend/src/pages/scheduled-tasks/types.ts`
- Modify: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`

**Steps:**
1. Add timezone-aware helper constants and conversion helpers in `utils.ts`.
2. Extend form state to carry the task timezone for edit/copy flows.
3. Update form hydration and payload construction to convert expiration dates through the task timezone.
4. Update expired-hint logic and table/date formatting to use the same timezone.
5. Re-run the frontend tests and confirm they pass.

### Task 3: Lock backend expiration semantics with failing tests

**Files:**
- Modify: `internal/services/scheduledtasks/service_expiration_test.go`

**Steps:**
1. Add a failing test showing a task in `Asia/Shanghai` with local end-of-day expiration should not expire before that local boundary.
2. Add a failing test showing the same task becomes expired once the task-timezone deadline passes.
3. Run the targeted Go test and confirm at least one new assertion fails.

### Task 4: Implement backend expiration normalization/checking

**Files:**
- Modify: `internal/services/scheduledtasks/service.go`

**Steps:**
1. Add constants and helpers that normalize expiration timestamps against the task timezone.
2. Use the timezone-aware helper in create/update normalization.
3. Use the same helper for expiration checks and expired-task sweeps.
4. Re-run the targeted Go tests and confirm they pass.

### Task 5: Verify the full change set

**Files:**
- No code changes required

**Steps:**
1. Run the affected frontend tests.
2. Run the affected Go tests.
3. Review the diff to make sure only scheduled-task expiration behavior changed.
