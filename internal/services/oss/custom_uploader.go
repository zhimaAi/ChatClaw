package oss

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// uploadWithCustom uploads a local image using the endpoint and auth settings
// defined in the "custom" section of uploader.yaml.
func uploadWithCustom(ctx context.Context, filePath string, cfg customConfig) (string, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return "", fmt.Errorf("custom OSS endpoint is empty — set custom.endpoint in uploader.yaml")
	}

	fileField := cfg.FileField
	if fileField == "" {
		fileField = "file"
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fileField, filepath.Base(filePath))
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("copy file content: %w", err)
	}
	_ = writer.Close()

	uploadURL := strings.TrimSuffix(strings.TrimSpace(cfg.Endpoint), "/")
	if cfg.AuthMethod == "query" && strings.TrimSpace(cfg.AuthQueryParam) != "" {
		u, parseErr := url.Parse(uploadURL)
		if parseErr != nil {
			return "", fmt.Errorf("parse custom endpoint URL: %w", parseErr)
		}
		q := u.Query()
		q.Set(cfg.AuthQueryParam, cfg.AuthQueryValue)
		u.RawQuery = q.Encode()
		uploadURL = u.String()
	}

	uploadCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(uploadCtx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if cfg.AuthMethod == "header" && strings.TrimSpace(cfg.AuthHeaderName) != "" {
		req.Header.Set(cfg.AuthHeaderName, cfg.AuthHeaderValue)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("upload API returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("decode upload response as JSON: %w", err)
	}

	if strings.TrimSpace(cfg.ResponseSuccessCodeField) != "" {
		codeVal, ok := extractJSONPath(result, cfg.ResponseSuccessCodeField)
		if !ok {
			return "", fmt.Errorf("upload response missing success code field %q", cfg.ResponseSuccessCodeField)
		}
		if fmt.Sprintf("%v", codeVal) != cfg.ResponseSuccessCodeValue {
			return "", fmt.Errorf("upload API returned non-success code: %v (expected %s)", codeVal, cfg.ResponseSuccessCodeValue)
		}
	}

	urlPath := strings.TrimSpace(cfg.ResponseURLPath)
	if urlPath == "" {
		urlPath = "url"
	}
	urlVal, ok := extractJSONPath(result, urlPath)
	if !ok {
		return "", fmt.Errorf("upload response does not contain field %q — check custom.response_url_path in uploader.yaml", urlPath)
	}
	imageURL, ok := urlVal.(string)
	if !ok || imageURL == "" {
		return "", fmt.Errorf("upload API returned empty or non-string URL at path %q", urlPath)
	}

	return strings.Replace(imageURL, "http://", "https://", 1), nil
}

// extractJSONPath traverses a decoded JSON value using dot-notation.
// Supports nested objects and numeric array indices.
//
//	"data.url"      ->  obj["data"]["url"]
//	"result.0.src"  ->  obj["result"][0]["src"]
func extractJSONPath(obj any, path string) (any, bool) {
	if path == "" {
		return obj, true
	}
	idx := strings.IndexByte(path, '.')
	var key, rest string
	if idx < 0 {
		key = path
	} else {
		key, rest = path[:idx], path[idx+1:]
	}

	switch v := obj.(type) {
	case map[string]any:
		val, ok := v[key]
		if !ok {
			return nil, false
		}
		if rest == "" {
			return val, true
		}
		return extractJSONPath(val, rest)

	case []any:
		var i int
		if _, err := fmt.Sscanf(key, "%d", &i); err != nil {
			return nil, false
		}
		if i < 0 || i >= len(v) {
			return nil, false
		}
		if rest == "" {
			return v[i], true
		}
		return extractJSONPath(v[i], rest)
	}

	return nil, false
}
