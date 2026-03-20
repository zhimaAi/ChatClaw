# Scheduled Task Native Date Trigger Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Keep the expiration field as a single input-like control with fixed Chinese year/month/day display while still using the native date picker.

**Architecture:** Render a readonly visible input for display and placeholder control, backed by a hidden native `type="date"` input that opens on click via `showPicker()` when available. Keep the stored value as the existing `YYYY-MM-DD` string.

**Tech Stack:** Vue 3, TypeScript, native HTML date input

---

### Task 1: Swap the expiration input interaction model

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`

**Step 1: Implement the minimal component change**

Replace the split year/month/day inputs with a single visible readonly input and a hidden native date input.

**Step 2: Verify type safety**

Run: `npm exec vue-tsc -- --noEmit`
Expected: PASS
