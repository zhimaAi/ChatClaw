package raptor

import (
	"context"
	"fmt"
)

// BuildTreePlan builds RAPTOR nodes fully in memory (no DB callbacks).
// It mutates ParentID on existing nodes and returns all nodes (including newly created summary nodes).
// IDs must be pre-populated on input nodes and will be used as stable temp IDs.
func (b *Builder) BuildTreePlan(ctx context.Context, nodes []*DocumentNode) ([]*DocumentNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	if len(nodes) < b.config.MinNodes {
		// Not enough nodes; no summaries.
		return nodes, nil
	}

	// Determine next temp ID.
	var maxID int64
	for _, n := range nodes {
		if n.ID > maxID {
			maxID = n.ID
		}
	}
	nextID := maxID + 1

	all := make([]*DocumentNode, 0, len(nodes)+8)
	all = append(all, nodes...)

	currentLevel := 0
	currentNodes := nodes

	for currentLevel < b.config.MaxLevel && len(currentNodes) >= b.config.MinNodes {
		k := b.calculateK(len(currentNodes))
		if k < 2 {
			// In-memory fallback: create a single root summary node.
			summary, err := b.generateSummary(ctx, currentNodes)
			if err != nil {
				return nil, fmt.Errorf("生成根摘要失败: %w", err)
			}
			root := &DocumentNode{
				ID:         nextID,
				LibraryID:  currentNodes[0].LibraryID,
				DocumentID: currentNodes[0].DocumentID,
				Content:    summary,
				Level:      currentLevel + 1,
				ChunkOrder: 0,
			}
			nextID++

			for _, child := range currentNodes {
				child.ParentID = &root.ID
			}

			if b.embedder != nil {
				vecs, err := b.embedder.EmbedStrings(ctx, []string{summary})
				if err != nil {
					return nil, fmt.Errorf("嵌入根摘要失败: %w", err)
				}
				if len(vecs) > 0 {
					root.Vector = vecs[0]
				}
			}

			all = append(all, root)
			currentLevel++
			currentNodes = []*DocumentNode{root}
			break
		}

		// vectors for clustering
		vectors := make([][]float64, len(currentNodes))
		for i, node := range currentNodes {
			vectors[i] = node.Vector
		}

		kmeans := NewKMeans(k, 100, 1e-6)
		assignments := kmeans.Cluster(vectors)
		clusters := GetClusters(currentNodes, assignments, k)

		summaryNodes := make([]*DocumentNode, 0, len(clusters))
		for i, cluster := range clusters {
			if len(cluster) == 0 {
				continue
			}

			summary, err := b.generateSummary(ctx, cluster)
			if err != nil {
				return nil, fmt.Errorf("为簇 %d 生成摘要失败: %w", i, err)
			}

			summaryNode := &DocumentNode{
				ID:         nextID,
				LibraryID:  cluster[0].LibraryID,
				DocumentID: cluster[0].DocumentID,
				Content:    summary,
				Level:      currentLevel + 1,
				ChunkOrder: i,
			}
			nextID++

			for _, child := range cluster {
				child.ParentID = &summaryNode.ID
			}

			if b.embedder != nil {
				vecs, err := b.embedder.EmbedStrings(ctx, []string{summary})
				if err != nil {
					return nil, fmt.Errorf("嵌入摘要失败: %w", err)
				}
				if len(vecs) > 0 {
					summaryNode.Vector = vecs[0]
				}
			}

			all = append(all, summaryNode)
			summaryNodes = append(summaryNodes, summaryNode)
		}

		currentLevel++
		currentNodes = summaryNodes
	}

	// Post-loop fallback: if we stopped early due to too few nodes, create a root summary.
	if currentLevel < b.config.MaxLevel && len(currentNodes) > 1 {
		summary, err := b.generateSummary(ctx, currentNodes)
		if err != nil {
			return nil, fmt.Errorf("生成根摘要失败: %w", err)
		}
		root := &DocumentNode{
			ID:         nextID,
			LibraryID:  currentNodes[0].LibraryID,
			DocumentID: currentNodes[0].DocumentID,
			Content:    summary,
			Level:      currentLevel + 1,
			ChunkOrder: 0,
		}
		nextID++

		for _, child := range currentNodes {
			child.ParentID = &root.ID
		}

		if b.embedder != nil {
			vecs, err := b.embedder.EmbedStrings(ctx, []string{summary})
			if err != nil {
				return nil, fmt.Errorf("嵌入根摘要失败: %w", err)
			}
			if len(vecs) > 0 {
				root.Vector = vecs[0]
			}
		}

		all = append(all, root)
	}

	return all, nil
}

