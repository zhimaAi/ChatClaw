package tools

import (
	"context"
	"fmt"

	"willchat/internal/services/retrieval"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// LibraryRetrieverInput defines the input parameters for the library retriever tool.
type LibraryRetrieverInput struct {
	Query string `json:"query" jsonschema:"description=Search query keywords or natural language question to find relevant content from the knowledge base"`
	Level *int   `json:"level,omitempty" jsonschema:"description=Retrieval level (optional). 2=summary overview (most general), 1=first-level summary, 0=original document chunks (most detailed). If not specified, searches all levels. Tip: Start with higher levels for overview, use level=0 for detailed original content"`
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

Usage suggestions:
1. First retrieval can omit level parameter to get results from all levels
2. For overview information, use level=2 (summary overview) or level=1 (first-level summary)
3. For detailed original content, use level=0 (original document chunks)
4. If results are not ideal, try:
   - Adjust keywords (synonyms, more specific/broader expressions)
   - Change retrieval level
   - Split complex questions into multiple simple searches
5. Multiple searches with different keywords can help find more comprehensive results`

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
				if level < 0 || level > 2 {
					return &LibraryRetrieverOutput{
						TotalCount:  0,
						Message:     fmt.Sprintf("Invalid level value: %d", level),
						Suggestions: "Level must be 0 (original chunks), 1 (first-level summary), or 2 (overview summary). Try without level parameter to search all levels.",
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
				suggestions := "Tip: Please try different keywords or synonyms. "
				if input.Level != nil {
					suggestions += fmt.Sprintf(
						"You specified level=%d, but some documents may not have content at this level (RAPTOR summarization may not be enabled). "+
							"Consider removing the level parameter or using level=0 to search original document chunks.",
						*input.Level,
					)
				} else {
					suggestions += "You can also try using level=0 to search only original document chunks for more detailed content."
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
