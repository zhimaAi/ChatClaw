package chatwiki

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestChatCompletions_RealEndpoint(t *testing.T) {
	apiEndpoint := strings.TrimRight(strings.TrimSpace(os.Getenv("CHATWIKI_TEST_API_ENDPOINT")), "/")
	apiKey := strings.TrimSpace(os.Getenv("CHATWIKI_TEST_API_KEY"))
	useModel := strings.TrimSpace(os.Getenv("CHATWIKI_TEST_USE_MODEL"))
	if useModel == "" {
		useModel = "qwen-plus"
	}

	if apiEndpoint == "" || apiKey == "" {
		t.Skip("set CHATWIKI_TEST_API_ENDPOINT and CHATWIKI_TEST_API_KEY to run this real integration test")
	}

	payload := map[string]any{
		"max_tokens":  512,
		"messages":    []map[string]string{{"content": "hello", "role": "user"}},
		"stream":      true,
		"temperature": 0.7,
		"use_model":   useModel,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	url := apiEndpoint + "/chat/completions"
	t.Logf("POST %s", url)
	t.Logf("request body: %s", string(body))

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("status: %s", resp.Status)
	t.Logf("content-type: %s", resp.Header.Get("Content-Type"))

	reader := bufio.NewReader(resp.Body)
	var lines []string
	for i := 0; i < 30; i++ {
		line, readErr := reader.ReadString('\n')
		if line != "" {
			lines = append(lines, strings.TrimRight(line, "\r\n"))
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			t.Fatalf("read response: %v", readErr)
		}
		if strings.Contains(line, "[DONE]") {
			break
		}
	}

	if len(lines) == 0 {
		rest, _ := io.ReadAll(reader)
		if len(rest) > 0 {
			t.Logf("response body: %s", string(rest))
		}
	} else {
		t.Logf("response lines:\n%s", strings.Join(lines, "\n"))
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
}
