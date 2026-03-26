# OpenClaw Cron Fixes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix OpenClaw cron history status, manual run duration display, and replace the custom schedule UI with a one-time datetime picker.

**Architecture:** Keep backend schedule semantics unchanged by mapping the new one-time picker UI to the existing `at` schedule kind. Fix history correctness in the backend merge/enrichment layer so all callers get the corrected status and duration. Reuse the scheduled task expiration picker visual language in the OpenClaw cron form and extend it with hour/minute selection.

**Tech Stack:** Go, Vue 3, TypeScript, Wails bindings

---

### Task 1: History Regression Tests

**Files:**
- Modify: `D:\willchat\chatclaw\internal\openclaw\cron\service_logic_test.go`

**Step 1: Write the failing test**

- Add a test proving a conversation history item marked `running` is updated to the terminal run-log status once a matching run entry exists.
- Add a test proving a pending history item receives `duration_ms` once the durable run entry is available.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/openclaw/cron -run "TestEnrichConversationHistoryItemsWithRunEntries|TestPreferHistoryItem" `

Expected: FAIL on status/duration assertions.

**Step 3: Write minimal implementation**

- Update backend enrichment/merge logic to backfill `status`, `duration`, and IDs from run-log entries.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/openclaw/cron -run "TestEnrichConversationHistoryItemsWithRunEntries|TestPreferHistoryItem" `

Expected: PASS

### Task 2: One-Time Datetime Picker

**Files:**
- Modify: `D:\willchat\chatclaw\frontend\src\pages\openclaw-cron\utils.ts`
- Modify: `D:\willchat\chatclaw\frontend\src\pages\openclaw-cron\OpenClawCronTaskDialog.vue`

**Step 1: Write the failing test**

- If frontend test coverage exists for cron form utils, add a test for parsing/building one-time `at` values.
- Otherwise validate through a targeted type/build check after implementation.

**Step 2: Implement minimal UI change**

- Rename the option/label from custom time to one-time time.
- Reuse the existing scheduled-task date-picker style and add hour/minute selectors.
- Map the picker output to existing `at` form state and preserve edit-mode hydration.

**Step 3: Verify**

Run the smallest available frontend verification command covering the modified files.

### Task 3: Final Verification

**Files:**
- No additional code changes expected.

**Step 1: Run focused backend tests**

Run: `go test ./internal/openclaw/cron`

**Step 2: Run focused frontend verification**

Run the smallest project command that validates Vue/TS compilation for the cron page.

**Step 3: Review diff**

Run: `git diff -- internal/openclaw/cron frontend/src/pages/openclaw-cron frontend/src/locales`
