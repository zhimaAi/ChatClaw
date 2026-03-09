# 定时任务列表展示调整设计

## 目标

仅调整定时任务列表页面的展示方式，使其更接近目标稿，同时保持现有数据结构、接口和交互语义不变。

## 范围

- 调整表头顺序与列结构。
- 将“启用”能力并入“状态”列展示。
- 将“操作”按钮组改为右侧三点下拉菜单。
- 在“上次运行”列中补充成功/失败图标和失败原因提示。
- 保留现有“立即运行 / 历史记录 / 编辑 / 删除 / 启停切换”能力。

## 非目标

- 不新增批量选择、批量操作。
- 不修改后端接口、DTO、数据库字段。
- 不调整任务创建、编辑、运行、历史记录功能。

## 设计说明

### 表格结构

表头调整为：

1. 任务标题
2. 执行时间
3. 上次运行
4. 下次运行
5. 状态
6. 操作

任务标题列继续显示任务名和提示词摘要，但收敛为更紧凑的层次。

### 上次运行展示

- 成功：显示绿色状态图标 + 时间。
- 失败：显示红色状态图标 + 时间，并通过 tooltip 展示失败原因。
- 无记录：显示 `-`。

失败原因 tooltip 继续复用现有 `last_error` 数据，不新增字段。

### 状态列展示

状态列展示“文本状态 + 开关”：

- 已暂停：任务未启用。
- 进行中：上次状态为 `running`。
- 失败：上次状态为 `failed`。
- 成功：上次状态为 `success`。
- 待运行：其他情况。

开关仍然直接控制 `task.enabled`，仅调整视觉布局，不改变事件流。

### 操作列展示

用三点触发的下拉菜单替代当前横向按钮组，菜单项保留：

- 立即运行
- 历史记录
- 编辑
- 删除

其中“删除”保持危险操作的视觉强调。

## 实现位置

- `frontend/src/pages/scheduled-tasks/components/TaskTable.vue`
- `frontend/src/pages/scheduled-tasks/components/TaskRunStatusBadge.vue`
- `frontend/src/locales/zh-CN.ts`
- `frontend/src/locales/en-US.ts`

## 验证

由于当前前端未配置组件测试基建，本次以静态验证为主：

- `npm run lint -- src/pages/scheduled-tasks/components/TaskTable.vue src/pages/scheduled-tasks/components/TaskRunStatusBadge.vue src/locales/zh-CN.ts src/locales/en-US.ts`
- 如有需要，补充 `npm run build` 或 `npm run build:dev`
