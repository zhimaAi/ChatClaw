package openclawagents

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
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func defaultWorkDir() string {
	dir, err := define.OpenClawDataRootDir()
	if err != nil {
		return ""
	}
	return dir
}

func (s *OpenClawAgentsService) GetDefaultWorkDir() string {
	return defaultWorkDir()
}

// GatewayAgentOps is the interface for direct Gateway agent operations.
// Implemented by openclawruntime.AgentService; injected to avoid circular imports.
type GatewayAgentOps interface {
	OnAgentCreated(agent OpenClawAgent)
	OnAgentUpdated(agent OpenClawAgent)
	OnAgentDeleted(openclawAgentID string)
}

type OpenClawAgentsService struct {
	app       *application.App
	testDB    *bun.DB
	gatewayMu sync.RWMutex
	gateway   GatewayAgentOps
}

func NewOpenClawAgentsService(app *application.App) *OpenClawAgentsService {
	return &OpenClawAgentsService{app: app}
}

// SetGateway injects the Gateway agent operations. Called once during bootstrap.
func (s *OpenClawAgentsService) SetGateway(gw GatewayAgentOps) {
	s.gatewayMu.Lock()
	defer s.gatewayMu.Unlock()
	s.gateway = gw
}

func (s *OpenClawAgentsService) db() (*bun.DB, error) {
	if s.testDB != nil {
		return s.testDB, nil
	}
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

func (s *OpenClawAgentsService) getGateway() GatewayAgentOps {
	s.gatewayMu.RLock()
	defer s.gatewayMu.RUnlock()
	return s.gateway
}

func newOpenClawAgentModel(name, openclawAgentID, icon string) *openClawAgentModel {
	return &openClawAgentModel{
		Name:            strings.TrimSpace(name),
		OpenClawAgentID: strings.TrimSpace(openclawAgentID),
		Icon:            strings.TrimSpace(icon),

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

		SandboxMode:    "off",
		SandboxNetwork: true,
		WorkDir:        defaultWorkDir(),

		MCPServerIDs:        "[]",
		MCPServerEnabledIDs: "[]",

		IdentityEmoji: "",
		IdentityTheme: "",

		GroupChatMentionPatterns: "[]",

		ToolsProfile: "",
		ToolsAllow:   "[]",
		ToolsDeny:    "[]",

		HeartbeatEvery: "",

		ParamsTemperature: "",
		ParamsMaxTokens:   "",
	}
}

func (s *OpenClawAgentsService) EnsureMainAgent() error {
	db, err := s.db()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent := newOpenClawAgentModel(
		define.DefaultAgentNameForLocale(i18n.GetLocale()),
		define.OpenClawMainAgentID,
		"",
	)

	if _, err := db.NewInsert().Model(agent).
		On("CONFLICT (openclaw_agent_id) DO NOTHING").
		Exec(ctx); err != nil {
		return errs.Wrap("error.agent_create_failed", err)
	}

	return nil
}

func (s *OpenClawAgentsService) ListAgents() ([]OpenClawAgent, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]openClawAgentModel, 0)
	if err := db.NewSelect().
		Model(&models).
		OrderExpr("updated_at DESC, id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.agent_list_failed", err)
	}

	out := make([]OpenClawAgent, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

// ListAgentsForOpenClawSync returns agents in a stable order for config reconciliation.
func (s *OpenClawAgentsService) ListAgentsForOpenClawSync() ([]OpenClawAgent, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]openClawAgentModel, 0)
	if err := db.NewSelect().
		Model(&models).
		OrderExpr("CASE WHEN openclaw_agent_id = ? THEN 0 ELSE 1 END ASC, id ASC", define.OpenClawMainAgentID).
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.agent_list_failed", err)
	}

	out := make([]OpenClawAgent, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

func (s *OpenClawAgentsService) GetAgent(id int64) (*OpenClawAgent, error) {
	if id <= 0 {
		return nil, errs.New("error.agent_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m openClawAgentModel
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

func (s *OpenClawAgentsService) CreateAgent(input CreateOpenClawAgentInput) (*OpenClawAgent, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.agent_name_required")
	}
	if len([]rune(name)) > 100 {
		return nil, errs.New("error.agent_name_too_long")
	}

	icon := strings.TrimSpace(input.Icon)
	if icon != "" && len(icon) > 250_000 {
		return nil, errs.New("error.agent_icon_too_large")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	m := newOpenClawAgentModel(name, define.NewOpenClawManagedAgentID(), icon)
	m.IdentityEmoji = strings.TrimSpace(input.IdentityEmoji)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.agent_create_failed", err)
	}

	dto := m.toDTO()
	if gw := s.getGateway(); gw != nil {
		go gw.OnAgentCreated(dto)
	}
	return &dto, nil
}

func (s *OpenClawAgentsService) GetDefaultPrompt() string {
	return ""
}

func (s *OpenClawAgentsService) ReadIconFile(path string) (string, error) {
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

func (s *OpenClawAgentsService) UpdateAgent(id int64, input UpdateOpenClawAgentInput) (*OpenClawAgent, error) {
	if id <= 0 {
		return nil, errs.New("error.agent_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

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

	q := db.NewUpdate().
		Model((*openClawAgentModel)(nil)).
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
		switch mode {
		case "off", "non-main", "all":
			// valid OpenClaw values
		default:
			mode = "off"
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

	if input.IdentityEmoji != nil {
		q = q.Set("identity_emoji = ?", strings.TrimSpace(*input.IdentityEmoji))
	}
	if input.IdentityTheme != nil {
		q = q.Set("identity_theme = ?", strings.TrimSpace(*input.IdentityTheme))
	}
	if input.GroupChatMentionPatterns != nil {
		q = q.Set("group_chat_mention_patterns = ?", strings.TrimSpace(*input.GroupChatMentionPatterns))
	}
	if input.ToolsProfile != nil {
		q = q.Set("tools_profile = ?", strings.TrimSpace(*input.ToolsProfile))
	}
	if input.ToolsAllow != nil {
		q = q.Set("tools_allow = ?", strings.TrimSpace(*input.ToolsAllow))
	}
	if input.ToolsDeny != nil {
		q = q.Set("tools_deny = ?", strings.TrimSpace(*input.ToolsDeny))
	}
	if input.HeartbeatEvery != nil {
		q = q.Set("heartbeat_every = ?", strings.TrimSpace(*input.HeartbeatEvery))
	}
	if input.ParamsTemperature != nil {
		q = q.Set("params_temperature = ?", strings.TrimSpace(*input.ParamsTemperature))
	}
	if input.ParamsMaxTokens != nil {
		q = q.Set("params_max_tokens = ?", strings.TrimSpace(*input.ParamsMaxTokens))
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
	if gw := s.getGateway(); gw != nil {
		go gw.OnAgentUpdated(*updated)
	}
	return updated, nil
}

func (s *OpenClawAgentsService) DeleteAgent(id int64) error {
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
		Table("openclaw_agents").
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
		Model((*openClawAgentModel)(nil)).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return errs.Wrap("error.agent_delete_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errs.Newf("error.agent_not_found", map[string]any{"ID": id})
	}

	if gw := s.getGateway(); gw != nil {
		go gw.OnAgentDeleted(openclawAgentID)
	}
	return nil
}

type FileEntry struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"is_dir"`
	Children []FileEntry `json:"children,omitempty"`
}

const workspaceTreeMaxDepth = 3

func idHash(id int64) string {
	h := sha256.Sum256([]byte("chatclaw:" + strconv.FormatInt(id, 10)))
	return hex.EncodeToString(h[:6])
}

func (s *OpenClawAgentsService) GetWorkspaceDir(agentID int64, conversationID int64) (string, error) {
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

func (s *OpenClawAgentsService) ListWorkspaceFiles(agentID int64, conversationID int64) ([]FileEntry, error) {
	dir, err := s.GetWorkspaceDir(agentID, conversationID)
	if err != nil {
		return nil, err
	}

	info, statErr := os.Stat(dir)
	if statErr != nil {
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
