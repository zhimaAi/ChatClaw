package mcp

import (
	"context"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/sqlite"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// MCPServer is the DB model and the DTO returned to the frontend.
type MCPServer struct {
	ID        string `json:"id"        bun:"id,pk"`
	Name      string `json:"name"      bun:"name"`
	Transport string `json:"transport" bun:"transport"` // "stdio" | "streamableHttp"
	// stdio fields
	Command string `json:"command" bun:"command"`
	Args    string `json:"args"    bun:"args"`    // JSON array string
	Env     string `json:"env"     bun:"env"`     // JSON object string
	// streamableHttp fields
	URL     string `json:"url"     bun:"url"`
	Headers string `json:"headers" bun:"headers"` // JSON object string
	// common
	Timeout   int    `json:"timeout"   bun:"timeout"` // seconds, default 30
	Enabled   bool   `json:"enabled"   bun:"enabled"`
	CreatedAt string `json:"createdAt" bun:"created_at"`
	UpdatedAt string `json:"updatedAt" bun:"updated_at"`
}

var _ bun.BeforeInsertHook = (*MCPServer)(nil)

func (*MCPServer) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// AddServerInput is the input for adding a new MCP server.
type AddServerInput struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Command   string `json:"command"`
	Args      string `json:"args"`
	Env       string `json:"env"`
	URL       string `json:"url"`
	Headers   string `json:"headers"`
	Timeout   int    `json:"timeout"`
}

// UpdateServerInput is the input for updating an existing MCP server.
type UpdateServerInput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Command   string `json:"command"`
	Args      string `json:"args"`
	Env       string `json:"env"`
	URL       string `json:"url"`
	Headers   string `json:"headers"`
	Timeout   int    `json:"timeout"`
}

// MCPService manages MCP server configurations.
type MCPService struct {
	app *application.App
}

func NewMCPService(app *application.App) *MCPService {
	return &MCPService{app: app}
}

func (s *MCPService) db() *bun.DB {
	return sqlite.DB()
}

// ListServers returns all configured MCP servers ordered by creation time.
func (s *MCPService) ListServers() ([]MCPServer, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var servers []MCPServer
	if err := db.NewSelect().
		Model(&servers).
		OrderExpr("created_at ASC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.mcp_list_failed", err)
	}
	if servers == nil {
		servers = []MCPServer{}
	}
	return servers, nil
}

// AddServer creates a new MCP server configuration.
func (s *MCPService) AddServer(input AddServerInput) (*MCPServer, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	if input.Name == "" {
		return nil, errs.New("error.mcp_name_required")
	}
	if input.Transport != "stdio" && input.Transport != "streamableHttp" {
		return nil, errs.New("error.mcp_invalid_transport")
	}

	if input.Args == "" {
		input.Args = "[]"
	}
	if input.Env == "" {
		input.Env = "{}"
	}
	if input.Headers == "" {
		input.Headers = "{}"
	}
	if input.Timeout <= 0 {
		input.Timeout = 30
	}

	server := &MCPServer{
		ID:        uuid.New().String(),
		Name:      input.Name,
		Transport: input.Transport,
		Command:   input.Command,
		Args:      input.Args,
		Env:       input.Env,
		URL:       input.URL,
		Headers:   input.Headers,
		Timeout:   input.Timeout,
		Enabled:   true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := db.NewInsert().Model(server).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.mcp_add_failed", err)
	}
	return server, nil
}

// UpdateServer updates an existing MCP server configuration.
func (s *MCPService) UpdateServer(input UpdateServerInput) (*MCPServer, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	if input.ID == "" {
		return nil, errs.New("error.mcp_id_required")
	}
	if input.Name == "" {
		return nil, errs.New("error.mcp_name_required")
	}
	if input.Transport != "stdio" && input.Transport != "streamableHttp" {
		return nil, errs.New("error.mcp_invalid_transport")
	}

	if input.Args == "" {
		input.Args = "[]"
	}
	if input.Env == "" {
		input.Env = "{}"
	}
	if input.Headers == "" {
		input.Headers = "{}"
	}
	if input.Timeout <= 0 {
		input.Timeout = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := sqlite.NowUTC()
	result, err := db.NewUpdate().
		Model((*MCPServer)(nil)).
		Set("name = ?", input.Name).
		Set("transport = ?", input.Transport).
		Set("command = ?", input.Command).
		Set("args = ?", input.Args).
		Set("env = ?", input.Env).
		Set("url = ?", input.URL).
		Set("headers = ?", input.Headers).
		Set("timeout = ?", input.Timeout).
		Set("updated_at = ?", now).
		Where("id = ?", input.ID).
		Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.mcp_update_failed", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, errs.New("error.mcp_not_found")
	}

	var server MCPServer
	if err := db.NewSelect().Model(&server).Where("id = ?", input.ID).Scan(ctx); err != nil {
		return nil, errs.Wrap("error.mcp_update_failed", err)
	}
	return &server, nil
}

// RemoveServer deletes an MCP server configuration.
func (s *MCPService) RemoveServer(id string) error {
	db := s.db()
	if db == nil {
		return errs.New("error.db_not_ready")
	}

	if id == "" {
		return errs.New("error.mcp_id_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := db.NewDelete().
		Model((*MCPServer)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.mcp_remove_failed", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errs.New("error.mcp_not_found")
	}
	return nil
}

// EnableServer enables an MCP server.
func (s *MCPService) EnableServer(id string) error {
	return s.setEnabled(id, true)
}

// DisableServer disables an MCP server.
func (s *MCPService) DisableServer(id string) error {
	return s.setEnabled(id, false)
}

func (s *MCPService) setEnabled(id string, enabled bool) error {
	db := s.db()
	if db == nil {
		return errs.New("error.db_not_ready")
	}

	if id == "" {
		return errs.New("error.mcp_id_required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := sqlite.NowUTC()
	result, err := db.NewUpdate().
		Model((*MCPServer)(nil)).
		Set("enabled = ?", enabled).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.mcp_update_failed", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errs.New("error.mcp_not_found")
	}
	return nil
}
