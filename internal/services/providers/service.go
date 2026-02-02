package providers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"willchat/internal/define"
	"willchat/internal/errs"
	"willchat/internal/sqlite"

	"github.com/cloudwego/eino-ext/components/model/claude"
	einogemini "github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
	"google.golang.org/genai"
)

// ProvidersService 供应商服务（暴露给前端调用）
type ProvidersService struct {
	app *application.App
}

func NewProvidersService(app *application.App) *ProvidersService {
	return &ProvidersService{app: app}
}

func (s *ProvidersService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// ListProviders 获取所有供应商列表
func (s *ProvidersService) ListProviders() ([]Provider, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	models := make([]providerModel, 0)
	err = db.NewSelect().
		Model(&models).
		OrderExpr("sort_order ASC, id ASC").
		Scan(ctx)
	if err != nil {
		return nil, errs.Wrap("error.provider_list_failed", err)
	}

	out := make([]Provider, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDTO())
	}
	return out, nil
}

// GetProvider 获取单个供应商详情
func (s *ProvidersService) GetProvider(providerID string) (*Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m providerModel
	err = db.NewSelect().
		Model(&m).
		Where("provider_id = ?", providerID).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.Newf("error.provider_not_found", map[string]any{"ProviderID": providerID})
		}
		return nil, errs.Wrap("error.provider_read_failed", err)
	}
	dto := m.toDTO()
	return &dto, nil
}

// GetProviderWithModels 获取供应商及其模型列表
func (s *ProvidersService) GetProviderWithModels(providerID string) (*ProviderWithModels, error) {
	provider, err := s.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 获取该供应商的所有模型
	models := make([]modelModel, 0)
	err = db.NewSelect().
		Model(&models).
		Where("provider_id = ?", providerID).
		OrderExpr("type ASC, sort_order ASC, id ASC").
		Scan(ctx)
	if err != nil {
		return nil, errs.Wrap("error.model_list_failed", err)
	}

	// 按类型分组
	groupMap := make(map[string][]Model)
	for i := range models {
		dto := models[i].toDTO()
		groupMap[dto.Type] = append(groupMap[dto.Type], dto)
	}

	// 转换为有序的分组列表（llm 在前，embedding 在后）
	typeOrder := []string{"llm", "embedding"}
	groups := make([]ModelGroup, 0)
	for _, t := range typeOrder {
		if ms, ok := groupMap[t]; ok {
			groups = append(groups, ModelGroup{
				Type:   t,
				Models: ms,
			})
		}
	}

	return &ProviderWithModels{
		Provider:    *provider,
		ModelGroups: groups,
	}, nil
}

// UpdateProvider 更新供应商信息
func (s *ProvidersService) UpdateProvider(providerID string, input UpdateProviderInput) (*Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 构建更新语句
	q := db.NewUpdate().
		Model((*providerModel)(nil)).
		Where("provider_id = ?", providerID).
		Set("updated_at = ?", time.Now().UTC())

	if input.Enabled != nil {
		q = q.Set("enabled = ?", *input.Enabled)
	}
	if input.APIKey != nil {
		q = q.Set("api_key = ?", *input.APIKey)
	}
	if input.APIEndpoint != nil {
		q = q.Set("api_endpoint = ?", *input.APIEndpoint)
	}
	if input.ExtraConfig != nil {
		q = q.Set("extra_config = ?", *input.ExtraConfig)
	}

	result, err := q.Exec(ctx)
	if err != nil {
		return nil, errs.Wrap("error.provider_update_failed", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, errs.Newf("error.provider_not_found", map[string]any{"ProviderID": providerID})
	}

	return s.GetProvider(providerID)
}

// ResetAPIEndpoint 重置供应商的 API 地址为默认值
func (s *ProvidersService) ResetAPIEndpoint(providerID string) (*Provider, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	// 从共享配置获取默认 API 地址
	defaultEndpoint, ok := define.GetBuiltinProviderDefaultEndpoint(providerID)
	if !ok {
		// 非内置供应商，清空地址
		defaultEndpoint = ""
	}

	input := UpdateProviderInput{
		APIEndpoint: &defaultEndpoint,
	}
	return s.UpdateProvider(providerID, input)
}

// CheckAPIKeyInput 检测 API Key 的输入参数
type CheckAPIKeyInput struct {
	APIKey      string `json:"api_key"`
	APIEndpoint string `json:"api_endpoint"`
	ExtraConfig string `json:"extra_config"`
}

// CheckAPIKeyResult 检测 API Key 的结果
type CheckAPIKeyResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// CheckAPIKey 检测供应商的 API Key 是否有效
func (s *ProvidersService) CheckAPIKey(providerID string, input CheckAPIKeyInput) (*CheckAPIKeyResult, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, errs.New("error.provider_id_required")
	}

	// 获取供应商信息
	provider, err := s.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	// 获取该供应商的第一个 LLM 模型作为测试模型
	testModelID, err := s.getFirstLLMModel(providerID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 根据供应商类型调用不同的 SDK
	switch provider.Type {
	case "openai":
		return s.checkOpenAI(ctx, input, testModelID)
	case "azure":
		return s.checkAzure(ctx, input, testModelID)
	case "anthropic":
		return s.checkClaude(ctx, input, testModelID)
	case "gemini":
		return s.checkGemini(ctx, input, testModelID)
	case "ollama":
		// Ollama 本地运行，直接尝试连接检测
		return s.checkOllama(ctx, input, testModelID)
	default:
		return nil, errs.Newf("error.unsupported_provider_type", map[string]any{"Type": provider.Type})
	}
}

// getFirstLLMModel 获取供应商的第一个 LLM 模型
func (s *ProvidersService) getFirstLLMModel(providerID string) (string, error) {
	db, err := s.db()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m modelModel
	err = db.NewSelect().
		Model(&m).
		Where("provider_id = ?", providerID).
		Where("type = ?", "llm").
		OrderExpr("sort_order ASC, id ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errs.Newf("error.no_llm_model", map[string]any{"ProviderID": providerID})
		}
		return "", errs.Wrap("error.model_read_failed", err)
	}
	return m.ModelID, nil
}

// ChatModelGenerator 定义可生成消息的聊天模型接口
type ChatModelGenerator interface {
	Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

// testChatModel 使用聊天模型发送测试消息
func testChatModel(ctx context.Context, chatModel ChatModelGenerator) *CheckAPIKeyResult {
	_, err := chatModel.Generate(ctx, []*schema.Message{
		{
			Role:    schema.User,
			Content: "hi",
		},
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}
	}
	return &CheckAPIKeyResult{
		Success: true,
		Message: "",
	}
}

// checkOpenAI 使用 OpenAI SDK 检测
func (s *ProvidersService) checkOpenAI(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  input.APIKey,
		Model:   modelID,
		BaseURL: input.APIEndpoint,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// checkAzure 使用 Azure OpenAI SDK 检测
func (s *ProvidersService) checkAzure(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	// 解析 Azure 的额外配置
	var extraConfig struct {
		APIVersion string `json:"api_version"`
	}
	if input.ExtraConfig != "" {
		if err := json.Unmarshal([]byte(input.ExtraConfig), &extraConfig); err != nil {
			return &CheckAPIKeyResult{
				Success: false,
				Message: "invalid extra_config: " + err.Error(),
			}, nil
		}
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:     input.APIKey,
		Model:      modelID,
		BaseURL:    input.APIEndpoint,
		ByAzure:    true,
		APIVersion: extraConfig.APIVersion,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// checkClaude 使用 Claude SDK 检测
func (s *ProvidersService) checkClaude(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	var baseURL *string
	if input.APIEndpoint != "" {
		baseURL = &input.APIEndpoint
	}

	chatModel, err := claude.NewChatModel(ctx, &claude.Config{
		APIKey:    input.APIKey,
		Model:     modelID,
		BaseURL:   baseURL,
		MaxTokens: 100,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// checkGemini 使用 Gemini SDK 检测
func (s *ProvidersService) checkGemini(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: input.APIKey,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	chatModel, err := einogemini.NewChatModel(ctx, &einogemini.Config{
		Client: client,
		Model:  modelID,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}

// checkOllama 使用 Ollama SDK 检测
func (s *ProvidersService) checkOllama(ctx context.Context, input CheckAPIKeyInput, modelID string) (*CheckAPIKeyResult, error) {
	chatModel, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: input.APIEndpoint,
		Model:   modelID,
	})
	if err != nil {
		return &CheckAPIKeyResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return testChatModel(ctx, chatModel), nil
}
