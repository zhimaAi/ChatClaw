package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"chatclaw/internal/services/mcp"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// MCPToolsResult holds the loaded MCP tools and a cleanup function
// that must be called when the tools are no longer needed.
type MCPToolsResult struct {
	Tools   []tool.BaseTool
	Cleanup func()
}

// LoadMCPTools connects to all enabled MCP servers, fetches their tools via
// eino-ext/mcp, and returns them as eino BaseTools. Individual server failures
// are logged but do not prevent other servers from loading.
//
// Each tool is prefixed with the server name (e.g. "mcp__chatwiki__search")
// so the model can identify which MCP server a tool belongs to.
func LoadMCPTools(ctx context.Context, servers []mcp.MCPServer, logger *slog.Logger) *MCPToolsResult {
	if len(servers) == 0 {
		return &MCPToolsResult{Cleanup: func() {}}
	}

	var allTools []tool.BaseTool
	var clients []*mcpclient.Client

	for _, srv := range servers {
		timeout := srv.Timeout
		if timeout <= 0 {
			timeout = 30
		}
		connCtx, connCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)

		cli, err := createMCPClient(connCtx, srv)
		connCancel()
		if err != nil {
			logger.Warn("[mcp] failed to create client", "server", srv.Name, "transport", srv.Transport, "error", err)
			continue
		}

		initCtx, initCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		initReq := mcpgo.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo = mcpgo.Implementation{
			Name:    "ChatClaw",
			Version: "1.0.0",
		}
		_, err = cli.Initialize(initCtx, initReq)
		initCancel()
		if err != nil {
			logger.Warn("[mcp] failed to initialize", "server", srv.Name, "error", err)
			cli.Close()
			continue
		}

		toolsCtx, toolsCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		rawTools, err := mcpp.GetTools(toolsCtx, &mcpp.Config{Cli: cli})
		toolsCancel()
		if err != nil {
			logger.Warn("[mcp] failed to get tools", "server", srv.Name, "error", err)
			cli.Close()
			continue
		}

		prefix := sanitizeServerName(srv.Name)
		for _, t := range rawTools {
			allTools = append(allTools, &mcpToolWrapper{inner: t, serverName: srv.Name, prefix: prefix})
		}

		logger.Info("[mcp] loaded tools from server", "server", srv.Name, "count", len(rawTools))
		clients = append(clients, cli)
	}

	return &MCPToolsResult{
		Tools: allTools,
		Cleanup: func() {
			for _, cli := range clients {
				cli.Close()
			}
		},
	}
}

// sanitizeServerName converts a user-defined server name into a safe
// identifier fragment for tool naming. Keeps Unicode letters (Chinese, etc.)
// and digits; replaces runs of other characters with "_"; strips
// leading/trailing underscores. ASCII letters are lowercased.
func sanitizeServerName(name string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if r < 128 {
				r = unicode.ToLower(r)
			}
			b.WriteRune(r)
			prevUnderscore = false
		} else if !prevUnderscore {
			b.WriteRune('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// mcpToolWrapper wraps an MCP tool to add a server-name prefix to its name
// and annotate its description, following the mcp__<server>__<tool> convention.
type mcpToolWrapper struct {
	inner      tool.BaseTool
	serverName string // original display name
	prefix     string // sanitized name for the tool ID
}

func (w *mcpToolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	info, err := w.inner.Info(ctx)
	if err != nil {
		return nil, err
	}
	wrapped := *info
	wrapped.Name = fmt.Sprintf("mcp__%s__%s", w.prefix, info.Name)
	wrapped.Desc = fmt.Sprintf("[MCP server: %s] %s", w.serverName, info.Desc)
	return &wrapped, nil
}

func (w *mcpToolWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	if invokable, ok := w.inner.(tool.InvokableTool); ok {
		return invokable.InvokableRun(ctx, argumentsInJSON, opts...)
	}
	return "", fmt.Errorf("underlying MCP tool does not implement InvokableRun")
}

func createMCPClient(_ context.Context, srv mcp.MCPServer) (*mcpclient.Client, error) {
	switch srv.Transport {
	case "stdio":
		if srv.Command == "" {
			return nil, fmt.Errorf("stdio server %q has no command", srv.Name)
		}

		var args []string
		if srv.Args != "" && srv.Args != "[]" {
			if err := json.Unmarshal([]byte(srv.Args), &args); err != nil {
				return nil, fmt.Errorf("invalid args JSON for %q: %w", srv.Name, err)
			}
		}

		env := mcp.BuildEnv(srv.Env)

		opts := []transport.StdioOption{
			transport.WithCommandFunc(mcp.StdioCmdFunc),
		}

		return mcpclient.NewStdioMCPClientWithOptions(srv.Command, env, args, opts...)

	case "streamableHttp":
		if srv.URL == "" {
			return nil, fmt.Errorf("streamableHttp server %q has no URL", srv.Name)
		}

		var opts []transport.StreamableHTTPCOption

		if srv.Headers != "" && srv.Headers != "{}" {
			var headers map[string]string
			if err := json.Unmarshal([]byte(srv.Headers), &headers); err == nil && len(headers) > 0 {
				opts = append(opts, transport.WithHTTPHeaders(headers))
			}
		}

		timeout := srv.Timeout
		if timeout <= 0 {
			timeout = 30
		}
		opts = append(opts, transport.WithHTTPTimeout(time.Duration(timeout)*time.Second))

		return mcpclient.NewStreamableHttpClient(srv.URL, opts...)

	default:
		return nil, fmt.Errorf("unsupported transport %q for server %q", srv.Transport, srv.Name)
	}
}
