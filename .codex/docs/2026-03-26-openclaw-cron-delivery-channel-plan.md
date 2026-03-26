# OpenClaw Cron Delivery Channel Implementation Plan

**Goal:** Add a simplified delivery configuration for OpenClaw cron create/edit flows using `channel platform + target mode + target id`, while preserving `bestEffortDeliver`.

**Architecture:** Keep OpenClaw-native cron payload semantics unchanged. Add business-layer fields to the cron form and input DTOs, resolve `last_active` to the current `channels.last_sender_id` snapshot during create/update, and continue writing the result into OpenClaw `delivery.channel` and `delivery.to`.

**Tech Stack:** Go, Wails DTO bindings, Vue 3, TypeScript

---

### Task 1: Lock backend delivery resolution with tests

**Files:**
- Modify: `D:\willchat\chatclaw\internal\openclaw\cron\service_logic_test.go`

**Steps:**
1. Add failing tests for resolving `last_active`, direct target IDs, missing channel matches, and ambiguous platform matches.
2. Run `go test ./internal/openclaw/cron -run "TestResolveCronDeliverySelection|TestResolveCronDeliverySelectionErrors"` and confirm failure.

### Task 2: Implement backend DTOs and resolution helpers

**Files:**
- Modify: `D:\willchat\chatclaw\internal\openclaw\cron\model.go`
- Modify: `D:\willchat\chatclaw\internal\openclaw\cron\service.go`

**Steps:**
1. Add business-layer delivery fields to create/update inputs.
2. Implement a resolver that maps business fields to OpenClaw-native `delivery_channel` / `delivery_to`.
3. Query OpenClaw-visible channels and resolve `last_active` using `channels.last_sender_id`.
4. Keep `bestEffortDeliver` untouched.

### Task 3: Update frontend form state and mapping

**Files:**
- Modify: `D:\willchat\chatclaw\frontend\src\pages\openclaw-cron\utils.ts`
- Modify: `D:\willchat\chatclaw\frontend\src\pages\openclaw-cron\OpenClawCronPage.vue`
- Modify: `D:\willchat\chatclaw\frontend\src\pages\openclaw-cron\OpenClawCronTaskDialog.vue`

**Steps:**
1. Replace raw delivery fields in form state with business fields.
2. Load OpenClaw channel list and derive unique configured platforms.
3. Render the simplified channel platform + target mode + target ID UI.
4. Keep `bestEffortDeliver` as a visible independent switch.

### Task 4: Update i18n copy and verify

**Files:**
- Modify: `D:\willchat\chatclaw\frontend\src\locales\zh-CN.ts`
- Modify: `D:\willchat\chatclaw\frontend\src\locales\en-US.ts`

**Steps:**
1. Add labels and hints for channel platform and target mode.
2. Remove obsolete direct-edit copy for raw delivery fields from the cron dialog.
3. Run targeted backend tests and the smallest frontend type/build check available.
