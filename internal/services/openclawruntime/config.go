package openclawruntime

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"

	"chatclaw/internal/services/settings"
)

const (
	keyGatewayPort  = "openclaw_gateway_port"
	keyGatewayToken = "openclaw_gateway_token"
)

type OpenClawConfig struct {
	GatewayPort  int
	GatewayToken string
}

type configStore struct {
	mu  sync.RWMutex
	cfg OpenClawConfig
	svc *settings.SettingsService
}

func newConfigStore(svc *settings.SettingsService) *configStore {
	return &configStore{
		svc: svc,
		cfg: OpenClawConfig{
			GatewayPort:  settings.GetInt(keyGatewayPort, 39960),
			GatewayToken: mustGetOrInit(svc, keyGatewayToken, generateToken),
		},
	}
}

func (s *configStore) Get() OpenClawConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func mustGetOrInit(svc *settings.SettingsService, key string, gen func() string) string {
	if v, ok := settings.GetValue(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	value := gen()
	if svc != nil {
		_, _ = svc.SetValue(key, value)
	}
	return value
}

func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "chatclaw-fallback-token"
	}
	return fmt.Sprintf("%x", b)
}
