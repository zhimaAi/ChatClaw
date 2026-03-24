package openclawruntime

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type deviceIdentity struct {
	DeviceID      string
	PublicKeyPEM  string
	PrivateKeyPEM string
}

type storedIdentity struct {
	Version       int    `json:"version"`
	DeviceID      string `json:"deviceId"`
	PublicKeyPEM  string `json:"publicKeyPem"`
	PrivateKeyPEM string `json:"privateKeyPem"`
	CreatedAtMs   int64  `json:"createdAtMs"`
}

type deviceTokenStore struct {
	Version int                          `json:"version"`
	Tokens  map[string]storedDeviceToken `json:"tokens"`
}

type storedDeviceToken struct {
	Token       string   `json:"token"`
	Role        string   `json:"role"`
	Scopes      []string `json:"scopes,omitempty"`
	CreatedAtMs int64    `json:"createdAtMs"`
}

func loadOrCreateDeviceIdentity(stateDir string) (*deviceIdentity, error) {
	path := filepath.Join(stateDir, "identity", "device.json")
	if raw, err := os.ReadFile(path); err == nil {
		var stored storedIdentity
		if json.Unmarshal(raw, &stored) == nil && stored.PublicKeyPEM != "" && stored.PrivateKeyPEM != "" {
			id := &deviceIdentity{PublicKeyPEM: stored.PublicKeyPEM, PrivateKeyPEM: stored.PrivateKeyPEM}
			if devID, err := deriveDeviceID(id.PublicKeyPEM); err == nil {
				id.DeviceID = devID
				if stored.DeviceID != devID {
					_ = writeJSONFile(path, storedIdentity{
						Version: 1, DeviceID: devID,
						PublicKeyPEM: id.PublicKeyPEM, PrivateKeyPEM: id.PrivateKeyPEM,
						CreatedAtMs: stored.CreatedAtMs,
					}, 0o600)
				}
				return id, nil
			}
		}
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate device identity: %w", err)
	}
	pubDER, _ := x509.MarshalPKIXPublicKey(pub)
	privDER, _ := x509.MarshalPKCS8PrivateKey(priv)
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}))
	devID := sha256Hex(pub)

	id := &deviceIdentity{DeviceID: devID, PublicKeyPEM: pubPEM, PrivateKeyPEM: privPEM}
	err = writeJSONFile(path, storedIdentity{
		Version: 1, DeviceID: devID,
		PublicKeyPEM: pubPEM, PrivateKeyPEM: privPEM,
		CreatedAtMs: time.Now().UnixMilli(),
	}, 0o600)
	if err != nil {
		return nil, err
	}
	return id, nil
}

func (d *deviceIdentity) PublicKeyBase64URL() (string, error) {
	key, err := parsePublicKey(d.PublicKeyPEM)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}

func (d *deviceIdentity) SignPayload(payload string) (string, error) {
	key, err := parsePrivateKey(d.PrivateKeyPEM)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(payload))), nil
}

func loadStoredDeviceToken(stateDir, role string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(stateDir, "identity", "device-auth.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read device token store: %w", err)
	}
	var stored deviceTokenStore
	if err := json.Unmarshal(raw, &stored); err != nil {
		return "", nil
	}
	if stored.Tokens == nil {
		return "", nil
	}
	return stored.Tokens[role].Token, nil
}

func storeDeviceToken(stateDir, role, token string, scopes []string) error {
	path := filepath.Join(stateDir, "identity", "device-auth.json")
	var stored deviceTokenStore
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &stored)
	}
	if stored.Tokens == nil {
		stored.Tokens = map[string]storedDeviceToken{}
	}
	stored.Version = 1
	stored.Tokens[role] = storedDeviceToken{
		Token: token, Role: role, Scopes: scopes, CreatedAtMs: time.Now().UnixMilli(),
	}
	return writeJSONFile(path, stored, 0o600)
}

func buildDeviceAuthPayloadV3(deviceID, clientID, clientMode, role string, scopes []string, signedAtMs int64, token, nonce, platform, deviceFamily string) string {
	norm := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	return fmt.Sprintf("v3|%s|%s|%s|%s|%s|%d|%s|%s|%s|%s",
		deviceID, clientID, clientMode, role,
		strings.Join(scopes, ","), signedAtMs, token, nonce,
		norm(platform), norm(deviceFamily),
	)
}

// --- crypto helpers ---

func deriveDeviceID(publicKeyPEM string) (string, error) {
	key, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return "", err
	}
	return sha256Hex(key), nil
}

func parsePublicKey(pemStr string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("decode public key pem")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("unexpected public key type")
	}
	return key, nil
}

func parsePrivateKey(pemStr string) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("decode private key pem")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unexpected private key type")
	}
	return key, nil
}

func writeJSONFile(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), mode); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
