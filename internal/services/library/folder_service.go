package library

import (
	"context"
	"database/sql"
	"errors"
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

	// Convert to DTO slice and build an index map.
	// Important: we keep pointers to the slice elements so that
	// updates to parent.Children are visible when we later return
	// the root folders.
	folders := make([]Folder, 0, len(models))
	folderMap := make(map[int64]*Folder)
	for i := range models {
		dto := models[i].toDTO()
		folders = append(folders, dto)
		folderMap[dto.ID] = &folders[i]
	}

	// Build tree structure (also detecting cycles).
	// We store pointers in roots to avoid losing the Children
	// relationships when returning.
	roots := make([]*Folder, 0)
	for i := range folders {
		if folders[i].ParentID == nil {
			roots = append(roots, &folders[i])
		} else {
			// 检查循环引用：如果父文件夹的 ID 等于当前文件夹 ID，则存在自引用
			parentID := *folders[i].ParentID
			if parentID == folders[i].ID {
				// 自引用（文件夹的父文件夹是自己），跳过这个有问题的文件夹
				continue
			}

			// 检查是否存在循环引用路径（通过检查父文件夹链）
			var checkCycle func(folderID int64, targetID int64, depth int) bool
			checkCycle = func(folderID int64, targetID int64, depth int) bool {
				if depth > 100 { // 防止无限递归
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

			if checkCycle(parentID, folders[i].ID, 0) {
				// 检测到循环引用，跳过这个文件夹，避免构建错误树形结构
				continue
			}

			// 添加到父文件夹的 children
			if parent, ok := folderMap[parentID]; ok {
				parent.Children = append(parent.Children, folders[i])
			}
		}
	}

	// Dereference root pointers into a value slice for the final result.
	result := make([]Folder, len(roots))
	for i, root := range roots {
		result[i] = *root
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
