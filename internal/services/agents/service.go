package agents

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/errs"
	"chatclaw/internal/services/i18n"
	"chatclaw/internal/services/memory"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func defaultWorkDir() string {
	dir, err := define.AppDataDir()
	if err != nil {
		return ""
	}
	return dir
}

// GetDefaultWorkDir returns the default working directory path for agents.
func (s *AgentsService) GetDefaultWorkDir() string {
	return defaultWorkDir()
}

// AgentsService 助手服务（暴露给前端调用）
type AgentsService struct {
	app        *application.App
	testDB     *bun.DB
	changeMu   sync.RWMutex
	changeHook func()
}

func NewAgentsService(app *application.App) *AgentsService {
	return &AgentsService{app: app}
}

func NewAgentsServiceForTest(db *bun.DB) *AgentsService {
	return &AgentsService{testDB: db}
}

func (s *AgentsService) db() (*bun.DB, error) {
	if s.testDB != nil {
		return s.testDB, nil
	}
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// SetChangeHook registers a callback invoked after successful agent mutations.
func (s *AgentsService) SetChangeHook(fn func()) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	s.changeHook = fn
}

func (s *AgentsService) notifyChange() {
	s.changeMu.RLock()
	fn := s.changeHook
	s.changeMu.RUnlock()
	if fn != nil {
		go fn()
	}
}

func newAgentModel(name, openclawAgentID, prompt, icon string) *agentModel {
	return &agentModel{
		Name:            strings.TrimSpace(name),
		OpenClawAgentID: strings.TrimSpace(openclawAgentID),
		Prompt:          strings.TrimSpace(prompt),
		Icon:            strings.TrimSpace(icon),

		// 允许为空：用户可在「模型设置」里选择
		DefaultLLMProviderID:    "",
		DefaultLLMModelID:       "",
		LLMTemperature:          0.5,
		LLMTopP:                 1.0,
		LLMMaxContextCount:      50,
		LLMMaxTokens:            1000,
		EnableLLMTemperature:    false,
		EnableLLMTopP:           false,
		EnableLLMMaxTokens:      false,
		RetrievalMatchThreshold: 0.5,
		RetrievalTopK:           20,

		SandboxMode:    "codex",
		SandboxNetwork: true,
		WorkDir:        defaultWorkDir(),

		MCPServerIDs:        "[]",
		MCPServerEnabledIDs: "[]",
	}
}

// EnsureMainAgent guarantees that the system default agent mapped to OpenClaw "main" exists.
// Uses INSERT ... ON CONFLICT DO NOTHING for atomic idempotent insertion.
func (s *AgentsService) EnsureMainAgent() error {
	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent := newAgentModel(
		define.DefaultAgentNameForLocale(i18n.GetLocale()),
		define.OpenClawMainAgentID,
		define.DefaultAgentPromptForLocale(i18n.GetLocale()),
		"",
	)

	result, err := db.NewInsert().Model(agent).
		On("CONFLICT (openclaw_agent_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.agent_create_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		s.notifyChange()
	}
	return nil
}

func (s *AgentsService) ListAgentsForMatching() ([]AgentMatch, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	matches := make([]AgentMatch, 0)
	if err := db.NewSelect().
		Table("agents").
		Column("id", "name").
		OrderExpr("updated_at DESC, id DESC").
		Scan(ctx, &matches); err != nil {
		return nil, errs.Wrap("error.agent_list_failed", err)
	}
	return matches, nil
}

func (s *AgentsService) MatchAgentsByName(query string) ([]AgentMatch, string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []AgentMatch{}, "none", nil
	}

	agents, err := s.ListAgentsForMatching()
	if err != nil {
		return nil, "", err
	}

	normalizedQuery := strings.ToLower(query)
	exact := make([]AgentMatch, 0)
	contains := make([]AgentMatch, 0)
	for _, agent := range agents {
		normalizedName := strings.ToLower(strings.TrimSpace(agent.Name))
		switch {
		case normalizedName == normalizedQuery:
			exact = append(exact, agent)
		case strings.Contains(normalizedName, normalizedQuery):
			contains = append(contains, agent)
		}
	}

	switch {
	case len(exact) == 1:
		return exact, "exact", nil
	case len(exact) > 1:
		return exact, "multiple", nil
	case len(contains) == 1:
		return contains, "single", nil
	case len(contains) > 1:
		return contains, "multiple", nil
	default:
		return []AgentMatch{}, "none", nil
	}
}

func (s *AgentsService) ListAgents() ([]Agent, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]agentModel, 0)
	if err := db.NewSelect().
		Model(&models).
		OrderExpr("updated_at DESC, id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.agent_list_failed", err)
	}

	out := make([]Agent, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

// ListAgentsForOpenClawSync returns agents in a stable order for config reconciliation.
func (s *AgentsService) ListAgentsForOpenClawSync() ([]Agent, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]agentModel, 0)
	if err := db.NewSelect().
		Model(&models).
		OrderExpr("CASE WHEN openclaw_agent_id = ? THEN 0 ELSE 1 END ASC, id ASC", define.OpenClawMainAgentID).
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.agent_list_failed", err)
	}

	out := make([]Agent, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

func (s *AgentsService) GetAgent(id int64) (*Agent, error) {
	if id <= 0 {
		return nil, errs.New("error.agent_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m agentModel
	if err := db.NewSelect().
		Model(&m).
		Where("id = ?", id).
		Limit(1).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.agent_not_found", map[string]any{"ID": id})
		}
		return nil, errs.Wrap("error.agent_read_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
}

func (s *AgentsService) CreateAgent(input CreateAgentInput) (*Agent, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.agent_name_required")
	}
	if len([]rune(name)) > 100 {
		return nil, errs.New("error.agent_name_too_long")
	}

	prompt := strings.TrimSpace(input.Prompt)
	if len([]rune(prompt)) > 1000 {
		return nil, errs.New("error.agent_prompt_too_long")
	}

	icon := strings.TrimSpace(input.Icon)
	if icon != "" && len(icon) > 250_000 {
		return nil, errs.New("error.agent_icon_too_large")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	m := newAgentModel(name, define.NewOpenClawManagedAgentID(), prompt, icon)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.agent_create_failed", err)
	}

	dto := m.toDTO()
	s.notifyChange()
	return &dto, nil
}

// GetDefaultPrompt returns the default prompt based on the current i18n locale.
func (s *AgentsService) GetDefaultPrompt() string {
	return define.DefaultAgentPromptForLocale(i18n.GetLocale())
}

// ReadIconFile 将本地图片文件读取为 data URL（供前端预览 + 写入 DB）。
// 约束：最大 100KB，仅允许常见图片格式（png/jpg/jpeg/gif/webp/svg）。
func (s *AgentsService) ReadIconFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errs.New("error.agent_icon_path_required")
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return "", errs.Wrap("error.agent_icon_read_failed", err)
	}
	if len(b) == 0 {
		return "", errs.New("error.agent_icon_invalid")
	}
	if len(b) > 100*1024 {
		return "", errs.New("error.agent_icon_too_large")
	}

	ext := strings.ToLower(filepath.Ext(path))
	mime := http.DetectContentType(b)

	// svg: DetectContentType 可能返回 text/xml；这里基于扩展名兜底
	if ext == ".svg" {
		mime = "image/svg+xml"
	}

	switch mime {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml":
		// ok
	default:
		return "", errs.New("error.agent_icon_type_not_allowed")
	}

	encoded := base64.StdEncoding.EncodeToString(b)
	return "data:" + mime + ";base64," + encoded, nil
}

func (s *AgentsService) UpdateAgent(id int64, input UpdateAgentInput) (*Agent, error) {
	if id <= 0 {
		return nil, errs.New("error.agent_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 读取旧值（用于 provider/model 的组合校验）
	existing, err := s.GetAgent(id)
	if err != nil {
		return nil, err
	}

	newProviderID := existing.DefaultLLMProviderID
	newModelID := existing.DefaultLLMModelID

	if input.DefaultLLMProviderID != nil {
		newProviderID = strings.TrimSpace(*input.DefaultLLMProviderID)
	}
	if input.DefaultLLMModelID != nil {
		newModelID = strings.TrimSpace(*input.DefaultLLMModelID)
	}
	if (input.DefaultLLMProviderID != nil) || (input.DefaultLLMModelID != nil) {
		// 允许清空（删除默认模型）：
		// - provider+model 同时为空：清空
		// - 任意一个为空：输入不完整
		if newProviderID == "" && newModelID == "" {
			// ok - clear
		} else if newProviderID == "" || newModelID == "" {
			return nil, errs.New("error.agent_default_llm_incomplete")
		} else {
			if err := ensureLLMModelExists(ctx, db, newProviderID, newModelID); err != nil {
				return nil, err
			}
		}
	}

	// BeforeUpdate hook 会自动设置 updated_at
	q := db.NewUpdate().
		Model((*agentModel)(nil)).
		Where("id = ?", id)

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, errs.New("error.agent_name_required")
		}
		if len([]rune(name)) > 100 {
			return nil, errs.New("error.agent_name_too_long")
		}
		q = q.Set("name = ?", name)
	}

	if input.Prompt != nil {
		prompt := strings.TrimSpace(*input.Prompt)
		if len([]rune(prompt)) > 1000 {
			return nil, errs.New("error.agent_prompt_too_long")
		}
		q = q.Set("prompt = ?", prompt)
	}

	if input.Icon != nil {
		q = q.Set("icon = ?", strings.TrimSpace(*input.Icon))
	}

	if input.DefaultLLMProviderID != nil {
		q = q.Set("default_llm_provider_id = ?", newProviderID)
	}
	if input.DefaultLLMModelID != nil {
		q = q.Set("default_llm_model_id = ?", newModelID)
	}

	if input.LLMTemperature != nil {
		q = q.Set("llm_temperature = ?", *input.LLMTemperature)
	}
	if input.LLMTopP != nil {
		q = q.Set("llm_top_p = ?", *input.LLMTopP)
	}
	if input.LLMMaxContextCount != nil {
		q = q.Set("llm_max_context_count = ?", *input.LLMMaxContextCount)
	}
	if input.LLMMaxTokens != nil {
		q = q.Set("llm_max_tokens = ?", *input.LLMMaxTokens)
	}
	if input.EnableLLMTemperature != nil {
		q = q.Set("enable_llm_temperature = ?", *input.EnableLLMTemperature)
	}
	if input.EnableLLMTopP != nil {
		q = q.Set("enable_llm_top_p = ?", *input.EnableLLMTopP)
	}
	if input.EnableLLMMaxTokens != nil {
		q = q.Set("enable_llm_max_tokens = ?", *input.EnableLLMMaxTokens)
	}
	if input.RetrievalMatchThreshold != nil {
		if *input.RetrievalMatchThreshold < 0 || *input.RetrievalMatchThreshold > 1 {
			return nil, errs.New("error.agent_retrieval_match_threshold_invalid")
		}
		q = q.Set("retrieval_match_threshold = ?", *input.RetrievalMatchThreshold)
	}
	if input.RetrievalTopK != nil {
		if *input.RetrievalTopK <= 0 {
			return nil, errs.New("error.agent_retrieval_topk_invalid")
		}
		q = q.Set("retrieval_top_k = ?", *input.RetrievalTopK)
	}

	if input.SandboxMode != nil {
		mode := strings.TrimSpace(*input.SandboxMode)
		if mode != "codex" && mode != "native" {
			mode = "codex"
		}
		q = q.Set("sandbox_mode = ?", mode)
	}
	if input.SandboxNetwork != nil {
		q = q.Set("sandbox_network = ?", *input.SandboxNetwork)
	}
	if input.WorkDir != nil {
		q = q.Set("work_dir = ?", strings.TrimSpace(*input.WorkDir))
	}
	if input.MCPEnabled != nil {
		q = q.Set("mcp_enabled = ?", *input.MCPEnabled)
	}
	if input.MCPServerIDs != nil {
		q = q.Set("mcp_server_ids = ?", *input.MCPServerIDs)
	}
	if input.MCPServerEnabledIDs != nil {
		q = q.Set("mcp_server_enabled_ids = ?", *input.MCPServerEnabledIDs)
	}

	result, err := q.Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.agent_update_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, errs.Newf("error.agent_not_found", map[string]any{"ID": id})
	}

	updated, err := s.GetAgent(id)
	if err != nil {
		return nil, err
	}
	s.notifyChange()
	return updated, nil
}

func (s *AgentsService) DeleteAgent(id int64) error {
	if id <= 0 {
		return errs.New("error.agent_id_required")
	}

	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var openclawAgentID string
	if err := db.NewSelect().
		Table("agents").
		Column("openclaw_agent_id").
		Where("id = ?", id).
		Limit(1).
		Scan(ctx, &openclawAgentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.Newf("error.agent_not_found", map[string]any{"ID": id})
		}
		return errs.Wrap("error.agent_read_failed", err)
	}
	if openclawAgentID == define.OpenClawMainAgentID {
		return errs.New("error.agent_default_delete_forbidden")
	}

	result, err := db.NewDelete().
		Model((*agentModel)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.agent_delete_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errs.Newf("error.agent_not_found", map[string]any{"ID": id})
	}

	// 跨库级联删除该 Agent 的长期记忆数据
	if err := memory.DeleteAgentMemories(ctx, id); err != nil {
		if s.app != nil {
			s.app.Logger.Warn("failed to delete agent memories", "agent_id", id, "error", err)
		}
	}

	s.notifyChange()
	return nil
}

// FileEntry represents a file or directory in the workspace tree.
type FileEntry struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"is_dir"`
	Children []FileEntry `json:"children,omitempty"`
}

const workspaceTreeMaxDepth = 3

// idHash produces the same 12-hex-char directory name used by the agent runtime
// (see internal/eino/agent/agent.go idHash).
func idHash(id int64) string {
	h := sha256.Sum256([]byte("chatclaw:" + strconv.FormatInt(id, 10)))
	return hex.EncodeToString(h[:6])
}

// GetWorkspaceDir returns the resolved workspace directory for a given agent and conversation.
func (s *AgentsService) GetWorkspaceDir(agentID int64, conversationID int64) (string, error) {
	agent, err := s.GetAgent(agentID)
	if err != nil {
		return "", err
	}
	workDir := agent.WorkDir
	if workDir == "" {
		workDir = defaultWorkDir()
	}
	dir := filepath.Join(workDir, "sessions", idHash(agentID))
	if conversationID > 0 {
		dir = filepath.Join(dir, idHash(conversationID))
	}
	return dir, nil
}

// ListWorkspaceFiles returns the directory tree for the given agent+conversation workspace.
func (s *AgentsService) ListWorkspaceFiles(agentID int64, conversationID int64) ([]FileEntry, error) {
	dir, err := s.GetWorkspaceDir(agentID, conversationID)
	if err != nil {
		return nil, err
	}

	info, statErr := os.Stat(dir)
	if statErr != nil {
		// A missing workspace directory is expected before any file output is generated.
		if errors.Is(statErr, os.ErrNotExist) {
			return []FileEntry{}, nil
		}
		return nil, errs.Wrap("error.agent_list_files_failed", statErr)
	}
	if !info.IsDir() {
		return nil, errs.New("error.agent_workspace_dir_invalid")
	}

	entries, err := readDirTree(dir, workspaceTreeMaxDepth)
	if err != nil {
		return nil, errs.Wrap("error.agent_list_files_failed", err)
	}
	return entries, nil
}

// readDirTree recursively reads a directory tree up to maxDepth levels.
func readDirTree(dir string, maxDepth int) ([]FileEntry, error) {
	if maxDepth <= 0 {
		return []FileEntry{}, nil
	}
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	result := make([]FileEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		entry := FileEntry{
			Name:  name,
			Path:  filepath.Join(dir, name),
			IsDir: de.IsDir(),
		}
		if de.IsDir() {
			children, err := readDirTree(filepath.Join(dir, name), maxDepth-1)
			if err == nil {
				entry.Children = children
			}
		}
		result = append(result, entry)
	}
	return result, nil
}

func ensureLLMModelExists(ctx context.Context, db *bun.DB, providerID, modelID string) error {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" {
		return errs.New("error.agent_default_llm_provider_required")
	}
	if modelID == "" {
		return errs.New("error.agent_default_llm_model_required")
	}

	cnt, err := db.NewSelect().
		Table("models").
		Where("provider_id = ?", providerID).
		Where("model_id = ?", modelID).
		Where("type = ?", "llm").
		Count(ctx)
	if err != nil {
		return errs.Wrap("error.agent_llm_model_check_failed", err)
	}
	if cnt == 0 {
		return errs.Newf("error.agent_llm_model_not_found", map[string]any{
			"ProviderID": providerID,
			"ModelID":    modelID,
		})
	}
	return nil
}
