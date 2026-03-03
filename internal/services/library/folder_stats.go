package library

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"chatclaw/internal/errs"
)

// FolderStats 聚合每个文件夹下文档数量及最近更新时间（用于前端展示）
type FolderStats struct {
	FolderID           int64     `json:"folder_id" bun:"folder_id"`
	DocCount           int       `json:"doc_count" bun:"doc_count"`
	LatestDocUpdatedAt time.Time `json:"latest_doc_updated_at" bun:"latest_doc_updated_at"`
}

// GetFolderStats 获取某个知识库下各文件夹的文档统计信息
func (s *LibraryService) GetFolderStats(libraryID int64) ([]FolderStats, error) {
	if libraryID <= 0 {
		return nil, errs.New("error.library_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stats := make([]FolderStats, 0)
	// 仅统计已归属到某个文件夹的文档（folder_id 非空）
	if err := db.NewSelect().
		Table("documents").
		ColumnExpr("folder_id").
		ColumnExpr("COUNT(1) AS doc_count").
		ColumnExpr("MAX(updated_at) AS latest_doc_updated_at").
		Where("library_id = ?", libraryID).
		Where("folder_id IS NOT NULL").
		Group("folder_id").
		Scan(ctx, &stats); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errs.Wrap("error.folder_list_failed", err)
	}

	return stats, nil
}

