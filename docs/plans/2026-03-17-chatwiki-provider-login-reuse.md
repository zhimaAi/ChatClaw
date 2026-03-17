# ChatWiki Provider Login Reuse Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reuse the same ChatWiki Cloud browser-login flow for both the settings login button and the provider detail "立即登录" button.

**Architecture:** Extract the ChatWiki Cloud login URL builder and browser opener into a shared frontend helper under `src/lib`. Keep the existing custom/open-source login path inside `ChatwikiSettings.vue`, but route the Cloud login entry points through the same helper.

**Tech Stack:** Vue 3, TypeScript, Wails bindings, Node 24 `node:test`

---

### Task 1: Add a failing shared-login helper test

**Files:**
- Create: `frontend/src/lib/chatwikiAuth.test.ts`
- Create: `frontend/src/lib/chatwikiAuth.ts`

**Step 1: Write the failing test**

Cover:
- Cloud login URL normalizes trailing slash
- login params are appended when available
- browser open is delegated through one helper

**Step 2: Run test to verify it fails**

Run: `node --test src/lib/chatwikiAuth.test.ts`
Expected: FAIL because `chatwikiAuth.ts` does not exist yet.

**Step 3: Write minimal implementation**

Implement:
- `buildChatWikiLoginUrl`
- `openChatWikiCloudLogin`

**Step 4: Run test to verify it passes**

Run: `node --test src/lib/chatwikiAuth.test.ts`
Expected: PASS

### Task 2: Reuse helper in both UI entry points

**Files:**
- Modify: `frontend/src/pages/settings/components/ChatwikiSettings.vue`
- Modify: `frontend/src/pages/settings/components/ChatwikiProviderDetail.vue`

**Step 1: Replace duplicated Cloud login logic**

Route:
- `handleLoginCloud`
- provider detail “立即登录”

through the shared helper.

**Step 2: Keep existing errors and non-Cloud flows intact**

Only change the Cloud login entry points; preserve custom URL auth flow and existing toasts.

**Step 3: Run verification**

Run: `npm run build`
Expected: PASS
