package processor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	"github.com/uptrace/bun"

	"willchat/internal/eino/chatmodel"
	einoembed "willchat/internal/eino/embedding"
	einoparser "willchat/internal/eino/parser"
	"willchat/internal/eino/raptor"
	"willchat/internal/eino/splitter"
	"willchat/internal/fts/tokenizer"
)

// DocumentNode 表示数据库中的文档节点
type DocumentNode struct {
	ID            int64     `bun:"id,pk,autoincrement"`
	LibraryID     int64     `bun:"library_id,notnull"`
	DocumentID    int64     `bun:"document_id,notnull"`
	Content       string    `bun:"content,notnull"`
	ContentTokens string    `bun:"content_tokens,notnull"`
	Level         int       `bun:"level,notnull"`
	ParentID      *int64    `bun:"parent_id"`
	ChunkOrder    int       `bun:"chunk_order,notnull"`
	Vector        []float64 `bun:"-"` // 不存储在此表中
}

// LibraryConfig 包含文档处理的知识库配置
type LibraryConfig struct {
	ID                        int64
	ChunkSize                 int
	ChunkOverlap              int
	SemanticSegmentProviderID string
	SemanticSegmentModelID    string
}

// EmbeddingConfig 包含全局嵌入配置
type EmbeddingConfig struct {
	ProviderID   string
	ModelID      string
	Dimension    int // 向量维度
	ProviderType string
	APIKey       string
	APIEndpoint  string
	ExtraConfig  string
}

// ProviderInfo 包含供应商信息
type ProviderInfo struct {
	ProviderType string
	APIKey       string
	APIEndpoint  string
	ExtraConfig  string
}

// ProcessResult 包含文档处理的结果
type ProcessResult struct {
	WordTotal  int
	SplitTotal int
	Error      error
}

// Processor 处理文档的解析、分割和嵌入
type Processor struct {
	db     *bun.DB
	parser parser.Parser
}

// ReembedDocumentNodes 仅对已有的 document_nodes 重新向量化（不重新解析/分段）
// 用于全局 embedding 模型/维度切换后的批量重建向量。
func (p *Processor) ReembedDocumentNodes(
	ctx context.Context,
	docID int64,
	embeddingConfig *EmbeddingConfig,
	onProgress func(progress int),
) error {
	if docID <= 0 {
		return errors.New("docID required")
	}
	if embeddingConfig == nil {
		return errors.New("embeddingConfig required")
	}

	embedder, err := p.createEmbedder(ctx, embeddingConfig)
	if err != nil {
		return fmt.Errorf("创建 embedder 失败: %w", err)
	}

	nodes := make([]*DocumentNode, 0, 256)
	if err := p.db.NewSelect().
		Model(&nodes).
		Column("id", "library_id", "document_id", "content", "level", "parent_id", "chunk_order").
		Where("document_id = ?", docID).
		OrderExpr("id ASC").
		Scan(ctx); err != nil {
		return fmt.Errorf("读取 document_nodes 失败: %w", err)
	}

	if len(nodes) == 0 {
		return errors.New("no document nodes")
	}

	return p.embedNodes(ctx, nodes, embedder, onProgress)
}

// NewProcessor 创建新的文档处理器
func NewProcessor(db *bun.DB) (*Processor, error) {
	ctx := context.Background()

	// 创建文档解析器
	docParser, err := einoparser.NewDocumentParser(ctx)
	if err != nil {
		return nil, fmt.Errorf("创建文档解析器失败: %w", err)
	}

	return &Processor{
		db:     db,
		parser: docParser,
	}, nil
}

// ProcessDocument 处理文档：解析、分割、存储节点和嵌入
func (p *Processor) ProcessDocument(
	ctx context.Context,
	docID int64,
	localPath string,
	libraryConfig *LibraryConfig,
	embeddingConfig *EmbeddingConfig,
	getProviderInfo func(providerID string) (*ProviderInfo, error),
	onProgress func(phase string, progress int),
) (*ProcessResult, error) {
	result := &ProcessResult{}

	// 阶段 1：解析文档
	if onProgress != nil {
		onProgress("parsing", 10)
	}

	docs, err := p.parseDocument(ctx, localPath)
	if err != nil {
		result.Error = fmt.Errorf("解析失败: %w", err)
		return result, result.Error
	}

	if len(docs) == 0 {
		result.Error = errors.New("未从文档中提取到内容")
		return result, result.Error
	}

	// 计算字数
	fullContent := ""
	for _, doc := range docs {
		fullContent += doc.Content
	}
	result.WordTotal = utf8.RuneCountInString(fullContent)

	if onProgress != nil {
		onProgress("parsing", 40)
	}

	// 提前创建 embedder（用于语义分割和后续向量化，同一实例复用）
	embedder, err := p.createEmbedder(ctx, embeddingConfig)
	if err != nil {
		result.Error = fmt.Errorf("创建 embedder 失败: %w", err)
		return result, result.Error
	}

	if onProgress != nil {
		onProgress("parsing", 50)
	}

	// 阶段 2：分割文档（Markdown 优先用 Header Splitter，否则按配置选择语义/递归分割）
	var splittingDone chan struct{}
	semanticEnabled := libraryConfig != nil &&
		libraryConfig.SemanticSegmentProviderID != "" &&
		libraryConfig.SemanticSegmentModelID != ""
	if semanticEnabled {
		ext := strings.ToLower(filepath.Ext(localPath))
		if ext != ".md" && ext != ".markdown" && onProgress != nil {
			splittingDone = make(chan struct{})
			go func() {
				defer func() {
					// avoid panic if close happens before start
					recover()
				}()
				ticker := time.NewTicker(900 * time.Millisecond)
				defer ticker.Stop()
				pct := 52
				for {
					select {
					case <-ctx.Done():
						return
					case <-splittingDone:
						return
					case <-ticker.C:
						if pct < 79 {
							pct += 2
							onProgress("parsing", pct)
						}
					}
				}
			}()
		}
	}
	chunks, err := p.splitDocument(ctx, docs, localPath, libraryConfig, embedder)
	if splittingDone != nil {
		close(splittingDone)
	}
	if err != nil {
		result.Error = fmt.Errorf("分割失败: %w", err)
		return result, result.Error
	}

	if onProgress != nil {
		onProgress("parsing", 80)
	}

	// 我们将“入库”延迟到所有计算完成后一次性提交：
	// - 先在内存中生成 level-0 节点与向量
	// - 可选：构建 RAPTOR 摘要节点（level 1/2）与向量
	// - 最后用一个事务写入 document_nodes + doc_vec（避免处理中间态）
	level0 := make([]*raptor.DocumentNode, 0, len(chunks))
	for i, chunk := range chunks {
		level0 = append(level0, &raptor.DocumentNode{
			ID:            int64(i + 1), // temp id
			LibraryID:     libraryConfig.ID,
			DocumentID:    docID,
			Content:       chunk.Content,
			ContentTokens: tokenizeContent(chunk.Content),
			Level:         0,
			ParentID:      nil,
			ChunkOrder:    i,
		})
	}
	result.SplitTotal = len(level0)

	if onProgress != nil {
		onProgress("parsing", 100)
	}

	// 阶段 4：嵌入 level-0 节点（内存中）
	if onProgress != nil {
		onProgress("embedding", 10)
	}
	if err := embedRaptorNodes(ctx, level0, embedder, func(progress int) {
		if onProgress != nil {
			onProgress("embedding", 10+progress*70/100)
		}
	}); err != nil {
		result.Error = fmt.Errorf("嵌入失败: %w", err)
		return result, result.Error
	}

	if onProgress != nil {
		onProgress("embedding", 80)
	}

	// 阶段 5：可选构建 RAPTOR（语义分段开启时）
	allNodes := level0
	if semanticEnabled {
		planned, err := p.buildRaptorPlan(ctx, libraryConfig, allNodes, embedder, getProviderInfo)
		if err != nil {
			// 非致命：只保留 level-0
			_ = err
		} else {
			allNodes = planned
		}
	}

	// 确保所有节点都有 content_tokens（摘要节点也需要）
	for _, n := range allNodes {
		if strings.TrimSpace(n.ContentTokens) == "" {
			n.ContentTokens = tokenizeContent(n.Content)
		}
	}

	// 最终一次性入库（事务）
	if err := p.persistNodesAndVectors(ctx, docID, allNodes); err != nil {
		result.Error = fmt.Errorf("入库失败: %w", err)
		return result, result.Error
	}

	if onProgress != nil {
		onProgress("embedding", 100)
	}

	// 更新文档统计信息
	if err := p.updateDocumentStats(ctx, docID, result.WordTotal, result.SplitTotal); err != nil {
		// 非致命错误
		_ = err
	}

	return result, nil
}

// parseDocument 解析文档文件并返回 schema.Document 列表
func (p *Processor) parseDocument(ctx context.Context, localPath string) ([]*schema.Document, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("打开文件: %w", err)
	}
	defer file.Close()

	docs, err := p.parser.Parse(ctx, file, parser.WithURI(localPath))
	if err != nil {
		return nil, err
	}

	return docs, nil
}

// splitDocument 将文档分割成块
// 分割器选择优先级：Markdown Header Splitter > Semantic Splitter > Recursive Splitter
func (p *Processor) splitDocument(
	ctx context.Context,
	docs []*schema.Document,
	localPath string,
	libraryConfig *LibraryConfig,
	embedder embedding.Embedder,
) ([]*schema.Document, error) {
	cfg := &splitter.Config{
		FilePath:     localPath, // 传入文件路径，用于判断是否使用 Markdown 分割器
		ChunkSize:    libraryConfig.ChunkSize,
		ChunkOverlap: libraryConfig.ChunkOverlap,
	}

	// 如果启用了语义分割，使用全局 embedder 进行语义分割
	// 注意：Markdown 文件会优先使用 Header Splitter，不受此配置影响
	if libraryConfig.SemanticSegmentProviderID != "" && libraryConfig.SemanticSegmentModelID != "" {
		cfg.SemanticEmbedder = embedder
		cfg.SemanticPercentile = 0.6
		cfg.SemanticMinChunkSize = 300
	}

	docSplitter, err := splitter.NewSplitter(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return docSplitter.Transform(ctx, docs)
}

// storeNodes 将文档块作为节点存储到数据库
// 使用事务保证一致性，通过 LastInsertId() 获取插入的 ID
func (p *Processor) storeNodes(ctx context.Context, docID, libraryID int64, chunks []*schema.Document) ([]*DocumentNode, error) {
	nodes := make([]*DocumentNode, 0, len(chunks))

	// 使用事务保证所有节点的插入一致性
	err := p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for i, chunk := range chunks {
			// 为 FTS 对内容进行分词
			tokens := tokenizeContent(chunk.Content)

			node := &DocumentNode{
				LibraryID:     libraryID,
				DocumentID:    docID,
				Content:       chunk.Content,
				ContentTokens: tokens,
				Level:         0,
				ChunkOrder:    i,
			}

			// 使用原始 SQL 插入并获取 LastInsertId
			res, err := tx.NewRaw(
				"INSERT INTO document_nodes (library_id, document_id, content, content_tokens, level, chunk_order) VALUES (?, ?, ?, ?, ?, ?)",
				node.LibraryID, node.DocumentID, node.Content, node.ContentTokens, node.Level, node.ChunkOrder,
			).Exec(ctx)
			if err != nil {
				return fmt.Errorf("插入节点 %d: %w", i, err)
			}

			// 使用 LastInsertId 获取插入的 ID
			lastID, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("获取节点 %d 的 id: %w", i, err)
			}
			node.ID = lastID

			nodes = append(nodes, node)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return nodes, nil
}

// createEmbedder 根据配置创建 embedding.Embedder
func (p *Processor) createEmbedder(ctx context.Context, config *EmbeddingConfig) (embedding.Embedder, error) {
	return einoembed.NewEmbedder(ctx, &einoembed.ProviderConfig{
		ProviderType: config.ProviderType,
		APIKey:       config.APIKey,
		APIEndpoint:  config.APIEndpoint,
		ModelID:      config.ModelID,
		Dimension:    config.Dimension,
		ExtraConfig:  config.ExtraConfig,
	})
}

// embedNodes 为节点生成嵌入向量并存储
func (p *Processor) embedNodes(ctx context.Context, nodes []*DocumentNode, embedder embedding.Embedder, onProgress func(int)) error {
	if len(nodes) == 0 {
		log.Printf("[Embedding] No nodes to embed")
		return nil
	}

	log.Printf("[Embedding] Starting embedding for %d nodes", len(nodes))

	// 批量嵌入以提高效率
	// 注意：通义千问等部分 API 限制 batch size 最大为 10
	batchSize := 10
	storedCount := 0
	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]

		// 收集批量嵌入的内容
		contents := make([]string, len(batch))
		for j, node := range batch {
			contents[j] = node.Content
		}

		log.Printf("[Embedding] Processing batch %d-%d/%d", i+1, end, len(nodes))

		// 生成嵌入
		vectors, err := embedder.EmbedStrings(ctx, contents)
		if err != nil {
			log.Printf("[Embedding] FAILED batch %d-%d: %v", i+1, end, err)
			return fmt.Errorf("嵌入批次（从 %d 开始）: %w", i, err)
		}

		log.Printf("[Embedding] Got %d vectors, dimension=%d", len(vectors), func() int {
			if len(vectors) > 0 {
				return len(vectors[0])
			}
			return 0
		}())

		// 存储向量
		for j, node := range batch {
			if j < len(vectors) {
				node.Vector = vectors[j]
				if err := p.storeVector(ctx, node.ID, vectors[j]); err != nil {
					return fmt.Errorf("存储节点 %d 的向量: %w", node.ID, err)
				}
				storedCount++
			}
		}

		// 报告进度
		if onProgress != nil {
			progress := (end * 100) / len(nodes)
			onProgress(progress)
		}
	}

	log.Printf("[Embedding] Completed: stored %d vectors for %d nodes", storedCount, len(nodes))
	return nil
}

// storeVector 将向量存储到 doc_vec 表
func (p *Processor) storeVector(ctx context.Context, nodeID int64, vector []float64) error {
	// 将 []float64 转换为适合 sqlite-vec 的格式
	// doc_vec 表使用 vec0 扩展
	vecStr := formatVector(vector)

	_, err := p.db.NewRaw(
		"INSERT INTO doc_vec (id, content) VALUES (?, ?)",
		nodeID, vecStr,
	).Exec(ctx)
	if err != nil {
		// 如果插入失败，尝试更新
		_, err = p.db.NewRaw(
			"UPDATE doc_vec SET content = ? WHERE id = ?",
			vecStr, nodeID,
		).Exec(ctx)
	}

	// 调试日志：输出向量存储结果
	if err != nil {
		log.Printf("[Vector] FAILED to store vector for node %d: %v", nodeID, err)
	} else {
		log.Printf("[Vector] SUCCESS stored vector for node %d, dimension=%d", nodeID, len(vector))
	}

	return err
}

// formatVector 将向量格式化为 sqlite-vec 存储格式
func formatVector(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = fmt.Sprintf("%f", v)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// buildRaptorTree 构建 RAPTOR 树结构
func (p *Processor) buildRaptorTree(
	ctx context.Context,
	docID int64,
	libraryConfig *LibraryConfig,
	nodes []*DocumentNode,
	embedder embedding.Embedder,
	getProviderInfo func(providerID string) (*ProviderInfo, error),
) error {
	// 获取 LLM 的供应商信息
	providerInfo, err := getProviderInfo(libraryConfig.SemanticSegmentProviderID)
	if err != nil {
		return fmt.Errorf("获取供应商信息: %w", err)
	}

	// 创建 LLM 聊天模型
	llm, err := chatmodel.NewChatModel(ctx, &chatmodel.ProviderConfig{
		ProviderType: providerInfo.ProviderType,
		APIKey:       providerInfo.APIKey,
		APIEndpoint:  providerInfo.APIEndpoint,
		ModelID:      libraryConfig.SemanticSegmentModelID,
		ExtraConfig:  providerInfo.ExtraConfig,
	})
	if err != nil {
		return fmt.Errorf("创建聊天模型: %w", err)
	}

	// 创建 RAPTOR 构建器
	raptorBuilder := raptor.NewBuilder(&raptor.Config{
		MaxLevel:    2,
		ClusterSize: 5,
		MinNodes:    3,
	}, embedder, llm)

	// 设置数据库操作的回调函数
	raptorBuilder.OnNodeCreated = func(ctx context.Context, node *raptor.DocumentNode) (int64, error) {
		return p.createRaptorNode(ctx, node)
	}
	raptorBuilder.OnNodeUpdated = func(ctx context.Context, node *raptor.DocumentNode) error {
		return p.updateRaptorNodeParent(ctx, node)
	}
	raptorBuilder.OnVectorStore = func(ctx context.Context, nodeID int64, vector []float64) error {
		return p.storeVector(ctx, nodeID, vector)
	}

	// 将 DocumentNode 转换为 raptor.DocumentNode
	raptorNodes := make([]*raptor.DocumentNode, len(nodes))
	for i, node := range nodes {
		raptorNodes[i] = &raptor.DocumentNode{
			ID:            node.ID,
			LibraryID:     node.LibraryID,
			DocumentID:    node.DocumentID,
			Content:       node.Content,
			ContentTokens: node.ContentTokens,
			Level:         node.Level,
			ParentID:      node.ParentID,
			ChunkOrder:    node.ChunkOrder,
			Vector:        node.Vector,
		}
	}

	return raptorBuilder.BuildTree(ctx, raptorNodes)
}

// createRaptorNode 在数据库中创建 RAPTOR 摘要节点
// 使用 LastInsertId() 获取插入的 ID
func (p *Processor) createRaptorNode(ctx context.Context, node *raptor.DocumentNode) (int64, error) {
	tokens := tokenizeContent(node.Content)

	res, err := p.db.NewRaw(
		"INSERT INTO document_nodes (library_id, document_id, content, content_tokens, level, chunk_order) VALUES (?, ?, ?, ?, ?, ?)",
		node.LibraryID, node.DocumentID, node.Content, tokens, node.Level, node.ChunkOrder,
	).Exec(ctx)
	if err != nil {
		return 0, err
	}

	// 使用 LastInsertId 获取插入的 ID
	lastID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取 RAPTOR 节点 id: %w", err)
	}

	return lastID, nil
}

// updateRaptorNodeParent 更新 RAPTOR 节点的 parent_id
func (p *Processor) updateRaptorNodeParent(ctx context.Context, node *raptor.DocumentNode) error {
	_, err := p.db.NewUpdate().
		TableExpr("document_nodes").
		Set("parent_id = ?", node.ParentID).
		Where("id = ?", node.ID).
		Exec(ctx)
	return err
}

// updateDocumentStats 更新文档统计信息
func (p *Processor) updateDocumentStats(ctx context.Context, docID int64, wordTotal, splitTotal int) error {
	_, err := p.db.NewUpdate().
		TableExpr("documents").
		Set("word_total = ?", wordTotal).
		Set("split_total = ?", splitTotal).
		Where("id = ?", docID).
		Exec(ctx)
	return err
}

// tokenizeContent 对内容进行分词，用于 FTS
// 使用 gse 进行中文/英文分词
func tokenizeContent(content string) string {
	return tokenizer.TokenizeContent(content)
}

// embedRaptorNodes embeds contents for raptor nodes (in-memory, no DB writes).
func embedRaptorNodes(ctx context.Context, nodes []*raptor.DocumentNode, embedder embedding.Embedder, onProgress func(int)) error {
	if len(nodes) == 0 {
		return nil
	}
	if embedder == nil {
		return errors.New("embedder is nil")
	}

	batchSize := 10
	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]

		contents := make([]string, len(batch))
		for j, n := range batch {
			contents[j] = n.Content
		}

		vecs, err := embedder.EmbedStrings(ctx, contents)
		if err != nil {
			return err
		}
		for j := range batch {
			if j < len(vecs) {
				batch[j].Vector = vecs[j]
			}
		}

		if onProgress != nil {
			onProgress((end * 100) / len(nodes))
		}
	}
	return nil
}

// buildRaptorPlan builds RAPTOR summary nodes in memory (no DB writes).
func (p *Processor) buildRaptorPlan(
	ctx context.Context,
	libraryConfig *LibraryConfig,
	level0 []*raptor.DocumentNode,
	embedder embedding.Embedder,
	getProviderInfo func(providerID string) (*ProviderInfo, error),
) ([]*raptor.DocumentNode, error) {
	// 获取 LLM 的供应商信息
	providerInfo, err := getProviderInfo(libraryConfig.SemanticSegmentProviderID)
	if err != nil {
		return nil, fmt.Errorf("获取供应商信息: %w", err)
	}

	// 创建 LLM 聊天模型
	llm, err := chatmodel.NewChatModel(ctx, &chatmodel.ProviderConfig{
		ProviderType: providerInfo.ProviderType,
		APIKey:       providerInfo.APIKey,
		APIEndpoint:  providerInfo.APIEndpoint,
		ModelID:      libraryConfig.SemanticSegmentModelID,
		ExtraConfig:  providerInfo.ExtraConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("创建聊天模型: %w", err)
	}

	builder := raptor.NewBuilder(&raptor.Config{
		MaxLevel:    2,
		ClusterSize: 5,
		MinNodes:    3,
	}, embedder, llm)

	return builder.BuildTreePlan(ctx, level0)
}

func (p *Processor) persistNodesAndVectors(ctx context.Context, docID int64, nodes []*raptor.DocumentNode) error {
	if docID <= 0 {
		return errors.New("docID required")
	}
	if len(nodes) == 0 {
		return errors.New("no nodes to persist")
	}

	// stable insert order: level asc, chunk_order asc, temp id asc
	sorted := make([]*raptor.DocumentNode, len(nodes))
	copy(sorted, nodes)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Level != sorted[j].Level {
			return sorted[i].Level < sorted[j].Level
		}
		if sorted[i].ChunkOrder != sorted[j].ChunkOrder {
			return sorted[i].ChunkOrder < sorted[j].ChunkOrder
		}
		return sorted[i].ID < sorted[j].ID
	})

	return p.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// delete old vectors first (doc_vec has no FK cascade)
		if _, err := tx.NewRaw(
			"DELETE FROM doc_vec WHERE id IN (SELECT id FROM document_nodes WHERE document_id = ?)",
			docID,
		).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewRaw("DELETE FROM document_nodes WHERE document_id = ?", docID).Exec(ctx); err != nil {
			return err
		}

		idMap := make(map[int64]int64, len(sorted)) // tempID -> dbID

		for _, n := range sorted {
			res, err := tx.NewRaw(
				"INSERT INTO document_nodes (library_id, document_id, content, content_tokens, level, chunk_order) VALUES (?, ?, ?, ?, ?, ?)",
				n.LibraryID, n.DocumentID, n.Content, n.ContentTokens, n.Level, n.ChunkOrder,
			).Exec(ctx)
			if err != nil {
				return err
			}
			dbID, err := res.LastInsertId()
			if err != nil {
				return err
			}
			idMap[n.ID] = dbID

			// store vector if exists
			if len(n.Vector) > 0 {
				vecStr := formatVector(n.Vector)
				if _, err := tx.NewRaw("INSERT INTO doc_vec (id, content) VALUES (?, ?)", dbID, vecStr).Exec(ctx); err != nil {
					// fallback update
					if _, err2 := tx.NewRaw("UPDATE doc_vec SET content = ? WHERE id = ?", vecStr, dbID).Exec(ctx); err2 != nil {
						return err
					}
				}
			}
		}

		// update parent relationships
		for _, n := range sorted {
			if n.ParentID == nil {
				continue
			}
			childID, ok1 := idMap[n.ID]
			parentID, ok2 := idMap[*n.ParentID]
			if !ok1 || !ok2 {
				continue
			}
			if _, err := tx.NewRaw("UPDATE document_nodes SET parent_id = ? WHERE id = ?", parentID, childID).Exec(ctx); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetLibraryConfig 从数据库获取知识库配置
func GetLibraryConfig(ctx context.Context, db *bun.DB, libraryID int64) (*LibraryConfig, error) {
	var config LibraryConfig
	err := db.NewSelect().
		TableExpr("library").
		Column("id", "chunk_size", "chunk_overlap", "semantic_segment_provider_id", "semantic_segment_model_id").
		Where("id = ?", libraryID).
		Scan(ctx, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetEmbeddingConfig 从设置中获取全局嵌入配置
func GetEmbeddingConfig(ctx context.Context, db *bun.DB) (*EmbeddingConfig, error) {
	config := &EmbeddingConfig{}

	// 从设置中获取嵌入供应商、模型和维度
	type settingRow struct {
		Key   string         `bun:"key"`
		Value sql.NullString `bun:"value"`
	}
	rows := make([]settingRow, 0, 3)
	err := db.NewSelect().
		TableExpr("settings").
		Column("key", "value").
		Where("key IN (?)", bun.In([]string{"embedding_provider_id", "embedding_model_id", "embedding_dimension"})).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}

	for _, r := range rows {
		if !r.Value.Valid {
			continue
		}
		switch r.Key {
		case "embedding_provider_id":
			config.ProviderID = strings.TrimSpace(r.Value.String)
		case "embedding_model_id":
			config.ModelID = strings.TrimSpace(r.Value.String)
		case "embedding_dimension":
			if dim, err := strconv.Atoi(strings.TrimSpace(r.Value.String)); err == nil && dim > 0 {
				config.Dimension = dim
			}
		}
	}

	if config.ProviderID == "" || config.ModelID == "" {
		return nil, errors.New("嵌入模型未配置")
	}

	// 获取供应商详情
	err = db.NewSelect().
		TableExpr("providers").
		Column("type", "api_key", "api_endpoint", "extra_config").
		Where("provider_id = ?", config.ProviderID).
		Scan(ctx, &config.ProviderType, &config.APIKey, &config.APIEndpoint, &config.ExtraConfig)
	if err != nil {
		return nil, fmt.Errorf("获取供应商详情: %w", err)
	}

	return config, nil
}

// GetProviderInfo 从数据库获取供应商信息
func GetProviderInfo(ctx context.Context, db *bun.DB, providerID string) (*ProviderInfo, error) {
	info := &ProviderInfo{}
	err := db.NewSelect().
		TableExpr("providers").
		Column("type", "api_key", "api_endpoint", "extra_config").
		Where("provider_id = ?", providerID).
		Scan(ctx, &info.ProviderType, &info.APIKey, &info.APIEndpoint, &info.ExtraConfig)
	if err != nil {
		return nil, err
	}
	return info, nil
}
