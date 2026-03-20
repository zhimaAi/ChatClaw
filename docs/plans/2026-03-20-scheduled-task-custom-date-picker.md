# Scheduled Task Custom Date Picker Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the scheduled task expiration native date picker with a custom calendar popover that matches the dialog style while keeping the existing `YYYY-MM-DD` form value.

**Architecture:** Keep all persistence and payload behavior unchanged and localize the UI change to the scheduled task form. Build a small calendar state layer in `TaskFormContent.vue`, render a styled popup panel from the existing trigger area, and verify the result with a frontend type/build check because this workspace currently has no dedicated scheduled-task frontend test runner configured.

**Tech Stack:** Vue 3 script setup, TypeScript, Tailwind utility classes, lucide-vue-next

---

### Task 1: Add calendar state and date helpers

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`

**Step 1: Define the missing behavior boundary**
Document the state needed for a custom picker: panel open state, visible month anchor, selected date parsing, calendar grid generation, and quick actions for today/clear.

**Step 2: Add the smallest calendar helpers**
Implement local helpers for parsing `YYYY-MM-DD`, formatting day values, generating the visible month title, and building a 6-row calendar grid including leading/trailing days.

**Step 3: Wire state to the existing form value**
Sync the visible month with the selected expiration date when present, otherwise default to the current month.

**Step 4: Verify TypeScript integrity**
Run: `pnpm exec vue-tsc --noEmit`
Expected: no new type errors from the helper/state additions.

### Task 2: Replace the native picker with a custom popup calendar

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`

**Step 1: Remove the native hidden date input path**
Delete the hidden `input[type="date"]` and the `showPicker()` forwarding logic.

**Step 2: Render a custom trigger and floating panel**
Use the current input shell as the trigger, then render month navigation, weekday headers, date cells, and footer actions inside an absolutely positioned card below the field.

**Step 3: Preserve current behavior expectations**
Keep read-only mode non-interactive, close the panel after selecting a date, preserve expired hint rendering, and allow clearing the expiration date.

**Step 4: Verify interaction compiles cleanly**
Run: `pnpm exec vue-tsc --noEmit`
Expected: no new type errors from template bindings and event handlers.

### Task 3: Run focused verification

**Files:**
- Modify: `docs/plans/2026-03-20-scheduled-task-custom-date-picker-design.md`
- Modify: `docs/plans/2026-03-20-scheduled-task-custom-date-picker.md`

**Step 1: Run frontend verification**
Run: `pnpm exec vue-tsc --noEmit`
Expected: PASS

**Step 2: Run a development build if the type check passes**
Run: `pnpm run build:dev`
Expected: frontend compiles successfully.

**Step 3: Record any verification gaps**
If the workspace still lacks a runnable frontend test target for this area, note that explicitly in the final handoff.
