# Scheduled Task History Iframe Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the in-dialog embedded assistant component with a real iframe-backed history conversation page.

**Architecture:** Add a new Vite multi-page entry dedicated to scheduled-task run conversations, then update the history dialog to render an iframe URL for the selected run. Reuse assistant UI pieces selectively inside the new iframe app, but do not mount the existing full `AssistantPage.vue` tree there.

**Tech Stack:** Vue 3, Pinia, Vite multi-page build, Wails runtime bindings, TypeScript

---

### Task 1: Add the iframe page entry

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\vite.config.ts`
- Create: `D:\willchat\willchat-client\frontend\history-run.html`
- Create: `D:\willchat\willchat-client\frontend\src\history-run\main.ts`
- Create: `D:\willchat\willchat-client\frontend\src\history-run\App.vue`

**Step 1: Write the failing test**

Use type/build verification as the red test:

```powershell
pnpm --dir frontend build:dev
```

Expected before implementation:

- the build output does not contain a `history-run` entry page

**Step 2: Run the failing verification**

Run:

```powershell
pnpm --dir frontend build:dev
```

Expected:

- build completes without `history-run.html` in output, confirming the new entry does not exist yet

**Step 3: Write the minimal implementation**

- add `history-run: "history-run.html"` to Vite rollup input
- create a minimal `history-run.html`
- bootstrap a standalone Vue app with Pinia and i18n
- mount a placeholder `App.vue`

**Step 4: Run verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
pnpm --dir frontend build:dev
```

Expected:

- TypeScript passes
- build output includes `history-run.html`

### Task 2: Build the history-run iframe app shell

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\src\history-run\App.vue`
- Create: `D:\willchat\willchat-client\frontend\src\history-run\utils.ts`

**Step 1: Write the failing test**

Define the behavior to implement:

- invalid or missing `conversationId` shows an inline error state
- valid params produce a shell with a conversation container

Verification target:

```powershell
pnpm --dir frontend exec vue-tsc
```

Expected before implementation:

- type errors or missing imports for the new shell logic

**Step 2: Run the failing verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
```

**Step 3: Write the minimal implementation**

- parse `conversationId` and `agentId` from `window.location.search`
- add constants and comments for query-key strings
- render:
  - loading state
  - invalid-param state
  - shell container for assistant content

**Step 4: Run verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
```

Expected:

- TypeScript passes

### Task 3: Load conversation data inside the iframe app

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\src\history-run\App.vue`
- Reuse/inspect: `D:\willchat\willchat-client\frontend\src\stores\chat.ts`
- Reuse/inspect: `D:\willchat\willchat-client\frontend\frontend\bindings\chatclaw\internal\services\conversations`

**Step 1: Write the failing test**

Behavior to verify:

- a valid conversation ID loads conversation metadata and messages
- a missing conversation shows an inline error state

Verification command:

```powershell
pnpm --dir frontend exec vue-tsc
```

Expected before implementation:

- missing load state fields or unresolved service calls

**Step 2: Run the failing verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
```

**Step 3: Write the minimal implementation**

- call `ConversationsService.GetConversation`
- load messages through `chatStore.loadMessages`
- render `ChatMessageList` using the loaded conversation id
- ensure the iframe app subscribes/unsubscribes chat store safely

**Step 4: Run verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
```

Expected:

- TypeScript passes

### Task 4: Add complete assistant-style input and sending inside the iframe

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\src\history-run\App.vue`
- Reuse/inspect: `D:\willchat\willchat-client\frontend\src\pages\assistant\components\ChatInputArea.vue`
- Reuse/inspect: `D:\willchat\willchat-client\frontend\src\pages\assistant\composables\useModelSelection.ts`

**Step 1: Write the failing test**

Behavior to verify:

- the iframe page renders a full assistant-like input area
- send is disabled when agent/model/content requirements are not met
- send triggers message dispatch for the current conversation only

Verification command:

```powershell
pnpm --dir frontend exec vue-tsc
```

Expected before implementation:

- missing props/state needed to render the assistant input

**Step 2: Run the failing verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
```

**Step 3: Write the minimal implementation**

- load the current agent and current conversation model context
- render `ChatInputArea` with iframe-local state
- wire `send` to `chatStore.sendMessage`
- keep constants extracted for query keys and iframe title text
- add concise comments to non-obvious control flow

**Step 4: Run verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
```

Expected:

- TypeScript passes

### Task 5: Switch the scheduled-task history dialog to a real iframe

**Files:**
- Modify: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\TaskRunHistoryDialog.vue`
- Delete: `D:\willchat\willchat-client\frontend\src\pages\assistant\components\EmbeddedAssistantPage.vue`

**Step 1: Write the failing test**

Behavior to verify:

- selected run changes iframe URL
- dialog no longer mounts the nested assistant page

Verification target:

```powershell
pnpm --dir frontend exec vue-tsc
```

Expected before implementation:

- existing code still imports `EmbeddedAssistantPage`

**Step 2: Run the failing verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
```

**Step 3: Write the minimal implementation**

- remove `EmbeddedAssistantPage`
- compute a relative `history-run.html` URL from the selected run detail
- render an `iframe` with:
  - `class="h-full w-full border-0"`
  - a stable `key`
  - descriptive `title`
- keep the parent dialog state management unchanged

**Step 4: Run verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
pnpm --dir frontend build:dev
```

Expected:

- TypeScript passes
- build passes with the iframe entry

### Task 6: Manual regression verification

**Files:**
- Modify if needed: `D:\willchat\willchat-client\frontend\src\pages\scheduled-tasks\components\TaskRunHistoryDialog.vue`
- Modify if needed: `D:\willchat\willchat-client\frontend\src\history-run\App.vue`

**Step 1: Verify the dialog interaction manually**

Check:

1. Open task history.
2. Click different run items repeatedly.
3. Close the dialog.
4. Reopen the dialog.

Expected:

- no duplicate dialog reopens
- run switching remains responsive

**Step 2: Verify iframe conversation interaction manually**

Check:

1. Scroll inside the right panel.
2. Type and send a message inside the iframe assistant.
3. Return to the left run list and switch again.

Expected:

- parent dialog remains stable
- iframe interaction does not block the left run list

**Step 3: Final verification**

Run:

```powershell
pnpm --dir frontend exec vue-tsc
pnpm --dir frontend build:dev
```

Expected:

- all checks pass cleanly
