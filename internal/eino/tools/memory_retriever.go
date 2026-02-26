package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"chatclaw/internal/services/memory"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type MemoryRetrieverConfig struct {
	AgentID        int64
	TopK           int
	MatchThreshold float64
}

// MemoryRetrieverTool is a tool that searches the agent's long-term memory.
func NewMemoryRetrieverTool(ctx context.Context, config *MemoryRetrieverConfig) (tool.BaseTool, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}

	return &memoryRetrieverTool{
		config: config,
	}, nil
}

type memoryRetrieverTool struct {
	config *MemoryRetrieverConfig
}

type memoryRetrieverInput struct {
	Queries []string `json:"queries" jsonschema:"description=One or more search queries to find relevant content from the agent's long-term memory. ALWAYS provide 2-5 queries from different angles or with different keywords for comprehensive results. Example: ['user programming habits', 'favorite languages', 'project context']"`
}

func (t *memoryRetrieverTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "memory_retriever",
		Desc: "Search and retrieve relevant information from the agent's long-term memory (thematic facts and event streams). Use this tool to recall user preferences, past conversations, and important facts.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"queries": {
				Type:     schema.Array,
				Desc:     "One or more search queries to find relevant content from the agent's long-term memory. ALWAYS provide 2-5 queries from different angles or with different keywords for comprehensive results.",
				Required: true,
			},
		}),
	}, nil
}

func (t *memoryRetrieverTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var input memoryRetrieverInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parse arguments failed: %w", err)
	}

	if len(input.Queries) == 0 {
		return "", fmt.Errorf("queries cannot be empty")
	}

	results, err := memory.SearchMemories(ctx, t.config.AgentID, input.Queries, t.config.TopK, t.config.MatchThreshold)
	if err != nil {
		return "", fmt.Errorf("search memories failed: %w", err)
	}

	if len(results) == 0 {
		return "No relevant memories found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Found the following relevant memories:\n\n")
	for i, res := range results {
		sb.WriteString(fmt.Sprintf("[%d] (Type: %s, Score: %.2f) %s\n", i+1, res.Type, res.Score, res.Content))
	}

	return sb.String(), nil
}
