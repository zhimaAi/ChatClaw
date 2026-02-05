package tools

// ToolsConfig defines the configuration for enabling/disabling tools.
// This is reserved for future use - currently all tools are enabled by default.
type ToolsConfig struct {
	Calculator          bool `json:"calculator"`
	DuckDuckGoSearch    bool `json:"duckduckgo_search"`
	BrowserUse          bool `json:"browser_use"`
	HTTPGet             bool `json:"http_get"`
	HTTPPost            bool `json:"http_post"`
	SequentialThinking  bool `json:"sequential_thinking"`
	Wikipedia           bool `json:"wikipedia_search"`
}

// DefaultToolsConfig returns a default configuration with all tools enabled.
func DefaultToolsConfig() *ToolsConfig {
	return &ToolsConfig{
		Calculator:         true,
		DuckDuckGoSearch:   true,
		BrowserUse:         true,
		HTTPGet:            true,
		HTTPPost:           true,
		SequentialThinking: true,
		Wikipedia:          true,
	}
}

// IsEnabled checks if a specific tool is enabled in the configuration.
func (c *ToolsConfig) IsEnabled(toolID string) bool {
	if c == nil {
		return true // Default to enabled if no config
	}
	switch toolID {
	case ToolIDCalculator:
		return c.Calculator
	case ToolIDDuckDuckGoSearch:
		return c.DuckDuckGoSearch
	case ToolIDBrowserUse:
		return c.BrowserUse
	case ToolIDHTTPGet:
		return c.HTTPGet
	case ToolIDHTTPPost:
		return c.HTTPPost
	case ToolIDSequentialThinking:
		return c.SequentialThinking
	case ToolIDWikipedia:
		return c.Wikipedia
	default:
		return false
	}
}

// Tool IDs
const (
	ToolIDCalculator          = "calculator"
	ToolIDDuckDuckGoSearch    = "duckduckgo_search"
	ToolIDBrowserUse          = "browser_use"
	ToolIDHTTPGet             = "http_get"
	ToolIDHTTPPost            = "http_post"
	ToolIDSequentialThinking  = "sequential_thinking"
	ToolIDWikipedia           = "wikipedia_search"
)
