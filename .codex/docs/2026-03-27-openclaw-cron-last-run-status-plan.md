# OpenClaw Cron Last Run Status Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the OpenClaw 定时任务列表中的“上次执行”状态基于最后一次运行日志，而不是仅依赖 jobs 快照状态。

**Architecture:** 在后端 `OpenClawCronService.ListJobs` 中，为每个任务读取最近 1 条 run log，并用该日志的时间、状态、错误覆盖列表 DTO 的对应字段。前端继续消费既有 `last_run_at_ms / last_status / last_error` 字段，只保留展示逻辑，确保无日志时回退为灰色时钟。

**Tech Stack:** Go, Bun/Wails DTO, Vue 3, TypeScript, Go test

---

### Task 1: Add regression tests for log-priority list data

**Files:**
- Modify: `internal/openclaw/cron/service_logic_test.go`

**Step 1: Write the failing test**

Add tests covering:
- `ListJobs` prefers the latest run log status/time/error over jobs store snapshot.
- `ListJobs` keeps existing fallback behavior when no run log exists.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/openclaw/cron -run "TestListJobs_" -count=1`
Expected: FAIL because `ListJobs` does not yet merge latest run log data.

**Step 3: Write minimal implementation**

Implement a helper that resolves the latest run log entry for a job and overlays:
- `LastRunAtMs`
- `LastStatus`
- `LastError`

Only apply overlay when a latest log entry exists.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/openclaw/cron -run "TestListJobs_" -count=1`
Expected: PASS

### Task 2: Keep frontend display logic aligned with backend semantics

**Files:**
- Modify: `frontend/src/pages/openclaw-cron/OpenClawCronPage.vue`

**Step 1: Write the minimal UI adjustment**

Ensure the list view consistently interprets:
- `failed` => red alert icon + tooltip with reason
- `success` => green check icon
- empty/no run => grey clock icon

**Step 2: Verify no unnecessary API changes**

Keep `reloadAll()` using `OpenClawCronService.ListJobs()` only.

### Task 3: Verify end-to-end behavior

**Files:**
- No code changes required

**Step 1: Run focused backend verification**

Run: `go test ./internal/openclaw/cron -count=1`

**Step 2: Run focused frontend/static verification if available**

Run: `pnpm exec vue-tsc --noEmit`

Use this only if it is already available and scoped enough for the repo.
