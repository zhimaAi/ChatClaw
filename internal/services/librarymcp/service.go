package librarymcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	einoembed "chatclaw/internal/eino/embedding"
	"chatclaw/internal/eino/processor"
	"chatclaw/internal/services/retrieval"
	"chatclaw/internal/services/settings"
	"chatclaw/internal/sqlite"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Service manages a global MCP server that exposes the local
// knowledge-base (library) as search/list tools. It starts
// automatically with the application and requires no UI interaction.
type Service struct {
	app *application.App

	mu         sync.Mutex
	httpServer *http.Server
	port       int
	token      string

	onStarted func() // called (in a goroutine) after the HTTP server starts
}

func NewService(app *application.App) *Service {
	return &Service{app: app}
}

// OnStarted registers a callback invoked after the MCP server starts successfully.
func (s *Service) OnStarted(fn func()) {
	s.onStarted = fn
}

func (s *Service) db() *bun.DB {
	return sqlite.DB()
}

// Start launches the MCP HTTP server in the background.
// Safe to call multiple times; subsequent calls are no-ops.
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer != nil {
		return
	}

	if !s.mcpEnabled() {
		return
	}

	db := s.db()
	if db == nil {
		return
	}

	// Check whether any libraries exist; skip if none.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var count int
	if err := db.NewSelect().TableExpr("library").ColumnExpr("COUNT(*)").Scan(ctx, &count); err != nil || count == 0 {
		if s.app != nil {
			s.app.Logger.Info("[library-mcp] no libraries found, skipping server start")
		}
		return
	}

	port, token := s.resolvePortAndToken()
	s.port = port
	s.token = token

	mcpSrv := mcpserver.NewMCPServer(
		"ChatClaw Knowledge Base",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	mcpSrv.AddTool(s.listLibrariesTool(), s.handleListLibraries)
	mcpSrv.AddTool(s.searchKnowledgeTool(), s.handleSearchKnowledge)

	mcpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv)

	expectedToken := s.token
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if expectedToken != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+expectedToken {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		mcpHTTP.ServeHTTP(w, r)
	})

	listenHost := "127.0.0.1"
	if define.IsServerMode() {
		listenHost = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", listenHost, port)
	s.httpServer = &http.Server{Addr: addr, Handler: mux}

	onStarted := s.onStarted
	go func() {
		if s.app != nil {
			s.app.Logger.Info("[library-mcp] starting server", "addr", addr)
		}
		if onStarted != nil {
			// Small delay so ListenAndServe has time to bind the port.
			time.Sleep(200 * time.Millisecond)
			onStarted()
		}
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if s.app != nil {
				s.app.Logger.Error("[library-mcp] server stopped", "error", err)
			}
		}
	}()
}

// Stop gracefully shuts down the MCP server.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer == nil {
		return
	}

	if s.app != nil {
		s.app.Logger.Info("[library-mcp] stopping server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.httpServer.Shutdown(ctx)
	s.httpServer = nil
}

// ConnectionInfo returns URL and token for external clients.
func (s *Service) ConnectionInfo() (url, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.httpServer == nil {
		return "", ""
	}

	host := "127.0.0.1"
	if define.IsServerMode() {
		host = "0.0.0.0"
	}
	return fmt.Sprintf("http://%s:%d/mcp", host, s.port), s.token
}

// IsRunning reports whether the MCP HTTP server is currently running.
func (s *Service) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.httpServer != nil
}

// ==================== MCP Tools ====================

func (s *Service) listLibrariesTool() mcpgo.Tool {
	return mcpgo.NewTool(
		"list_libraries",
		mcpgo.WithDescription("List all available knowledge bases (libraries) with their IDs and names"),
	)
}

func (s *Service) handleListLibraries(_ context.Context, _ mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	db := s.db()
	if db == nil {
		return mcpgo.NewToolResultError("database not ready"), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type libRow struct {
		ID   int64  `bun:"id"   json:"id"`
		Name string `bun:"name" json:"name"`
	}

	var libs []libRow
	if err := db.NewSelect().
		TableExpr("library").
		Column("id", "name").
		OrderExpr("sort_order ASC, id ASC").
		Scan(ctx, &libs); err != nil {
		return mcpgo.NewToolResultError("failed to list libraries: " + err.Error()), nil
	}

	if len(libs) == 0 {
		return mcpgo.NewToolResultText("No knowledge bases found."), nil
	}

	data, _ := json.Marshal(libs)
	return mcpgo.NewToolResultText(string(data)), nil
}

func (s *Service) searchKnowledgeTool() mcpgo.Tool {
	return mcpgo.NewTool(
		"search_knowledge",
		mcpgo.WithDescription("Search across knowledge bases using hybrid vector + full-text retrieval. Returns relevant document chunks ranked by relevance score."),
		mcpgo.WithString("query",
			mcpgo.Description("The search query in natural language"),
			mcpgo.Required(),
		),
		mcpgo.WithNumber("library_id",
			mcpgo.Description("Optional: restrict search to a specific library ID. If omitted, searches all libraries."),
		),
		mcpgo.WithNumber("top_k",
			mcpgo.Description("Maximum number of results to return (default: 10, max: 50)"),
		),
	)
}

func (s *Service) handleSearchKnowledge(_ context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil || strings.TrimSpace(query) == "" {
		return mcpgo.NewToolResultError("query parameter is required"), nil
	}

	db := s.db()
	if db == nil {
		return mcpgo.NewToolResultError("database not ready"), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := request.GetArguments()

	// Determine library IDs
	var libraryIDs []int64
	if libIDRaw, ok := args["library_id"]; ok {
		if libIDFloat, ok := libIDRaw.(float64); ok && libIDFloat > 0 {
			libraryIDs = []int64{int64(libIDFloat)}
		}
	}

	if len(libraryIDs) == 0 {
		if err := db.NewSelect().
			TableExpr("library").
			Column("id").
			OrderExpr("sort_order ASC, id ASC").
			Scan(ctx, &libraryIDs); err != nil {
			return mcpgo.NewToolResultError("failed to list libraries: " + err.Error()), nil
		}
	}

	if len(libraryIDs) == 0 {
		return mcpgo.NewToolResultText("No knowledge bases found."), nil
	}

	topK := 10
	if topKRaw, ok := args["top_k"]; ok {
		if topKFloat, ok := topKRaw.(float64); ok && topKFloat > 0 {
			topK = int(topKFloat)
			if topK > 50 {
				topK = 50
			}
		}
	}

	// Build embedder from global embedding config
	embeddingConfig, err := processor.GetEmbeddingConfig(ctx, db)
	if err != nil {
		return mcpgo.NewToolResultError("embedding not configured: " + err.Error()), nil
	}

	embedder, err := einoembed.NewEmbedder(ctx, &einoembed.ProviderConfig{
		ProviderID:   embeddingConfig.ProviderID,
		ProviderType: embeddingConfig.ProviderType,
		APIKey:       embeddingConfig.APIKey,
		APIEndpoint:  embeddingConfig.APIEndpoint,
		ModelID:      embeddingConfig.ModelID,
		Dimension:    embeddingConfig.Dimension,
		ExtraConfig:  embeddingConfig.ExtraConfig,
	})
	if err != nil {
		return mcpgo.NewToolResultError("failed to create embedder: " + err.Error()), nil
	}

	retrievalService := retrieval.NewService(db, embedder)
	results, err := retrievalService.Search(ctx, retrieval.SearchInput{
		LibraryIDs: libraryIDs,
		Query:      query,
		TopK:       topK,
	})
	if err != nil {
		return mcpgo.NewToolResultError("search failed: " + err.Error()), nil
	}

	if len(results) == 0 {
		return mcpgo.NewToolResultText("No relevant results found for: " + query), nil
	}

	type resultItem struct {
		DocumentName string  `json:"document_name"`
		Content      string  `json:"content"`
		Score        float64 `json:"score"`
	}

	items := make([]resultItem, len(results))
	for i, r := range results {
		items[i] = resultItem{
			DocumentName: r.DocumentName,
			Content:      r.Content,
			Score:        r.Score,
		}
	}

	data, _ := json.Marshal(items)
	return mcpgo.NewToolResultText(string(data)), nil
}

// BridgeScriptPath returns the path to the generated stdio-to-HTTP bridge
// script. Returns "" if AppDataDir cannot be resolved.
func (s *Service) BridgeScriptPath() string {
	dir, err := define.AppDataDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "mcp-bridge.mjs")
}

// EnsureBridgeScript writes (or overwrites) a tiny Node.js ESM script that
// bridges MCP stdio ↔ Streamable HTTP so that OpenClaw (stdio-only) can
// talk to our HTTP MCP server.
func (s *Service) EnsureBridgeScript() error {
	scriptPath := s.BridgeScriptPath()
	if scriptPath == "" {
		return fmt.Errorf("cannot resolve app data dir")
	}

	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return err
	}

	return os.WriteFile(scriptPath, []byte(bridgeScriptContent), 0o755)
}

const bridgeScriptContent = `#!/usr/bin/env node
// stdio-to-HTTP MCP bridge generated by ChatClaw.
// Reads JSON-RPC from stdin, POSTs to the Streamable HTTP MCP endpoint,
// and writes responses to stdout.

import { createInterface } from "readline";
import { request as httpRequest } from "http";

const MCP_URL = process.env.CHATCLAW_MCP_URL;
const MCP_TOKEN = process.env.CHATCLAW_MCP_TOKEN || "";

if (!MCP_URL) {
  process.stderr.write("CHATCLAW_MCP_URL is not set\n");
  process.exit(1);
}

const url = new URL(MCP_URL);
let sessionId = "";

function post(body) {
  return new Promise((resolve, reject) => {
    const headers = {
      "Content-Type": "application/json",
      Accept: "application/json, text/event-stream",
    };
    if (MCP_TOKEN) headers["Authorization"] = "Bearer " + MCP_TOKEN;
    if (sessionId) headers["Mcp-Session-Id"] = sessionId;

    const req = httpRequest(
      {
        hostname: url.hostname,
        port: url.port,
        path: url.pathname,
        method: "POST",
        headers,
      },
      (res) => {
        const sid = res.headers["mcp-session-id"];
        if (sid) sessionId = sid;

        const ct = (res.headers["content-type"] || "").toLowerCase();
        let chunks = [];
        res.on("data", (d) => chunks.push(d));
        res.on("end", () => {
          const raw = Buffer.concat(chunks).toString();
          if (ct.includes("text/event-stream")) {
            for (const line of raw.split("\n")) {
              if (line.startsWith("data: ")) {
                const payload = line.slice(6).trim();
                if (payload) {
                  process.stdout.write(payload + "\n");
                }
              }
            }
          } else if (raw.trim()) {
            process.stdout.write(raw.trim() + "\n");
          }
          resolve();
        });
      }
    );
    req.on("error", reject);
    req.write(body);
    req.end();
  });
}

const rl = createInterface({ input: process.stdin, terminal: false });
rl.on("line", async (line) => {
  const trimmed = line.trim();
  if (!trimmed) return;
  try {
    await post(trimmed);
  } catch (err) {
    process.stderr.write("bridge error: " + err.message + "\n");
  }
});
rl.on("close", () => process.exit(0));
`

// ==================== Helpers ====================

func (s *Service) mcpEnabled() bool {
	return settings.GetBool("mcp_enabled", false)
}

// resolvePortAndToken reads persisted port/token from settings or generates new ones.
func (s *Service) resolvePortAndToken() (int, string) {
	port := 0
	token := ""

	if v, ok := settings.GetValue("library_mcp_port"); ok && v != "" {
		fmt.Sscanf(v, "%d", &port)
	}
	if v, ok := settings.GetValue("library_mcp_token"); ok && v != "" {
		token = v
	}

	needSave := false

	if port == 0 || !isPortAvailable(port) {
		if p, err := findAvailablePort(); err == nil {
			port = p
			needSave = true
		}
	}

	if token == "" {
		token = generateToken()
		needSave = true
	}

	if needSave {
		db := s.db()
		if db != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			s.upsertSetting(ctx, db, "library_mcp_port", fmt.Sprintf("%d", port))
			s.upsertSetting(ctx, db, "library_mcp_token", token)
		}
	}

	return port, token
}

func (s *Service) upsertSetting(ctx context.Context, db *bun.DB, key, value string) {
	_, _ = db.NewRaw(`
		INSERT INTO settings (key, value, type, category, description, created_at, updated_at)
		VALUES (?, ?, 'string', 'mcp', '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, key, value).Exec(ctx)
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port, nil
}

func isPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}
