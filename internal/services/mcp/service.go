package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/toolchain"
	"chatclaw/internal/sqlite"

	"github.com/google/uuid"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// MCPServer is the DB model and the DTO returned to the frontend.
type MCPServer struct {
	ID          string `json:"id"          bun:"id,pk"`
	Name        string `json:"name"        bun:"name"`
	Description string `json:"description" bun:"description"`
	Transport   string `json:"transport"   bun:"transport"` // "stdio" | "streamableHttp"
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
	Name        string `json:"name"`
	Description string `json:"description"`
	Transport   string `json:"transport"`
	Command     string `json:"command"`
	Args        string `json:"args"`
	Env         string `json:"env"`
	URL         string `json:"url"`
	Headers     string `json:"headers"`
	Timeout     int    `json:"timeout"`
}

// UpdateServerInput is the input for updating an existing MCP server.
type UpdateServerInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Transport   string `json:"transport"`
	Command     string `json:"command"`
	Args        string `json:"args"`
	Env         string `json:"env"`
	URL         string `json:"url"`
	Headers     string `json:"headers"`
	Timeout     int    `json:"timeout"`
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

// ListEnabledServers returns all enabled MCP servers (for agent integration).
// This is a package-level function that does not require an MCPService instance.
func ListEnabledServers() ([]MCPServer, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var servers []MCPServer
	if err := db.NewSelect().
		Model(&servers).
		Where("enabled = ?", true).
		OrderExpr("created_at ASC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.mcp_list_failed", err)
	}
	return servers, nil
}

// ListEnabledServersByIDs returns enabled MCP servers filtered by the given IDs.
// This is a package-level function used by the chat service to load
// only the MCP servers selected for a specific agent.
func ListEnabledServersByIDs(ids []string) ([]MCPServer, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var servers []MCPServer
	if err := db.NewSelect().
		Model(&servers).
		Where("enabled = ?", true).
		Where("id IN (?)", bun.In(ids)).
		OrderExpr("created_at ASC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.mcp_list_failed", err)
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
	if input.Description == "" {
		return nil, errs.New("error.mcp_description_required")
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
		ID:          uuid.New().String(),
		Name:        input.Name,
		Description: input.Description,
		Transport:   input.Transport,
		Command:     input.Command,
		Args:        input.Args,
		Env:         input.Env,
		URL:         input.URL,
		Headers:     input.Headers,
		Timeout:     input.Timeout,
		Enabled:     true,
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
	if input.Description == "" {
		return nil, errs.New("error.mcp_description_required")
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
		Set("description = ?", input.Description).
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

// connect creates an MCP client from server config, performs the Initialize
// handshake, and returns the ready-to-use client. Caller must close it.
func connect(ctx context.Context, server MCPServer) (*mcpclient.Client, error) {
	if server.Transport != "stdio" && server.Transport != "streamableHttp" {
		return nil, errs.New("error.mcp_invalid_transport")
	}

	var c *mcpclient.Client
	var err error

	switch server.Transport {
	case "stdio":
		if server.Command == "" {
			return nil, errs.New("error.mcp_command_required")
		}

		var args []string
		if server.Args != "" && server.Args != "[]" {
			if jsonErr := json.Unmarshal([]byte(server.Args), &args); jsonErr != nil {
				return nil, errs.Wrap("error.mcp_invalid_args", jsonErr)
			}
		}

		env := BuildEnv(server.Env)

		opts := []transport.StdioOption{
			transport.WithCommandFunc(StdioCmdFunc),
		}

		c, err = mcpclient.NewStdioMCPClientWithOptions(server.Command, env, args, opts...)
		if err != nil {
			return nil, errs.Wrap("error.mcp_connect_failed", err)
		}

	case "streamableHttp":
		if server.URL == "" {
			return nil, errs.New("error.mcp_url_required")
		}

		timeout := server.Timeout
		if timeout <= 0 {
			timeout = 30
		}

		var opts []transport.StreamableHTTPCOption
		if server.Headers != "" && server.Headers != "{}" {
			var headers map[string]string
			if jsonErr := json.Unmarshal([]byte(server.Headers), &headers); jsonErr == nil && len(headers) > 0 {
				opts = append(opts, transport.WithHTTPHeaders(headers))
			}
		}
		opts = append(opts, transport.WithHTTPTimeout(time.Duration(timeout)*time.Second))

		c, err = mcpclient.NewStreamableHttpClient(server.URL, opts...)
		if err != nil {
			return nil, errs.Wrap("error.mcp_connect_failed", err)
		}
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "ChatClaw",
		Version: "1.0.0",
	}

	if _, err = c.Initialize(ctx, initReq); err != nil {
		_ = c.Close()
		return nil, errs.Wrap("error.mcp_connect_failed", err)
	}

	return c, nil
}

// TestServer verifies that an MCP server is reachable by performing an
// Initialize handshake and then immediately closing the connection.
func (s *MCPService) TestServer(input AddServerInput) error {
	timeout := input.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	server := MCPServer{
		Transport: input.Transport,
		Command:   input.Command,
		Args:      input.Args,
		Env:       input.Env,
		URL:       input.URL,
		Headers:   input.Headers,
		Timeout:   input.Timeout,
	}

	c, err := connect(ctx, server)
	if err != nil {
		return err
	}
	defer c.Close()
	return nil
}

// ValidateEnabledServers tests every enabled MCP server in parallel,
// disabling any that fail the connectivity check.
// Returns the list of server IDs that were disabled.
func (s *MCPService) ValidateEnabledServers() ([]string, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}

	listCtx, listCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer listCancel()

	var servers []MCPServer
	if err := db.NewSelect().
		Model(&servers).
		Where("enabled = ?", true).
		Scan(listCtx); err != nil {
		return nil, errs.Wrap("error.mcp_list_failed", err)
	}

	if len(servers) == 0 {
		return []string{}, nil
	}

	type result struct {
		id  string
		err error
	}
	ch := make(chan result, len(servers))

	for _, srv := range servers {
		go func(server MCPServer) {
			timeout := server.Timeout
			if timeout <= 0 {
				timeout = 30
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			c, err := connect(ctx, server)
			if err != nil {
				ch <- result{id: server.ID, err: err}
				return
			}
			c.Close()
			ch <- result{id: server.ID, err: nil}
		}(srv)
	}

	var disabledIDs []string
	for range servers {
		r := <-ch
		if r.err != nil {
			_ = s.setEnabled(r.id, false)
			disabledIDs = append(disabledIDs, r.id)
		}
	}
	return disabledIDs, nil
}

// ==================== Inspect ====================

// MCPToolInfo represents a tool provided by an MCP server.
type MCPToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPPromptInfo represents a prompt provided by an MCP server.
type MCPPromptInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MCPResourceInfo represents a resource provided by an MCP server.
type MCPResourceInfo struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	Description string `json:"description"`
	MIMEType    string `json:"mimeType"`
}

// InspectResult contains the capabilities of an MCP server.
type InspectResult struct {
	Tools     []MCPToolInfo     `json:"tools"`
	Prompts   []MCPPromptInfo   `json:"prompts"`
	Resources []MCPResourceInfo `json:"resources"`
}

// InspectServer connects to an MCP server by ID, queries its tools, prompts,
// and resources, then returns the aggregated result.
func (s *MCPService) InspectServer(id string) (*InspectResult, error) {
	db := s.db()
	if db == nil {
		return nil, errs.New("error.db_not_ready")
	}
	if id == "" {
		return nil, errs.New("error.mcp_id_required")
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer dbCancel()

	var server MCPServer
	if err := db.NewSelect().Model(&server).Where("id = ?", id).Scan(dbCtx); err != nil {
		return nil, errs.Wrap("error.mcp_not_found", err)
	}

	timeout := server.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	c, err := connect(ctx, server)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	result := &InspectResult{
		Tools:     []MCPToolInfo{},
		Prompts:   []MCPPromptInfo{},
		Resources: []MCPResourceInfo{},
	}

	if toolsRes, err := c.ListTools(ctx, mcp.ListToolsRequest{}); err == nil && toolsRes != nil {
		for _, t := range toolsRes.Tools {
			result.Tools = append(result.Tools, MCPToolInfo{
				Name:        t.Name,
				Description: t.Description,
			})
		}
	}

	if promptsRes, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{}); err == nil && promptsRes != nil {
		for _, p := range promptsRes.Prompts {
			result.Prompts = append(result.Prompts, MCPPromptInfo{
				Name:        p.Name,
				Description: p.Description,
			})
		}
	}

	if resourcesRes, err := c.ListResources(ctx, mcp.ListResourcesRequest{}); err == nil && resourcesRes != nil {
		for _, r := range resourcesRes.Resources {
			result.Resources = append(result.Resources, MCPResourceInfo{
				Name:        r.Name,
				URI:         r.URI,
				Description: r.Description,
				MIMEType:    r.MIMEType,
			})
		}
	}

	return result, nil
}

// StdioCmdFunc resolves the command binary using the augmented PATH from
// envVars before creating the exec.Cmd. This is necessary because GUI apps
// (especially on Windows) inherit a minimal system PATH that doesn't include
// directories like the toolchain bin folder where npx.exe / bun.exe / uvx.exe live.
//
// It also rewrites "npx" → "bunx" when the resolved npx lives inside the
// toolchain bin directory, because bun's npx shim is not compatible with
// the Node.js npx CLI syntax (e.g. npx @scope/pkg@version).
func StdioCmdFunc(ctx context.Context, command string, envVars []string, args []string) (*exec.Cmd, error) {
	mergedEnv := mergeEnvVars(os.Environ(), envVars)

	resolved := command
	for _, e := range envVars {
		upper := e
		if len(upper) > 5 {
			upper = upper[:5]
		}
		if equalFoldASCII(upper, "PATH=") {
			if p := lookPathIn(command, e[5:]); p != "" {
				resolved = p
			}
			break
		}
	}

	resolved = rewriteToolchainAlias(resolved)

	cmd := exec.CommandContext(ctx, resolved, args...)
	cmd.Env = mergedEnv

	// Hide console window on Windows to avoid flashing CMD popup
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}
	}

	return cmd, nil
}

// rewriteToolchainAlias rewrites commands that are bun aliases in the
// toolchain bin dir to their correct equivalents.
// "npx" → "bunx": bun's npx shim doesn't support Node.js npx syntax.
func rewriteToolchainAlias(resolved string) string {
	binDir := toolchain.BinDirIfReady()
	if binDir == "" {
		return resolved
	}

	base := filepath.Base(resolved)
	dir := filepath.Dir(resolved)

	// Only rewrite if the binary lives in the toolchain bin directory.
	cleanDir := filepath.Clean(dir)
	cleanBin := filepath.Clean(binDir)
	if cleanDir != cleanBin {
		return resolved
	}

	switch stripExe(base) {
	case "npx":
		replacement := "bunx"
		if runtime.GOOS == "windows" {
			replacement = "bunx.exe"
		}
		return filepath.Join(dir, replacement)
	}

	return resolved
}

func stripExe(name string) string {
	if runtime.GOOS == "windows" {
		for _, ext := range []string{".exe", ".cmd", ".bat", ".com"} {
			if len(name) > len(ext) && equalFoldASCII(name[len(name)-len(ext):], ext) {
				return name[:len(name)-len(ext)]
			}
		}
	}
	return name
}

// mergeEnvVars merges extra vars into base, replacing existing keys
// (case-insensitive on Windows, case-sensitive elsewhere).
func mergeEnvVars(base, extra []string) []string {
	result := make([]string, len(base))
	copy(result, base)

	for _, ev := range extra {
		idx := indexByte(ev, '=')
		if idx <= 0 {
			continue
		}
		key := ev[:idx]
		replaced := false
		for i, existing := range result {
			eIdx := indexByte(existing, '=')
			if eIdx <= 0 {
				continue
			}
			if envKeyEqual(existing[:eIdx], key) {
				result[i] = ev
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, ev)
		}
	}
	return result
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func envKeyEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return equalFoldASCII(a, b)
	}
	return a == b
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// lookPathIn searches for an executable in the given PATH string
// without modifying the global environment (thread-safe).
func lookPathIn(name, pathEnv string) string {
	var exts []string
	if runtime.GOOS == "windows" {
		exts = []string{"", ".exe", ".cmd", ".bat", ".com"}
	} else {
		exts = []string{""}
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			p := filepath.Join(dir, name+ext)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
	}
	return ""
}

// BuildEnv returns extra env vars to pass to the MCP subprocess.
// The mcp-go library already inherits os.Environ(), so we only return
// the toolchain PATH override and user-defined vars — NOT the full env.
// The toolchain bin dir is prepended to PATH so that the app-managed
// tools (bun, uv, etc.) are found even if the user has no global install.
func BuildEnv(envJSON string) []string {
	var extra []string

	if binDir := toolchain.BinDirIfReady(); binDir != "" {
		currentPath := os.Getenv("PATH")
		pathKey := "PATH"
		if runtime.GOOS == "windows" {
			pathKey = "Path"
		}
		extra = append(extra, fmt.Sprintf("%s=%s%c%s", pathKey, binDir, os.PathListSeparator, currentPath))
	}

	if envJSON == "" || envJSON == "{}" {
		return extra
	}

	var userEnv map[string]string
	if err := json.Unmarshal([]byte(envJSON), &userEnv); err != nil {
		return extra
	}

	for k, v := range userEnv {
		extra = append(extra, fmt.Sprintf("%s=%s", k, v))
	}
	return extra
}
