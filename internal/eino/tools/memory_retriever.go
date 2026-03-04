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
		Desc: selectDesc(
			`Search the user's long-term memory. MUST be called before responding whenever the user's message involves anything personal or could benefit from prior context. Provide 2-5 queries with varied keywords.`,
			`搜索用户长期记忆。当用户消息涉及个人信息或可从先前上下文获益时，必须在回复前调用。提供 2-5 个不同角度的查询关键词。`,
		),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"queries": {
				Type: schema.Array,
				Desc: selectDesc(
					"One or more search queries to find relevant content from the agent's long-term memory. ALWAYS provide 2-5 queries from different angles or with different keywords for comprehensive results.",
					"一个或多个搜索查询，用于从智能体长期记忆中查找相关内容。请始终提供 2-5 个不同角度或不同关键词的查询以获得全面结果。",
				),
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
