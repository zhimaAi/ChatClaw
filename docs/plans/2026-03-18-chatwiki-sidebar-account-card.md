# ChatWiki Sidebar Account Card Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a ChatWiki account status card above the Settings button in the global side navigation, with cloud-bound and login-required states.

**Architecture:** Introduce a small sidebar-specific ChatWiki account card component plus a helper that maps binding and catalog data into a renderable view model. Integrate the component into `SideNav.vue`, reuse existing ChatWiki cache/store/navigation hooks, and verify behavior with focused Node TypeScript tests.

**Tech Stack:** Vue 3, Pinia, TypeScript, node:test, existing Wails-generated service bindings

---

### Task 1: Add failing tests for sidebar card state mapping

**Files:**
- Create: `frontend/src/components/layout/chatwikiSidebarAccountCard.ts`
- Create: `frontend/src/components/layout/chatwikiSidebarAccountCard.test.ts`

**Step 1: Write the failing test**

Add tests for:
- unbound binding -> `login`
- `dev` binding -> `login`
- cloud binding with `user_name` and `all_surplus` -> `bound`
- cloud binding without `user_name` and with `user_id` fallback -> `bound`

**Step 2: Run test to verify it fails**

Run: `node --test frontend/src/components/layout/chatwikiSidebarAccountCard.test.ts`
Expected: FAIL because the helper module does not exist yet.

**Step 3: Write minimal implementation**

Implement a helper that:
- detects cloud-bound state from `binding.chatwiki_version`
- formats account label
- formats credits label
- returns the action mode for click handling

**Step 4: Run test to verify it passes**

Run: `node --test frontend/src/components/layout/chatwikiSidebarAccountCard.test.ts`
Expected: PASS

### Task 2: Add the sidebar card component

**Files:**
- Create: `frontend/src/components/layout/ChatWikiSidebarAccountCard.vue`
- Modify: `frontend/src/components/layout/SideNav.vue`

**Step 1: Write the failing test**

Use the helper-based tests from Task 1 as the behavioral contract. Do not add new production code before the helper tests pass.

**Step 2: Implement the component**

Create a component that:
- loads binding from `@/lib/chatwikiCache`
- loads model catalog only for cloud-bound accounts
- renders the correct card state
- emits or handles click behavior based on helper output

**Step 3: Integrate into `SideNav.vue`**

Render the new component above the Settings button only when the global sidebar is expanded.

**Step 4: Run targeted verification**

Run: `node --test frontend/src/components/layout/chatwikiSidebarAccountCard.test.ts`
Expected: PASS

### Task 3: Wire navigation behavior and verify build safety

**Files:**
- Modify: `frontend/src/components/layout/ChatWikiSidebarAccountCard.vue`
- Modify: `frontend/src/components/layout/SideNav.vue`

**Step 1: Implement click behavior**

- login state -> request cloud login, set ChatWiki menu, navigate to Settings
- bound state -> set model service menu, navigate to Settings

**Step 2: Run type/build verification**

Run: `pnpm exec vue-tsc --noEmit`
Expected: PASS

**Step 3: Commit**

```bash
git add docs/plans/2026-03-18-chatwiki-sidebar-account-card-design.md docs/plans/2026-03-18-chatwiki-sidebar-account-card.md frontend/src/components/layout/chatwikiSidebarAccountCard.ts frontend/src/components/layout/chatwikiSidebarAccountCard.test.ts frontend/src/components/layout/ChatWikiSidebarAccountCard.vue frontend/src/components/layout/SideNav.vue
git commit -m "feat: add chatwiki sidebar account card"
```
