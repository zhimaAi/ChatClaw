package tools

import (
	"context"

	"github.com/cloudwego/eino-ext/components/tool/sequentialthinking"
	"github.com/cloudwego/eino/components/tool"
)

// NewSequentialThinkingTool creates a new sequential thinking tool.
// It supports dynamic and reflective problem solving through structured thought processes.
func NewSequentialThinkingTool(_ context.Context) (tool.InvokableTool, error) {
	return sequentialthinking.NewTool()
}
