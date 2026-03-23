# Scheduled Task Operation Log Page Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the scheduled-task top-right operation-log dialog with an in-module page that has a back arrow, while keeping row detail as the existing dialog.

**Architecture:** Keep the change inside the current `scheduled-tasks` module by adding a small page-view state in `ScheduledTasksPage.vue`. Move the operation-log table UI into a page component and reuse the existing detail dialog inside that component.

**Tech Stack:** Vue 3, TypeScript, Pinia-free local page state, Wails bindings, Node 24 built-in test runner, `vue-tsc`

---

### Task 1: Add scheduled-task page-view helpers and test them first

**Files:**
- Create: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\scheduledTasksView.ts`
- Create: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\__tests__\scheduledTasksView.test.ts`

**Step 1: Write the failing test**

Write a small Node test that expects:

- `enter-operation-log` switches `task-list` to `operation-log-list`
- `back-to-task-list` switches `operation-log-list` to `task-list`
- invalid actions keep the current view

**Step 2: Run test to verify it fails**

Run: `node --test --experimental-strip-types frontend/src/pages/scheduled-tasks/__tests__/scheduledTasksView.test.ts`

Expected: FAIL because the helper module does not exist yet.

**Step 3: Write minimal implementation**

Implement:

- page-view constants
- action constants
- a pure `transitionScheduledTasksView` helper

**Step 4: Run test to verify it passes**

Run: `node --test --experimental-strip-types frontend/src/pages/scheduled-tasks/__tests__/scheduledTasksView.test.ts`

Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/pages/scheduled-tasks/scheduledTasksView.ts frontend/src/pages/scheduled-tasks/__tests__/scheduledTasksView.test.ts
git commit -m "test: add scheduled task page view helper"
```

### Task 2: Convert the operation-log dialog body into a page component

**Files:**
- Create: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\OperationLogListPage.vue`
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\OperationLogListDialog.vue`

**Step 1: Write the failing test**

No extra UI test harness exists in this repo, so reuse the Task 1 helper coverage and verify behavior with type-check plus manual flow.

**Step 2: Implement the page component**

Move the current list-loading and table-rendering logic into `OperationLogListPage.vue`:

- load logs on mount
- keep toast error handling
- keep `OperationLogDetailDialog`
- remove dialog wrapper concerns from the page body

**Step 3: Keep compatibility or reduce churn**

Either:

- delete `OperationLogListDialog.vue` if unused after migration, or
- leave it as a thin wrapper around the new page body if that lowers risk

**Step 4: Run type-check**

Run: `pnpm --dir frontend exec vue-tsc`

Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/pages/scheduled-tasks/components/OperationLogListPage.vue frontend/src/pages/scheduled-tasks/components/OperationLogListDialog.vue
git commit -m "refactor: extract scheduled task operation log page"
```

### Task 3: Switch scheduled-task page from dialog flow to page-view flow

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\ScheduledTasksPage.vue`
- Modify if needed: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\types.ts`

**Step 1: Implement local page state**

Use the helper from Task 1 to manage:

- `task-list`
- `operation-log-list`

**Step 2: Replace the button behavior**

Change the top-right `操作记录` button so it transitions to `operation-log-list` instead of opening a dialog.

**Step 3: Add the operation-log page header**

Render:

- top-left back arrow button
- page title `操作记录`

The back arrow transitions back to `task-list`.

**Step 4: Render the correct body**

- `task-list` shows the existing summary cards and task table
- `operation-log-list` shows `OperationLogListPage`

**Step 5: Remove obsolete dialog state**

Remove:

- `operationLogOpen`
- direct usage of `OperationLogListDialog`

**Step 6: Run type-check**

Run: `pnpm --dir frontend exec vue-tsc`

Expected: PASS

**Step 7: Commit**

```bash
git add frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue frontend/src/pages/scheduled-tasks/types.ts
git commit -m "feat: open scheduled task operation log as page"
```

### Task 4: Verify the final flow

**Files:**
- Modify if needed: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\ScheduledTasksPage.vue`
- Modify if needed: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\OperationLogListPage.vue`

**Step 1: Run automated checks**

Run:

```powershell
node --test --experimental-strip-types frontend/src/pages/scheduled-tasks/__tests__/scheduledTasksView.test.ts
pnpm --dir frontend exec vue-tsc
```

Expected:

- Node test passes
- TypeScript passes

**Step 2: Manual verification**

1. Open scheduled tasks.
2. Click `操作记录`.
3. Verify it opens a standalone page with a back arrow.
4. Click the back arrow.
5. Verify the task list page returns.
6. Open `操作记录` again.
7. Click `查看详情`.
8. Verify the detail dialog still opens.

**Step 3: Commit**

```bash
git add frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue frontend/src/pages/scheduled-tasks/components/OperationLogListPage.vue
git commit -m "chore: verify scheduled task operation log page flow"
```
