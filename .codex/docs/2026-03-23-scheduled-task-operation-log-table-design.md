# Scheduled Task Operation Log Table Design

**Date:** 2026-03-23

**Goal:** Optimize the scheduled-task operation-log page so each changed field renders as its own aligned row, and schedule JSON values are displayed as readable Chinese descriptions.

**Problem Summary**

The current operation-log page renders each log record as one table row and stacks all changed fields inside three cells:

- `操作项`
- `修改前`
- `修改后`

This causes alignment issues when one log contains multiple changed fields. The schedule-time values also leak raw JSON like `{"hour":9,"minute":0,"day_of_month":4}` instead of user-facing text.

## Chosen Approach

Flatten each log record into multiple display rows on the frontend:

- one changed field becomes one display row
- shared metadata columns render only on the first row of the group
- `操作项 / 修改前 / 修改后` use fixed widths and single-line truncation

Schedule-time values are formatted in the same frontend helper before rendering:

- daily custom schedule -> `每天 09:00`
- weekly custom schedule -> `每周一 周二 09:00`
- monthly custom schedule -> `每月 4 号 09:00`
- interval custom schedule -> `每 15 分钟`

## Testing Strategy

Add a Node test for a pure helper that verifies:

- schedule-time JSON gets formatted to readable text
- logs with multiple changed fields become multiple display rows
- only the first row keeps the shared metadata values
