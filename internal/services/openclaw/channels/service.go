package openclawchannels

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatclaw/internal/errs"
	"chatclaw/internal/services/channels"
	"chatclaw/internal/services/openclawagents"
	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// OpenClawChannelService provides Feishu-focused channel management for OpenClaw.
// It delegates to the shared channels infrastructure while filtering by OpenClaw agents.
type OpenClawChannelService struct {
	app        *application.App
	gateway    *channels.Gateway
	agentsSvc  *openclawagents.OpenClawAgentsService
	channelSvc *channels.ChannelService
}

func NewOpenClawChannelService(
	app *application.App,
	gw *channels.Gateway,
	agentsSvc *openclawagents.OpenClawAgentsService,
	channelSvc *channels.ChannelService,
) *OpenClawChannelService {
	return &OpenClawChannelService{
		app:        app,
		gateway:    gw,
		agentsSvc:  agentsSvc,
		channelSvc: channelSvc,
	}
}

func (s *OpenClawChannelService) db() (*bun.DB, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, errs.New("error.sqlite_not_initialized")
	}
	return db, nil
}

// openClawAgentIDs returns all OpenClaw agent IDs for filtering.
func (s *OpenClawChannelService) openClawAgentIDs() ([]int64, error) {
	agents, err := s.agentsSvc.ListAgents()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(agents))
	for _, a := range agents {
		ids = append(ids, a.ID)
	}
	return ids, nil
}

// ListChannels returns channels bound to OpenClaw agents (Feishu only).
func (s *OpenClawChannelService) ListChannels() ([]channels.Channel, error) {
	agentIDs, err := s.openClawAgentIDs()
	if err != nil {
		return nil, err
	}
	if len(agentIDs) == 0 {
		return []channels.Channel{}, nil
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var models []channelModel
	if err := db.NewSelect().
		Model(&models).
		Where("agent_id IN (?)", bun.In(agentIDs)).
		Where("platform = ?", channels.PlatformFeishu).
		OrderExpr("id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.channel_list_failed", err)
	}

	out := make([]channels.Channel, 0, len(models))
	for i := range models {
		ch := models[i].toDTO()
		if s.gateway.IsConnected(ch.ID) {
			ch.Status = channels.StatusOnline
		}
		out = append(out, ch)
	}
	return out, nil
}

// ListAllFeishuChannels returns all Feishu channels (including unbound ones)
// for the "add existing bot" workflow.
func (s *OpenClawChannelService) ListAllFeishuChannels() ([]channels.Channel, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var models []channelModel
	if err := db.NewSelect().
		Model(&models).
		Where("platform = ?", channels.PlatformFeishu).
		OrderExpr("id DESC").
		Scan(ctx); err != nil {
		return nil, errs.Wrap("error.channel_list_failed", err)
	}

	out := make([]channels.Channel, 0, len(models))
	for i := range models {
		ch := models[i].toDTO()
		if s.gateway.IsConnected(ch.ID) {
			ch.Status = channels.StatusOnline
		}
		out = append(out, ch)
	}
	return out, nil
}

// GetChannelStats returns stats for OpenClaw Feishu channels.
func (s *OpenClawChannelService) GetChannelStats() (*channels.ChannelStats, error) {
	chList, err := s.ListChannels()
	if err != nil {
		return nil, err
	}

	stats := &channels.ChannelStats{Total: len(chList)}
	for _, ch := range chList {
		if ch.Status == channels.StatusOnline {
			stats.Connected++
		} else {
			stats.Disconnected++
		}
	}
	return stats, nil
}

// GetSupportedPlatforms returns Feishu as the only supported platform for OpenClaw.
func (s *OpenClawChannelService) GetSupportedPlatforms() []channels.PlatformMeta {
	return []channels.PlatformMeta{
		{ID: channels.PlatformFeishu, Name: "Feishu", AuthType: "token"},
	}
}

// CreateChannel creates a new Feishu channel and binds it to the specified OpenClaw agent.
func (s *OpenClawChannelService) CreateChannel(input CreateChannelInput) (*channels.Channel, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errs.New("error.channel_name_required")
	}
	if input.AgentID <= 0 {
		return nil, errs.New("error.agent_id_required")
	}

	ch, err := s.channelSvc.CreateChannel(channels.CreateChannelInput{
		Platform:       channels.PlatformFeishu,
		Name:           name,
		Avatar:         input.Avatar,
		ConnectionType: channels.ConnTypeGateway,
		ExtraConfig:    input.ExtraConfig,
	})
	if err != nil {
		return nil, err
	}

	if err := s.channelSvc.BindAgent(ch.ID, input.AgentID); err != nil {
		return nil, err
	}

	ch.AgentID = input.AgentID
	return ch, nil
}

// UpdateChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) UpdateChannel(id int64, input channels.UpdateChannelInput) (*channels.Channel, error) {
	return s.channelSvc.UpdateChannel(id, input)
}

// DeleteChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) DeleteChannel(id int64) error {
	return s.channelSvc.DeleteChannel(id)
}

// BindAgent delegates to the shared ChannelService.
func (s *OpenClawChannelService) BindAgent(id int64, agentID int64) error {
	return s.channelSvc.BindAgent(id, agentID)
}

// UnbindAgent delegates to the shared ChannelService.
func (s *OpenClawChannelService) UnbindAgent(id int64) error {
	return s.channelSvc.UnbindAgent(id)
}

// ConnectChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) ConnectChannel(id int64) error {
	return s.channelSvc.ConnectChannel(id)
}

// DisconnectChannel delegates to the shared ChannelService.
func (s *OpenClawChannelService) DisconnectChannel(id int64) error {
	return s.channelSvc.DisconnectChannel(id)
}

// VerifyChannelConfig verifies Feishu credentials.
func (s *OpenClawChannelService) VerifyChannelConfig(extraConfig string) error {
	return s.channelSvc.VerifyChannelConfig(channels.PlatformFeishu, extraConfig)
}

// EnsureAgentForChannel auto-creates an OpenClaw agent and binds it to the channel.
func (s *OpenClawChannelService) EnsureAgentForChannel(channelID int64) (int64, error) {
	if channelID <= 0 {
		return 0, errs.New("error.channel_id_required")
	}

	db, err := s.db()
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var m channelModel
	if err := db.NewSelect().Model(&m).Where("id = ?", channelID).Limit(1).Scan(ctx); err != nil {
		return 0, errs.Wrap("error.channel_read_failed", err)
	}
	if m.AgentID != 0 {
		return m.AgentID, nil
	}

	agent, err := s.agentsSvc.CreateAgent(openclawagents.CreateOpenClawAgentInput{
		Name: fmt.Sprintf("%s Agent", m.Name),
	})
	if err != nil {
		return 0, errs.Wrap("error.channel_agent_create_failed", err)
	}

	if err := s.channelSvc.BindAgent(channelID, agent.ID); err != nil {
		return 0, err
	}

	return agent.ID, nil
}

// ListAgents returns all OpenClaw agents for the bind dialog.
func (s *OpenClawChannelService) ListAgents() ([]openclawagents.OpenClawAgent, error) {
	return s.agentsSvc.ListAgents()
}

// CreateChannelInput for OpenClaw channel creation.
type CreateChannelInput struct {
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	ExtraConfig string `json:"extra_config"`
	AgentID     int64  `json:"agent_id"`
}

// appCredentialsJSON is used to parse/build extra_config.
type appCredentialsJSON struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

func parseAppCredentials(extraConfig string) (appID string) {
	extraConfig = strings.TrimSpace(extraConfig)
	if extraConfig == "" {
		return ""
	}
	var cfg appCredentialsJSON
	if err := json.Unmarshal([]byte(extraConfig), &cfg); err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.AppID)
}
