# Scheduled Task Table Display Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 调整定时任务列表展示，使其采用新的列布局、状态列结构和三点菜单操作，同时不修改后端数据与行为。

**Architecture:** 仅在前端页面组件内完成展示重组。核心改动集中在 `TaskTable.vue`，通过复用现有任务事件和状态字段完成 UI 替换；必要时微调状态徽标组件与文案键值，但不扩散到后端。

**Tech Stack:** Vue 3、TypeScript、Tailwind、shadcn-vue/reka-ui、vue-i18n

---

### Task 1: 明确定义展示状态映射

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`
- Modify: `frontend/src/locales/zh-CN.ts`
- Modify: `frontend/src/locales/en-US.ts`

**Step 1: 写失败测试前确认测试策略**

当前仓库未配置前端组件测试基建。先与用户确认本次是否接受使用 `vue-tsc + eslint` 作为验证，避免为了展示改动引入超范围测试设施。

**Step 2: 补充缺失文案**

为表头、状态文字、错误提示补齐需要的 i18n 文案键，确保展示层不再依赖硬编码中文。

**Step 3: 运行静态校验**

Run: `npm run lint -- src/locales/zh-CN.ts src/locales/en-US.ts`

Expected: PASS

### Task 2: 重构任务表格展示

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskRunStatusBadge.vue`

**Step 1: 先让当前实现不满足目标稿**

通过对照设计文档，确认当前实现在以下方面不满足目标：
- “启用”仍是独立列
- “操作”仍是按钮组
- “上次运行”未内联图标和失败原因入口

**Step 2: 写最小实现**

- 合并“状态 + 开关”布局
- 改造“操作”为三点下拉菜单
- 重做“上次运行”单元格内容
- 让状态徽标输出业务文案，而不是原始状态码

**Step 3: 运行静态校验**

Run: `npm run lint -- src/pages/scheduled-tasks/components/TaskTable.vue src/pages/scheduled-tasks/components/TaskRunStatusBadge.vue`

Expected: PASS

### Task 3: 做最终验证

**Files:**
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`
- Modify: `frontend/src/pages/scheduled-tasks/components/TaskRunStatusBadge.vue`
- Modify: `frontend/src/locales/zh-CN.ts`
- Modify: `frontend/src/locales/en-US.ts`

**Step 1: 运行针对性 lint**

Run: `npm run lint -- src/pages/scheduled-tasks/components/TaskTable.vue src/pages/scheduled-tasks/components/TaskRunStatusBadge.vue src/locales/zh-CN.ts src/locales/en-US.ts`

Expected: PASS

**Step 2: 运行类型或构建验证**

Run: `npm run build:dev`

Expected: PASS

**Step 3: 整理结果**

记录本次仅为展示层调整，没有修改后端逻辑；若构建未执行或失败，需要明确说明原因。
