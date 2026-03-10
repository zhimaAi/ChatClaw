# Schedule Task Modal Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让定时任务新建/编辑弹窗尽量贴近参考图，同时修复中文乱码并保持现有创建逻辑不变。

**Architecture:** 保持 `ScheduledTasksPage.vue` 到 `CreateTaskDialog.vue` 的数据流不变，只在弹窗组件内部重构展示结构，并在 `constants.ts`、`utils.ts` 修复与调度显示相关的中文文案。由于当前前端包没有现成测试框架，本次以最小范围的类型检查和生产构建作为验证基线。

**Tech Stack:** Vue 3、TypeScript、Tailwind CSS、shadcn-vue dialog/input/switch、Vite

---

### Task 1: 修复调度文案基础数据

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/constants.ts`
- Modify: `frontend/src/pages/scheduled-tasks/utils.ts`

**Step 1: 先修正文案常量**

把预设时间和星期标签改为正常中文，保证弹窗和列表描述共用统一文案。

**Step 2: 修复自定义时间描述输出**

更新 `describeSchedule` 中的中文拼接结果，避免列表中继续显示乱码。

**Step 3: 运行类型检查**

Run: `npm run build`
Workdir: `frontend`
Expected: 通过 `vue-tsc`，构建成功或只暴露与本次改动无关的问题。

### Task 2: 重构创建任务弹窗视觉结构

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/CreateTaskDialog.vue`

**Step 1: 调整弹窗壳层**

重做标题区、副标题、关闭按钮位置、内容区宽度和底部按钮布局。

**Step 2: 重做表单输入区**

统一任务名称、提示词、助手选择器的样式、间距和 placeholder 文案。

**Step 3: 重做时间设置区**

把三种调度模式改为贴近设计稿的切换按钮，同时保留原有字段绑定。

**Step 4: 重做启用状态区**

将启用开关区域改为左说明右开关的结构，并补充描述文案。

**Step 5: 运行构建验证**

Run: `npm run build`
Workdir: `frontend`
Expected: 构建通过。

### Task 3: 回归检查

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/CreateTaskDialog.vue`
- Modify: `frontend/src/pages/scheduled-tasks/constants.ts`
- Modify: `frontend/src/pages/scheduled-tasks/utils.ts`

**Step 1: 检查创建与编辑模式文案**

确认标题、按钮和字段文案在两种模式下都合理。

**Step 2: 检查调度模式切换完整性**

确认 `preset / custom / cron` 仍然驱动原有表单字段。

**Step 3: 最终验证**

Run: `npm run build`
Workdir: `frontend`
Expected: 构建成功。
