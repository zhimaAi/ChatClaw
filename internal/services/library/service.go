package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"willchat/internal/errs"
	"willchat/internal/services/settings"
	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// LibraryService 知识库服务（暴露给前端调用）
type LibraryService struct {
	app *application.App
}

func NewLibraryService(app *application.App) *LibraryService {
	return &LibraryService{app: app}
}

func (s *LibraryService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// ListLibraries 获取知识库列表（个人知识库）
func (s *LibraryService) ListLibraries() ([]Library, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]libraryModel, 0)
	if err := db.NewSelect().
		Model(&models).
		OrderExpr("sort_order DESC, id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.library_list_failed", err)
	}

	out := make([]Library, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

// CreateLibrary 创建知识库
func (s *LibraryService) CreateLibrary(input CreateLibraryInput) (*Library, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.library_name_required")
	}
	if len([]rune(name)) > 30 {
		return nil, errs.New("error.library_name_too_long")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 检查名称是否已存在
	var nameCount int
	if err := db.NewSelect().
		Table("library").
		ColumnExpr("COUNT(1)").
		Where("name = ?", name).
		Scan(ctx, &nameCount); err != nil {
		return nil, errs.Wrap("error.library_create_failed", err)
	}
	if nameCount > 0 {
		return nil, errs.Newf("error.library_name_duplicate", map[string]any{"Name": name})
	}

	// 全局嵌入配置（来自 settings 缓存）
	embeddingProviderID, ok := settings.GetValue("embedding_provider_id")
	if !ok || strings.TrimSpace(embeddingProviderID) == "" {
		return nil, errs.New("error.library_embedding_global_not_set")
	}
	embeddingModelID, ok := settings.GetValue("embedding_model_id")
	if !ok || strings.TrimSpace(embeddingModelID) == "" {
		return nil, errs.New("error.library_embedding_global_not_set")
	}
	embeddingProviderID = strings.TrimSpace(embeddingProviderID)
	embeddingModelID = strings.TrimSpace(embeddingModelID)

	// 语义分段模型（可选）
	semanticSegmentProviderID := strings.TrimSpace(input.SemanticSegmentProviderID)
	semanticSegmentModelID := strings.TrimSpace(input.SemanticSegmentModelID)
	// 两者要么都为空（不使用），要么都有值
	if (semanticSegmentProviderID == "") != (semanticSegmentModelID == "") {
		return nil, errs.New("error.library_semantic_segment_incomplete")
	}

	// 默认值（与 migrations 中的 DEFAULT 保持一致）
	topK := 20
	chunkSize := 1024
	chunkOverlap := 100
	matchThreshold := 0.5

	if input.TopK != nil && *input.TopK > 0 {
		topK = *input.TopK
	}
	if input.ChunkSize != nil {
		if *input.ChunkSize < 500 || *input.ChunkSize > 5000 {
			return nil, errs.New("error.library_chunk_size_invalid")
		}
		chunkSize = *input.ChunkSize
	}
	if input.ChunkOverlap != nil {
		if *input.ChunkOverlap < 0 || *input.ChunkOverlap > 1000 {
			return nil, errs.New("error.library_chunk_overlap_invalid")
		}
		chunkOverlap = *input.ChunkOverlap
	}
	if input.MatchThreshold != nil && *input.MatchThreshold >= 0 && *input.MatchThreshold <= 1 {
		matchThreshold = *input.MatchThreshold
	}

	// embedding 配置为全局 settings（不落库到 library 表），但创建前必须校验：
	// 1) provider 已启用
	// 2) embedding 模型存在且已启用（type=embedding）
	{
		var providerCount int
		if err := db.NewSelect().
			Table("providers").
			ColumnExpr("COUNT(1)").
			Where("provider_id = ?", embeddingProviderID).
			Where("enabled = ?", true).
			Scan(ctx, &providerCount); err != nil {
			return nil, errs.Wrap("error.library_create_failed", err)
		}
		if providerCount == 0 {
			return nil, errs.New("error.library_embedding_global_not_set")
		}

		var modelCount int
		if err := db.NewSelect().
			Table("models").
			ColumnExpr("COUNT(1)").
			Where("provider_id = ?", embeddingProviderID).
			Where("model_id = ?", embeddingModelID).
			Where("type = ?", "embedding").
			Where("enabled = ?", true).
			Scan(ctx, &modelCount); err != nil {
			return nil, errs.Wrap("error.library_create_failed", err)
		}
		if modelCount == 0 {
			return nil, errs.New("error.library_embedding_global_not_set")
		}
	}

	// sort_order 自动 +1（越新越大）
	var maxSort sql.NullInt64
	if err := db.NewSelect().
		Table("library").
		ColumnExpr("MAX(sort_order)").
		Scan(ctx, &maxSort); err != nil {
		return nil, errs.Wrap("error.library_create_failed", err)
	}
	sortOrder := 1
	if maxSort.Valid {
		sortOrder = int(maxSort.Int64) + 1
	}

	m := &libraryModel{
		Name: name,

		SemanticSegmentProviderID: semanticSegmentProviderID,
		SemanticSegmentModelID:    semanticSegmentModelID,

		TopK:           topK,
		ChunkSize:      chunkSize,
		ChunkOverlap:   chunkOverlap,
		MatchThreshold: matchThreshold,
		SortOrder:      sortOrder,
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.library_create_failed", fmt.Errorf("insert: %w", err))
	}

	dto := m.toDTO()
	return &dto, nil
}

// UpdateLibrary 更新知识库（用于重命名/设置）
func (s *LibraryService) UpdateLibrary(id int64, input UpdateLibraryInput) (*Library, error) {
	if id <= 0 {
		return nil, errs.New("error.library_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	q := db.NewUpdate().
		Model((*libraryModel)(nil)).
		Where("id = ?", id).
		Set("updated_at = ?", time.Now().UTC())

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, errs.New("error.library_name_required")
		}
		if len([]rune(name)) > 30 {
			return nil, errs.New("error.library_name_too_long")
		}
		// 检查名称是否与其他知识库重复（排除当前 ID）
		var nameCount int
		if err := db.NewSelect().
			Table("library").
			ColumnExpr("COUNT(1)").
			Where("name = ?", name).
			Where("id != ?", id).
			Scan(ctx, &nameCount); err != nil {
			return nil, errs.Wrap("error.library_update_failed", err)
		}
		if nameCount > 0 {
			return nil, errs.Newf("error.library_name_duplicate", map[string]any{"Name": name})
		}
		q = q.Set("name = ?", name)
	}

	if input.SemanticSegmentProviderID != nil || input.SemanticSegmentModelID != nil {
		sp := ""
		sm := ""
		if input.SemanticSegmentProviderID != nil {
			sp = strings.TrimSpace(*input.SemanticSegmentProviderID)
		}
		if input.SemanticSegmentModelID != nil {
			sm = strings.TrimSpace(*input.SemanticSegmentModelID)
		}
		// 两者要么都为空（清空），要么都有值
		if (sp == "") != (sm == "") {
			return nil, errs.New("error.library_semantic_segment_incomplete")
		}
		q = q.Set("semantic_segment_provider_id = ?", sp).Set("semantic_segment_model_id = ?", sm)
	}

	if input.TopK != nil {
		if *input.TopK <= 0 {
			return nil, errs.New("error.library_topk_invalid")
		}
		q = q.Set("top_k = ?", *input.TopK)
	}
	if input.ChunkSize != nil {
		if *input.ChunkSize < 500 || *input.ChunkSize > 5000 {
			return nil, errs.New("error.library_chunk_size_invalid")
		}
		q = q.Set("chunk_size = ?", *input.ChunkSize)
	}
	if input.ChunkOverlap != nil {
		if *input.ChunkOverlap < 0 || *input.ChunkOverlap > 1000 {
			return nil, errs.New("error.library_chunk_overlap_invalid")
		}
		q = q.Set("chunk_overlap = ?", *input.ChunkOverlap)
	}
	if input.MatchThreshold != nil {
		if *input.MatchThreshold < 0 || *input.MatchThreshold > 1 {
			return nil, errs.New("error.library_match_threshold_invalid")
		}
		q = q.Set("match_threshold = ?", *input.MatchThreshold)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.library_update_failed", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, errs.Newf("error.library_not_found", map[string]any{"ID": id})
	}

	var m libraryModel
	if err := db.NewSelect().Model(&m).Where("id = ?", id).Limit(1).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.library_not_found", map[string]any{"ID": id})
		}
		return nil, errs.Wrap("error.library_read_failed", err)
	}
	dto := m.toDTO()
	return &dto, nil
}

// DeleteLibrary 删除知识库
func (s *LibraryService) DeleteLibrary(id int64) error {
	if id <= 0 {
		return errs.New("error.library_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := db.NewDelete().Model((*libraryModel)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return errs.Wrap("error.library_delete_failed", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errs.Newf("error.library_not_found", map[string]any{"ID": id})
	}
	return nil
}
