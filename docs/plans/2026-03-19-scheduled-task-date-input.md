# Scheduled Task Date Input Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the native expiration date input with a fixed year/month/day UI so the placeholder and labels are fully controlled.

**Architecture:** Keep the stored form value as the existing `YYYY-MM-DD` string, but render it through three controlled inputs in the scheduled-task form. Reuse the existing payload conversion so no backend behavior changes.

**Tech Stack:** Vue 3, TypeScript, existing UI input components

---

### Task 1: Replace the expiration date field UI

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`
- Modify: `frontend/src/pages/scheduled-tasks/utils.ts`

**Step 1: Implement the minimal UI change**

Replace the native `type="date"` field with a fixed year/month/day input group while keeping the same form state.

**Step 2: Verify type safety**

Run: `npm exec vue-tsc -- --noEmit`
Expected: PASS
