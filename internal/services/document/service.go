package document

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"willchat/internal/define"
	"willchat/internal/eino/processor"
	"willchat/internal/errs"
	"willchat/internal/fts/tokenizer"
	"willchat/internal/services/thumbnail"
	"willchat/internal/sqlite"
	"willchat/internal/taskmanager"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Job type constants for document tasks.
const (
	JobTypeThumbnail = "thumbnail" // Generate document thumbnail
	JobTypeProcess   = "process"   // Parse and embed document
	JobTypeReembed   = "reembed"   // Re-embed existing nodes only
)

// ThumbnailJobData holds data for thumbnail generation job.
type ThumbnailJobData struct {
	DocID     int64  `json:"doc_id"`
	LibraryID int64  `json:"library_id"`
	LocalPath string `json:"local_path"`
}

// ProcessJobData holds data for document processing job.
type ProcessJobData struct {
	DocID     int64  `json:"doc_id"`
	LibraryID int64  `json:"library_id"`
	RunID     string `json:"run_id"`
}

// DocumentService 文档服务（暴露给前端调用）
type DocumentService struct {
	app *application.App
}

func NewDocumentService(app *application.App) *DocumentService {
	return &DocumentService{app: app}
}

// ServiceStartup 实现 Wails 服务生命周期接口
// 在应用启动时注册任务处理器并启动任务管理器
func (s *DocumentService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.registerTaskHandlers()
	taskmanager.Get().Start()
	// Warm up tokenizer in background to avoid first-call latency (e.g. gse dict load).
	go func() {
		_ = tokenizer.TokenizeName("warmup.txt")
		_ = tokenizer.TokenizeContent("warmup content")
	}()
	return nil
}

// registerTaskHandlers registers document-related job handlers with the task manager.
func (s *DocumentService) registerTaskHandlers() {
	tm := taskmanager.Get()
	if tm == nil {
		return
	}

	// Register thumbnail generation handler
	tm.RegisterHandler(taskmanager.QueueThumbnail, JobTypeThumbnail, func(ctx context.Context, info *taskmanager.TaskInfo, data []byte) error {
		var jobData ThumbnailJobData
		if err := json.Unmarshal(data, &jobData); err != nil {
			s.app.Logger.Error("failed to unmarshal thumbnail job data", "error", err)
			return nil // Don't retry malformed jobs
		}
		s.generateThumbnail(jobData.DocID, jobData.LibraryID, jobData.LocalPath, info)
		return nil
	})

	// Register document processing handler
	tm.RegisterHandler(taskmanager.QueueDocument, JobTypeProcess, func(ctx context.Context, info *taskmanager.TaskInfo, data []byte) error {
		var jobData ProcessJobData
		if err := json.Unmarshal(data, &jobData); err != nil {
			s.app.Logger.Error("failed to unmarshal process job data", "error", err)
			return nil // Don't retry malformed jobs
		}
		s.processDocument(jobData.DocID, jobData.LibraryID, jobData.RunID, info)
		return nil
	})

	// Register embedding-only handler
	tm.RegisterHandler(taskmanager.QueueDocument, JobTypeReembed, func(ctx context.Context, info *taskmanager.TaskInfo, data []byte) error {
		var jobData ProcessJobData
		if err := json.Unmarshal(data, &jobData); err != nil {
			s.app.Logger.Error("failed to unmarshal reembed job data", "error", err)
			return nil
		}
		s.reembedDocument(jobData.DocID, jobData.LibraryID, jobData.RunID, info)
		return nil
	})
}

func (s *DocumentService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// GetDocumentsDir 获取文档存储目录
func (s *DocumentService) GetDocumentsDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", errs.Wrap("error.document_dir_failed", err)
	}
	return filepath.Join(cfgDir, define.AppID, "documents"), nil
}

// ListDocumentsPage 获取知识库文档分页（cursor 分页）
// - 无关键词时：按 sort_by 排序，支持 before_id 游标分页
//   - "created_desc"（默认）: id DESC, before_id 为上一页最小 id
//   - "created_asc": id ASC, before_id 为上一页最大 id（此时 before_id 语义变为 after_id）
// - 有关键词时：按 FTS5 BM25 相关度降序排列，不使用 before_id（搜索结果一次性返回），sort_by 被忽略
// - 每次返回 limit（默认/最大 100）
func (s *DocumentService) ListDocumentsPage(input ListDocumentsPageInput) ([]Document, error) {
	if input.LibraryID <= 0 {
		return nil, errs.New("error.library_id_required")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models := make([]documentModel, 0, limit)
	keyword := strings.TrimSpace(input.Keyword)

	if keyword != "" {
		// Build FTS match query
		matchQuery := tokenizer.BuildMatchQuery(keyword)
		if matchQuery == "" {
			return []Document{}, nil
		}

		// Combine keyword match with library_id filter in FTS MATCH for better performance
		// FTS5 syntax: (keyword tokens) AND library_id:value
		ftsMatch := fmt.Sprintf("(%s) AND library_id:%d", matchQuery, input.LibraryID)

		// Query FTS directly with library_id filter, then JOIN for full document data
		// Sort by BM25 relevance regardless of sort_by parameter
		err := db.NewRaw(`
			SELECT d.*
			FROM doc_name_fts
			INNER JOIN documents d ON d.id = doc_name_fts.rowid
			WHERE doc_name_fts MATCH ?
			ORDER BY doc_name_fts.rank, d.id DESC
			LIMIT ?
		`, ftsMatch, limit).Scan(ctx, &models)
		if err != nil {
			return nil, errs.Wrap("error.document_list_failed", err)
		}
	} else {
		// No keyword: use cursor pagination
		q := db.NewSelect().
			Model(&models).
			Where("d.library_id = ?", input.LibraryID)

		if input.SortBy == "created_asc" {
			// Ascending order: before_id acts as "after_id" (return rows with id > before_id)
			if input.BeforeID > 0 {
				q = q.Where("d.id > ?", input.BeforeID)
			}
			if err := q.OrderExpr("d.id ASC").Limit(limit).Scan(ctx); err != nil {
				return nil, errs.Wrap("error.document_list_failed", err)
			}
		} else {
			// Default: descending order
			if input.BeforeID > 0 {
				q = q.Where("d.id < ?", input.BeforeID)
			}
			if err := q.OrderExpr("d.id DESC").Limit(limit).Scan(ctx); err != nil {
				return nil, errs.Wrap("error.document_list_failed", err)
			}
		}
	}

	out := make([]Document, 0, len(models))
	for i := range models {
		doc := models[i].toDTO()
		if doc.LocalPath != "" {
			if _, err := os.Stat(doc.LocalPath); os.IsNotExist(err) {
				doc.FileMissing = true
			}
		}
		out = append(out, doc)
	}
	return out, nil
}

// ListDocuments 获取知识库的文档列表
// keyword: 可选的搜索关键词（按文件名搜索，使用 FTS，按相关度降序排列）
func (s *DocumentService) ListDocuments(libraryID int64, keyword string) ([]Document, error) {
	if libraryID <= 0 {
		return nil, errs.New("error.library_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models := make([]documentModel, 0)
	keyword = strings.TrimSpace(keyword)

	if keyword != "" {
		// Build FTS match query from keyword
		matchQuery := tokenizer.BuildMatchQuery(keyword)
		if matchQuery != "" {
			// 调试日志
			s.app.Logger.Debug("FTS search", "keyword", keyword, "matchQuery", matchQuery, "libraryID", libraryID)
			// Combine keyword match with library_id filter in FTS MATCH for better performance
			ftsMatch := fmt.Sprintf("(%s) AND library_id:%d", matchQuery, libraryID)

			// Query FTS directly with library_id filter, then JOIN for full document data
			err := db.NewRaw(`
				SELECT d.*
				FROM doc_name_fts
				INNER JOIN documents d ON d.id = doc_name_fts.rowid
				WHERE doc_name_fts MATCH ?
				ORDER BY doc_name_fts.rank, d.id DESC
			`, ftsMatch).Scan(ctx, &models)
			if err != nil {
				return nil, errs.Wrap("error.document_list_failed", err)
			}
		}
		// If matchQuery is empty (keyword was all punctuation), return empty result
	} else {
		// No keyword, return all documents for this library
		if err := db.NewSelect().
			Model(&models).
			Where("library_id = ?", libraryID).
			OrderExpr("id DESC").
			Scan(ctx); err != nil {
			return nil, errs.Wrap("error.document_list_failed", err)
		}
	}

	out := make([]Document, 0, len(models))
	for i := range models {
		doc := models[i].toDTO()
		// 检查本地文件是否存在
		if doc.LocalPath != "" {
			if _, err := os.Stat(doc.LocalPath); os.IsNotExist(err) {
				doc.FileMissing = true
			}
		}
		out = append(out, doc)
	}
	return out, nil
}

// UploadDocuments 上传文档
func (s *DocumentService) UploadDocuments(input UploadInput) ([]Document, error) {
	if input.LibraryID <= 0 {
		return nil, errs.New("error.library_id_required")
	}
	if len(input.FilePaths) == 0 {
		return nil, errs.New("error.document_file_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	docsDir, err := s.GetDocumentsDir()
	if err != nil {
		return nil, err
	}

	// 确保目录存在
	libraryDir := filepath.Join(docsDir, fmt.Sprintf("%d", input.LibraryID))
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		return nil, errs.Wrap("error.document_upload_failed", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	uploaded := make([]Document, 0, len(input.FilePaths))
	total := len(input.FilePaths)
	done := 0

	emitUploadProgress := func() {
		s.app.Event.Emit("document:upload_progress", UploadProgressEvent{
			LibraryID: input.LibraryID,
			Total:     total,
			Done:      done,
		})
	}
	emitUploadProgress()

	for _, srcPath := range input.FilePaths {
		doc, err := s.uploadSingleFile(ctx, db, input.LibraryID, libraryDir, srcPath)
		done++
		emitUploadProgress()
		if err != nil {
			// 记录错误但继续处理其他文件
			s.app.Logger.Warn("upload file failed", "path", srcPath, "error", err)
			continue
		}
		uploaded = append(uploaded, *doc)

		// 可选：实时通知前端新增文档（用于即时渲染/反馈）
		s.app.Event.Emit("document:uploaded", *doc)

		// 启动异步处理任务
		s.startProcessingTask(doc)

		// 启动缩略图生成任务
		s.startThumbnailTask(doc)
	}

	if len(uploaded) == 0 {
		return nil, errs.New("error.document_upload_failed")
	}

	return uploaded, nil
}

// uploadSingleFile 上传单个文件
func (s *DocumentService) uploadSingleFile(ctx context.Context, db *bun.DB, libraryID int64, libraryDir, srcPath string) (*Document, error) {
	// 检查文件是否存在
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	// 获取文件扩展名（去掉小数点前缀）
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(srcPath)), ".")
	if !IsSupportedExtension(ext) {
		return nil, errs.Newf("error.document_file_type_not_supported", map[string]any{"Ext": ext})
	}

	// 计算文件 hash
	hash, err := s.calculateFileHash(srcPath)
	if err != nil {
		return nil, fmt.Errorf("calculate hash: %w", err)
	}

	// 检查是否已存在相同文件，如果存在则删除旧记录（覆盖上传）
	var existingDoc documentModel
	err = db.NewSelect().
		Model(&existingDoc).
		Where("library_id = ?", libraryID).
		Where("content_hash = ?", hash).
		Scan(ctx)
	if err == nil {
		// 存在相同文件，取消旧任务并删除旧记录和文件
		if tm := taskmanager.Get(); tm != nil {
			tm.Cancel(fmt.Sprintf("doc:%d", existingDoc.ID))
			tm.Cancel(fmt.Sprintf("thumb:%d", existingDoc.ID))
		}
		// 删除旧文件
		if existingDoc.LocalPath != "" {
			os.Remove(existingDoc.LocalPath)
		}
		// 删除旧记录
		if _, err := db.NewDelete().Model(&existingDoc).Where("id = ?", existingDoc.ID).Exec(ctx); err != nil {
			s.app.Logger.Error("delete existing document failed", "id", existingDoc.ID, "error", err)
		}
	}

	// 生成目标文件名：hash_原始文件名
	originalName := filepath.Base(srcPath)
	destName := fmt.Sprintf("%s_%s", hash[:8], originalName)
	destPath := filepath.Join(libraryDir, destName)

	// 复制文件
	if err := s.copyFile(srcPath, destPath); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	// 生成运行 ID
	runID := uuid.New().String()

	// 插入数据库记录
	m := &documentModel{
		LibraryID:       libraryID,
		OriginalName:    originalName,
		NameTokens:      tokenizer.TokenizeName(originalName),
		ThumbIcon:       "",
		FileSize:        srcInfo.Size(),
		ContentHash:     hash,
		Extension:       ext,
		MimeType:        GetMimeType(ext),
		SourceType:      "local",
		LocalPath:       destPath,
		ProcessingRunID: runID,
		ParsingStatus:   StatusPending,
		EmbeddingStatus: StatusPending,
	}

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		// 插入失败，删除已复制的文件
		os.Remove(destPath)
		return nil, fmt.Errorf("insert record: %w", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// RenameDocument 重命名文档
func (s *DocumentService) RenameDocument(input RenameInput) (*Document, error) {
	if input.ID <= 0 {
		return nil, errs.New("error.document_id_required")
	}
	newName := strings.TrimSpace(input.NewName)
	if newName == "" {
		return nil, errs.New("error.document_name_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 查询文档
	var m documentModel
	if err := db.NewSelect().Model(&m).Where("id = ?", input.ID).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.document_not_found", map[string]any{"ID": input.ID})
		}
		return nil, errs.Wrap("error.document_read_failed", err)
	}

	// 获取新的扩展名（保持原扩展名）
	oldExt := filepath.Ext(m.OriginalName)
	newExt := filepath.Ext(newName)
	if newExt == "" {
		newName = newName + oldExt
	}

	// 重命名物理文件
	if m.LocalPath != "" {
		oldPath := m.LocalPath
		dir := filepath.Dir(oldPath)
		// 保持 hash 前缀
		hashPrefix := m.ContentHash[:8]
		newFileName := fmt.Sprintf("%s_%s", hashPrefix, newName)
		newPath := filepath.Join(dir, newFileName)

		if err := os.Rename(oldPath, newPath); err != nil {
			if !os.IsNotExist(err) {
				return nil, errs.Wrap("error.document_rename_failed", err)
			}
			// 文件不存在，仍然更新数据库
		}
		m.LocalPath = newPath
	}

	// 更新数据库
	m.OriginalName = newName
	m.NameTokens = tokenizer.TokenizeName(newName)
	if _, err := db.NewUpdate().Model(&m).
		Column("original_name", "name_tokens", "local_path", "updated_at").
		Where("id = ?", input.ID).
		Exec(ctx); err != nil {
		return nil, errs.Wrap("error.document_rename_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

// ReprocessDocument 重新学习文档（删除旧节点并重新解析/向量化）
func (s *DocumentService) ReprocessDocument(id int64) error {
	if id <= 0 {
		return errs.New("error.document_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. 查询文档
	var m documentModel
	if err := db.NewSelect().Model(&m).Where("id = ?", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.Newf("error.document_not_found", map[string]any{"ID": id})
		}
		return errs.Wrap("error.document_read_failed", err)
	}

	// 2. 取消正在进行的任务
	if tm := taskmanager.Get(); tm != nil {
		tm.Cancel(fmt.Sprintf("doc:%d", id))
	}

	// 3. 查询并删除向量（doc_vec 没有外键约束，需要手动删除）
	var nodeIDs []int64
	if err := db.NewSelect().
		Table("document_nodes").
		Column("id").
		Where("document_id = ?", id).
		Scan(ctx, &nodeIDs); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.app.Logger.Warn("query document_nodes failed", "error", err)
	}
	if len(nodeIDs) > 0 {
		if _, err := db.ExecContext(ctx,
			"DELETE FROM doc_vec WHERE id IN (?)", bun.In(nodeIDs)); err != nil {
			s.app.Logger.Warn("delete doc_vec failed", "error", err)
		}
	}

	// 4. 删除旧节点（触发器会自动清理 FTS 索引）
	if _, err := db.NewDelete().Table("document_nodes").Where("document_id = ?", id).Exec(ctx); err != nil {
		s.app.Logger.Warn("delete document_nodes failed", "error", err)
	}

	// 5. 生成新的处理运行 ID 并重置状态
	runID := fmt.Sprintf("%d-%d", id, time.Now().UnixNano())
	if _, err := db.NewUpdate().Model(&m).
		Set("processing_run_id = ?", runID).
		Set("parsing_status = ?", StatusPending).
		Set("parsing_progress = ?", 0).
		Set("parsing_error = ?", "").
		Set("embedding_status = ?", StatusPending).
		Set("embedding_progress = ?", 0).
		Set("embedding_error = ?", "").
		Set("word_total = ?", 0).
		Set("split_total = ?", 0).
		Where("id = ?", id).
		Exec(ctx); err != nil {
		return errs.Wrap("error.document_update_failed", err)
	}

	// 6. 启动新的处理任务
	doc := m.toDTO()
	doc.ProcessingRunID = runID
	doc.ParsingStatus = StatusPending
	doc.ParsingProgress = 0
	doc.ParsingError = ""
	doc.EmbeddingStatus = StatusPending
	doc.EmbeddingProgress = 0
	doc.EmbeddingError = ""
	s.startProcessingTask(&doc)

	return nil
}

// DeleteDocument 删除文档
func (s *DocumentService) DeleteDocument(id int64) error {
	if id <= 0 {
		return errs.New("error.document_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 查询文档
	var m documentModel
	if err := db.NewSelect().Model(&m).Where("id = ?", id).Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.Newf("error.document_not_found", map[string]any{"ID": id})
		}
		return errs.Wrap("error.document_read_failed", err)
	}

	// 取消正在进行的任务
	if tm := taskmanager.Get(); tm != nil {
		tm.Cancel(fmt.Sprintf("doc:%d", id))
		tm.Cancel(fmt.Sprintf("thumb:%d", id))
	}

	// 删除物理文件（即使失败也继续删除数据库记录）
	if m.LocalPath != "" {
		if err := os.Remove(m.LocalPath); err != nil && !os.IsNotExist(err) {
			s.app.Logger.Warn("delete file failed", "path", m.LocalPath, "error", err)
		}
	}

	// 手动删除 doc_vec（vec0 虚拟表不支持外键 CASCADE）
	var nodeIDs []int64
	if err := db.NewSelect().
		Table("document_nodes").
		Column("id").
		Where("document_id = ?", id).
		Scan(ctx, &nodeIDs); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.app.Logger.Warn("query document_nodes failed", "error", err)
	}
	if len(nodeIDs) > 0 {
		if _, err := db.ExecContext(ctx,
			"DELETE FROM doc_vec WHERE id IN (?)", bun.In(nodeIDs)); err != nil {
			s.app.Logger.Warn("delete doc_vec failed", "error", err)
		}
	}

	// 手动删除 document_nodes（确保清理干净）
	if _, err := db.NewDelete().Table("document_nodes").Where("document_id = ?", id).Exec(ctx); err != nil {
		s.app.Logger.Warn("delete document_nodes failed", "error", err)
	}

	// 删除文档记录
	if _, err := db.NewDelete().Model(&m).Where("id = ?", id).Exec(ctx); err != nil {
		return errs.Wrap("error.document_delete_failed", err)
	}

	return nil
}

// calculateFileHash 计算文件 SHA256 哈希
func (s *DocumentService) calculateFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile 复制文件
func (s *DocumentService) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// startProcessingTask 启动文档处理任务
func (s *DocumentService) startProcessingTask(doc *Document) {
	tm := taskmanager.Get()
	if tm == nil {
		return
	}

	taskKey := fmt.Sprintf("doc:%d", doc.ID)
	runID := doc.ProcessingRunID

	jobData, _ := json.Marshal(ProcessJobData{
		DocID:     doc.ID,
		LibraryID: doc.LibraryID,
		RunID:     runID,
	})

	tm.Submit(taskmanager.QueueDocument, JobTypeProcess, taskKey, runID, jobData)
}

// startThumbnailTask 启动缩略图生成任务
func (s *DocumentService) startThumbnailTask(doc *Document) {
	tm := taskmanager.Get()
	if tm == nil {
		return
	}

	// 使用独立的任务 key，避免与文档处理任务冲突
	taskKey := fmt.Sprintf("thumb:%d", doc.ID)

	jobData, _ := json.Marshal(ThumbnailJobData{
		DocID:     doc.ID,
		LibraryID: doc.LibraryID,
		LocalPath: doc.LocalPath,
	})

	tm.Submit(taskmanager.QueueThumbnail, JobTypeThumbnail, taskKey, doc.ProcessingRunID, jobData)
}

// generateThumbnail 生成文档缩略图
func (s *DocumentService) generateThumbnail(docID, libraryID int64, localPath string, info *taskmanager.TaskInfo) {
	if info.IsCancelled() {
		return
	}

	tm := taskmanager.Get()
	db, err := s.db()
	if err != nil {
		return
	}

	ctx := context.Background()

	// Skip stale thumbnail jobs (e.g. after restart or document reprocess)
	var currentRunID string
	if err := db.NewSelect().
		Table("documents").
		Column("processing_run_id").
		Where("id = ?", docID).
		Scan(ctx, &currentRunID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.app.Logger.Error("query document processing_run_id failed", "docID", docID, "error", err)
	}
	if info != nil && currentRunID != "" && info.RunID != "" && info.RunID != currentRunID {
		return
	}

	// 生成缩略图
	result := thumbnail.Generate(localPath)

	// 更新数据库
	thumbIcon := result.DataURI
	if thumbIcon != "" {
		_, err = db.NewUpdate().
			Table("documents").
			Set("thumb_icon = ?", thumbIcon).
			Set("updated_at = ?", sqlite.NowUTC()).
			Where("id = ?", docID).
			Exec(ctx)

		if err != nil {
			s.app.Logger.Warn("failed to update thumbnail", "docID", docID, "error", err)
			return
		}
	}

	// 发送事件通知前端
	if tm != nil {
		tm.Emit("document:thumbnail", ThumbnailEvent{
			DocumentID: docID,
			LibraryID:  libraryID,
			ThumbIcon:  thumbIcon,
		})
	}
}

// processDocument 处理文档（解析 + 分段 + 向量化 + RAPTOR）
func (s *DocumentService) processDocument(docID, libraryID int64, runID string, info *taskmanager.TaskInfo) {
	tm := taskmanager.Get()
	db, err := s.db()
	if err != nil {
		return
	}

	ctx := context.Background()

	// 辅助函数：检查任务是否应该继续
	shouldContinue := func() bool {
		if info.IsCancelled() {
			return false
		}
		// 检查 runID 是否匹配
		var currentRunID string
		if err := db.NewSelect().
			Table("documents").
			Column("processing_run_id").
			Where("id = ?", docID).
			Scan(ctx, &currentRunID); err != nil {
			return false
		}
		return currentRunID == runID
	}

	// 辅助函数：更新状态并发送事件
	updateAndEmit := func(parsingStatus, parsingProgress int, parsingError string, embeddingStatus, embeddingProgress int, embeddingError string) {
		if _, err := db.NewUpdate().
			Table("documents").
			Set("parsing_status = ?", parsingStatus).
			Set("parsing_progress = ?", parsingProgress).
			Set("parsing_error = ?", parsingError).
			Set("embedding_status = ?", embeddingStatus).
			Set("embedding_progress = ?", embeddingProgress).
			Set("embedding_error = ?", embeddingError).
			Set("updated_at = ?", sqlite.NowUTC()).
			Where("id = ?", docID).
			Where("processing_run_id = ?", runID). // 只更新当前运行的任务
			Exec(ctx); err != nil {
			s.app.Logger.Error("update document status failed", "docID", docID, "error", err)
		}

		if tm != nil {
			tm.Emit("document:progress", ProgressEvent{
				DocumentID:        docID,
				LibraryID:         libraryID,
				ParsingStatus:     parsingStatus,
				ParsingProgress:   parsingProgress,
				ParsingError:      parsingError,
				EmbeddingStatus:   embeddingStatus,
				EmbeddingProgress: embeddingProgress,
				EmbeddingError:    embeddingError,
			})
		}
	}

	// 检查任务是否应该继续
	if !shouldContinue() {
		return
	}

	// 获取文档信息
	var doc documentModel
	if err := db.NewSelect().Model(&doc).Where("id = ?", docID).Scan(ctx); err != nil {
		updateAndEmit(StatusFailed, 0, "获取文档信息失败: "+err.Error(), StatusPending, 0, "")
		return
	}
	// Skip stale jobs (e.g. after restart or "relearn" created a new run)
	if runID != "" && doc.ProcessingRunID != "" && runID != doc.ProcessingRunID {
		return
	}

	// 开始解析
	updateAndEmit(StatusProcessing, 0, "", StatusPending, 0, "")

	// 获取知识库配置
	libraryConfig, err := processor.GetLibraryConfig(ctx, db, libraryID)
	if err != nil {
		updateAndEmit(StatusFailed, 0, "获取知识库配置失败: "+err.Error(), StatusPending, 0, "")
		return
	}

	// 获取全局嵌入模型配置
	embeddingConfig, err := processor.GetEmbeddingConfig(ctx, db)
	if err != nil {
		updateAndEmit(StatusFailed, 0, "获取嵌入模型配置失败: "+err.Error(), StatusPending, 0, "")
		return
	}

	// 创建文档处理器
	proc, err := processor.NewProcessor(db)
	if err != nil {
		updateAndEmit(StatusFailed, 0, "创建处理器失败: "+err.Error(), StatusPending, 0, "")
		return
	}

	// 获取供应商信息的回调函数
	getProviderInfo := func(providerID string) (*processor.ProviderInfo, error) {
		return processor.GetProviderInfo(ctx, db, providerID)
	}

	// 进度回调
	var lastPhase string
	onProgress := func(phase string, progress int) {
		if !shouldContinue() {
			return
		}
		if phase == "parsing" {
			updateAndEmit(StatusProcessing, progress, "", StatusPending, 0, "")
		} else if phase == "embedding" {
			if lastPhase != "embedding" {
				// 解析完成，开始向量化
				updateAndEmit(StatusCompleted, 100, "", StatusProcessing, progress, "")
			} else {
				updateAndEmit(StatusCompleted, 100, "", StatusProcessing, progress, "")
			}
		}
		lastPhase = phase
	}

	// 执行文档处理
	result, err := proc.ProcessDocument(
		ctx,
		docID,
		doc.LocalPath,
		libraryConfig,
		embeddingConfig,
		getProviderInfo,
		onProgress,
	)

	if !shouldContinue() {
		return
	}

	if err != nil {
		// Classify error by processor phase to update statuses correctly.
		errMsg := err.Error()
		var pe *processor.PhaseError
		if errors.As(err, &pe) {
			switch pe.Phase {
			case processor.PhaseParsing, processor.PhaseSplitting:
				updateAndEmit(StatusFailed, 0, errMsg, StatusPending, 0, "")
			default:
				updateAndEmit(StatusCompleted, 100, "", StatusFailed, 0, errMsg)
			}
		} else {
			// Fallback: treat as embedding failure (parsing likely completed if we reached here).
			updateAndEmit(StatusCompleted, 100, "", StatusFailed, 0, errMsg)
		}
		return
	}

	// 更新文档统计信息
	if _, err := db.NewUpdate().
		Table("documents").
		Set("word_total = ?", result.WordTotal).
		Set("split_total = ?", result.SplitTotal).
		Set("updated_at = ?", sqlite.NowUTC()).
		Where("id = ?", docID).
		Where("processing_run_id = ?", runID).
		Exec(ctx); err != nil {
		s.app.Logger.Warn("update document stats failed", "docID", docID, "error", err)
	}

	// 全部完成
	updateAndEmit(StatusCompleted, 100, "", StatusCompleted, 100, "")
}

// reembedDocument 仅对已有节点重新向量化（不重新解析/分段）
func (s *DocumentService) reembedDocument(docID, libraryID int64, runID string, info *taskmanager.TaskInfo) {
	if info != nil && info.IsCancelled() {
		return
	}

	db, err := s.db()
	if err != nil {
		return
	}

	ctx := context.Background()

	// Load document and validate runID to avoid stale jobs
	var doc documentModel
	if err := db.NewSelect().Model(&doc).Where("id = ?", docID).Scan(ctx); err != nil {
		return
	}
	if runID != "" && doc.ProcessingRunID != "" && runID != doc.ProcessingRunID {
		return
	}

	parsingStatus := doc.ParsingStatus
	parsingProgress := doc.ParsingProgress
	parsingError := doc.ParsingError

	emitProgress := func(status int, progress int, errMsg string) {
		// Update DB
		q := db.NewUpdate().
			Table("documents").
			Set("embedding_status = ?", status).
			Set("embedding_progress = ?", progress).
			Set("embedding_error = ?", errMsg).
			Where("id = ?", docID)
		if runID != "" {
			q = q.Where("processing_run_id = ?", runID)
		}
		if _, err := q.Exec(ctx); err != nil {
			s.app.Logger.Error("update document embedding progress failed", "docID", docID, "error", err)
		}

		// Emit event
		s.app.Event.Emit("document:progress", ProgressEvent{
			DocumentID:        docID,
			LibraryID:         libraryID,
			ParsingStatus:     parsingStatus,
			ParsingProgress:   parsingProgress,
			ParsingError:      parsingError,
			EmbeddingStatus:   status,
			EmbeddingProgress: progress,
			EmbeddingError:    errMsg,
		})
	}

	// Start embedding
	emitProgress(StatusProcessing, 0, "")

	// Load embedding config (global)
	embeddingConfig, err := processor.GetEmbeddingConfig(ctx, db)
	if err != nil {
		emitProgress(StatusFailed, 0, "获取嵌入模型配置失败: "+err.Error())
		return
	}

	// Create processor
	proc, err := processor.NewProcessor(db)
	if err != nil {
		emitProgress(StatusFailed, 0, "创建处理器失败: "+err.Error())
		return
	}

	err = proc.ReembedDocumentNodes(ctx, docID, embeddingConfig, func(p int) {
		if info != nil && info.IsCancelled() {
			return
		}
		emitProgress(StatusProcessing, p, "")
	})
	if err != nil {
		emitProgress(StatusFailed, 0, err.Error())
		return
	}

	emitProgress(StatusCompleted, 100, "")
}
