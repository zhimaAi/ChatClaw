# ChatWiki Binding Version Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Persist the ChatWiki binding callback field `chatwiki_version` into `chatwiki_bindings` with a default value of `dev`.

**Architecture:** Extend the ChatWiki binding schema and binding DTOs with a `chatwiki_version` column, thread the deep-link callback value through `HandleURL` into `SaveBinding`, and apply an application-side default of `dev` when the callback omits the field. Cover both explicit and defaulted values with backend regression tests.

**Tech Stack:** Go, Bun, SQLite, Wails

---

### Task 1: Add failing binding persistence tests

**Files:**
- Modify: `internal/services/chatwiki/service_test.go`

**Step 1: Write the failing test**

Add tests that:
- save a binding with `chatwiki_version = "release"` and expect `GetBinding().ChatWikiVersion == "release"`;
- save a binding with an empty version and expect `GetBinding().ChatWikiVersion == "dev"`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/chatwiki -run TestSaveBinding -count=1`
Expected: FAIL because the binding structs and save path do not yet include `chatwiki_version`.

**Step 3: Write minimal implementation**

Update the schema, binding structs, deep-link parsing, and save logic to persist the new field with defaulting.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/chatwiki -run TestSaveBinding -count=1`
Expected: PASS

### Task 2: Add migration coverage in code changes

**Files:**
- Modify: `internal/sqlite/migrations/202603100800_create_chatwiki_bindings_table.go`
- Create: `internal/sqlite/migrations/202603171200_add_chatwiki_binding_version.go`

**Step 1: Update schema creation**

Ensure fresh databases create `chatwiki_version TEXT NOT NULL DEFAULT 'dev'`.

**Step 2: Add upgrade migration**

Add a forward migration that appends the column for existing databases and a safe rollback that recreates the table without it if needed.

**Step 3: Run focused tests**

Run: `go test ./internal/services/chatwiki -run TestSaveBinding -count=1`
Expected: PASS

### Task 3: Keep frontend callback types aligned

**Files:**
- Modify: `internal/deeplink/deeplink.go`
- Modify: `frontend/src/pages/settings/components/ChatwikiSettings.vue`
- Modify: `frontend/bindings/chatclaw/internal/services/chatwiki/models.ts`

**Step 1: Thread callback payload**

Parse and emit `chatwiki_version` in the deep-link callback payload.

**Step 2: Keep local TS types aligned**

Extend the local callback/interface typing and generated binding model shape so the repo reflects the new backend contract.

**Step 3: Run verification**

Run: `go test ./internal/services/chatwiki -run TestSaveBinding -count=1`
Expected: PASS

