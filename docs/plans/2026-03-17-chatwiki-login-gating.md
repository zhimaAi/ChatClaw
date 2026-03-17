# Chatwiki Login Gating Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Keep Chatwiki models visible while making them unselectable everywhere when the Chatwiki account is not bound, and clear any saved Chatwiki selections in that state.

**Architecture:** Add a shared frontend helper that derives Chatwiki binding state and model availability from provider/model keys. Reuse that helper across settings, assistant, knowledge, and memory flows so disabled rendering, fallback selection, and forced clearing all follow one rule.

**Tech Stack:** Vue 3, TypeScript, Pinia, Wails bindings, Node 24 `node:test`

---

### Task 1: Add failing availability-rule tests

**Files:**
- Create: `frontend/src/lib/chatwikiModelAvailability.test.ts`
- Create: `frontend/src/lib/chatwikiModelAvailability.ts`

**Step 1: Write the failing test**

Cover:
- `chatwiki` is unavailable when unbound
- non-Chatwiki providers remain selectable
- an existing `chatwiki::model` selection is cleared when unbound
- fallback selection skips unbound Chatwiki models

**Step 2: Run test to verify it fails**

Run: `node --test src/lib/chatwikiModelAvailability.test.ts`
Expected: FAIL because helper module does not exist yet.

**Step 3: Write minimal implementation**

Implement pure helper functions for:
- provider/model key parsing
- provider model disabled state
- clearing invalid Chatwiki selections
- choosing the first selectable model

**Step 4: Run test to verify it passes**

Run: `node --test src/lib/chatwikiModelAvailability.test.ts`
Expected: PASS

### Task 2: Wire shared gating into assistant flows

**Files:**
- Modify: `frontend/src/pages/assistant/composables/useModelSelection.ts`
- Modify: `frontend/src/pages/assistant/components/AgentSettingsDialog.vue`
- Modify: `frontend/src/pages/assistant/components/ChatInputArea.vue`

**Step 1: Write the failing behavior check**

Use the helper test plus targeted type/build verification to prove assistant flows still allow unbound Chatwiki selections before the fix.

**Step 2: Run verification to confirm current failure**

Run: `npm run build`
Expected: Existing code compiles but still lacks the new gating behavior.

**Step 3: Write minimal implementation**

Apply the shared helper to:
- disable Chatwiki LLM items in selectors
- clear saved/default selections when unbound
- skip unbound Chatwiki models during auto-selection

**Step 4: Run verification**

Run: `npm run build`
Expected: PASS

### Task 3: Wire shared gating into knowledge and memory flows

**Files:**
- Modify: `frontend/src/pages/knowledge/components/EmbeddingSettingsDialog.vue`
- Modify: `frontend/src/pages/knowledge/components/CreateLibraryDialog.vue`
- Modify: `frontend/src/pages/knowledge/components/EditLibraryDialog.vue`
- Modify: `frontend/src/pages/settings/components/MemorySettings.vue`

**Step 1: Write the failing behavior check**

Use the helper test and build verification to confirm these selectors currently accept unbound Chatwiki keys.

**Step 2: Run verification to confirm current failure**

Run: `npm run build`
Expected: Existing code compiles but still lacks gating/clearing behavior.

**Step 3: Write minimal implementation**

Apply the shared helper to:
- disable Chatwiki items
- clear invalid saved keys when dialogs/settings load
- choose fallback models only from selectable options

**Step 4: Run verification**

Run: `npm run build`
Expected: PASS

### Task 4: Update Chatwiki settings status display

**Files:**
- Modify: `frontend/src/pages/settings/components/ChatwikiProviderDetail.vue`

**Step 1: Write the failing behavior check**

Confirm the provider detail does not yet show the requested unbound status text.

**Step 2: Implement minimal UI change**

Update the detail header/status area to make the unbound state explicit without changing the left provider list label.

**Step 3: Run verification**

Run: `npm run build`
Expected: PASS
