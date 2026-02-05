package tools

// ToolsConfig defines the configuration for enabling/disabling tools.
// This is reserved for future use - currently all tools are enabled by default.
type ToolsConfig struct {
	Calculator       bool `json:"calculator"`
	DuckDuckGoSearch bool `json:"duckduckgo_search"`
}

// DefaultToolsConfig returns a default configuration with all tools enabled.
func DefaultToolsConfig() *ToolsConfig {
	return &ToolsConfig{
		Calculator:       true,
		DuckDuckGoSearch: true,
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
	default:
		return false
	}
}

// Tool IDs
const (
	ToolIDCalculator       = "calculator"
	ToolIDDuckDuckGoSearch = "duckduckgo_search"
)
