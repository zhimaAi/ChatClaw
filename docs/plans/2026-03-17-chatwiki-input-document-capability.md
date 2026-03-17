# ChatWiki Input Document Capability Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make ChatWiki models expose the file icon when remote catalog items return `input_document: "1"` by syncing that capability into the local `models` table.

**Architecture:** Extend ChatWiki catalog parsing so `input_document` is normalized into the shared `capabilities` list as `"file"`. Persist the capability through the existing `models` table sync path so assistant model lists automatically render the file icon without adding ChatWiki-only frontend logic.

**Tech Stack:** Go, Bun, SQLite, Vue, Wails

---

### Task 1: Add failing parser and sync tests

**Files:**
- Modify: `internal/services/chatwiki/model_catalog_test.go`

**Step 1: Write the failing test**

Add a decode test proving a ChatWiki LLM item with `input_document: "1"` yields `capabilities == ["text", "file"]`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/chatwiki -run "TestDecodeModelCatalogResponse_.*Document|TestSyncModelCatalogToDB_.*Document" -count=1`
Expected: FAIL because file capability is not parsed or persisted yet.

**Step 3: Write minimal implementation**

Update ChatWiki capability parsing and model sync mapping so `input_document` is included as `"file"` in capabilities.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/chatwiki -run "TestDecodeModelCatalogResponse_.*Document|TestSyncModelCatalogToDB_.*Document" -count=1`
Expected: PASS

### Task 2: Verify persistence through `models`

**Files:**
- Modify: `internal/services/chatwiki/model_catalog_test.go`
- Modify: `internal/services/chatwiki/model_catalog.go`

**Step 1: Write the failing test**

Add a sync test proving `syncModelCatalogToDB` stores `"file"` in the `capabilities` column for ChatWiki models with `input_document: "1"`.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/services/chatwiki -run "TestSyncModelCatalogToDB_.*Document" -count=1`
Expected: FAIL because the synced row lacks `"file"`.

**Step 3: Write minimal implementation**

Map parsed file capability through `flattenCatalogModelsForSync` without changing unrelated provider behavior.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/services/chatwiki -run "TestSyncModelCatalogToDB_.*Document" -count=1`
Expected: PASS

### Task 3: Final regression verification

**Files:**
- Modify: `internal/services/chatwiki/model_catalog.go`
- Modify: `internal/services/chatwiki/model_catalog_test.go`

**Step 1: Run focused verification**

Run: `go test ./internal/services/chatwiki -count=1`
Expected: PASS

**Step 2: Commit**

```bash
git add docs/plans/2026-03-17-chatwiki-input-document-capability.md internal/services/chatwiki/model_catalog.go internal/services/chatwiki/model_catalog_test.go
git commit -m "fix: sync chatwiki document model capability"
```
