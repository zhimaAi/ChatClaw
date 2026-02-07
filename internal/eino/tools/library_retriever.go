package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"willchat/internal/services/retrieval"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// Retrieval level constants for RAPTOR hierarchical structure
const (
	LevelOriginal = 0 // Original document chunks (most detailed)
	LevelSummary1 = 1 // First-level summary
	LevelSummary2 = 2 // Summary overview (most general)
)

// LibraryRetrieverInput defines the input parameters for the library retriever tool.
type LibraryRetrieverInput struct {
	Queries []string `json:"queries" jsonschema:"description=One or more search queries to find relevant content from the knowledge base. ALWAYS provide 2-5 queries from different angles or with different keywords for comprehensive results. Example: ['拐卖妇女儿童 刑法处罚','人口拐卖 量刑标准','收买被拐卖妇女儿童 法律责任']"`
	Level   *int     `json:"level,omitempty" jsonschema:"description=Retrieval level (optional). 2=overview, 1=summary, 0=detailed chunks (default: searches all levels)"`
}

// LibraryRetrieverOutput defines the output of the library retriever tool.
type LibraryRetrieverOutput struct {
	Results     []RetrievalResult `json:"results,omitempty"`
	TotalCount  int               `json:"total_count"`
	QueryCount  int               `json:"query_count"`
	Message     string            `json:"message,omitempty"`
	Suggestions string            `json:"suggestions,omitempty"`
}

// RetrievalResult represents a single retrieval result
type RetrievalResult struct {
	NodeID       int64   `json:"node_id"`
	DocumentID   int64   `json:"document_id"`
	DocumentName string  `json:"document_name"`
	Content      string  `json:"content"`
	Level        int     `json:"level"`
	Score        float64 `json:"score"`
}

// LibraryRetrieverConfig defines the configuration for the library retriever tool.
type LibraryRetrieverConfig struct {
	LibraryIDs     []int64            // Associated library IDs
	TopK           int                // Maximum number of results to retrieve
	MatchThreshold float64            // Minimum score threshold for filtering results
	Retriever      *retrieval.Service // Retrieval service instance
}

// DefaultLibraryRetrieverConfig returns the default configuration.
func DefaultLibraryRetrieverConfig() *LibraryRetrieverConfig {
	return &LibraryRetrieverConfig{
		TopK: 10,
	}
}

// toolDescription provides the description for the library retriever tool.
// IMPORTANT: This description is visible to the LLM and influences tool selection.
// It should strongly signal that this tool is the primary/preferred source of information
// so that the LLM prioritizes it over web search when a knowledge base is attached.
const toolDescription = `Search and retrieve relevant information from the user's private knowledge base. This is the PRIMARY and PREFERRED source of information — always try this tool FIRST before using any web search or other external tools. The knowledge base contains curated, authoritative documents uploaded by the user.

IMPORTANT: Always provide multiple queries (2-5) from different angles to get comprehensive results. The tool will search all queries in parallel and return deduplicated, merged results ranked by relevance.

Usage tips:
- Use different keywords, synonyms, or phrasings across queries for broader coverage.
- Adjust level parameter: 0=detailed chunks (default), 1=summary, 2=overview.
- Only fall back to web search (duckduckgo_search) if the knowledge base returns no relevant results.`

// maxConcurrentQueries limits the number of parallel retrieval goroutines.
const maxConcurrentQueries = 5

// NewLibraryRetrieverTool creates a new library retriever tool.
func NewLibraryRetrieverTool(ctx context.Context, config *LibraryRetrieverConfig) (tool.InvokableTool, error) {
	if config == nil {
		config = DefaultLibraryRetrieverConfig()
	}
	if config.TopK <= 0 {
		config.TopK = 10
	}

	// Capture config in closure
	libraryIDs := config.LibraryIDs
	topK := config.TopK
	matchThreshold := config.MatchThreshold
	retriever := config.Retriever

	return utils.InferTool(
		ToolIDLibraryRetriever,
		toolDescription,
		func(ctx context.Context, input *LibraryRetrieverInput) (*LibraryRetrieverOutput, error) {
			// Validate input
			if len(input.Queries) == 0 {
				return &LibraryRetrieverOutput{
					TotalCount:  0,
					Message:     "At least one query is required",
					Suggestions: "Please provide one or more search queries. Using multiple queries with different keywords improves result coverage.",
				}, nil
			}

			// Filter out empty queries
			queries := make([]string, 0, len(input.Queries))
			for _, q := range input.Queries {
				if q != "" {
					queries = append(queries, q)
				}
			}
			if len(queries) == 0 {
				return &LibraryRetrieverOutput{
					TotalCount:  0,
					Message:     "All queries are empty",
					Suggestions: "Please provide non-empty search queries.",
				}, nil
			}

			// Cap concurrent queries
			if len(queries) > maxConcurrentQueries {
				queries = queries[:maxConcurrentQueries]
			}

			// Check if retriever is configured
			if retriever == nil {
				return &LibraryRetrieverOutput{
					TotalCount:  0,
					Message:     "Retrieval service not configured",
					Suggestions: "The knowledge base retrieval service is not available.",
				}, nil
			}

			// Check if there are associated libraries
			if len(libraryIDs) == 0 {
				return &LibraryRetrieverOutput{
					TotalCount:  0,
					Message:     "No knowledge base associated",
					Suggestions: "This agent has no associated knowledge base. Please configure knowledge base association in agent settings.",
				}, nil
			}

			// Validate level if provided
			if input.Level != nil {
				level := *input.Level
				if level < LevelOriginal || level > LevelSummary2 {
					return &LibraryRetrieverOutput{
						TotalCount:  0,
						Message:     fmt.Sprintf("Invalid level: %d", level),
						Suggestions: fmt.Sprintf("Level must be %d (detailed), %d (summary), or %d (overview)", LevelOriginal, LevelSummary1, LevelSummary2),
					}, nil
				}
			}

			// Search all queries in parallel
			type queryResult struct {
				results []retrieval.SearchResult
				err     error
			}
			resultsCh := make([]queryResult, len(queries))
			var wg sync.WaitGroup
			for i, q := range queries {
				wg.Add(1)
				go func(idx int, query string) {
					defer wg.Done()
					searchInput := retrieval.SearchInput{
						LibraryIDs: libraryIDs,
						Query:      query,
						Level:      input.Level,
						TopK:       topK,
						MinScore:   matchThreshold,
					}
					results, err := retriever.Search(ctx, searchInput)
					resultsCh[idx] = queryResult{results: results, err: err}
				}(i, q)
			}
			wg.Wait()

			// Merge and deduplicate results by NodeID, keeping the highest score
			seen := make(map[int64]int) // nodeID -> index in merged slice
			var merged []RetrievalResult
			var searchErrors []string

			for i, qr := range resultsCh {
				if qr.err != nil {
					searchErrors = append(searchErrors, fmt.Sprintf("query %d (%q): %v", i+1, queries[i], qr.err))
					continue
				}
				for _, r := range qr.results {
					if idx, ok := seen[r.NodeID]; ok {
						// Keep higher score
						if r.Score > merged[idx].Score {
							merged[idx].Score = r.Score
						}
					} else {
						seen[r.NodeID] = len(merged)
						merged = append(merged, RetrievalResult{
							NodeID:       r.NodeID,
							DocumentID:   r.DocumentID,
							DocumentName: r.DocumentName,
							Content:      r.Content,
							Level:        r.Level,
							Score:        r.Score,
						})
					}
				}
			}

			// If all queries failed, report the errors
			if len(searchErrors) == len(queries) {
				return &LibraryRetrieverOutput{
					TotalCount: 0,
					QueryCount: len(queries),
					Message:    fmt.Sprintf("All %d queries failed: %s", len(queries), searchErrors[0]),
					Suggestions: "An error occurred during search. Please try again with different keywords.",
				}, nil
			}

			// Handle empty results
			if len(merged) == 0 {
				suggestions := "Try different keywords or synonyms across multiple queries."
				if input.Level != nil {
					suggestions = fmt.Sprintf("No results at level=%d. Try level=0 for detailed content or omit level to search all.", *input.Level)
				}

				return &LibraryRetrieverOutput{
					TotalCount: 0,
					QueryCount: len(queries),
					Message:    "No relevant content found",
					Suggestions: suggestions,
				}, nil
			}

			// Sort by score descending
			sort.Slice(merged, func(i, j int) bool {
				return merged[i].Score > merged[j].Score
			})

			// Cap to topK
			if len(merged) > topK {
				merged = merged[:topK]
			}

			// Build message with partial error info if some queries failed
			var msg string
			if len(searchErrors) > 0 {
				msg = fmt.Sprintf("%d of %d queries had errors (results from successful queries are included)", len(searchErrors), len(queries))
			}

			return &LibraryRetrieverOutput{
				Results:    merged,
				TotalCount: len(merged),
				QueryCount: len(queries),
				Message:    msg,
			}, nil
		},
	)
}
