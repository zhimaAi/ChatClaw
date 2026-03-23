# Scheduled Task Operation Log Page Design

**Date:** 2026-03-23

**Goal:** Change the scheduled-task top-right "操作记录" entry from a dialog into a dedicated in-module page, while keeping "查看详情" as the existing dialog flow.

**Problem Summary**

The current scheduled-task page opens operation logs with a modal dialog:

- `ScheduledTasksPage.vue` toggles `operationLogOpen`
- `OperationLogListDialog.vue` renders the list in a `Dialog`
- `OperationLogDetailDialog.vue` is used for the row-level detail popup

This does not match the requested interaction. The user wants:

- clicking the top-right "操作记录" button to enter a standalone page
- a small back arrow in the top-left corner of that page
- row-level "查看详情" to remain a dialog

The scheduled-task run history dialog is explicitly out of scope.

## Chosen Approach

Keep everything inside the existing `scheduled-tasks` module and switch the content with a local page-view state instead of introducing a new global navigation module or a second top-level tab.

This gives us:

- no new sidebar item
- no extra top tab
- low-risk changes limited to the scheduled-task page
- a natural back-arrow flow from operation-log page to task list

## Rejected Alternatives

### 1. Keep using a dialog and make it look like a page

Rejected because the user explicitly asked for a standalone page instead of a popup.

### 2. Add a new top-level navigation module

Rejected because operation log belongs to scheduled tasks and should not become a separate primary module in the left navigation.

### 3. Open a separate browser-style entry page

Rejected because the requested flow is an in-app page with a back arrow, not a detached app entry.

## Target Architecture

### Parent page state

`ScheduledTasksPage.vue` will own a local page state:

- `task-list`
- `operation-log-list`

The page state controls:

- which header is shown
- which main content is rendered
- whether the top-right "操作记录" button is visible

### Operation log page

Create a standalone content component for the page body:

- `OperationLogListPage.vue`

Responsibilities:

- load operation logs from `ScheduledTasksService.ListScheduledTaskOperationLogs`
- render the table layout that currently lives in the dialog
- keep the existing detail dialog behavior
- expose no parent dialog state

### Detail behavior

Keep `OperationLogDetailDialog.vue` unchanged in behavior:

- clicking "查看详情" still opens the detail dialog
- the detail dialog remains local to the operation-log page component

## UI Behavior

### Scheduled-task home

Keep the existing scheduled-task header:

- title
- subtitle
- top-right buttons

### Operation-log page header

Show a compact header that matches the requested visual direction:

- a small left arrow button in the top-left
- title text `操作记录`

The back arrow returns to `task-list`.

### Main content

Render the current operation log list table as page content instead of modal content.

The table columns remain:

- 任务
- 操作类型
- 操作方式
- 操作项
- 修改前
- 修改后
- 操作时间
- 操作

## Error Handling

The operation-log page keeps the existing behavior:

- loading state
- empty state
- toast on list load failure
- toast on detail load failure

## Testing Strategy

### Automated

Add a minimal Node test for the new scheduled-task page-view helpers using Node 24 `--experimental-strip-types`.

This verifies:

- operation-log entry switches to the page view
- back action returns to the task list view
- unsupported actions do not mutate the current view

### Verification

Run:

```powershell
node --test --experimental-strip-types frontend/src/pages/scheduled-tasks/__tests__/scheduledTasksView.test.ts
pnpm --dir frontend exec vue-tsc
```

### Manual

1. Open the scheduled-task page.
2. Click the top-right `操作记录`.
3. Verify the dialog no longer appears.
4. Verify a standalone page appears with a top-left back arrow.
5. Click the back arrow and verify the task list page returns.
6. Enter operation log again and click `查看详情`.
7. Verify the detail dialog still works.

## Acceptance Criteria

- Top-right `操作记录` opens an in-module page, not a dialog.
- The operation-log page shows a top-left back arrow.
- Clicking the back arrow returns to the scheduled-task list page.
- `查看详情` still opens the existing detail dialog.
- Scheduled-task run history remains unchanged.
