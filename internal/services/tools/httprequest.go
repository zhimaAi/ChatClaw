package tools

import (
	"context"
	"net/http"
	"time"

	httpget "github.com/cloudwego/eino-ext/components/tool/httprequest/get"
	httppost "github.com/cloudwego/eino-ext/components/tool/httprequest/post"
	"github.com/cloudwego/eino/components/tool"
)

// HTTPRequestConfig defines the configuration for HTTP request tools.
type HTTPRequestConfig struct {
	Headers map[string]string
	Timeout time.Duration
}

// DefaultHTTPRequestConfig returns the default configuration.
func DefaultHTTPRequestConfig() *HTTPRequestConfig {
	return &HTTPRequestConfig{
		Timeout: 30 * time.Second,
	}
}

// NewHTTPGetTool creates a new HTTP GET request tool.
func NewHTTPGetTool(ctx context.Context, config *HTTPRequestConfig) (tool.InvokableTool, error) {
	if config == nil {
		config = DefaultHTTPRequestConfig()
	}
	return httpget.NewTool(ctx, &httpget.Config{
		ToolName:   ToolIDHTTPGet,
		ToolDesc:   "Send an HTTP GET request to fetch content from a URL. Use this tool when you need to retrieve data from web APIs or web pages.",
		Headers:    config.Headers,
		HttpClient: &http.Client{Timeout: config.Timeout},
	})
}

// NewHTTPPostTool creates a new HTTP POST request tool.
func NewHTTPPostTool(ctx context.Context, config *HTTPRequestConfig) (tool.InvokableTool, error) {
	if config == nil {
		config = DefaultHTTPRequestConfig()
	}
	return httppost.NewTool(ctx, &httppost.Config{
		ToolName:   ToolIDHTTPPost,
		ToolDesc:   "Send an HTTP POST request to submit data to a URL. Use this tool when you need to send data to web APIs.",
		Headers:    config.Headers,
		HttpClient: &http.Client{Timeout: config.Timeout},
	})
}
