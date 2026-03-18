package oss

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatclaw/internal/define"
	"chatclaw/internal/services/providers"
)

// UploadImage uploads a local image file to the configured OSS provider and returns the public HTTPS URL.
// The active provider is determined by the "mode" field in uploader.yaml (chatclaw | custom).
func UploadImage(ctx context.Context, filePath string) (string, error) {
	cfg := loadUploaderConfig()
	if cfg.Mode == "custom" {
		return uploadWithCustom(ctx, filePath, cfg.Custom)
	}
	return uploadWithChatClaw(ctx, filePath)
}

// uploadWithChatClaw uploads a local image file to the ChatClaw OSS endpoint and returns the public HTTPS URL.
func uploadWithChatClaw(ctx context.Context, filePath string) (string, error) {
	provider, err := providers.GetChatClawConfig()
	if err != nil {
		return "", fmt.Errorf("get chatclaw provider: %w", err)
	}

	apiKey := strings.TrimSpace(provider.APIKey)
	if apiKey == "" {
		return "", fmt.Errorf("chatclaw api_key not configured")
	}

	apiEndpoint := strings.TrimSuffix(strings.TrimSpace(provider.APIEndpoint), "/")
	if apiEndpoint == "" {
		apiEndpoint = strings.TrimSuffix(define.ServerURL, "/")
	}
	uploadURL := apiEndpoint + "/upload/image"

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("copy file content: %w", err)
	}
	_ = writer.Close()

	uploadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(uploadCtx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read upload response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(respBytes)
		if len(preview) > 256 {
			preview = preview[:256] + "..."
		}
		return "", fmt.Errorf("upload API returned HTTP %d: %s", resp.StatusCode, preview)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		preview := string(respBytes)
		if len(preview) > 256 {
			preview = preview[:256] + "..."
		}
		return "", fmt.Errorf("decode upload response (body: %s): %w", preview, err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("upload API error (code=%d): %s", result.Code, result.Message)
	}
	if result.Data.URL == "" {
		return "", fmt.Errorf("upload API returned empty URL")
	}
	// DingTalk Markdown renderer requires HTTPS; upgrade plain HTTP OSS URLs.
	ossURL := strings.Replace(result.Data.URL, "http://", "https://", 1)

	return ossURL, nil
}
