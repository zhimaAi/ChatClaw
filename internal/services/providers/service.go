package providers

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"willchat/internal/define"
	"willchat/internal/errs"
	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
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
