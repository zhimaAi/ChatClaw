package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"willclaw/internal/errs"
	"willclaw/internal/services/settings"
	"willclaw/internal/sqlite"
	"willclaw/internal/taskmanager"

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

	// 语义分段开关（默认关闭）
	semanticSegmentationEnabled := false
	if input.SemanticSegmentationEnabled != nil {
		semanticSegmentationEnabled = *input.SemanticSegmentationEnabled
	}

	// RAPTOR LLM 配置（可选）
	raptorLLMProviderID := strings.TrimSpace(input.RaptorLLMProviderID)
	raptorLLMModelID := strings.TrimSpace(input.RaptorLLMModelID)
	// 两者要么都为空（不使用），要么都有值
	if (raptorLLMProviderID == "") != (raptorLLMModelID == "") {
		return nil, errs.New("error.library_raptor_llm_incomplete")
	}

	// 默认值（与 migrations 中的 DEFAULT 保持一致）
	chunkSize := 1024
	chunkOverlap := 100

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

		SemanticSegmentationEnabled: semanticSegmentationEnabled,
		RaptorLLMProviderID:         raptorLLMProviderID,
		RaptorLLMModelID:            raptorLLMModelID,

		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
		SortOrder:    sortOrder,
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

	// 语义分段开关总是更新（bool 类型，前端总是传递）
	q = q.Set("semantic_segmentation_enabled = ?", input.SemanticSegmentationEnabled)

	if input.RaptorLLMProviderID != nil || input.RaptorLLMModelID != nil {
		// 允许"只更新其中一个字段"的局部更新：先读当前值再合并更新
		type row struct {
			RaptorLLMProviderID string `bun:"raptor_llm_provider_id"`
			RaptorLLMModelID    string `bun:"raptor_llm_model_id"`
		}
		var cur row
		if err := db.NewSelect().
			Table("library").
			Column("raptor_llm_provider_id", "raptor_llm_model_id").
			Where("id = ?", id).
			Limit(1).
			Scan(ctx, &cur); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errs.Newf("error.library_not_found", map[string]any{"ID": id})
			}
			return nil, errs.Wrap("error.library_read_failed", err)
		}

		rp := strings.TrimSpace(cur.RaptorLLMProviderID)
		rm := strings.TrimSpace(cur.RaptorLLMModelID)

		if input.RaptorLLMProviderID != nil {
			rp = strings.TrimSpace(*input.RaptorLLMProviderID)
		}
		if input.RaptorLLMModelID != nil {
			rm = strings.TrimSpace(*input.RaptorLLMModelID)
		}
		// 两者要么都为空（清空），要么都有值
		if (rp == "") != (rm == "") {
			return nil, errs.New("error.library_raptor_llm_incomplete")
		}
		q = q.Set("raptor_llm_provider_id = ?", rp).Set("raptor_llm_model_id = ?", rm)
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

// DeleteLibrary 删除知识库及其所有关联数据
func (s *LibraryService) DeleteLibrary(id int64) error {
	if id <= 0 {
		return errs.New("error.library_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. 查询该知识库下所有文档的 ID 和本地路径
	type docInfo struct {
		ID        int64  `bun:"id"`
		LocalPath string `bun:"local_path"`
	}
	var docs []docInfo
	if err := db.NewSelect().
		Table("documents").
		Column("id", "local_path").
		Where("library_id = ?", id).
		Scan(ctx, &docs); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errs.Wrap("error.library_delete_failed", err)
	}

	// 2. 取消所有正在进行的任务
	if tm := taskmanager.Get(); tm != nil {
		for _, doc := range docs {
			tm.Cancel(fmt.Sprintf("doc:%d", doc.ID))
			tm.Cancel(fmt.Sprintf("thumb:%d", doc.ID))
		}
	}

	// 3. 删除向量表中的数据（doc_vec 没有外键约束，需要手动删除）
	if len(docs) > 0 {
		docIDs := make([]int64, len(docs))
		for i, doc := range docs {
			docIDs[i] = doc.ID
		}
		// 查询 document_nodes 的 ID 用于删除向量
		var nodeIDs []int64
		if err := db.NewSelect().
			Table("document_nodes").
			Column("id").
			Where("document_id IN (?)", bun.In(docIDs)).
			Scan(ctx, &nodeIDs); err != nil && !errors.Is(err, sql.ErrNoRows) {
			s.app.Logger.Warn("query document_nodes failed", "error", err)
		}
		if len(nodeIDs) > 0 {
			if _, err := db.ExecContext(ctx,
				"DELETE FROM doc_vec WHERE id IN (?)", bun.In(nodeIDs)); err != nil {
				s.app.Logger.Warn("delete doc_vec failed", "error", err)
			}
		}
	}

	// 4. 删除知识库（CASCADE 会自动删除 documents、document_nodes，触发器会处理 FTS）
	res, err := db.NewDelete().Model((*libraryModel)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return errs.Wrap("error.library_delete_failed", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errs.Newf("error.library_not_found", map[string]any{"ID": id})
	}

	// 5. 删除物理文件（在数据库删除成功后执行，失败不影响整体结果）
	for _, doc := range docs {
		if doc.LocalPath != "" {
			if err := os.Remove(doc.LocalPath); err != nil && !os.IsNotExist(err) {
				s.app.Logger.Warn("delete file failed", "path", doc.LocalPath, "error", err)
			}
		}
	}

	return nil
}
