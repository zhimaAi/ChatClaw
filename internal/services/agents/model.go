package agents

import (
	"context"
	"time"

	"willchat/internal/sqlite"

	"github.com/uptrace/bun"
)

// Agent 助手 DTO（暴露给前端）
type Agent struct {
	ID int64 `json:"id"`

	Name   string `json:"name"`
	Prompt string `json:"prompt"`
	Icon   string `json:"icon"`

	DefaultLLMProviderID string  `json:"default_llm_provider_id"`
	DefaultLLMModelID    string  `json:"default_llm_model_id"`
	LLMTemperature       float64 `json:"llm_temperature"`
	LLMTopP              float64 `json:"llm_top_p"`
	LLMMaxContextCount   int     `json:"llm_max_context_count"`
	LLMMaxTokens         int     `json:"llm_max_tokens"`
	EnableLLMTemperature      bool    `json:"enable_llm_temperature"`
	EnableLLMTopP             bool    `json:"enable_llm_top_p"`
	EnableLLMMaxTokens        bool    `json:"enable_llm_max_tokens"`
	RetrievalMatchThreshold   float64 `json:"retrieval_match_threshold"`
	RetrievalTopK             int     `json:"retrieval_top_k"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateAgentInput struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
	Icon   string `json:"icon"`
}

type UpdateAgentInput struct {
	Name   *string `json:"name"`
	Prompt *string `json:"prompt"`
	Icon   *string `json:"icon"`

	DefaultLLMProviderID *string `json:"default_llm_provider_id"`
	DefaultLLMModelID    *string `json:"default_llm_model_id"`

	LLMTemperature       *float64 `json:"llm_temperature"`
	LLMTopP              *float64 `json:"llm_top_p"`
	LLMMaxContextCount   *int     `json:"llm_max_context_count"`
	LLMMaxTokens         *int     `json:"llm_max_tokens"`
	EnableLLMTemperature    *bool    `json:"enable_llm_temperature"`
	EnableLLMTopP           *bool    `json:"enable_llm_top_p"`
	EnableLLMMaxTokens      *bool    `json:"enable_llm_max_tokens"`
	RetrievalMatchThreshold *float64 `json:"retrieval_match_threshold"`
	RetrievalTopK           *int     `json:"retrieval_top_k"`
}

type agentModel struct {
	bun.BaseModel `bun:"table:agents,alias:a"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	Name   string `bun:"name,notnull"`
	Prompt string `bun:"prompt,notnull"`
	Icon   string `bun:"icon,notnull"`

	DefaultLLMProviderID string  `bun:"default_llm_provider_id,notnull"`
	DefaultLLMModelID    string  `bun:"default_llm_model_id,notnull"`
	LLMTemperature       float64 `bun:"llm_temperature,notnull"`
	LLMTopP              float64 `bun:"llm_top_p,notnull"`
	LLMMaxContextCount   int     `bun:"llm_max_context_count,notnull"`
	LLMMaxTokens         int     `bun:"llm_max_tokens,notnull"`
	EnableLLMTemperature    bool    `bun:"enable_llm_temperature,notnull"`
	EnableLLMTopP           bool    `bun:"enable_llm_top_p,notnull"`
	EnableLLMMaxTokens      bool    `bun:"enable_llm_max_tokens,notnull"`
	RetrievalMatchThreshold float64 `bun:"retrieval_match_threshold,notnull"`
	RetrievalTopK           int     `bun:"retrieval_top_k,notnull"`
}

// BeforeInsert 在 INSERT 时自动设置 created_at 和 updated_at（字符串格式）
var _ bun.BeforeInsertHook = (*agentModel)(nil)

func (*agentModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

// BeforeUpdate 在 UPDATE 时自动设置 updated_at（字符串格式）
var _ bun.BeforeUpdateHook = (*agentModel)(nil)

func (*agentModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *agentModel) toDTO() Agent {
	return Agent{
		ID: m.ID,

		Name:   m.Name,
		Prompt: m.Prompt,
		Icon:   m.Icon,

		DefaultLLMProviderID: m.DefaultLLMProviderID,
		DefaultLLMModelID:    m.DefaultLLMModelID,
		LLMTemperature:       m.LLMTemperature,
		LLMTopP:              m.LLMTopP,
		LLMMaxContextCount:   m.LLMMaxContextCount,
		LLMMaxTokens:         m.LLMMaxTokens,
		EnableLLMTemperature:    m.EnableLLMTemperature,
		EnableLLMTopP:           m.EnableLLMTopP,
		EnableLLMMaxTokens:      m.EnableLLMMaxTokens,
		RetrievalMatchThreshold: m.RetrievalMatchThreshold,
		RetrievalTopK:           m.RetrievalTopK,

		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
