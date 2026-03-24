# Scheduled Task I18n Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Remove hardcoded user-facing copy from the scheduled task module and move it into frontend i18n so all existing locales can render the same pages.

**Architecture:** Keep the change scoped to `frontend/src/pages/scheduled-tasks/` plus locale files. Reuse the existing `scheduledTasks` locale namespace, add missing keys for operation log pages and task form controls, and route shared non-component formatting through the global `i18n` instance.

**Tech Stack:** Vue 3, TypeScript, vue-i18n, Node test runner

---

### Task 1: Add regression tests for localized scheduled-task text helpers

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\__tests__\operationLogTable.test.mjs`
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\__tests__\createTaskDialogState.test.mjs`

**Step 1: Write the failing test**

- Assert operation-log schedule formatting follows the active locale.
- Assert copied task names use a translated suffix instead of a hardcoded Chinese suffix.

**Step 2: Run test to verify it fails**

Run: `node --test --experimental-strip-types frontend/src/pages/scheduled-tasks/__tests__/operationLogTable.test.mjs frontend/src/pages/scheduled-tasks/__tests__/createTaskDialogState.test.mjs`

**Step 3: Write minimal implementation**

- Move schedule description formatting to i18n-backed helpers.
- Resolve copy suffix via `i18n.global.t(...)`.

**Step 4: Run test to verify it passes**

Run the same command and confirm both files pass.

### Task 2: Localize scheduled-task page and dialogs

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\ScheduledTasksPage.vue`
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\TaskFormContent.vue`
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\OperationLogListPage.vue`
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\OperationLogListDialog.vue`
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\OperationLogDetailDialog.vue`
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\composables\useScheduledTasks.ts`

**Step 1: Replace hardcoded copy**

- Convert page titles, table headers, action labels, placeholders, dropdown labels, helper text, and status copy to `t(...)`.

**Step 2: Keep non-component strings localized**

- Route shared formatting through module helpers or `i18n.global`.

**Step 3: Verify no hardcoded user copy remains**

Run: `rg -n "[\\p{Han}]" frontend/src/pages/scheduled-tasks`
Expected: only comments or translation keys remain.

### Task 3: Extend scheduled-task locale keys and propagate to all locales

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\src\locales\zh-CN.ts`
- Modify: `D:\willchat\willchat-client\frontend\src\locales\en-US.ts`
- Modify: every other file in `D:\willchat\willchat-client\frontend\src\locales\`

**Step 1: Add source-of-truth keys**

- Add missing `scheduledTasks.operationLog.*`, `scheduledTasks.form.*`, `scheduledTasks.dialog.*`, `scheduledTasks.notification.*`, and helper keys in `zh-CN.ts` and `en-US.ts`.

**Step 2: Fill remaining locales**

- Add the same keys to every locale file so runtime fallback is not relied on for this module.

**Step 3: Verify locale completeness**

Run: `python .cursor/skills/i18n-check/scripts/compare_frontend.py`

### Task 4: Final verification

**Files:**
- Verify changed files only

**Step 1: Run focused tests**

Run: `node --test --experimental-strip-types frontend/src/pages/scheduled-tasks/__tests__/operationLogTable.test.mjs frontend/src/pages/scheduled-tasks/__tests__/createTaskDialogState.test.mjs frontend/src/pages/scheduled-tasks/__tests__/scheduledTasksView.test.mjs`

**Step 2: Run locale comparison**

Run: `python .cursor/skills/i18n-check/scripts/compare_frontend.py`

**Step 3: Run a final hardcoded-string scan**

Run: `rg -n "[\\p{Han}]" frontend/src/pages/scheduled-tasks`
