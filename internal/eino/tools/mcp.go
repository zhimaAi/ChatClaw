package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"chatclaw/internal/services/mcp"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
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
		tools, err := mcpp.GetTools(toolsCtx, &mcpp.Config{Cli: cli})
		toolsCancel()
		if err != nil {
			logger.Warn("[mcp] failed to get tools", "server", srv.Name, "error", err)
			cli.Close()
			continue
		}

		logger.Info("[mcp] loaded tools from server", "server", srv.Name, "count", len(tools))
		allTools = append(allTools, tools...)
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
		return mcpclient.NewStdioMCPClient(srv.Command, env, args...)

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
