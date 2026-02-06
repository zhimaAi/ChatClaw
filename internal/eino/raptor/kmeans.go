package raptor

import (
	"math"
	"math/rand"
)

// KMeans 对向量执行 K-Means 聚类
type KMeans struct {
	k           int
	maxIter     int
	tolerance   float64
	centroids   [][]float64
	assignments []int
}

// NewKMeans 创建新的 K-Means 聚类器
func NewKMeans(k, maxIter int, tolerance float64) *KMeans {
	if maxIter <= 0 {
		maxIter = 100
	}
	if tolerance <= 0 {
		tolerance = 1e-6
	}
	return &KMeans{
		k:         k,
		maxIter:   maxIter,
		tolerance: tolerance,
	}
}

// Cluster 执行 K-Means 聚类并返回簇分配结果
// 返回一个切片，其中 assignments[i] 是 vectors[i] 所属的簇索引
func (km *KMeans) Cluster(vectors [][]float64) []int {
	if len(vectors) == 0 || km.k <= 0 {
		return nil
	}

	// 如果向量数量少于 k，调整 k 值
	k := km.k
	if k > len(vectors) {
		k = len(vectors)
	}

	dim := len(vectors[0])

	// 使用 K-Means++ 算法初始化质心
	km.centroids = km.initCentroidsKMeansPlusPlus(vectors, k)
	km.assignments = make([]int, len(vectors))

	for iter := 0; iter < km.maxIter; iter++ {
		// 分配步骤：将每个向量分配到最近的质心
		for i, vec := range vectors {
			km.assignments[i] = km.findNearestCentroid(vec)
		}

		// 更新步骤：重新计算质心
		newCentroids := make([][]float64, k)
		counts := make([]int, k)

		for i := 0; i < k; i++ {
			newCentroids[i] = make([]float64, dim)
		}

		for i, vec := range vectors {
			cluster := km.assignments[i]
			counts[cluster]++
			for j := 0; j < dim; j++ {
				newCentroids[cluster][j] += vec[j]
			}
		}

		// 计算每个质心的均值
		for i := 0; i < k; i++ {
			if counts[i] > 0 {
				for j := 0; j < dim; j++ {
					newCentroids[i][j] /= float64(counts[i])
				}
			}
		}

		// 检查收敛性
		maxDiff := 0.0
		for i := 0; i < k; i++ {
			diff := euclideanDistance(km.centroids[i], newCentroids[i])
			if diff > maxDiff {
				maxDiff = diff
			}
		}

		km.centroids = newCentroids

		if maxDiff < km.tolerance {
			break
		}
	}

	return km.assignments
}

// initCentroidsKMeansPlusPlus 使用 K-Means++ 算法初始化质心
func (km *KMeans) initCentroidsKMeansPlusPlus(vectors [][]float64, k int) [][]float64 {
	n := len(vectors)
	dim := len(vectors[0])
	centroids := make([][]float64, k)

	// 随机选择第一个质心
	idx := rand.Intn(n)
	centroids[0] = make([]float64, dim)
	copy(centroids[0], vectors[idx])

	// 以与距离平方成正比的概率选择剩余质心
	for i := 1; i < k; i++ {
		// 计算到最近质心的距离
		distances := make([]float64, n)
		totalDist := 0.0

		for j, vec := range vectors {
			minDist := math.MaxFloat64
			for c := 0; c < i; c++ {
				dist := euclideanDistance(vec, centroids[c])
				if dist < minDist {
					minDist = dist
				}
			}
			distances[j] = minDist * minDist // 距离平方
			totalDist += distances[j]
		}

		// 以与距离平方成正比的概率选择下一个质心
		target := rand.Float64() * totalDist
		cumulative := 0.0
		chosenIdx := 0
		for j := 0; j < n; j++ {
			cumulative += distances[j]
			if cumulative >= target {
				chosenIdx = j
				break
			}
		}

		centroids[i] = make([]float64, dim)
		copy(centroids[i], vectors[chosenIdx])
	}

	return centroids
}

// findNearestCentroid 查找给定向量最近的质心索引
func (km *KMeans) findNearestCentroid(vec []float64) int {
	minDist := math.MaxFloat64
	nearest := 0

	for i, centroid := range km.centroids {
		dist := euclideanDistance(vec, centroid)
		if dist < minDist {
			minDist = dist
			nearest = i
		}
	}

	return nearest
}

// euclideanDistance 计算两个向量之间的欧几里得距离
func euclideanDistance(a, b []float64) float64 {
	if len(a) != len(b) {
		return math.MaxFloat64
	}

	sum := 0.0
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

// cosineSimilarity 计算两个向量之间的余弦相似度
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	dotProduct := 0.0
	normA := 0.0
	normB := 0.0

	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// GetClusters 返回按簇分组的节点
func GetClusters[T any](nodes []T, assignments []int, k int) [][]T {
	clusters := make([][]T, k)
	for i := 0; i < k; i++ {
		clusters[i] = make([]T, 0)
	}

	for i, node := range nodes {
		if i < len(assignments) {
			cluster := assignments[i]
			if cluster >= 0 && cluster < k {
				clusters[cluster] = append(clusters[cluster], node)
			}
		}
	}

	return clusters
}
