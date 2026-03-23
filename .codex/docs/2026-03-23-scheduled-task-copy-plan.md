# Scheduled Task Copy Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a copy action to the scheduled task list so users can open the create dialog with the selected task prefilled and the task name suffixed with "副本".

**Architecture:** Reuse the existing scheduled task page state instead of introducing a second dialog flow. Build a pure form-cloning helper from the existing `taskToForm` conversion so the copy behavior stays deterministic and easy to verify.

**Tech Stack:** Vue 3, TypeScript, vue-i18n, Wails bindings

---

### Task 1: Add copy-form transformation

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/constants.ts`
- Modify: `frontend/src/pages/scheduled-tasks/utils.ts`

**Step 1: Write the failing test**

The repo currently has no runnable frontend unit-test setup, so use a pure helper function with manual verification as the smallest safe substitute for this UI-only change.

**Step 2: Run test to verify it fails**

Not applicable until frontend unit-test infrastructure exists.

**Step 3: Write minimal implementation**

Add a shared copy suffix constant and a helper that converts a task into a create-form payload with `id = null` and `name += 副本`.

**Step 4: Run test to verify it passes**

Verify the helper through type-check/build validation after wiring it into the page.

**Step 5: Commit**

Skip commit in this session unless requested.

### Task 2: Wire the copy action into the page and menu

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`
- Modify: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Modify: `frontend/src/locales/zh-CN.ts`

**Step 1: Write the failing test**

No existing UI test harness is available, so use the helper-first approach and then verify through build/type-check plus code-path review.

**Step 2: Run test to verify it fails**

Not applicable until UI test infrastructure exists.

**Step 3: Write minimal implementation**

Expose a `copy` event from the table menu, handle it in the page by resetting edit state, filling the form with the copied payload, and opening the existing create dialog.

**Step 4: Run test to verify it passes**

Run frontend type-check/build to confirm the new event and helper integrate cleanly.

**Step 5: Commit**

Skip commit in this session unless requested.
