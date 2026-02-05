package raptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// DocumentNode 表示 RAPTOR 树中的一个节点
type DocumentNode struct {
	ID            int64
	LibraryID     int64
	DocumentID    int64
	Content       string
	ContentTokens string
	Level         int
	ParentID      *int64
	ChunkOrder    int
	Vector        []float64
}

// Config RAPTOR 构建器的配置
type Config struct {
	// MaxLevel 树的最大层级（默认：2）
	MaxLevel int
	// ClusterSize 每个簇的目标节点数（默认：5）
	ClusterSize int
	// MinNodes 执行聚类所需的最小节点数（默认：3）
	MinNodes int
	// MaxTokensPerSummary 摘要内容的最大 token 数（默认：4000）
	MaxTokensPerSummary int
}

// Builder 构建 RAPTOR 树结构
type Builder struct {
	config   *Config
	embedder embedding.Embedder
	llm      model.ChatModel

	// 数据库操作的回调函数
	OnNodeCreated func(ctx context.Context, node *DocumentNode) (int64, error)
	OnNodeUpdated func(ctx context.Context, node *DocumentNode) error
	OnVectorStore func(ctx context.Context, nodeID int64, vector []float64) error
}

// NewBuilder 创建新的 RAPTOR 构建器
func NewBuilder(cfg *Config, embedder embedding.Embedder, llm model.ChatModel) *Builder {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.MaxLevel <= 0 {
		cfg.MaxLevel = 2
	}
	if cfg.ClusterSize <= 0 {
		cfg.ClusterSize = 5
	}
	if cfg.MinNodes <= 0 {
		cfg.MinNodes = 3
	}
	if cfg.MaxTokensPerSummary <= 0 {
		cfg.MaxTokensPerSummary = 4000
	}

	return &Builder{
		config:   cfg,
		embedder: embedder,
		llm:      llm,
	}
}

// BuildTree 从给定节点构建 RAPTOR 树结构
// 节点应该是已计算向量的 level-0 节点
func (b *Builder) BuildTree(ctx context.Context, nodes []*DocumentNode) error {
	if len(nodes) < b.config.MinNodes {
		// 节点数量不足，无法构建树
		return nil
	}

	currentLevel := 0
	currentNodes := nodes

	for currentLevel < b.config.MaxLevel && len(currentNodes) >= b.config.MinNodes {
		// 计算簇的数量
		k := b.calculateK(len(currentNodes))
		if k < 2 {
			// Fallback: generate a single root summary so we can still produce a top-level node
			// even when node count is too small for clustering (e.g. only 2 level-1 nodes).
			summary, err := b.generateSummary(ctx, currentNodes)
			if err != nil {
				return fmt.Errorf("生成根摘要失败: %w", err)
			}

			root := &DocumentNode{
				LibraryID:  currentNodes[0].LibraryID,
				DocumentID: currentNodes[0].DocumentID,
				Content:    summary,
				Level:      currentLevel + 1,
				ChunkOrder: 0,
			}
			if b.OnNodeCreated != nil {
				nodeID, err := b.OnNodeCreated(ctx, root)
				if err != nil {
					return fmt.Errorf("创建根摘要节点失败: %w", err)
				}
				root.ID = nodeID
			}
			for _, child := range currentNodes {
				child.ParentID = &root.ID
				if b.OnNodeUpdated != nil {
					if err := b.OnNodeUpdated(ctx, child); err != nil {
						return fmt.Errorf("更新子节点 parent_id 失败: %w", err)
					}
				}
			}
			if b.embedder != nil {
				vecs, err := b.embedder.EmbedStrings(ctx, []string{summary})
				if err != nil {
					return fmt.Errorf("嵌入根摘要失败: %w", err)
				}
				if len(vecs) > 0 {
					root.Vector = vecs[0]
					if b.OnVectorStore != nil {
						if err := b.OnVectorStore(ctx, root.ID, vecs[0]); err != nil {
							return fmt.Errorf("存储根摘要向量失败: %w", err)
						}
					}
				}
			}

			// Move to next level and stop (no further clustering possible)
			currentLevel++
			currentNodes = []*DocumentNode{root}
			break
		}

		// 从节点中获取向量
		vectors := make([][]float64, len(currentNodes))
		for i, node := range currentNodes {
			vectors[i] = node.Vector
		}

		// 执行 K-Means 聚类
		kmeans := NewKMeans(k, 100, 1e-6)
		assignments := kmeans.Cluster(vectors)

		// 按簇分组节点
		clusters := GetClusters(currentNodes, assignments, k)

		// 为每个簇生成摘要
		summaryNodes := make([]*DocumentNode, 0, len(clusters))
		for i, cluster := range clusters {
			if len(cluster) == 0 {
				continue
			}

			// 为此簇生成摘要
			summary, err := b.generateSummary(ctx, cluster)
			if err != nil {
				return fmt.Errorf("为簇 %d 生成摘要失败: %w", i, err)
			}

			// 创建摘要节点
			summaryNode := &DocumentNode{
				LibraryID:  cluster[0].LibraryID,
				DocumentID: cluster[0].DocumentID,
				Content:    summary,
				Level:      currentLevel + 1,
				ChunkOrder: i,
			}

			// 将摘要节点保存到数据库
			if b.OnNodeCreated != nil {
				nodeID, err := b.OnNodeCreated(ctx, summaryNode)
				if err != nil {
					return fmt.Errorf("创建摘要节点失败: %w", err)
				}
				summaryNode.ID = nodeID
			}

			// 更新子节点的 parent_id
			for _, child := range cluster {
				child.ParentID = &summaryNode.ID
				if b.OnNodeUpdated != nil {
					if err := b.OnNodeUpdated(ctx, child); err != nil {
						return fmt.Errorf("更新子节点 parent_id 失败: %w", err)
					}
				}
			}

			// 为摘要节点生成向量
			if b.embedder != nil {
				vecs, err := b.embedder.EmbedStrings(ctx, []string{summary})
				if err != nil {
					return fmt.Errorf("嵌入摘要失败: %w", err)
				}
				if len(vecs) > 0 {
					summaryNode.Vector = vecs[0]
					if b.OnVectorStore != nil {
						if err := b.OnVectorStore(ctx, summaryNode.ID, vecs[0]); err != nil {
							return fmt.Errorf("存储摘要向量失败: %w", err)
						}
					}
				}
			}

			summaryNodes = append(summaryNodes, summaryNode)
		}

		// 移动到下一层级
		currentLevel++
		currentNodes = summaryNodes
	}

	// Post-loop fallback:
	// If we stopped early due to too few nodes for clustering (e.g. got 2 level-1 nodes),
	// generate a single top summary node for the next level (root).
	if currentLevel < b.config.MaxLevel && len(currentNodes) > 1 {
		summary, err := b.generateSummary(ctx, currentNodes)
		if err != nil {
			return fmt.Errorf("生成根摘要失败: %w", err)
		}

		root := &DocumentNode{
			LibraryID:  currentNodes[0].LibraryID,
			DocumentID: currentNodes[0].DocumentID,
			Content:    summary,
			Level:      currentLevel + 1,
			ChunkOrder: 0,
		}
		if b.OnNodeCreated != nil {
			nodeID, err := b.OnNodeCreated(ctx, root)
			if err != nil {
				return fmt.Errorf("创建根摘要节点失败: %w", err)
			}
			root.ID = nodeID
		}
		for _, child := range currentNodes {
			child.ParentID = &root.ID
			if b.OnNodeUpdated != nil {
				if err := b.OnNodeUpdated(ctx, child); err != nil {
					return fmt.Errorf("更新子节点 parent_id 失败: %w", err)
				}
			}
		}
		if b.embedder != nil {
			vecs, err := b.embedder.EmbedStrings(ctx, []string{summary})
			if err != nil {
				return fmt.Errorf("嵌入根摘要失败: %w", err)
			}
			if len(vecs) > 0 {
				root.Vector = vecs[0]
				if b.OnVectorStore != nil {
					if err := b.OnVectorStore(ctx, root.ID, vecs[0]); err != nil {
						return fmt.Errorf("存储根摘要向量失败: %w", err)
					}
				}
			}
		}
	}

	return nil
}

// calculateK 根据节点数量计算簇的数量
func (b *Builder) calculateK(nodeCount int) int {
	k := nodeCount / b.config.ClusterSize
	if k < 2 {
		k = 2
	}
	// 确保簇数量不超过节点数量
	if k > nodeCount {
		k = nodeCount
	}
	return k
}

// generateSummary 使用 LLM 为一组节点生成摘要
func (b *Builder) generateSummary(ctx context.Context, cluster []*DocumentNode) (string, error) {
	// 拼接簇内容
	var contentBuilder strings.Builder
	totalLen := 0
	maxLen := b.config.MaxTokensPerSummary * 4 // 粗略估计：1 token ≈ 4 字符

	for i, node := range cluster {
		if totalLen >= maxLen {
			break
		}
		if i > 0 {
			contentBuilder.WriteString("\n\n---\n\n")
		}
		content := node.Content
		if totalLen+len(content) > maxLen {
			content = content[:maxLen-totalLen]
		}
		contentBuilder.WriteString(content)
		totalLen += len(content)
	}

	clusterContent := contentBuilder.String()

	// 使用 LLM 生成摘要
	if b.llm == nil {
		// 如果没有 LLM，返回拼接的内容（截断）
		if len(clusterContent) > 1000 {
			return clusterContent[:1000] + "...", nil
		}
		return clusterContent, nil
	}

	prompt := fmt.Sprintf(`请为以下文档片段生成一个简洁的摘要，保留关键信息。摘要应该：
1. 概括主要内容和关键点
2. 保持客观准确
3. 长度控制在 200-500 字

文档片段：
%s

摘要：`, clusterContent)

	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: prompt,
		},
	}

	response, err := b.llm.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("LLM 生成失败: %w", err)
	}

	return response.Content, nil
}

// BuildTreeWithVectors 构建 RAPTOR 树，同时处理 level-0 节点的向量生成
// 这是一个便捷方法，结合了嵌入和树构建
func (b *Builder) BuildTreeWithVectors(ctx context.Context, nodes []*DocumentNode) error {
	if len(nodes) == 0 {
		return nil
	}

	// 为 level-0 节点生成向量（如果尚未生成）
	for _, node := range nodes {
		if node.Level == 0 && len(node.Vector) == 0 && b.embedder != nil {
			vecs, err := b.embedder.EmbedStrings(ctx, []string{node.Content})
			if err != nil {
				return fmt.Errorf("嵌入节点内容失败: %w", err)
			}
			if len(vecs) > 0 {
				node.Vector = vecs[0]
				if b.OnVectorStore != nil {
					if err := b.OnVectorStore(ctx, node.ID, vecs[0]); err != nil {
						return fmt.Errorf("存储节点向量失败: %w", err)
					}
				}
			}
		}
	}

	// 构建树
	return b.BuildTree(ctx, nodes)
}
