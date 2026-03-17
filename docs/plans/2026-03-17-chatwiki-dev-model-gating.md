# ChatWiki Dev Model Gating Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Disable ChatWiki model selection when the current binding version is `dev`, and annotate the ChatWiki provider label as `（非ChatWiki Cloud）`.

**Architecture:** Extend the shared frontend ChatWiki model-availability helper to derive availability from binding state instead of a plain boolean. Update model-selection entry points to pass binding-derived status, then cover the new behavior with focused unit tests before implementation.

**Tech Stack:** Vue 3, TypeScript, Node test runner, Wails bindings

---

### Task 1: Add failing availability tests

**Files:**
- Modify: `frontend/src/lib/chatwikiModelAvailability.test.ts`

**Step 1: Write the failing test**

Add cases that verify:
- ChatWiki `dev` binding disables model selection.
- Fallback selection skips ChatWiki `dev`.
- Provider label shows `ChatWiki（非ChatWiki Cloud）` for `dev`.

**Step 2: Run test to verify it fails**

Run: `node --test frontend/src/lib/chatwikiModelAvailability.test.ts`
Expected: FAIL because the helper only understands a boolean bound/unbound state.

**Step 3: Write minimal implementation**

Update the helper API and its call sites to use binding-derived availability and label rules.

**Step 4: Run test to verify it passes**

Run: `node --test frontend/src/lib/chatwikiModelAvailability.test.ts`
Expected: PASS

### Task 2: Wire binding-aware availability into model selectors

**Files:**
- Modify: `frontend/src/lib/chatwikiModelAvailability.ts`
- Modify: `frontend/src/pages/assistant/composables/useModelSelection.ts`
- Modify: `frontend/src/pages/assistant/components/ChatInputArea.vue`
- Modify: `frontend/src/pages/assistant/components/AgentSettingsDialog.vue`

**Step 1: Replace boolean-only checks**

Thread binding version or a derived availability status into the shared helper calls.

**Step 2: Keep selection cleanup aligned**

Ensure saved ChatWiki selections are cleared when the binding is unavailable because of `dev`.

**Step 3: Run focused test**

Run: `node --test frontend/src/lib/chatwikiModelAvailability.test.ts`
Expected: PASS

### Task 3: Verify no regression in local flows

**Files:**
- Review: `frontend/src/pages/knowledge/components/CreateLibraryDialog.vue`
- Review: `frontend/src/pages/knowledge/components/EditLibraryDialog.vue`
- Review: `frontend/src/pages/knowledge/components/EmbeddingSettingsDialog.vue`

**Step 1: Check reuse points**

Confirm whether these screens rely on the shared helper or need the same binding-derived availability semantics.

**Step 2: Apply minimal follow-up edits if needed**

Keep behavior consistent without broad refactors.

**Step 3: Run verification**

Run: `node --test frontend/src/lib/chatwikiModelAvailability.test.ts`
Expected: PASS
