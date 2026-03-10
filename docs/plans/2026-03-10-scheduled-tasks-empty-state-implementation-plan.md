# Scheduled Tasks Empty State Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the scheduled tasks page show the Figma empty state when there are no tasks, while preserving the existing create-task flow.

**Architecture:** Move empty-state rendering to `ScheduledTasksPage.vue` so the page decides between loading, empty, and table states. Keep `TaskTable.vue` focused on populated table rendering only, and add the missing empty description copy to i18n.

**Tech Stack:** Vue 3, TypeScript, vue-i18n, Tailwind CSS, lucide-vue-next

---

### Task 1: Add page-level empty state copy and condition

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Modify: `frontend/src/locales/zh-CN.ts`
- Modify: `frontend/src/locales/en-US.ts`

**Step 1: Write the failing test**

Repository currently has no visible Vue component test setup. Confirm whether to add minimal test infrastructure or proceed with type/build verification only before touching production code.

**Step 2: Run test to verify it fails**

If test infrastructure is approved, run the targeted component test and verify it fails because the empty state is not rendered yet.

**Step 3: Write minimal implementation**

- Add `hasTasks` computed state
- Render loading, empty, and table branches explicitly
- Add the empty-state icon, title, description, and CTA button
- Bind the CTA button to `openCreateDialog`
- Add localized empty description copy

**Step 4: Run test to verify it passes**

If a component test was added, rerun the targeted test and verify the empty-state assertions now pass.

**Step 5: Commit**

```bash
git add frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue frontend/src/locales/zh-CN.ts frontend/src/locales/en-US.ts
git commit -m "feat: add scheduled tasks empty state"
```

### Task 2: Remove table-owned empty placeholder

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`

**Step 1: Write the failing test**

If component tests are available, add or extend a test asserting that `TaskTable` only renders table markup when it receives tasks, and does not own the page-level empty state.

**Step 2: Run test to verify it fails**

Run the targeted test and verify failure reflects the old empty placeholder behavior.

**Step 3: Write minimal implementation**

- Remove `hasTasks` empty placeholder branch
- Keep the existing populated table rendering unchanged

**Step 4: Run test to verify it passes**

Run the targeted test and confirm populated table rendering still passes.

**Step 5: Commit**

```bash
git add frontend/src/pages/scheduled-tasks/components/TaskTable.vue
git commit -m "refactor: keep scheduled task table data-only"
```

### Task 3: Verify integration

**Files:**
- Test: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Test: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`

**Step 1: Run targeted verification**

Run: `npx vue-tsc --noEmit`
Expected: pass without new type errors

**Step 2: Run build verification**

Run: `npm run build`
Expected: production build succeeds

**Step 3: Review worktree state**

Run: `git status --short`
Expected: only intended frontend/docs changes are present alongside any pre-existing unrelated edits
