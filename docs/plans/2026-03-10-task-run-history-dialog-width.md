# Task Run History Dialog Width Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将运行记录弹窗整体宽度增加 200px，并让左侧列表的时间展示不换行且字号更小。

**Architecture:** 本次仅调整 `TaskRunHistoryDialog.vue` 的样式类，不改动组件状态、数据流或子组件接口。通过修改弹窗容器宽度和时间文本的 Tailwind 类完成需求，控制影响范围在单文件内。

**Tech Stack:** Vue 3, TypeScript, Tailwind utility classes

---

### Task 1: 调整运行记录弹窗样式

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskRunHistoryDialog.vue`

**Step 1: 确认当前样式入口**

查看 `DialogContent` 的 `max-width` 和左侧时间文本的字号类，确保只修改需求涉及的位置。

**Step 2: 修改弹窗宽度**

将 `sm:max-w-[1400px]` 调整为 `sm:max-w-[1600px]`。

**Step 3: 修改左侧时间展示**

将时间文本从 `text-sm` 调整为 `text-xs`，并增加 `whitespace-nowrap` 防止换行。

**Step 4: 检查变更**

核对 diff，确认只有目标文件和计划文档被修改，没有带出无关格式化。

**Step 5: 验证**

运行针对性搜索或 diff 检查，确认类名变更已生效。
