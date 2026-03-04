package library

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"chatclaw/internal/errs"
)

// ListFolders 获取知识库下的所有文件夹（返回树形结构）
func (s *LibraryService) ListFolders(libraryID int64) ([]Folder, error) {
	if libraryID <= 0 {
		return nil, errs.New("error.library_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]libraryFolderModel, 0)
	if err := db.NewSelect().
		Model(&models).
		Where("library_id = ?", libraryID).
		OrderExpr("sort_order ASC, id ASC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.folder_list_failed", err)
	}

	// Convert to DTO pointers (do NOT build the final tree by appending value copies).
	// We build parent->children relationships using pointers, then materialize a value tree recursively.
	folderMap := make(map[int64]*Folder, len(models))
	all := make([]*Folder, 0, len(models))
	for i := range models {
		dto := models[i].toDTO()
		f := dto // create a distinct variable so we can take its address safely
		folderMap[f.ID] = &f
		all = append(all, &f)
	}

	// Build parent -> children pointers (with cycle detection).
	childrenMap := make(map[int64][]*Folder)

	var checkCycle func(folderID int64, targetID int64, depth int) bool
	checkCycle = func(folderID int64, targetID int64, depth int) bool {
		if depth > 100 { // prevent infinite loops in corrupted data
			return true
		}
		if folderID == targetID {
			return true
		}
		if folder, ok := folderMap[folderID]; ok && folder.ParentID != nil {
			return checkCycle(*folder.ParentID, targetID, depth+1)
		}
		return false
	}

	for _, f := range all {
		if f.ParentID == nil {
			continue
		}
		parentID := *f.ParentID
		// self reference
		if parentID == f.ID {
			continue
		}
		// cycle reference
		if checkCycle(parentID, f.ID, 0) {
			continue
		}
		if _, ok := folderMap[parentID]; !ok {
			// parent missing (corrupted data), skip attaching
			continue
		}
		childrenMap[parentID] = append(childrenMap[parentID], f)
	}

	// Sort children under each parent (stable, deterministic).
	for pid, items := range childrenMap {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].SortOrder != items[j].SortOrder {
				return items[i].SortOrder < items[j].SortOrder
			}
			return items[i].ID < items[j].ID
		})
		childrenMap[pid] = items
	}

	// Collect and sort roots.
	roots := make([]*Folder, 0)
	for _, f := range all {
		if f.ParentID == nil {
			roots = append(roots, f)
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].SortOrder != roots[j].SortOrder {
			return roots[i].SortOrder < roots[j].SortOrder
		}
		return roots[i].ID < roots[j].ID
	})

	// Materialize a value tree recursively so that nested children are preserved.
	var buildTree func(src *Folder, depth int) Folder
	buildTree = func(src *Folder, depth int) Folder {
		if src == nil {
			return Folder{}
		}
		// Copy node value.
		out := *src
		out.Children = nil

		if depth > 100 {
			return out
		}

		children := childrenMap[src.ID]
		if len(children) == 0 {
			out.Children = []Folder{}
			return out
		}

		out.Children = make([]Folder, 0, len(children))
		for _, ch := range children {
			out.Children = append(out.Children, buildTree(ch, depth+1))
		}
		return out
	}

	result := make([]Folder, 0, len(roots))
	for _, r := range roots {
		result = append(result, buildTree(r, 0))
	}
	return result, nil
}

// CreateFolder 创建文件夹
func (s *LibraryService) CreateFolder(input CreateFolderInput) (*Folder, error) {
	if input.LibraryID <= 0 {
		return nil, errs.New("error.library_id_required")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.folder_name_required")
	}
	if len([]rune(name)) > 50 {
		return nil, errs.New("error.folder_name_too_long")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 检查知识库是否存在
	var libraryCount int
	if err := db.NewSelect().
		Table("library").
		ColumnExpr("COUNT(1)").
		Where("id = ?", input.LibraryID).
		Scan(ctx, &libraryCount); err != nil {
		return nil, errs.Wrap("error.folder_create_failed", err)
	}
	if libraryCount == 0 {
		return nil, errs.Newf("error.library_not_found", map[string]any{"ID": input.LibraryID})
	}

	// 如果指定了父文件夹，验证父文件夹存在且属于同一知识库，并检查深度和循环引用
	if input.ParentID != nil && *input.ParentID > 0 {
		var parent libraryFolderModel
		if err := db.NewSelect().Model(&parent).
			Where("id = ?", *input.ParentID).
			Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errs.Newf("error.folder_not_found", map[string]any{"ID": *input.ParentID})
			}
			return nil, errs.Wrap("error.folder_create_failed", err)
		}
		if parent.LibraryID != input.LibraryID {
			return nil, errs.New("error.folder_parent_library_mismatch")
		}

		// 检查文件夹深度（最多支持 10 层嵌套）
		const maxDepth = 10
		depth := 1
		currentParentID := parent.ParentID
		for currentParentID != nil && *currentParentID > 0 {
			depth++
			if depth > maxDepth {
				return nil, errs.New("error.folder_depth_exceeded")
			}
			var currentParent libraryFolderModel
			if err := db.NewSelect().Model(&currentParent).
				Where("id = ?", *currentParentID).
				Scan(ctx); err != nil {
				// 如果查询失败，可能是数据不一致，但为了安全起见，停止检查
				break
			}
			currentParentID = currentParent.ParentID
		}
	}

	// 检查同一父文件夹下名称是否唯一
	var nameCount int
	query := db.NewSelect().
		Table("library_folders").
		ColumnExpr("COUNT(1)").
		Where("library_id = ?", input.LibraryID).
		Where("name = ?", name)
	if input.ParentID != nil && *input.ParentID > 0 {
		query = query.Where("parent_id = ?", *input.ParentID)
	} else {
		query = query.Where("parent_id IS NULL")
	}
	if err := query.Scan(ctx, &nameCount); err != nil {
		return nil, errs.Wrap("error.folder_create_failed", err)
	}
	if nameCount > 0 {
		return nil, errs.Newf("error.folder_name_duplicate", map[string]any{"Name": name})
	}

	// 获取当前父文件夹下最大的 sort_order
	var maxSort sql.NullInt64
	sortQuery := db.NewSelect().
		Table("library_folders").
		ColumnExpr("MAX(sort_order)").
		Where("library_id = ?", input.LibraryID)
	if input.ParentID != nil && *input.ParentID > 0 {
		sortQuery = sortQuery.Where("parent_id = ?", *input.ParentID)
	} else {
		sortQuery = sortQuery.Where("parent_id IS NULL")
	}
	if err := sortQuery.Scan(ctx, &maxSort); err != nil {
		return nil, errs.Wrap("error.folder_create_failed", err)
	}
	sortOrder := 0
	if maxSort.Valid {
		sortOrder = int(maxSort.Int64) + 1
	}

	m := &libraryFolderModel{
		LibraryID: input.LibraryID,
		ParentID:  input.ParentID,
		Name:      name,
		SortOrder: sortOrder,
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.folder_create_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// RenameFolder 重命名文件夹
func (s *LibraryService) RenameFolder(input RenameFolderInput) (*Folder, error) {
	if input.ID <= 0 {
		return nil, errs.New("error.folder_id_required")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.folder_name_required")
	}
	if len([]rune(name)) > 50 {
		return nil, errs.New("error.folder_name_too_long")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 查询文件夹信息
	var m libraryFolderModel
	if err := db.NewSelect().Model(&m).Where("id = ?", input.ID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.folder_not_found", map[string]any{"ID": input.ID})
		}
		return nil, errs.Wrap("error.folder_read_failed", err)
	}

	// 检查同一父文件夹下名称是否唯一（排除当前文件夹）
	var nameCount int
	query := db.NewSelect().
		Table("library_folders").
		ColumnExpr("COUNT(1)").
		Where("library_id = ?", m.LibraryID).
		Where("name = ?", name).
		Where("id != ?", input.ID)
	if m.ParentID != nil && *m.ParentID > 0 {
		query = query.Where("parent_id = ?", *m.ParentID)
	} else {
		query = query.Where("parent_id IS NULL")
	}
	if err := query.Scan(ctx, &nameCount); err != nil {
		return nil, errs.Wrap("error.folder_rename_failed", err)
	}
	if nameCount > 0 {
		return nil, errs.Newf("error.folder_name_duplicate", map[string]any{"Name": name})
	}

	// 更新名称
	m.Name = name
	if _, err := db.NewUpdate().Model(&m).
		Column("name", "updated_at").
		Where("id = ?", input.ID).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.folder_rename_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// DeleteFolder 删除文件夹（保留文档，将文档的 folder_id 设为 NULL）
func (s *LibraryService) DeleteFolder(input DeleteFolderInput) error {
	if input.ID <= 0 {
		return errs.New("error.folder_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 查询文件夹是否存在
	var m libraryFolderModel
	if err := db.NewSelect().Model(&m).Where("id = ?", input.ID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.Newf("error.folder_not_found", map[string]any{"ID": input.ID})
		}
		return errs.Wrap("error.folder_read_failed", err)
	}

	// 使用事务：先更新文档的 folder_id，再删除子文件夹，最后删除当前文件夹
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errs.Wrap("error.folder_delete_failed", err)
	}
	defer tx.Rollback()

	// 递归删除所有子文件夹下的文档（移到"未分组"）
	var deleteSubFolders func(folderID int64) error
	deleteSubFolders = func(folderID int64) error {
		// 获取所有子文件夹
		var subFolders []libraryFolderModel
		if err := tx.NewSelect().
			Model(&subFolders).
			Where("parent_id = ?", folderID).
			Scan(ctx); err != nil {
			return err
		}

		// 递归处理子文件夹
		for _, subFolder := range subFolders {
			if err := deleteSubFolders(subFolder.ID); err != nil {
				return err
			}
		}

		// 将该文件夹下的文档移到"未分组"（folder_id = NULL）
		if _, err := tx.NewUpdate().
			Table("documents").
			Set("folder_id = NULL").
			Where("folder_id = ?", folderID).
			Exec(ctx); err != nil {
			return err
		}

		// 删除子文件夹
		if _, err := tx.NewDelete().
			Table("library_folders").
			Where("parent_id = ?", folderID).
			Exec(ctx); err != nil {
			return err
		}

		return nil
	}

	// 删除所有子文件夹
	if err := deleteSubFolders(input.ID); err != nil {
		return errs.Wrap("error.folder_delete_failed", err)
	}

	// 将该文件夹下的文档移到"未分组"（folder_id = NULL）
	if _, err := tx.NewUpdate().
		Table("documents").
		Set("folder_id = NULL").
		Where("folder_id = ?", input.ID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.folder_delete_failed", err)
	}

	// 删除文件夹
	if _, err := tx.NewDelete().Model(&m).Where("id = ?", input.ID).Exec(ctx); err != nil {
		return errs.Wrap("error.folder_delete_failed", err)
	}

	if err := tx.Commit(); err != nil {
		return errs.Wrap("error.folder_delete_failed", err)
	}

	return nil
}

// MoveFolder 移动文件夹到其他文件夹
func (s *LibraryService) MoveFolder(input MoveFolderInput) (*Folder, error) {
	if input.ID <= 0 {
		return nil, errs.New("error.folder_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 查询要移动的文件夹信息
	var folder libraryFolderModel
	if err := db.NewSelect().Model(&folder).Where("id = ?", input.ID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.folder_not_found", map[string]any{"ID": input.ID})
		}
		return nil, errs.Wrap("error.folder_read_failed", err)
	}

	// 如果目标 parent_id 不为空，验证目标文件夹存在且属于同一知识库
	if input.ParentID != nil && *input.ParentID > 0 {
		var targetFolder libraryFolderModel
		if err := db.NewSelect().Model(&targetFolder).
			Where("id = ?", *input.ParentID).
			Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errs.Newf("error.folder_not_found", map[string]any{"ID": *input.ParentID})
			}
			return nil, errs.Wrap("error.folder_read_failed", err)
		}

		// 验证目标文件夹属于同一知识库
		if targetFolder.LibraryID != folder.LibraryID {
			return nil, errs.New("error.folder_library_mismatch")
		}

		// 检查是否会造成循环引用：不能移动到自己的子文件夹中
		var checkCycle func(folderID int64, targetID int64, depth int) bool
		checkCycle = func(folderID int64, targetID int64, depth int) bool {
			if depth > 100 { // prevent infinite loops
				return true
			}
			if folderID == targetID {
				return true
			}
			var parent libraryFolderModel
			if err := db.NewSelect().Model(&parent).Where("id = ?", folderID).Scan(ctx); err != nil {
				return false
			}
			if parent.ParentID != nil && *parent.ParentID > 0 {
				return checkCycle(*parent.ParentID, targetID, depth+1)
			}
			return false
		}

		if checkCycle(*input.ParentID, input.ID, 0) {
			return nil, errs.New("error.folder_move_cycle_detected")
		}
	}

	// 检查目标位置是否已有同名文件夹
	var nameCount int
	query := db.NewSelect().
		Table("library_folders").
		ColumnExpr("COUNT(1)").
		Where("library_id = ?", folder.LibraryID).
		Where("name = ?", folder.Name).
		Where("id != ?", input.ID)
	if input.ParentID != nil && *input.ParentID > 0 {
		query = query.Where("parent_id = ?", *input.ParentID)
	} else {
		query = query.Where("parent_id IS NULL")
	}
	if err := query.Scan(ctx, &nameCount); err != nil {
		return nil, errs.Wrap("error.folder_move_failed", err)
	}
	if nameCount > 0 {
		return nil, errs.Newf("error.folder_name_duplicate", map[string]any{"Name": folder.Name})
	}

	// 更新文件夹的 parent_id
	folder.ParentID = input.ParentID
	if _, err := db.NewUpdate().Model(&folder).
		Column("parent_id", "updated_at").
		Where("id = ?", input.ID).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.folder_move_failed", err)
	}

	dto := folder.toDTO()
	return &dto, nil
}

// MoveDocumentToFolder 移动文档到文件夹
func (s *LibraryService) MoveDocumentToFolder(input MoveDocumentToFolderInput) error {
	if input.DocumentID <= 0 {
		return errs.New("error.document_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 查询文档信息
	type docRow struct {
		ID        int64 `bun:"id"`
		LibraryID int64 `bun:"library_id"`
	}
	var doc docRow
	if err := db.NewSelect().
		Table("documents").
		Column("id", "library_id").
		Where("id = ?", input.DocumentID).
		Scan(ctx, &doc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.Newf("error.document_not_found", map[string]any{"ID": input.DocumentID})
		}
		return errs.Wrap("error.document_read_failed", err)
	}

	// 如果 folder_id 不为空，验证文件夹存在且属于同一知识库
	if input.FolderID != nil && *input.FolderID > 0 {
		var folder libraryFolderModel
		if err := db.NewSelect().Model(&folder).
			Where("id = ?", *input.FolderID).
			Scan(ctx); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errs.Newf("error.folder_not_found", map[string]any{"ID": *input.FolderID})
			}
			return errs.Wrap("error.folder_read_failed", err)
		}

		// 验证文件夹属于同一知识库
		if folder.LibraryID != doc.LibraryID {
			return errs.New("error.folder_document_library_mismatch")
		}
	}

	// 更新文档的 folder_id
	var folderIDValue interface{}
	if input.FolderID != nil && *input.FolderID > 0 {
		folderIDValue = *input.FolderID
	} else {
		folderIDValue = nil
	}

	if _, err := db.NewUpdate().
		Table("documents").
		Set("folder_id = ?", folderIDValue).
		Where("id = ?", input.DocumentID).
		Exec(ctx); err != nil {
		return errs.Wrap("error.document_move_failed", err)
	}

	return nil
}
