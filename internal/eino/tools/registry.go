package tools

import (
	"context"
	"log"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

// ToolFactory is a function that creates a tool.
// Returns tool.BaseTool to support both InvokableTool and BaseTool implementations.
type ToolFactory func(ctx context.Context) (tool.BaseTool, error)

// ToolRegistry manages all available tools.
type ToolRegistry struct {
	mu        sync.RWMutex
	factories map[string]ToolFactory
	cached    map[string]tool.BaseTool // lazily populated cache
}

// NewToolRegistry creates a new tool registry with default tools.
func NewToolRegistry() *ToolRegistry {
	r := &ToolRegistry{
		factories: make(map[string]ToolFactory),
		cached:    make(map[string]tool.BaseTool),
	}

	// Register default tools
	r.Register(ToolIDCalculator, func(ctx context.Context) (tool.BaseTool, error) {
		return NewCalculatorTool(ctx)
	})

	r.Register(ToolIDDuckDuckGoSearch, func(ctx context.Context) (tool.BaseTool, error) {
		return NewDuckDuckGoTool(ctx, nil)
	})

	r.Register(ToolIDBrowserUse, func(ctx context.Context) (tool.BaseTool, error) {
		return NewBrowserUseTool(ctx, nil)
	})

	r.Register(ToolIDHTTPGet, func(ctx context.Context) (tool.BaseTool, error) {
		return NewHTTPGetTool(ctx, nil)
	})

	r.Register(ToolIDHTTPPost, func(ctx context.Context) (tool.BaseTool, error) {
		return NewHTTPPostTool(ctx, nil)
	})

	r.Register(ToolIDSequentialThinking, func(ctx context.Context) (tool.BaseTool, error) {
		return NewSequentialThinkingTool(ctx)
	})

	r.Register(ToolIDWikipedia, func(ctx context.Context) (tool.BaseTool, error) {
		return NewWikipediaTool(ctx, nil)
	})

	return r
}

// Register registers a tool factory with the given ID.
func (r *ToolRegistry) Register(id string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[id] = factory
	// Invalidate cache when a factory is re-registered
	delete(r.cached, id)
}

// getOrCreate returns a cached tool instance or creates one via the factory.
// Must be called with write lock held.
func (r *ToolRegistry) getOrCreate(ctx context.Context, id string) (tool.BaseTool, error) {
	// Fast path: already cached (caller holds at least RLock)
	if t, ok := r.cached[id]; ok {
		return t, nil
	}

	// Slow path: create and cache (need write lock)
	factory, ok := r.factories[id]
	if !ok {
		return nil, nil
	}

	t, err := factory(ctx)
	if err != nil {
		return nil, err
	}
	r.cached[id] = t
	return t, nil
}

// GetAllTools returns all registered tools.
// Currently, all tools are enabled by default.
func (r *ToolRegistry) GetAllTools(ctx context.Context) ([]tool.BaseTool, error) {
	return r.GetEnabledTools(ctx, nil)
}

// GetEnabledTools returns tools based on the configuration.
// If config is nil, all tools are returned (default behavior).
// Tool instances are cached after first creation.
func (r *ToolRegistry) GetEnabledTools(ctx context.Context, config *ToolsConfig) ([]tool.BaseTool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If no config, use default (all enabled)
	if config == nil {
		config = DefaultToolsConfig()
	}

	var tools []tool.BaseTool
	for id := range r.factories {
		if config.IsEnabled(id) {
			t, err := r.getOrCreate(ctx, id)
			if err != nil {
				// Log warning but continue - don't fail the whole operation for one tool
				log.Printf("[tools] warning: failed to create tool %s (skipping): %v", id, err)
				continue
			}
			if t != nil {
				tools = append(tools, t)
			}
		}
	}

	return tools, nil
}

// GetTool returns a specific tool by ID (cached).
func (r *ToolRegistry) GetTool(ctx context.Context, id string) (tool.BaseTool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.getOrCreate(ctx, id)
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

// AddTool adds a pre-created tool instance directly to the cache.
// This is useful for tools that require runtime configuration (e.g., LibraryRetrieverTool).
// Note: This bypasses the factory mechanism and adds the tool directly.
func (r *ToolRegistry) AddTool(id string, t tool.BaseTool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cached[id] = t
	// Also register a factory that returns the cached instance
	r.factories[id] = func(ctx context.Context) (tool.BaseTool, error) {
		return t, nil
	}
}

// RemoveTool removes a tool from both the cache and factories.
// This is useful for removing dynamically added tools.
func (r *ToolRegistry) RemoveTool(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cached, id)
	delete(r.factories, id)
}
