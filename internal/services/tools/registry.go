package tools

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

// ToolFactory is a function that creates a tool.
type ToolFactory func(ctx context.Context) (tool.InvokableTool, error)

// ToolRegistry manages all available tools.
type ToolRegistry struct {
	mu        sync.RWMutex
	factories map[string]ToolFactory
}

// NewToolRegistry creates a new tool registry with default tools.
func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{
		factories: make(map[string]ToolFactory),
	}

	// Register default tools
	r.Register(ToolIDCalculator, func(ctx context.Context) (tool.InvokableTool, error) {
		return NewCalculatorTool(ctx)
	})

	r.Register(ToolIDDuckDuckGoSearch, func(ctx context.Context) (tool.InvokableTool, error) {
		return NewDuckDuckGoTool(ctx, nil)
	})

	return r
}

// Register registers a tool factory with the given ID.
func (r *ToolRegistry) Register(id string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[id] = factory
}

// GetAllTools returns all registered tools.
// Currently, all tools are enabled by default.
func (r *ToolRegistry) GetAllTools(ctx context.Context) ([]tool.BaseTool, error) {
	return r.GetEnabledTools(ctx, nil)
}

// GetEnabledTools returns tools based on the configuration.
// If config is nil, all tools are returned (default behavior).
func (r *ToolRegistry) GetEnabledTools(ctx context.Context, config *ToolsConfig) ([]tool.BaseTool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// If no config, use default (all enabled)
	if config == nil {
		config = DefaultToolsConfig()
	}

	var tools []tool.BaseTool
	for id, factory := range r.factories {
		if config.IsEnabled(id) {
			t, err := factory(ctx)
			if err != nil {
				return nil, err
			}
			tools = append(tools, t)
		}
	}

	return tools, nil
}

// GetTool returns a specific tool by ID.
func (r *ToolRegistry) GetTool(ctx context.Context, id string) (tool.InvokableTool, error) {
	r.mu.RLock()
	factory, ok := r.factories[id]
	r.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	return factory(ctx)
}

// ListToolIDs returns all registered tool IDs.
func (r *ToolRegistry) ListToolIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.factories))
	for id := range r.factories {
		ids = append(ids, id)
	}
	return ids
}
