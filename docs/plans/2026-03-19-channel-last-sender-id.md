# Channel Last Sender ID Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Record each channel's most recent sender ID and use it as the direct push target for channel notifications.

**Architecture:** Add a `last_sender_id` column to `channels`, update it whenever an incoming message is handled, and make scheduled task notifications read this field instead of reverse-parsing `conversations.external_id`. Keep the field backend-only by storing it in the Bun persistence model without exposing it on the frontend DTO.

**Tech Stack:** Go, Bun, SQLite, existing scheduled task notification flow

---

### Task 1: Add notification target regression tests

**Files:**
- Modify: `internal/services/scheduledtasks/service_notification_test.go`

**Step 1: Write the failing test**

Add a test proving `getLatestChannelTarget` reads `channels.last_sender_id` and rejects empty values.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/scheduledtasks -run TestGetLatestChannelTarget`

Expected: FAIL because the test schema or implementation does not support `last_sender_id`.

**Step 3: Write minimal implementation**

Update scheduled task notification lookup to query `channels.last_sender_id`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/scheduledtasks -run TestGetLatestChannelTarget`

Expected: PASS

### Task 2: Add channel sender tracking coverage

**Files:**
- Create: `internal/services/channels/last_sender_test.go`
- Modify: `internal/services/channels/model.go`
- Modify: `internal/bootstrap/app.go`

**Step 1: Write the failing test**

Add tests for a helper that persists `last_sender_id` only when `sender_id` is non-empty.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/channels -run TestUpdateChannelLastSenderID`

Expected: FAIL because the helper or schema field does not exist.

**Step 3: Write minimal implementation**

Add the persistence field to the Bun model, implement the helper, and call it from incoming message handling.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/channels -run TestUpdateChannelLastSenderID`

Expected: PASS

### Task 3: Add migration and verify end state

**Files:**
- Create: `internal/sqlite/migrations/202603191500_add_channel_last_sender_id.go`

**Step 1: Add migration**

Create a SQLite migration adding `last_sender_id text not null default ''` to `channels`.

**Step 2: Run focused verification**

Run: `go test ./internal/services/scheduledtasks ./internal/services/channels`

Expected: PASS
