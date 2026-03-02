## 知识库文件夹 & 文档详情功能开发计划

> 本文档用于记录本次功能的详细开发计划，避免因会话中断或 token 限制导致方案丢失。后续实现如有调整，请在此文同步更新。

---

## 1. 功能目标与范围

- **目标 1：知识库内支持文件夹分组**
  - 在单个知识库中引入“文件夹”维度，文档可归属到某个文件夹或“未分组”。
  - 支持文件夹的创建、重命名、删除，以及文档在文件夹之间移动。
- **目标 2：文档详情查看**
  - 在知识库中可打开单个文档的详情视图，查看基础信息、处理状态及统计信息。
  - 后续如需要，可以扩展为包含预览或更多元数据，但本期以信息展示为主。

> 说明：当前文档数据模型为“按 library_id 扁平存储”，本次改造在此基础上增加 folder 维度。**支持多级文件夹嵌套**（最多 10 层），通过 `parent_id` 字段实现树形结构。

---

## 2. 后端设计与开发任务（Go / Wails）

### 2.1 数据结构与 Migration

- **新增表 `library_folders`**
  - 字段建议：
    - `id`：主键，自增。
    - `created_at`, `updated_at`：时间字段，使用 `sqlite.NowUTC()` 统一维护。
    - `library_id`：所属知识库，外键指向 `libraries.id`（具体表名以现有实现为准），`ON DELETE CASCADE`。
    - `parent_id`：父文件夹 ID，可空，外键指向 `library_folders.id`，`NULL` 表示根文件夹。支持多级嵌套（最多 10 层）。
    - `name`：文件夹名称，限制长度（例如 50 字符），同一父文件夹下唯一（同一 `parent_id` 下唯一）。
    - `sort_order`：排序权重，整型，可为空，默认 0。
  - 索引：
    - `(library_id, sort_order)`：用于按库内排序。
    - `(library_id, parent_id, name)`：用于快速查重（同一父文件夹下名称唯一）。
    - `(parent_id)`：用于快速查找子文件夹。

- **在 `documents` 表中增加 `folder_id` 字段**
  - `folder_id`：可空，外键指向 `library_folders.id`，建议 `ON DELETE SET NULL`。
  - 现有文档数据默认 `NULL`，前端视作“未分组”。
  - 根据现有 ORM 模型 `documentModel`：
    - 在 `internal/services/document/model.go` 中新增字段：
      - `FolderID int64 \`bun:"folder_id"\``（或 `*int64`，视可空策略决定）。
    - 在 DTO `Document` 中同步增加 `FolderID` 字段。

- **Migration 编写**
  - 在 `internal/sqlite/migrations` 目录内新增 migration：
    - 创建表 `library_folders`。
    - 为 `documents` 表添加 `folder_id` 字段及外键约束。
  - 确认与 FTS/向量表逻辑兼容：
    - `doc_name_fts`、`doc_vec` 等表主要通过 `document_id` 关联，不需额外改造。

### 2.2 Folder 领域模型与 Service 接口

- **数据模型**
  - 在 `internal/services/library` 包中新建 `folder_model.go`（名称可调整）：
    - 定义数据库模型 `libraryFolderModel`，包含上述字段及 `BeforeInsert/BeforeUpdate` 钩子。
    - 定义 DTO `Folder`：
      - `ID`, `LibraryID`, `Name`, `SortOrder`, `CreatedAt`, `UpdatedAt` 等。
  - 添加 `toDTO()` 方法实现模型与 DTO 的转换。

- **对前端暴露的 Service 接口（Wails 服务）**
  - 可在现有 `LibraryService` 中扩展，也可新建 `FolderService`；建议保持与现有服务组织方式一致。
  - 预期接口：
    - `ListFolders(libraryID int64) ([]Folder, error)`
      - 入参：`libraryID` 必填。
      - 出参：该库下所有文件夹，按 `sort_order, id` 排序。
    - `CreateFolder(input CreateFolderInput) (*Folder, error)`
      - 入参包含：`LibraryID`, `ParentID`（可选），`Name`。
      - 校验：
        - `LibraryID > 0`，`Name` 去前后空格后非空。
        - 如果指定 `ParentID`，验证父文件夹存在且属于同一知识库。
        - **深度限制**：最多支持 10 层嵌套，超过限制返回 `error.folder_depth_exceeded`。
        - **循环引用检测**：创建时检查父文件夹链，防止循环引用。
        - 同一父文件夹下名称唯一（`WHERE library_id = ? AND parent_id = ? AND name = ?`）。
      - 失败时返回统一错误码（通过 `errs.New/Wrap`），便于前端提示。
    - `RenameFolder(input RenameFolderInput) (*Folder, error)`
      - 根据 `ID` 查找并更新 `Name`，同样做名称唯一性校验。
    - `DeleteFolder(input DeleteFolderInput) error`
      - 最少支持一种策略：删除文件夹但**保留文档**，将该文件夹下文档的 `folder_id` 设为 `NULL`。
      - `DeleteFolderInput` 中保留扩展字段 `Mode`，未来如需支持"连文档一并删除"可复用。
      - 实现方式：
        - 使用事务，递归删除所有子文件夹：
          - 递归查找并删除所有子文件夹（`WHERE parent_id = ?`）
          - `UPDATE documents SET folder_id = NULL WHERE folder_id = ?`（对每个文件夹及其子文件夹）
          - `DELETE FROM library_folders WHERE id = ?`（对每个文件夹及其子文件夹）
    - `MoveDocumentToFolder(docID int64, folderID *int64) error`
      - `folderID == nil` 或 `0` 表示移到“未分组”。
      - 校验：
        - 文档存在且属于目标库。
        - 非空 `folderID` 时对应文件夹存在且同属同一 `library_id`。
      - 更新 `documents.folder_id` 即可，无需影响 FTS/向量数据。

### 2.3 文档列表与详情接口扩展

- **分页列表 `ListDocumentsPage` 支持按文件夹过滤**
  - 在 `internal/services/document/model.go` 中：
    - 为 `ListDocumentsPageInput` 新增字段：
      - `FolderID int64 \`json:"folder_id"\``（0 表示“不过滤”，或前后端约定 `-1` 表示“未分组”）。
  - 在 `internal/services/document/service.go` 的 `ListDocumentsPage` 实现中：
    - 在无关键词分支，`q := db.NewSelect().Model(&models).Where("d.library_id = ?", input.LibraryID)` 之后：
      - 如 `input.FolderID > 0`，追加 `Where("d.folder_id = ?", input.FolderID)`。
      - 若前后端约定 `FolderID == -1` 表示“仅未分组”，则可 `Where("d.folder_id IS NULL")`。
  - `ListDocuments`（非分页版本）根据需求可选是否同步增加 `folder` 过滤；如前端不再使用可保持现状。

- **文档详情接口（可选）**
  - 如果前端仅需 `Document` DTO 中已有字段（如 `FileSize`, `SourceType`, `WordTotal`, `SplitTotal` 等），可以直接复用列表数据，在前端保存当前选中项即可。
  - 若需要：
    - 更多 DB 字段；
    - 或从其他表（如 `document_nodes`）汇总额外统计；
    - 则增加接口：`GetDocument(id int64) (*Document, error)`：
      - 查询并返回单条记录，沿用 `errs` 体系（`error.document_not_found` 等）。

---

## 3. 前端设计与开发任务（Vue 3 / shadcn-vue）

> 需遵守项目前端架构规范（Vue 3 + script setup + TypeScript + Tailwind + shadcn-vue），SVG 图标使用 `currentColor`，Toast 使用 `bg-popover` + 黑白灰科技风样式。

### 3.1 文件夹 UI 与交互设计

- **页面位置**
  - 在现有知识库内容区域组件（例如 `KnowledgePage` / `LibraryContentArea`，具体文件名以实际为准）中扩展：
    - 右侧顶部区域通常包含：标题 / 搜索 / 上传按钮等。
    - 在此区域增加“文件夹选择器 + 管理入口”。

- **文件夹选择器（Folder Select）**
  - 新增组件示例：`FolderSelect.vue`：
    - 属性：
      - `libraryId: number` 当前知识库 ID。
      - `folders: Folder[]` 文件夹列表。
      - `modelValue: number | null` 当前选中文件夹 ID（`null` 或特殊值表示“全部/未分组”）。
    - 事件：
      - `update:modelValue`：切换选中文件夹。
      - `create-folder` / `rename-folder` / `delete-folder`：向父组件抛出管理操作。
    - 交互：
      - 下拉或菜单中固定展示：
        - “全部文件”（可选，取决于产品定义是否保留该选项）。
        - “未分组”。
        - 其余文件夹列表。
      - 提供“新建文件夹”入口（按钮或菜单项）。

- **文件夹管理弹窗**
  - 新增（或复用）对话框组件，例如：
    - `FolderCreateDialog.vue`：输入名称并调用 `CreateFolder` 接口。
    - `FolderRenameDialog.vue`：重命名。
    - 删除操作可使用统一的 `AlertDialog` 提示：
      - 明确提示：删除文件夹会将其下文档移动到“未分组”，不会删除文档本身（与后端行为保持一致）。

- **文档与文件夹的关系操作**
  - 在文档卡片组件（例如 `DocumentCard.vue`）的菜单中增加：
    - “移动到文件夹”菜单项，点击后：
      - 弹出文件夹选择对话框（展示当前库下所有文件夹 + “未分组”）。
      - 用户选择目标文件夹后，调用 `MoveDocumentToFolder` 接口。
      - 成功后：
        - 如当前列表按某个文件夹过滤，且目标文件夹与当前不同，则从当前列表中移除该文档。
        - 如移动到当前文件夹，则更新卡片状态即可。
  - 可选增强：支持拖拽（Drag & Drop）在列表与文件夹之间移动，优先级可低于基础交互。

### 3.2 状态管理与数据流调整

- **新增响应式状态**
  - 在知识库内容主组件中增加：
    - `const folders = ref<Folder[]>([])`
    - `const activeFolderId = ref<number | null>(null)`（`null` 或约定值表示未过滤）。

- **数据加载逻辑**
  - 在切换 `libraryId` 时：
    - 已有逻辑会重新加载文档列表；在此基础上：
      - 调用后端 `ListFolders(libraryID)` 填充 `folders`。
      - 将 `activeFolderId` 重置为默认值（例如“全部文件”或“未分组”）。
  - 在调用 `ListDocumentsPage` / 文档搜索方法时：
    - 请求参数中增加 `folder_id`（由 `activeFolderId` 决定）。
    - 注意兼容“关键字搜索”场景：
      - 若关键字非空时仍需要按文件夹过滤，则在后端的 FTS 查询中增加 `folder_id` 条件；
      - 如果产品定义“搜索跨所有文件夹”，可在搜索时忽略 `folder_id`。

- **文件夹列表的本地更新**
  - 创建/重命名/删除文件夹成功后：
    - 直接更新本地的 `folders` 列表，无需重新拉取全部数据。
  - 对于 `MoveDocumentToFolder`：
    - 本地列表中同步更新 `document.folder_id`。
    - 如果当前过滤条件与目标文件夹不符，则从当前 `documents` 列表中移除该条目。

### 3.3 文档详情视图设计

- **触发与展现方式**
  - 推荐使用右侧抽屉（Drawer）或中心对话框（Dialog），与项目现有交互风格保持一致。
  - 触发方式可选：
    - 在 `DocumentCard` 上增加“查看详情”菜单项；
    - 或单击卡片打开详情（若当前单击已有重要行为，则保持在菜单中）。

- **详情内容结构（初稿）**
  - **头部**
    - 文档名称（可复用重命名入口）。
    - 所属文件夹 & 知识库信息。
  - **基础信息区块**
    - 文件名（原始名）。
    - 文件类型（扩展名 / MIME）。
    - 文件大小（人类可读格式）。
    - 上传时间（`CreatedAt`）。
    - 来源：`SourceType`（local / web），以及 `WebURL`（如有）。
    - 文件状态：本地文件是否存在（`FileMissing`）。
  - **处理状态区块**
    - 解析状态：`ParsingStatus` + `ParsingProgress` + 错误消息。
    - 嵌入状态：`EmbeddingStatus` + `EmbeddingProgress` + 错误消息。
    - 统一使用项目已有的状态枚举和样式组件（例如状态标签、进度条）。
  - **统计信息区块**
    - 单词数：`WordTotal`。
    - 分片数：`SplitTotal`。
  - **操作区块**
    - “重新学习文档”（调用 `ReprocessDocument`）。
    - “移动到文件夹”。
    - “重命名”。
    - “删除文档”（带确认弹窗）。

- **数据获取策略**
  - 优先复用列表中已有的 `Document` 数据：
    - 将选中的 `Document` 透传给详情组件；
    - 详情中仅在需要时（例如展示更详细信息）再通过 `GetDocument` 接口补全。
  - 如后端暂不提供 `GetDocument`，则详情视图基于当前 DTO 可展示的字段实现第一版。

### 3.4 i18n 与视觉规范

- **i18n Key 建议**
  - 文件夹相关：
    - `knowledge.folder.all`
    - `knowledge.folder.uncategorized`
    - `knowledge.folder.create`
    - `knowledge.folder.rename`
    - `knowledge.folder.delete.title`
    - `knowledge.folder.delete.description`
  - 文档详情相关：
    - `knowledge.detail.title`
    - `knowledge.detail.basicInfo`
    - `knowledge.detail.processingInfo`
    - `knowledge.detail.stats`
    - `knowledge.detail.field.filename`
    - `knowledge.detail.field.size`
    - `knowledge.detail.field.type`
    - `knowledge.detail.field.createdAt`
    - `knowledge.detail.field.source`
    - `knowledge.detail.field.folder`
    - `knowledge.detail.field.wordTotal`
    - `knowledge.detail.field.splitTotal`
    - `knowledge.detail.status.parsing`
    - `knowledge.detail.status.embedding`

- **视觉规范注意事项**
  - 图标：
    - SVG 中禁止写死颜色，使用 `stroke="currentColor"` / `fill="currentColor"`。
    - 颜色由使用处通过 Tailwind 语义色控制，如 `text-muted-foreground`。
  - Toast：
    - 使用统一底色：`bg-popover text-popover-foreground border-border`。
    - 区分状态以图标或细边框为主，避免大面积彩色背景。

---

## 4. TAPD / 任务拆分建议

> 下面是建议拆分到 TAPD 的任务列表，便于跟踪与协作。

- **后端**
  - 「后端」知识库文件夹数据模型 & Migration（`library_folders` + `documents.folder_id`）
  - 「后端」文件夹 CRUD 接口（列表/创建/重命名/删除/移动文档）
  - 「后端」文档列表按文件夹过滤 & 文档详情接口（如需要）

- **前端**
  - 「前端」知识库页面文件夹 UI 与交互（文件夹选择、新建/重命名/删除/移动文档）
  - 「前端」文档详情抽屉/对话框实现（含进度、统计及操作入口）
  - 「前端」i18n 文案与视觉规范接入（图标/Toast 等）

- **测试与联调**
  - 「联调」多知识库、多文件夹、多文档场景下的完整流程验证
  - 「测试」兼容性与回归测试：升级前数据、失败重试、删除文件夹与删除文档等边界场景

---

## 5. 性能优化与安全措施

### 5.1 性能优化

- **前端优化**：
  - 使用 `Map<number, Folder>` 缓存文件夹查找，将 `findFolderById` 的时间复杂度从 O(n) 优化到 O(1)。
  - 文件夹列表变化时自动更新缓存，避免重复递归查找。
  - `displayFolders` computed 属性使用缓存快速获取当前文件夹的子文件夹。

- **后端优化**：
  - 一次性加载知识库下所有文件夹并构建树形结构，减少数据库查询次数。
  - 使用 `folderMap` 在内存中快速查找父文件夹，构建树形结构的时间复杂度为 O(n)。
  - 删除文件夹时使用递归删除所有子文件夹，确保数据一致性。

### 5.2 安全措施

- **深度限制**：
  - 创建文件夹时检查深度，最多支持 10 层嵌套。
  - 超过限制返回错误 `error.folder_depth_exceeded`。

- **循环引用检测**：
  - 创建文件夹时检查父文件夹链，防止循环引用。
  - `ListFolders` 构建树形结构时检测并跳过存在循环引用的文件夹，避免构建错误树形结构。

- **数据一致性**：
  - 删除文件夹时递归删除所有子文件夹，并将子文件夹下的文档移到"未分组"。
  - 使用数据库事务确保操作的原子性。

## 6. 后续可能的扩展方向（非本期范围）

- 文档内容预览（PDF 预览、文本片段预览等）。
- 按文件夹维度的统计视图（文档数、总 Tokens、最近更新时间等）。
- 文件夹级别的批量操作（批量重新学习、批量删除等）。
- 文件夹拖拽排序功能。
- 文件夹搜索功能。

