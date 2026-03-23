# Scheduled Task Expiration Year Month Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add year and month quick switching to the scheduled-task expiration calendar while still requiring the user to pick a concrete date.

**Architecture:** Keep the existing `YYYY-MM-DD` expiration value unchanged and extend the in-component calendar state with year/month selector options derived from the visible month. Only the calendar header and helper logic change; selecting a date remains the commit point for form data.

**Tech Stack:** Vue 3, TypeScript, node:test, vue-tsc

---

### Task 1: 提炼并测试年月切换辅助逻辑

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/taskFormExpirationCalendar.ts`
- Create: `frontend/src/pages/scheduled-tasks/components/__tests__/taskFormExpirationCalendar.test.ts`

**Step 1: Write the failing test**

为年份范围生成、月份选项和年月切换结果编写最小失败用例。

**Step 2: Run test to verify it fails**

Run: `node --test frontend/src/pages/scheduled-tasks/components/__tests__/taskFormExpirationCalendar.test.ts`

Expected: FAIL because helper module does not exist yet.

**Step 3: Write minimal implementation**

新增日期辅助模块，导出构建年份选项和根据目标年月生成可见月份的方法。

**Step 4: Run test to verify it passes**

Run: `node --test frontend/src/pages/scheduled-tasks/components/__tests__/taskFormExpirationCalendar.test.ts`

Expected: PASS

### Task 2: 接入到期时间组件头部

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`

**Step 1: Write the failing behavior check**

让现有组件依赖新的辅助逻辑；如果类型不匹配或缺少状态，`vue-tsc` 应失败。

**Step 2: Run check to verify it fails**

Run: `pnpm --dir frontend exec vue-tsc --noEmit`

Expected: FAIL until component state and template are wired correctly.

**Step 3: Write minimal implementation**

在日历头部增加年份、月份下拉，切换时仅更新 `visibleExpirationMonth`，保留现有按钮和关闭行为。

**Step 4: Run check to verify it passes**

Run: `pnpm --dir frontend exec vue-tsc --noEmit`

Expected: PASS

### Task 3: 最终验证

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskFormContent.vue`
- Create: `frontend/src/pages/scheduled-tasks/components/taskFormExpirationCalendar.ts`
- Create: `frontend/src/pages/scheduled-tasks/components/__tests__/taskFormExpirationCalendar.test.ts`

**Step 1: Run focused tests**

Run: `node --test frontend/src/pages/scheduled-tasks/components/__tests__/taskFormExpirationCalendar.test.ts`

Expected: PASS

**Step 2: Run frontend type-check**

Run: `pnpm --dir frontend exec vue-tsc --noEmit`

Expected: PASS
