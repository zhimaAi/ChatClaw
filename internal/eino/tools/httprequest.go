package tools

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// HTTPRequestConfig defines the server-side configuration for the HTTP request tool.
type HTTPRequestConfig struct {
	// DefaultHeaders are headers included in every request (e.g. User-Agent).
	DefaultHeaders map[string]string
	// Timeout for a single HTTP request.
	Timeout time.Duration
	// DefaultMaxResponseSize is the default maximum bytes to read from the response body.
	// The LLM can override this per-request via the max_response_size input parameter.
	// Default: 32 KB.
	DefaultMaxResponseSize int
	// HardMaxResponseSize is the absolute upper limit, even if the LLM requests more.
	// Default: 512 KB.
	HardMaxResponseSize int
	// AllowInsecure skips TLS certificate verification (useful for local dev).
	AllowInsecure bool
}

// DefaultHTTPRequestConfig returns the default configuration.
func DefaultHTTPRequestConfig() *HTTPRequestConfig {
	return &HTTPRequestConfig{
		DefaultHeaders: map[string]string{
			"User-Agent": "WillClaw-HTTPTool/1.0",
		},
		Timeout:                30 * time.Second,
		DefaultMaxResponseSize: 32 * 1024,  // 32 KB
		HardMaxResponseSize:    512 * 1024, // 512 KB
		AllowInsecure:          false,
	}
}

// HTTPRequestInput defines the input parameters for the HTTP request tool.
type HTTPRequestInput struct {
	// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS).
	Method string `json:"method" jsonschema:"description=HTTP method: GET POST PUT PATCH DELETE HEAD OPTIONS,enum=GET,enum=POST,enum=PUT,enum=PATCH,enum=DELETE,enum=HEAD,enum=OPTIONS"`
	// URL is the full URL to send the request to.
	URL string `json:"url" jsonschema:"description=The full URL to send the request to (e.g. https://api.example.com/users)"`
	// Headers is an optional map of request headers (e.g. Authorization, Content-Type).
	Headers map[string]string `json:"headers,omitempty" jsonschema:"description=Optional request headers as key-value pairs. Example: {\"Content-Type\": \"application/json\"\\, \"Authorization\": \"Bearer token\"}"`
	// Body is the optional request body (for POST, PUT, PATCH).
	Body string `json:"body,omitempty" jsonschema:"description=Optional request body string. For JSON APIs send a JSON string. Ignored for GET/DELETE/HEAD/OPTIONS."`
	// MaxResponseSize is the maximum number of bytes to read from the response body.
	// Recommended: 32768 (32KB) for normal APIs, 131072 (128KB) for larger responses.
	// The server enforces an upper limit of 512KB.
	MaxResponseSize int `json:"max_response_size" jsonschema:"description=Maximum response body size in bytes. Use 32768 (32KB) for normal APIs or 131072 (128KB) for larger responses. Server hard limit: 524288 (512KB)."`
}

// HTTPRequestOutput defines the output of the HTTP request tool.
type HTTPRequestOutput struct {
	// StatusCode is the HTTP response status code.
	StatusCode int `json:"status_code"`
	// Status is the human-readable status text (e.g. "200 OK").
	Status string `json:"status"`
	// ResponseHeaders contains selected response headers.
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	// Body is the response body (may be truncated).
	Body string `json:"body"`
	// Truncated indicates whether the body was truncated due to size limits.
	Truncated bool `json:"truncated,omitempty"`
	// Error is set if the request failed at the transport level.
	Error string `json:"error,omitempty"`
}

// NewHTTPRequestTool creates the HTTP request tool.
func NewHTTPRequestTool(ctx context.Context, config *HTTPRequestConfig) (tool.InvokableTool, error) {
	if config == nil {
		config = DefaultHTTPRequestConfig()
	}

	transport := &http.Transport{}
	if config.AllowInsecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	client := &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	defaultMaxSize := config.DefaultMaxResponseSize
	if defaultMaxSize <= 0 {
		defaultMaxSize = 32 * 1024
	}
	hardMaxSize := config.HardMaxResponseSize
	if hardMaxSize <= 0 {
		hardMaxSize = 512 * 1024
	}
	defaultHeaders := config.DefaultHeaders

	return utils.InferTool(
		ToolIDHTTPRequest,
		`Send an HTTP request to any URL. Supports all common HTTP methods (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS).
You can set custom headers (e.g. Authorization, Content-Type) and send a request body.
You can control the max response size via max_response_size (default 32KB, max 512KB).
Returns the status code, response headers, and response body.
Use this tool when you need to interact with web APIs, fetch web content, or test endpoints.`,
		func(ctx context.Context, input *HTTPRequestInput) (*HTTPRequestOutput, error) {
			// Determine effective max response size
			effectiveMaxSize := defaultMaxSize
			if input.MaxResponseSize > 0 {
				effectiveMaxSize = input.MaxResponseSize
			}
			if effectiveMaxSize > hardMaxSize {
				effectiveMaxSize = hardMaxSize
			}
			return executeHTTPRequest(ctx, client, input, defaultHeaders, effectiveMaxSize)
		},
	)
}

// executeHTTPRequest performs the actual HTTP request.
func executeHTTPRequest(
	ctx context.Context,
	client *http.Client,
	input *HTTPRequestInput,
	defaultHeaders map[string]string,
	maxResponseSize int,
) (*HTTPRequestOutput, error) {
	// Validate method
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	validMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true,
		"DELETE": true, "HEAD": true, "OPTIONS": true,
	}
	if !validMethods[method] {
		return &HTTPRequestOutput{
			Error: fmt.Sprintf("unsupported HTTP method: %s. Supported: GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS", input.Method),
		}, nil
	}

	// Validate URL
	url := strings.TrimSpace(input.URL)
	if url == "" {
		return &HTTPRequestOutput{
			Error: "URL is required",
		}, nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	// Build request body
	var bodyReader io.Reader
	if input.Body != "" && (method == "POST" || method == "PUT" || method == "PATCH") {
		bodyReader = strings.NewReader(input.Body)
	}

	// Create request
	httpReq, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return &HTTPRequestOutput{
			Error: fmt.Sprintf("failed to create request: %v", err),
		}, nil
	}

	// Apply default headers first
	for k, v := range defaultHeaders {
		httpReq.Header.Set(k, v)
	}

	// Apply user-specified headers (override defaults)
	for k, v := range input.Headers {
		httpReq.Header.Set(k, v)
	}

	// If body is present and Content-Type not set, default to JSON
	if input.Body != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	resp, err := client.Do(httpReq)
	if err != nil {
		return &HTTPRequestOutput{
			Error: fmt.Sprintf("request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// Read response body with size limit
	limitedReader := io.LimitReader(resp.Body, int64(maxResponseSize)+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return &HTTPRequestOutput{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Error:      fmt.Sprintf("failed to read response body: %v", err),
		}, nil
	}

	truncated := len(bodyBytes) > maxResponseSize
	if truncated {
		bodyBytes = bodyBytes[:maxResponseSize]
	}

	// Ensure valid UTF-8 (replace invalid bytes)
	bodyStr := ensureValidUTF8(string(bodyBytes))
	if truncated {
		bodyStr += "\n\n[... response truncated, exceeded size limit ...]"
	}

	// Extract useful response headers
	responseHeaders := extractResponseHeaders(resp)

	return &HTTPRequestOutput{
		StatusCode:      resp.StatusCode,
		Status:          resp.Status,
		ResponseHeaders: responseHeaders,
		Body:            bodyStr,
		Truncated:       truncated,
	}, nil
}

// extractResponseHeaders picks out commonly useful response headers.
func extractResponseHeaders(resp *http.Response) map[string]string {
	headers := make(map[string]string)
	interestingHeaders := []string{
		"Content-Type",
		"Content-Length",
		"Location",
		"Set-Cookie",
		"X-Request-Id",
		"X-RateLimit-Remaining",
		"X-RateLimit-Limit",
		"Retry-After",
		"WWW-Authenticate",
	}
	for _, h := range interestingHeaders {
		if v := resp.Header.Get(h); v != "" {
			headers[h] = v
		}
	}
	return headers
}

// ensureValidUTF8 replaces invalid UTF-8 sequences with the replacement character.
func ensureValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += size
	}
	return b.String()
}
