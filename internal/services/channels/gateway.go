package channels

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"chatclaw/internal/sqlite"
)

// Gateway manages all active platform adapter connections.
type Gateway struct {
	mu       sync.RWMutex
	adapters map[int64]PlatformAdapter // channelID -> adapter
	logger   *slog.Logger
	handler  MessageHandler
}

// NewGateway creates a new Gateway instance.
func NewGateway(logger *slog.Logger, handler MessageHandler) *Gateway {
	if handler == nil {
		handler = func(msg IncomingMessage) {}
	}
	return &Gateway{
		adapters: make(map[int64]PlatformAdapter),
		logger:   logger,
		handler:  handler,
	}
}

// ConnectChannel creates an adapter for the channel and connects it.
func (g *Gateway) ConnectChannel(ctx context.Context, ch Channel) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if existing, ok := g.adapters[ch.ID]; ok && existing.IsConnected() {
		return nil
	}

	adapter := NewAdapter(ch.Platform)
	if adapter == nil {
		return fmt.Errorf("no adapter registered for platform %q", ch.Platform)
	}

	if err := adapter.Connect(ctx, ch.ID, ch.ExtraConfig, g.handler); err != nil {
		g.updateChannelStatus(ch.ID, StatusError)
		return fmt.Errorf("connect channel %d (%s): %w", ch.ID, ch.Platform, err)
	}

	g.adapters[ch.ID] = adapter
	g.updateChannelStatus(ch.ID, StatusOnline)
	g.logger.Info("channel connected", "channel_id", ch.ID, "platform", ch.Platform)
	return nil
}

// DisconnectChannel disconnects and removes the adapter for a channel.
func (g *Gateway) DisconnectChannel(ctx context.Context, channelID int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	adapter, ok := g.adapters[channelID]
	if !ok {
		return nil
	}

	if err := adapter.Disconnect(ctx); err != nil {
		g.logger.Warn("disconnect error", "channel_id", channelID, "error", err)
	}

	delete(g.adapters, channelID)
	g.updateChannelStatus(channelID, StatusOffline)
	g.logger.Info("channel disconnected", "channel_id", channelID)
	return nil
}

// IsConnected checks if a specific channel is connected.
func (g *Gateway) IsConnected(channelID int64) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	adapter, ok := g.adapters[channelID]
	if !ok {
		return false
	}
	return adapter.IsConnected()
}

// GetAdapter returns the adapter for a channel (for sending messages, etc.).
func (g *Gateway) GetAdapter(channelID int64) PlatformAdapter {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.adapters[channelID]
}

// StartAll loads enabled channels from DB and connects them.
func (g *Gateway) StartAll(ctx context.Context) {
	db := sqlite.DB()
	if db == nil {
		return
	}

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var models []channelModel
	if err := db.NewSelect().
		Model(&models).
		Where("enabled = ?", true).
		Scan(queryCtx); err != nil {
		g.logger.Error("load enabled channels failed", "error", err)
		return
	}

	for _, m := range models {
		ch := m.toDTO()
		if err := g.ConnectChannel(ctx, ch); err != nil {
			g.logger.Warn("auto-connect channel failed", "channel_id", ch.ID, "error", err)
		}
	}
}

// StopAll disconnects all active channels.
func (g *Gateway) StopAll(ctx context.Context) {
	g.mu.Lock()
	ids := make([]int64, 0, len(g.adapters))
	for id := range g.adapters {
		ids = append(ids, id)
	}
	g.mu.Unlock()

	for _, id := range ids {
		_ = g.DisconnectChannel(ctx, id)
	}
}

// RefreshStatuses checks each connected adapter and updates DB status.
func (g *Gateway) RefreshStatuses(ctx context.Context) {
	g.mu.RLock()
	snapshot := make(map[int64]PlatformAdapter, len(g.adapters))
	for id, a := range g.adapters {
		snapshot[id] = a
	}
	g.mu.RUnlock()

	for id, adapter := range snapshot {
		if adapter.IsConnected() {
			g.updateChannelStatus(id, StatusOnline)
		} else {
			g.updateChannelStatus(id, StatusOffline)
			g.mu.Lock()
			delete(g.adapters, id)
			g.mu.Unlock()
		}
	}
}

// updateChannelStatus updates the status column in the database.
func (g *Gateway) updateChannelStatus(channelID int64, status string) {
	db := sqlite.DB()
	if db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	q := db.NewUpdate().
		Model((*channelModel)(nil)).
		Where("id = ?", channelID).
		Set("status = ?", status).
		Set("updated_at = ?", sqlite.NowUTC())

	if status == StatusOnline {
		q = q.Set("last_connected_at = ?", sqlite.NowUTC())
	}

	if _, err := q.Exec(ctx); err != nil {
		g.logger.Warn("update channel status failed", "channel_id", channelID, "error", err)
	}
}
