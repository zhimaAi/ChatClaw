# Scheduled Task Operation Log Design

**Date:** 2026-03-19

**Scope:** Add operation logging for scheduled tasks, including list display, read-only history detail, and source attribution for manual versus AI assistant changes.

---

## Problem

The current scheduled task module stores:

- current task state in `scheduled_tasks`
- execution history in `scheduled_task_runs`

It does not store configuration change history. That means the product cannot show:

- who changed a task
- whether the action came from the manual UI or the AI assistant
- which fields changed in one operation
- the full historical configuration for a deleted or modified task

The requested "operation log" page needs a real audit trail, not a frontend reconstruction.

---

## Confirmed Requirements

### Included operations

- Create task
- Update task
- Delete task
- Enable task
- Disable task

### Operation source

- `manual`: changes triggered from the scheduled task UI
- `ai`: changes triggered through the chat assistant scheduled-task tool flow

### Record granularity

One user-visible operation creates one log record.

Examples:

- creating a task produces 1 record
- deleting a task produces 1 record
- toggling enabled/disabled produces 1 record
- editing prompt, schedule, and notification channel in one save produces 1 record

### List display

Columns:

- Task
- Operation Type
- Operation Source
- Changed Fields
- Before
- After
- Operation Time
- View Detail

If one operation changes multiple fields, the `Changed Fields`, `Before`, and `After` cells render multiple lines in matching order.

### Detail display

Clicking `View Detail` opens a read-only scheduled-task form.

The form content must come from the logged snapshot, not from the current live task.

For delete operations, the snapshot is the last complete task data before deletion.

### Header entry point

On the scheduled task home page, add an `Operation Log` button to the right of `Refresh`, using the same visual style as the refresh button.

---

## Recommended Data Model

Use one dedicated table: `scheduled_task_operation_logs`.

### Table fields

- `id`
- `task_id`
- `task_name_snapshot`
- `operation_type`
- `operation_source`
- `changed_fields_json`
- `task_snapshot_json`
- `created_at`

### Why one table is enough

- The UI is one operation per row, not one changed field per row.
- The detail view depends on the full task snapshot.
- `changed_fields_json` can provide ordered multi-line list rendering without a second table.
- A single table keeps querying and migrations simpler.

---

## JSON Shapes

### `changed_fields_json`

Store a stable ordered array of display-ready values.

Example:

```json
[
  {
    "field_key": "prompt",
    "field_label": "提示词",
    "before": "之前的提示词",
    "after": "改为现在的提示词"
  },
  {
    "field_key": "schedule_time",
    "field_label": "执行时间",
    "before": "每日9点",
    "after": "每日10点"
  }
]
```

Rules:

- Values should already be formatted for display.
- Do not store only ids for agents or channels.
- Preserve display ordering in the array.

### `task_snapshot_json`

Store the complete scheduled-task configuration snapshot needed by the read-only detail form.

Recommended shape: close to frontend form state, not raw DB-only shape.

Include at least:

- name
- prompt
- agent id and agent display name
- notification platform
- notification channel ids and display names
- enabled
- schedule type
- schedule value
- cron expression
- display-ready schedule description
- any custom schedule fields needed to rehydrate the existing form

Delete logs store the final full state before deletion.

---

## Audited Fields

The first version should diff and display these fields:

- `状态`
- `名称`
- `提示词`
- `关联助手`
- `通知渠道`
- `执行时间`

Do not add speculative fields that are not implemented in the product yet.

### Field formatting rules

#### Status

Display values:

- `启用`
- `停用`

#### Agent

Store and display the agent name, not only `agent_id`.

#### Notification channels

Store and display readable text, for example:

- `微信: 渠道A, 渠道B`

#### Schedule time

Always store and display the user's configured expression, not internal derived code.

Examples:

- preset: `每日9点`
- custom weekly: `每周一、三、五 09:00`
- cron: the user-entered cron expression or a directly derived human-readable string if the original text is preserved

---

## Service-Layer Logging Strategy

Logging should be written inside `ScheduledTasksService`, so all write paths are covered consistently.

### Operations that must emit logs

- `CreateScheduledTask`
- `UpdateScheduledTask`
- `DeleteScheduledTask`
- `SetScheduledTaskEnabled`

### Source propagation

Add an operation context or explicit source parameter to the service entrypoints.

Expected values:

- `manual` from the scheduled task UI
- `ai` from the chat assistant tool path

This must be explicit. Do not infer source from partial heuristics.

### Update flow

Recommended update order:

1. Load the existing task
2. Apply the new input in memory
3. Build ordered diff items for auditable fields
4. Persist the updated task
5. Write one operation log record containing:
   - summary metadata
   - `changed_fields_json`
   - `task_snapshot_json`

### Delete flow

Recommended delete order:

1. Load the existing task
2. Build delete log snapshot from the pre-delete state
3. Persist the soft delete
4. Write one delete log record

This ensures deleted tasks still have viewable history.

---

## Frontend Detail View Strategy

Reuse the scheduled task form instead of building a second detail-only layout.

Recommended refactor:

- extract reusable task form content from `CreateTaskDialog.vue`
- add a `readonly` or `disabled` mode
- reuse the same component for:
  - create
  - edit
  - operation-log detail

Read-only mode rules:

- all inputs disabled
- no save action
- only close action

This keeps the history view consistent with the live editing experience.

---

## API Requirements

Minimum backend API support:

- list operation logs
- get one operation log detail

Suggested response structure:

- list item:
  - task name snapshot
  - operation metadata
  - ordered changed fields array
  - created time
- detail item:
  - operation metadata
  - ordered changed fields array
  - task snapshot

---

## Risks And Guardrails

- Do not reconstruct old values from the current task. Always log at write time.
- Do not rely on ids only for agent/channel history. Store display names in the log payload.
- Keep cron/history values user-facing.
- Avoid duplicate logs when `SetScheduledTaskEnabled` internally routes through update logic.
- Keep snapshot shape stable enough for read-only form hydration across future UI changes.

---

## Acceptance Criteria

- A manual create/update/delete/enable/disable action creates an operation log.
- An AI assistant create/update/delete/enable/disable action creates an operation log with source `ai`.
- Multi-field edits appear as one row in the operation list with aligned multi-line `Changed Fields`, `Before`, and `After`.
- Deleted tasks still show readable history and detail snapshots.
- `View Detail` opens a read-only scheduled-task form using the stored snapshot.
- The scheduled task page shows an `Operation Log` button beside `Refresh`.
