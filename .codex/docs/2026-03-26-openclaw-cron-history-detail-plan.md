# OpenClaw Cron History Detail Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a right-panel loading animation and explicit error reason for OpenClaw cron history detail loading.

**Architecture:** Keep the change inside the existing history dialog component. Introduce dedicated detail loading and error refs, guard async races with a request sequence, and render distinct right-panel states for loading, error, preparing, and ready.

**Tech Stack:** Vue 3, TypeScript, vue-i18n, Wails bindings, Tailwind utility classes

---

### Task 1: Add right-panel detail state

**Files:**
- Modify: `frontend/src/pages/openclaw-cron/OpenClawCronHistoryDialog.vue`

**Step 1: Add detail-loading and detail-error refs**

Track detail fetch lifecycle independently from the left history list.

**Step 2: Add async request sequencing**

Prevent stale detail requests from overwriting the latest selected run.

**Step 3: Update `loadDetail`**

Set loading before fetch, clear error on retry, store parsed error reason on failure, and always leave the right panel in a non-blank state.

### Task 2: Render right-panel loading and error UI

**Files:**
- Modify: `frontend/src/pages/openclaw-cron/OpenClawCronHistoryDialog.vue`

**Step 1: Add loading placeholder**

Render an in-place animated skeleton while detail loading is pending.

**Step 2: Add error card**

Render a title, description, concrete reason, and retry button when detail loading fails.

**Step 3: Keep existing preparing and ready states**

Only show preparing when detail is neither loading nor failed.

### Task 3: Verify frontend integrity

**Files:**
- Modify: `frontend/src/pages/openclaw-cron/OpenClawCronHistoryDialog.vue`

**Step 1: Run type-check**

Run: `pnpm -C frontend exec vue-tsc --noEmit`

**Step 2: Review failures**

If the command fails due to unrelated baseline issues, document them clearly and separate them from this change.
