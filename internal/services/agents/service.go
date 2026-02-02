package agents

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"willchat/internal/errs"
	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// AgentsService 助手服务（暴露给前端调用）
type AgentsService struct {
	app *application.App
}

func NewAgentsService(app *application.App) *AgentsService {
	return &AgentsService{app: app}
}

func (s *AgentsService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
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
	if prompt == "" {
		return nil, errs.New("error.agent_prompt_required")
	}
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

	m := &agentModel{
		Name:   name,
		Prompt: prompt,
		Icon:   icon,

		// 允许为空：用户可在「模型设置」里选择
		DefaultLLMProviderID: "",
		DefaultLLMModelID:    "",
		LLMTemperature:       0.5,
		LLMTopP:              1.0,
		ContextCount:         50,
		LLMMaxTokens:         1000,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, errs.Wrap("error.agent_create_failed", err)
	}

	dto := m.toDTO()
	return &dto, nil
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
		if prompt == "" {
			return nil, errs.New("error.agent_prompt_required")
		}
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
	if input.ContextCount != nil {
		q = q.Set("context_count = ?", *input.ContextCount)
	}
	if input.LLMMaxTokens != nil {
		q = q.Set("llm_max_tokens = ?", *input.LLMMaxTokens)
	}

	result, err := q.Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.agent_update_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, errs.Newf("error.agent_not_found", map[string]any{"ID": id})
	}

	return s.GetAgent(id)
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
	return nil
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
