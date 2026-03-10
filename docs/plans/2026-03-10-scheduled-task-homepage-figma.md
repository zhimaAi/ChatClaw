# Scheduled Task Homepage Figma Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让定时任务首页在不改业务逻辑的前提下尽量贴近 Figma 设计稿。

**Architecture:** 保留 `useScheduledTasks` 的数据获取与事件流，只重构页面壳层、统计卡片和表格展示组件。通过局部样式和结构调整完成视觉还原，避免影响创建任务、任务启停、运行记录等既有行为。

**Tech Stack:** Vue 3、TypeScript、Tailwind、Wails bindings、lucide-vue-next

---

### Task 1: 页面头部与文案

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/ScheduledTasksPage.vue`
- Modify: `frontend/src/locales/zh-CN.ts`
- Modify: `frontend/src/locales/en-US.ts`

**Step 1: 调整页面头部结构**

- 标题区改成 Figma 的主副标题结构。
- 右上新增 `刷新` 次按钮并绑定 `reloadAll`。
- 主按钮文案改成 `添加任务`，继续绑定 `openCreateDialog`。

**Step 2: 补充缺失文案**

- 新增副标题与刷新按钮文案。

**Step 3: 运行类型检查**

Run: `npx vue-tsc --noEmit`
Expected: 无新增类型错误

### Task 2: 统计卡片还原

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskSummaryCards.vue`

**Step 1: 重构卡片数据驱动结构**

- 增加图标、数字和标签的统一配置。

**Step 2: 按 Figma 调整卡片样式**

- 改为 16px 圆角、轻阴影、浅灰图标底。
- 优化桌面四列、窄屏自动换行。

**Step 3: 运行类型检查**

Run: `npx vue-tsc --noEmit`
Expected: 无新增类型错误

### Task 3: 列表表格还原

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`

**Step 1: 调整列表容器和表头**

- 移除额外内部标题栏。
- 将容器样式贴近 Figma 的大卡片观感。

**Step 2: 重排单元格信息层级**

- 任务列两行信息。
- 执行时间列改成主次两行。
- 状态列改成开关 + 文案。
- 操作列保留三点菜单。

**Step 3: 检查桌面与小屏兼容**

- 保持横向滚动可用，不出现内容重叠。

**Step 4: 运行构建验证**

Run: `npm run build`
Expected: 构建成功
