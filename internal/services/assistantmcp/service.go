package assistantmcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"
	"chatclaw/internal/sqlite"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ToolEntry represents one agent exposed as an MCP tool.
type ToolEntry struct {
	AgentID         int64  `json:"agentId"`
	ToolName        string `json:"toolName"`
	ToolDescription string `json:"toolDescription"`
}

// AssistantMCP is the DB model and the DTO returned to the frontend.
type AssistantMCP struct {
	ID          string `json:"id"          bun:"id,pk"`
	Name        string `json:"name"        bun:"name"`
	Description string `json:"description" bun:"description"`
	Enabled     bool   `json:"enabled"     bun:"enabled"`
	Port        int    `json:"port"        bun:"port"`
	Token       string `json:"token"       bun:"token"`
	Tools       string `json:"tools"       bun:"tools"` // JSON array of ToolEntry
	CreatedAt   string `json:"createdAt"   bun:"created_at"`
	UpdatedAt   string `json:"updatedAt"   bun:"updated_at"`
}

var _ bun.BeforeInsertHook = (*AssistantMCP)(nil)

func (*AssistantMCP) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// ParseTools parses the Tools JSON string into a slice of ToolEntry.
func (a *AssistantMCP) ParseTools() []ToolEntry {
	var entries []ToolEntry
	if a.Tools == "" || a.Tools == "[]" {
		return entries
	}
	_ = json.Unmarshal([]byte(a.Tools), &entries)
	return entries
}

type CreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type AddToolsInput struct {
	ID       string  `json:"id"`
	AgentIDs []int64 `json:"agentIds"`
}

type UpdateToolInput struct {
	ID              string `json:"id"`
	AgentID         int64  `json:"agentId"`
	ToolName        string `json:"toolName"`
	ToolDescription string `json:"toolDescription"`
}

type RemoveToolInput struct {
	ID      string `json:"id"`
	AgentID int64  `json:"agentId"`
}

// runningServer tracks a running MCP HTTP server instance.
type runningServer struct {
	httpServer *http.Server
	port       int
	cancel     context.CancelFunc
}

// AssistantMCPService manages assistant MCP server configurations.
type AssistantMCPService struct {
	app        *application.App
	chatBridge ChatBridge
	mu         sync.Mutex
	servers    map[string]*runningServer // keyed by AssistantMCP.ID
}

func NewAssistantMCPService(app *application.App) *AssistantMCPService {
	return &AssistantMCPService{
		app:     app,
		servers: make(map[string]*runningServer),
	}
}

// SetChatBridge injects the chat bridge used by MCP tool handlers.
// Called by bootstrap after both services are created.
func (s *AssistantMCPService) SetChatBridge(bridge ChatBridge) {
	s.chatBridge = bridge
}

func (s *AssistantMCPService) db() *bun.DB {
	return sqlite.DB()
}

// List returns all assistant MCP servers ordered by creation time.
func (s *AssistantMCPService) List() ([]AssistantMCP, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var items []AssistantMCP
	if err := db.NewSelect().
		Model(&items).
		OrderExpr("created_at ASC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.assistant_mcp_list_failed", err)
	}
	if items == nil {
		items = []AssistantMCP{}
	}
	return items, nil
}

// Create adds a new assistant MCP configuration.
func (s *AssistantMCPService) Create(input CreateInput) (*AssistantMCP, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	if input.Name == "" {
		return nil, errs.New("error.assistant_mcp_name_required")
	}

	port, _ := findAvailablePort()

	item := &AssistantMCP{
		ID:          uuid.New().String(),
		Name:        input.Name,
		Description: input.Description,
		Enabled:     true,
		Port:        port,
		Token:       generateToken(),
		Tools:       "[]",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := db.NewInsert().Model(item).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.assistant_mcp_create_failed", err)
	}
	return item, nil
}

// Update modifies an existing assistant MCP configuration.
func (s *AssistantMCPService) Update(input UpdateInput) (*AssistantMCP, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	if input.ID == "" {
		return nil, errs.New("error.assistant_mcp_id_required")
	}
	if input.Name == "" {
		return nil, errs.New("error.assistant_mcp_name_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := sqlite.NowUTC()
	result, err := db.NewUpdate().
		Model((*AssistantMCP)(nil)).
		Set("name = ?", input.Name).
		Set("description = ?", input.Description).
		Set("updated_at = ?", now).
		Where("id = ?", input.ID).
		Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, errs.New("error.assistant_mcp_not_found")
	}

	var item AssistantMCP
	if err := db.NewSelect().Model(&item).Where("id = ?", input.ID).Scan(ctx); err != nil {
		return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
	}
	return &item, nil
}

// Delete removes an assistant MCP configuration and stops its server.
func (s *AssistantMCPService) Delete(id string) error {
	db := s.db()
	if db == nil {
		return errs.New("error.db_not_ready")
	}

	if id == "" {
		return errs.New("error.assistant_mcp_id_required")
	}

	s.stopServer(id)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := db.NewDelete().
		Model((*AssistantMCP)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.assistant_mcp_delete_failed", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errs.New("error.assistant_mcp_not_found")
	}
	return nil
}

// Enable enables an assistant MCP server and starts its HTTP server.
func (s *AssistantMCPService) Enable(id string) error {
	if err := s.setEnabled(id, true); err != nil {
		return err
	}
	return s.startServerByID(id)
}

// Disable disables an assistant MCP server and stops its HTTP server.
func (s *AssistantMCPService) Disable(id string) error {
	s.stopServer(id)
	return s.setEnabled(id, false)
}

func (s *AssistantMCPService) setEnabled(id string, enabled bool) error {
	db := s.db()
	if db == nil {
		return errs.New("error.db_not_ready")
	}

	if id == "" {
		return errs.New("error.assistant_mcp_id_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := sqlite.NowUTC()
	result, err := db.NewUpdate().
		Model((*AssistantMCP)(nil)).
		Set("enabled = ?", enabled).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.assistant_mcp_update_failed", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errs.New("error.assistant_mcp_not_found")
	}
	return nil
}

// AddTools associates agents as tools for an assistant MCP.
// It auto-generates tool names from agent names (sanitized to function-name convention)
// and uses the agent's prompt as the initial tool description.
func (s *AssistantMCPService) AddTools(input AddToolsInput) (*AssistantMCP, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	if input.ID == "" {
		return nil, errs.New("error.assistant_mcp_id_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var item AssistantMCP
	if err := db.NewSelect().Model(&item).Where("id = ?", input.ID).Scan(ctx); err != nil {
		return nil, errs.New("error.assistant_mcp_not_found")
	}

	existing := item.ParseTools()
	existingMap := make(map[int64]bool)
	for _, t := range existing {
		existingMap[t.AgentID] = true
	}

	// Load agent info to get names and prompts
	type agentInfo struct {
		ID     int64  `bun:"id"`
		Name   string `bun:"name"`
		Prompt string `bun:"prompt"`
	}
	var agents []agentInfo
	if len(input.AgentIDs) > 0 {
		if err := db.NewSelect().
			Table("agents").
			Column("id", "name", "prompt").
			Where("id IN (?)", bun.In(input.AgentIDs)).
			Scan(ctx, &agents); err != nil {
			return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
		}
	}

	usedNames := make(map[string]bool)
	for _, t := range existing {
		usedNames[t.ToolName] = true
	}

	for _, agent := range agents {
		if existingMap[agent.ID] {
			continue
		}
		toolName := generateToolName(agent.Name, usedNames)
		usedNames[toolName] = true

		desc := truncateString(agent.Prompt, 200)
		existing = append(existing, ToolEntry{
			AgentID:         agent.ID,
			ToolName:        toolName,
			ToolDescription: desc,
		})
	}

	toolsJSON, _ := json.Marshal(existing)
	now := sqlite.NowUTC()
	_, err := db.NewUpdate().
		Model((*AssistantMCP)(nil)).
		Set("tools = ?", string(toolsJSON)).
		Set("updated_at = ?", now).
		Where("id = ?", input.ID).
		Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
	}

	if err := db.NewSelect().Model(&item).Where("id = ?", input.ID).Scan(ctx); err != nil {
		return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
	}

	if item.Enabled {
		_ = s.restartServer(input.ID)
	}

	return &item, nil
}

// UpdateTool updates a specific tool's name and description.
func (s *AssistantMCPService) UpdateTool(input UpdateToolInput) (*AssistantMCP, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	if input.ID == "" {
		return nil, errs.New("error.assistant_mcp_id_required")
	}
	if input.ToolName == "" {
		return nil, errs.New("error.assistant_mcp_tool_name_required")
	}
	if !isValidToolName(input.ToolName) {
		return nil, errs.New("error.assistant_mcp_tool_name_invalid")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var item AssistantMCP
	if err := db.NewSelect().Model(&item).Where("id = ?", input.ID).Scan(ctx); err != nil {
		return nil, errs.New("error.assistant_mcp_not_found")
	}

	tools := item.ParseTools()
	found := false
	for i, t := range tools {
		if t.AgentID == input.AgentID {
			// Check name uniqueness among other tools
			for j, other := range tools {
				if j != i && other.ToolName == input.ToolName {
					return nil, errs.New("error.assistant_mcp_tool_name_duplicate")
				}
			}
			tools[i].ToolName = input.ToolName
			tools[i].ToolDescription = input.ToolDescription
			found = true
			break
		}
	}
	if !found {
		return nil, errs.New("error.assistant_mcp_tool_not_found")
	}

	toolsJSON, _ := json.Marshal(tools)
	now := sqlite.NowUTC()
	_, err := db.NewUpdate().
		Model((*AssistantMCP)(nil)).
		Set("tools = ?", string(toolsJSON)).
		Set("updated_at = ?", now).
		Where("id = ?", input.ID).
		Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
	}

	if err := db.NewSelect().Model(&item).Where("id = ?", input.ID).Scan(ctx); err != nil {
		return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
	}

	if item.Enabled {
		_ = s.restartServer(input.ID)
	}

	return &item, nil
}

// RemoveTool removes a specific tool from an assistant MCP.
func (s *AssistantMCPService) RemoveTool(input RemoveToolInput) (*AssistantMCP, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	if input.ID == "" {
		return nil, errs.New("error.assistant_mcp_id_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var item AssistantMCP
	if err := db.NewSelect().Model(&item).Where("id = ?", input.ID).Scan(ctx); err != nil {
		return nil, errs.New("error.assistant_mcp_not_found")
	}

	tools := item.ParseTools()
	filtered := make([]ToolEntry, 0, len(tools))
	for _, t := range tools {
		if t.AgentID != input.AgentID {
			filtered = append(filtered, t)
		}
	}

	toolsJSON, _ := json.Marshal(filtered)
	now := sqlite.NowUTC()
	_, err := db.NewUpdate().
		Model((*AssistantMCP)(nil)).
		Set("tools = ?", string(toolsJSON)).
		Set("updated_at = ?", now).
		Where("id = ?", input.ID).
		Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
	}

	if err := db.NewSelect().Model(&item).Where("id = ?", input.ID).Scan(ctx); err != nil {
		return nil, errs.Wrap("error.assistant_mcp_update_failed", err)
	}

	if item.Enabled {
		_ = s.restartServer(input.ID)
	}

	return &item, nil
}

// ==================== MCP HTTP Server ====================

func (s *AssistantMCPService) startServerByID(id string) error {
	db := s.db()
	if db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var item AssistantMCP
	if err := db.NewSelect().Model(&item).Where("id = ?", id).Scan(ctx); err != nil {
		return nil
	}

	if !item.Enabled {
		return nil
	}

	return s.startServer(item)
}

func (s *AssistantMCPService) startServer(item AssistantMCP) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, running := s.servers[item.ID]; running {
		return nil
	}

	if !s.getMCPEnabled() {
		return nil
	}

	tools := item.ParseTools()
	if len(tools) == 0 {
		return nil
	}

	mcpSrv := mcpserver.NewMCPServer(
		item.Name,
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	for _, te := range tools {
		entry := te
		mcpTool := mcpgo.NewTool(
			entry.ToolName,
			mcpgo.WithDescription(entry.ToolDescription),
			mcpgo.WithString("message",
				mcpgo.Description("The message to send to the AI assistant"),
				mcpgo.Required(),
			),
		)
		mcpSrv.AddTool(mcpTool, s.createToolHandler(entry))
	}

	port := item.Port
	if port > 0 {
		if !isPortAvailable(port) {
			newPort, err := findAvailablePort()
			if err != nil {
				if s.app != nil {
					s.app.Logger.Error("[assistant-mcp] port fallback failed", "id", item.ID, "error", err)
				}
				return fmt.Errorf("failed to find port: %w", err)
			}
			port = newPort
		}
	} else {
		newPort, err := findAvailablePort()
		if err != nil {
			if s.app != nil {
				s.app.Logger.Error("[assistant-mcp] failed to find available port", "id", item.ID, "error", err)
			}
			return fmt.Errorf("failed to find port: %w", err)
		}
		port = newPort
	}

	mcpHTTP := mcpserver.NewStreamableHTTPServer(mcpSrv)

	expectedToken := item.Token
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

		if s.app != nil {
			s.app.Logger.Info("[assistant-mcp] request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		}

		if expectedToken != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+expectedToken {
				if s.app != nil {
					s.app.Logger.Warn("[assistant-mcp] unauthorized request", "remote", r.RemoteAddr)
				}
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		mcpHTTP.ServeHTTP(w, r)
	})

	_, srvCancel := context.WithCancel(context.Background())
	listenHost := "127.0.0.1"
	if define.IsServerMode() {
		listenHost = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", listenHost, port)
	httpSrv := &http.Server{Addr: addr, Handler: mux}

	s.servers[item.ID] = &runningServer{
		httpServer: httpSrv,
		port:       port,
		cancel:     srvCancel,
	}

	// Persist the port (may have changed due to fallback)
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dbCancel()
	_, _ = s.db().NewUpdate().
		Model((*AssistantMCP)(nil)).
		Set("port = ?", port).
		Where("id = ?", item.ID).
		Exec(dbCtx)

	go func() {
		if s.app != nil {
			s.app.Logger.Info("[assistant-mcp] starting server", "name", item.Name, "addr", addr)
		}
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if s.app != nil {
				s.app.Logger.Error("[assistant-mcp] server stopped", "name", item.Name, "error", err)
			}
		}
	}()

	return nil
}

func (s *AssistantMCPService) stopServer(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	srv, ok := s.servers[id]
	if !ok {
		return
	}

	if s.app != nil {
		s.app.Logger.Info("[assistant-mcp] stopping server", "id", id)
	}

	srv.cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.httpServer.Shutdown(shutdownCtx)

	delete(s.servers, id)
}

func (s *AssistantMCPService) restartServer(id string) error {
	s.stopServer(id)
	return s.startServerByID(id)
}

// createToolHandler returns an MCP tool handler that invokes the chat service
// via the ChatBridge interface.
func (s *AssistantMCPService) createToolHandler(entry ToolEntry) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		message, err := request.RequireString("message")
		if err != nil || strings.TrimSpace(message) == "" {
			return mcpgo.NewToolResultError("message parameter is required"), nil
		}

		if s.chatBridge == nil {
			return mcpgo.NewToolResultError("chat service not available"), nil
		}

		externalID := fmt.Sprintf("assistant-mcp:%d", entry.AgentID)
		convID, convErr := s.chatBridge.FindOrCreateConversation(entry.AgentID, externalID, "MCP: "+entry.ToolName)
		if convErr != nil {
			return mcpgo.NewToolResultError("failed to get conversation: " + convErr.Error()), nil
		}

		sendResult, sendErr := s.chatBridge.SendMessage(SendMessageInput{
			ConversationID: convID,
			Content:        message,
			TabID:          "assistant_mcp",
		})
		if sendErr != nil {
			return mcpgo.NewToolResultError("failed to send message: " + sendErr.Error()), nil
		}

		if waitErr := s.chatBridge.WaitForGeneration(convID, sendResult.RequestID); waitErr != nil {
			return mcpgo.NewToolResultError("generation failed: " + waitErr.Error()), nil
		}

		reply, replyErr := s.chatBridge.GetLatestReply(convID)
		if replyErr != nil || reply == "" {
			return mcpgo.NewToolResultError("no response from assistant"), nil
		}

		return mcpgo.NewToolResultText(reply), nil
	}
}

// StartEnabledServers starts MCP HTTP servers for all enabled assistant MCPs.
// Called during application startup and when the user turns global MCP on.
func (s *AssistantMCPService) StartEnabledServers() {
	if !s.getMCPEnabled() {
		return
	}

	items, err := s.List()
	if err != nil {
		if s.app != nil {
			s.app.Logger.Warn("[assistant-mcp] failed to list for startup", "error", err)
		}
		return
	}

	for _, item := range items {
		if item.Enabled && len(item.ParseTools()) > 0 {
			if err := s.startServer(item); err != nil && s.app != nil {
				s.app.Logger.Warn("[assistant-mcp] failed to start server on startup", "name", item.Name, "error", err)
			}
		}
	}
}

// StopAllServers stops all running MCP servers. Called during shutdown.
func (s *AssistantMCPService) StopAllServers() {
	s.mu.Lock()
	ids := make([]string, 0, len(s.servers))
	for id := range s.servers {
		ids = append(ids, id)
	}
	s.mu.Unlock()

	for _, id := range ids {
		s.stopServer(id)
	}
}

// ==================== Connection Info ====================

// ConnectionInfo is the DTO returned to the frontend for display.
type ConnectionInfo struct {
	URL           string `json:"url"`
	Authorization string `json:"authorization"`
}

// GetConnectionInfo returns the full URL and Authorization header for an assistant MCP.
func (s *AssistantMCPService) GetConnectionInfo(id string) (*ConnectionInfo, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}
	if id == "" {
		return nil, errs.New("error.assistant_mcp_id_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var item AssistantMCP
	if err := db.NewSelect().Model(&item).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, errs.New("error.assistant_mcp_not_found")
	}

	host := "127.0.0.1"
	if define.IsServerMode() {
		if ip := getPublicIP(); ip != "" {
			host = ip
		}
	}

	info := &ConnectionInfo{
		URL:           fmt.Sprintf("http://%s:%d/mcp", host, item.Port),
		Authorization: fmt.Sprintf("Authorization: Bearer %s", item.Token),
	}
	return info, nil
}

// getPublicIP tries to detect the machine's public/external IP address.
func getPublicIP() string {
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}
	client := &http.Client{Timeout: 3 * time.Second}
	for _, svc := range services {
		resp, err := client.Get(svc)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if ip != "" {
			return ip
		}
	}
	return ""
}

// ==================== Naming Helpers ====================

// generateToolName creates a unique, function-name-safe identifier using a
// random 8-char lowercase alphanumeric string prefixed with "tool_".
func generateToolName(_ string, usedNames map[string]bool) string {
	for i := 0; i < 100; i++ {
		candidate := "tool_" + randomAlphaNum(8)
		if !usedNames[candidate] {
			return candidate
		}
	}
	return "tool_" + randomAlphaNum(12)
}

const alphaNumChars = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomAlphaNum(n int) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphaNumChars))))
		b[i] = alphaNumChars[idx.Int64()]
	}
	return string(b)
}

// isValidToolName checks if a name is a valid function-like identifier.
func isValidToolName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
				return false
			}
		} else {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
				return false
			}
		}
	}
	return true
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// generateToken creates a 32-byte hex-encoded bearer token.
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

// getMCPEnabled reads the global mcp_enabled setting from the settings table.
// When false, assistant MCP servers must not start (client and server both disabled).
func (s *AssistantMCPService) getMCPEnabled() bool {
	db := s.db()
	if db == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var value string
	err := db.NewSelect().
		Table("settings").
		Column("value").
		Where("key = ?", "mcp_enabled").
		Limit(1).
		Scan(ctx, &value)
	if err != nil {
		return false
	}
	return value == "true"
}
