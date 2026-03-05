package chatwiki

import (
	"context"
	"strconv"
	"time"

	"chatclaw/internal/sqlite"

	"github.com/uptrace/bun"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Binding represents a ChatWiki binding record exposed to the frontend.
type Binding struct {
	ID        int64  `json:"id"`
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	TTL       int64  `json:"ttl"`
	Exp       int64  `json:"exp"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// bindingModel is the bun ORM model for the chatwiki_bindings table.
type bindingModel struct {
	bun.BaseModel `bun:"table:chatwiki_bindings"`

	ID        int64     `bun:"id,pk,autoincrement"`
	ServerURL string    `bun:"server_url,notnull"`
	Token     string    `bun:"token,notnull"`
	TTL       int64     `bun:"ttl,notnull"`
	Exp       int64     `bun:"exp,notnull"`
	UserID    string    `bun:"user_id,notnull"`
	UserName  string    `bun:"user_name,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

// ChatWikiService exposes ChatWiki binding operations to the frontend via Wails.
type ChatWikiService struct {
	app *application.App
}

func NewChatWikiService(app *application.App) *ChatWikiService {
	return &ChatWikiService{app: app}
}

// GetBinding returns the current binding, or nil if none exists.
func (s *ChatWikiService) GetBinding() (*Binding, error) {
	db := sqlite.DB()
	if db == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := new(bindingModel)
	err := db.NewSelect().Model(m).OrderExpr("id DESC").Limit(1).Scan(ctx)
	if err != nil {
		return nil, nil
	}
	return toBinding(m), nil
}

// SaveBinding creates or replaces the binding. Called from deeplink handler.
func SaveBinding(app *application.App, serverURL, token, ttl, exp, userID, userName string) error {
	db := sqlite.DB()
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ttlInt, _ := strconv.ParseInt(ttl, 10, 64)
	expInt, _ := strconv.ParseInt(exp, 10, 64)
	now := time.Now().UTC()

	// Delete old bindings, keep only latest
	if _, err := db.NewDelete().Model((*bindingModel)(nil)).Where("1=1").Exec(ctx); err != nil {
		app.Logger.Warn("Failed to delete old chatwiki bindings", "error", err)
	}

	m := &bindingModel{
		ServerURL: serverURL,
		Token:     token,
		TTL:       ttlInt,
		Exp:       expInt,
		UserID:    userID,
		UserName:  userName,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		app.Logger.Error("Failed to save chatwiki binding", "error", err)
		return err
	}
	app.Logger.Info("ChatWiki binding saved", "user_id", userID, "user_name", userName)
	return nil
}

// DeleteBinding removes the current binding.
func (s *ChatWikiService) DeleteBinding() error {
	db := sqlite.DB()
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := db.NewDelete().Model((*bindingModel)(nil)).Where("1=1").Exec(ctx)
	return err
}

func toBinding(m *bindingModel) *Binding {
	return &Binding{
		ID:        m.ID,
		ServerURL: m.ServerURL,
		Token:     m.Token,
		TTL:       m.TTL,
		Exp:       m.Exp,
		UserID:    m.UserID,
		UserName:  m.UserName,
		CreatedAt: m.CreatedAt.Format(sqlite.DateTimeFormat),
		UpdatedAt: m.UpdatedAt.Format(sqlite.DateTimeFormat),
	}
}
