package deeplink

import (
	"net/url"
	"strings"

	"chatclaw/internal/define"
	"chatclaw/internal/services/chatwiki"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AuthCallbackData is emitted to the frontend when a chatclaw://auth/callback URL is received.
type AuthCallbackData struct {
	Token    string `json:"token"`
	TTL      string `json:"ttl"`
	Exp      string `json:"exp"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
}

// HandleURL processes a single chatclaw:// URL (e.g. from macOS Apple Event or
// Windows/Linux command-line argument). If the URL matches the auth callback
// pattern, it saves the binding to DB and emits "chatwiki:auth-callback" to the frontend.
func HandleURL(app *application.App, rawURL string) {
	if !strings.HasPrefix(rawURL, "chatclaw://") {
		return
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	if parsed.Host != "auth" || !strings.HasPrefix(parsed.Path, "/callback") {
		return
	}
	q := parsed.Query()
	token := q.Get("token")
	if token == "" {
		return
	}
	serverURL := q.Get("server_url")
	if serverURL == "" {
		serverURL = define.GetChatWikiCloudURL()
	}
	ttl := q.Get("ttl")
	exp := q.Get("exp")
	userID := q.Get("user_id")
	userName := q.Get("user_name")

	if err := chatwiki.SaveBinding(app, serverURL, token, ttl, exp, userID, userName); err != nil {
		app.Logger.Error("Failed to save chatwiki binding from deeplink", "error", err)
	}

	payload := AuthCallbackData{
		Token:    token,
		TTL:      ttl,
		Exp:      exp,
		UserID:   userID,
		UserName: userName,
	}
	app.Logger.Info("Deep link auth callback received", "user_id", userID, "user_name", userName)
	app.Event.Emit("chatwiki:auth-callback", payload)
}

// HandleSecondInstance inspects the args from a second-instance launch.
// On Windows/Linux, the URL Scheme is passed as a command-line argument.
// On macOS, this typically won't contain the URL (handled via Apple Event instead).
func HandleSecondInstance(app *application.App, data application.SecondInstanceData) {
	for _, arg := range data.Args {
		HandleURL(app, arg)
	}
}
