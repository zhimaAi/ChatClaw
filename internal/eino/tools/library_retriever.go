package tools

import (
	"context"
	"fmt"

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
	Query string `json:"query" jsonschema:"description=Search query keywords or natural language question to find relevant content from the knowledge base"`
	Level *int   `json:"level,omitempty" jsonschema:"description=Retrieval level (optional). 2=overview, 1=summary, 0=detailed chunks (default: searches all levels)"`
}

// LibraryRetrieverOutput defines the output of the library retriever tool.
type LibraryRetrieverOutput struct {
	Results     []RetrievalResult `json:"results,omitempty"`
	TotalCount  int               `json:"total_count"`
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

// toolDescription provides the description for the library retriever tool
const toolDescription = `Retrieve relevant document fragments from the associated knowledge base.

Tips: Try different keywords or adjust level (0=detailed, 1=summary, 2=overview) for better results.`

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
			if input.Query == "" {
				return &LibraryRetrieverOutput{
					TotalCount:  0,
					Message:     "Query is required",
					Suggestions: "Please provide a search query to retrieve relevant content from the knowledge base.",
				}, nil
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

			// Perform retrieval
			searchInput := retrieval.SearchInput{
				LibraryIDs: libraryIDs,
				Query:      input.Query,
				Level:      input.Level,
				TopK:       topK,
				MinScore:   matchThreshold,
			}

			results, err := retriever.Search(ctx, searchInput)
			if err != nil {
				return &LibraryRetrieverOutput{
					TotalCount:  0,
					Message:     fmt.Sprintf("Search failed: %v", err),
					Suggestions: "An error occurred during search. Please try again with different keywords.",
				}, nil
			}

			// Handle empty results
			if len(results) == 0 {
				suggestions := "Try different keywords or synonyms."
				if input.Level != nil {
					suggestions = fmt.Sprintf("No results at level=%d. Try level=0 for detailed content or omit level to search all.", *input.Level)
				}

				return &LibraryRetrieverOutput{
					TotalCount:  0,
					Message:     "No relevant content found",
					Suggestions: suggestions,
				}, nil
			}

			// Convert results
			outputResults := make([]RetrievalResult, len(results))
			for i, r := range results {
				outputResults[i] = RetrievalResult{
					NodeID:       r.NodeID,
					DocumentID:   r.DocumentID,
					DocumentName: r.DocumentName,
					Content:      r.Content,
					Level:        r.Level,
					Score:        r.Score,
				}
			}

			return &LibraryRetrieverOutput{
				Results:    outputResults,
				TotalCount: len(outputResults),
			}, nil
		},
	)
}
