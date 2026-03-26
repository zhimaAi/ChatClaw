package openclawagents

import (
	"context"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
)

type OpenClawAgent struct {
	ID int64 `json:"id"`

	Name            string `json:"name"`
	OpenClawAgentID string `json:"openclaw_agent_id"`
	Icon            string `json:"icon"`

	DefaultLLMProviderID    string  `json:"default_llm_provider_id"`
	DefaultLLMModelID       string  `json:"default_llm_model_id"`
	LLMTemperature          float64 `json:"llm_temperature"`
	LLMTopP                 float64 `json:"llm_top_p"`
	LLMMaxContextCount      int     `json:"llm_max_context_count"`
	LLMMaxTokens            int     `json:"llm_max_tokens"`
	EnableLLMTemperature    bool    `json:"enable_llm_temperature"`
	EnableLLMTopP           bool    `json:"enable_llm_top_p"`
	EnableLLMMaxTokens      bool    `json:"enable_llm_max_tokens"`
	RetrievalMatchThreshold float64 `json:"retrieval_match_threshold"`
	RetrievalTopK           int     `json:"retrieval_top_k"`

	SandboxMode    string `json:"sandbox_mode"`
	SandboxNetwork bool   `json:"sandbox_network"`
	WorkDir        string `json:"work_dir"`

	MCPEnabled          bool   `json:"mcp_enabled"`
	MCPServerIDs        string `json:"mcp_server_ids"`
	MCPServerEnabledIDs string `json:"mcp_server_enabled_ids"`

	// OpenClaw identity
	IdentityEmoji string `json:"identity_emoji"`
	IdentityTheme string `json:"identity_theme"`

	// OpenClaw group chat
	GroupChatMentionPatterns string `json:"group_chat_mention_patterns"`

	// OpenClaw tools
	ToolsProfile string `json:"tools_profile"`
	ToolsAllow   string `json:"tools_allow"`
	ToolsDeny    string `json:"tools_deny"`

	// OpenClaw heartbeat
	HeartbeatEvery string `json:"heartbeat_every"`

	// OpenClaw per-agent model params
	ParamsTemperature string `json:"params_temperature"`
	ParamsMaxTokens   string `json:"params_max_tokens"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type OpenClawAgentMatch struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type CreateOpenClawAgentInput struct {
	Name          string `json:"name"`
	Icon          string `json:"icon"`
	IdentityEmoji string `json:"identity_emoji"`
}

type UpdateOpenClawAgentInput struct {
	Name *string `json:"name"`
	Icon *string `json:"icon"`

	DefaultLLMProviderID *string `json:"default_llm_provider_id"`
	DefaultLLMModelID    *string `json:"default_llm_model_id"`

	LLMTemperature          *float64 `json:"llm_temperature"`
	LLMTopP                 *float64 `json:"llm_top_p"`
	LLMMaxContextCount      *int     `json:"llm_max_context_count"`
	LLMMaxTokens            *int     `json:"llm_max_tokens"`
	EnableLLMTemperature    *bool    `json:"enable_llm_temperature"`
	EnableLLMTopP           *bool    `json:"enable_llm_top_p"`
	EnableLLMMaxTokens      *bool    `json:"enable_llm_max_tokens"`
	RetrievalMatchThreshold *float64 `json:"retrieval_match_threshold"`
	RetrievalTopK           *int     `json:"retrieval_top_k"`

	SandboxMode    *string `json:"sandbox_mode"`
	SandboxNetwork *bool   `json:"sandbox_network"`
	WorkDir        *string `json:"work_dir"`

	MCPEnabled          *bool   `json:"mcp_enabled"`
	MCPServerIDs        *string `json:"mcp_server_ids"`
	MCPServerEnabledIDs *string `json:"mcp_server_enabled_ids"`

	IdentityEmoji *string `json:"identity_emoji"`
	IdentityTheme *string `json:"identity_theme"`

	GroupChatMentionPatterns *string `json:"group_chat_mention_patterns"`

	ToolsProfile *string `json:"tools_profile"`
	ToolsAllow   *string `json:"tools_allow"`
	ToolsDeny    *string `json:"tools_deny"`

	HeartbeatEvery *string `json:"heartbeat_every"`

	ParamsTemperature *string `json:"params_temperature"`
	ParamsMaxTokens   *string `json:"params_max_tokens"`
}

type openClawAgentModel struct {
	bun.BaseModel `bun:"table:openclaw_agents,alias:oa"`

	ID        int64     `bun:"id,pk,autoincrement"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`

	Name            string `bun:"name,notnull"`
	OpenClawAgentID string `bun:"openclaw_agent_id,notnull"`
	Icon            string `bun:"icon,notnull"`

	DefaultLLMProviderID    string  `bun:"default_llm_provider_id,notnull"`
	DefaultLLMModelID       string  `bun:"default_llm_model_id,notnull"`
	LLMTemperature          float64 `bun:"llm_temperature,notnull"`
	LLMTopP                 float64 `bun:"llm_top_p,notnull"`
	LLMMaxContextCount      int     `bun:"llm_max_context_count,notnull"`
	LLMMaxTokens            int     `bun:"llm_max_tokens,notnull"`
	EnableLLMTemperature    bool    `bun:"enable_llm_temperature,notnull"`
	EnableLLMTopP           bool    `bun:"enable_llm_top_p,notnull"`
	EnableLLMMaxTokens      bool    `bun:"enable_llm_max_tokens,notnull"`
	RetrievalMatchThreshold float64 `bun:"retrieval_match_threshold,notnull"`
	RetrievalTopK           int     `bun:"retrieval_top_k,notnull"`

	SandboxMode    string `bun:"sandbox_mode,notnull"`
	SandboxNetwork bool   `bun:"sandbox_network,notnull"`
	WorkDir        string `bun:"work_dir,notnull"`

	MCPEnabled          bool   `bun:"mcp_enabled,notnull"`
	MCPServerIDs        string `bun:"mcp_server_ids,notnull"`
	MCPServerEnabledIDs string `bun:"mcp_server_enabled_ids,notnull"`

	IdentityEmoji string `bun:"identity_emoji,notnull"`
	IdentityTheme string `bun:"identity_theme,notnull"`

	GroupChatMentionPatterns string `bun:"group_chat_mention_patterns,notnull"`

	ToolsProfile string `bun:"tools_profile,notnull"`
	ToolsAllow   string `bun:"tools_allow,notnull"`
	ToolsDeny    string `bun:"tools_deny,notnull"`

	HeartbeatEvery string `bun:"heartbeat_every,notnull"`

	ParamsTemperature string `bun:"params_temperature,notnull"`
	ParamsMaxTokens   string `bun:"params_max_tokens,notnull"`
}

var _ bun.BeforeInsertHook = (*openClawAgentModel)(nil)

func (*openClawAgentModel) BeforeInsert(ctx context.Context, query *bun.InsertQuery) error {
	now := sqlite.NowUTC()
	query.Value("created_at", "?", now)
	query.Value("updated_at", "?", now)
	return nil
}

var _ bun.BeforeUpdateHook = (*openClawAgentModel)(nil)

func (*openClawAgentModel) BeforeUpdate(ctx context.Context, query *bun.UpdateQuery) error {
	query.Set("updated_at = ?", sqlite.NowUTC())
	return nil
}

func (m *openClawAgentModel) toDTO() OpenClawAgent {
	return OpenClawAgent{
		ID: m.ID,

		Name:            m.Name,
		OpenClawAgentID: m.OpenClawAgentID,
		Icon:            m.Icon,

		DefaultLLMProviderID:    m.DefaultLLMProviderID,
		DefaultLLMModelID:       m.DefaultLLMModelID,
		LLMTemperature:          m.LLMTemperature,
		LLMTopP:                 m.LLMTopP,
		LLMMaxContextCount:      m.LLMMaxContextCount,
		LLMMaxTokens:            m.LLMMaxTokens,
		EnableLLMTemperature:    m.EnableLLMTemperature,
		EnableLLMTopP:           m.EnableLLMTopP,
		EnableLLMMaxTokens:      m.EnableLLMMaxTokens,
		RetrievalMatchThreshold: m.RetrievalMatchThreshold,
		RetrievalTopK:           m.RetrievalTopK,

		SandboxMode:    m.SandboxMode,
		SandboxNetwork: m.SandboxNetwork,
		WorkDir:        m.WorkDir,

		MCPEnabled:          m.MCPEnabled,
		MCPServerIDs:        m.MCPServerIDs,
		MCPServerEnabledIDs: m.MCPServerEnabledIDs,

		IdentityEmoji: m.IdentityEmoji,
		IdentityTheme: m.IdentityTheme,

		GroupChatMentionPatterns: m.GroupChatMentionPatterns,

		ToolsProfile: m.ToolsProfile,
		ToolsAllow:   m.ToolsAllow,
		ToolsDeny:    m.ToolsDeny,

		HeartbeatEvery: m.HeartbeatEvery,

		ParamsTemperature: m.ParamsTemperature,
		ParamsMaxTokens:   m.ParamsMaxTokens,

		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
