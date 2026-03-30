package openclawchannels

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chatclaw/internal/errs"
)

// WeCom Work "qc" APIs (server-side to avoid browser CORS).
const (
	wecomAuthQRSource      = "chatclaw"
	wecomAuthQRGenerateURL = "https://work.weixin.qq.com/ai/qc/generate"
	wecomAuthQRQueryURL    = "https://work.weixin.qq.com/ai/qc/query_result"
)

var wecomAuthQRHTTPClient = &http.Client{Timeout: 20 * time.Second}

// WecomAuthQRGenerateResult is the inner payload exposed to the frontend.
type WecomAuthQRGenerateResult struct {
	Scode   string `json:"scode"`
	AuthURL string `json:"auth_url"`
}

// WecomAuthQRQueryResult mirrors query_result data for the frontend.
type WecomAuthQRQueryResult struct {
	Status  string         `json:"status"`
	BotInfo map[string]any `json:"bot_info,omitempty"`
}

type wecomAuthQRGenerateEnvelope struct {
	Data *struct {
		Scode   string `json:"scode"`
		AuthURL string `json:"auth_url"`
	} `json:"data"`
}

type wecomAuthQRQueryEnvelope struct {
	Data *struct {
		Status  string         `json:"status"`
		BotInfo map[string]any `json:"bot_info"`
	} `json:"data"`
}

// WecomAuthQRGenerate calls WeCom generate endpoint (GET, source=chatclaw).
func (*OpenClawChannelService) WecomAuthQRGenerate() (*WecomAuthQRGenerateResult, error) {
	u, err := url.Parse(wecomAuthQRGenerateURL)
	if err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_request_failed", err)
	}
	q := u.Query()
	q.Set("source", wecomAuthQRSource)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_request_failed", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ChatClaw-OpenClaw/1.0")

	resp, err := wecomAuthQRHTTPClient.Do(req)
	if err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_request_failed", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_request_failed", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.Newf("error.wecom_auth_qr_http_status", map[string]any{"Code": resp.StatusCode})
	}

	var env wecomAuthQRGenerateEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_invalid_response", err)
	}
	if env.Data == nil || strings.TrimSpace(env.Data.Scode) == "" || strings.TrimSpace(env.Data.AuthURL) == "" {
		return nil, errs.New("error.wecom_auth_qr_invalid_response")
	}
	return &WecomAuthQRGenerateResult{
		Scode:   strings.TrimSpace(env.Data.Scode),
		AuthURL: strings.TrimSpace(env.Data.AuthURL),
	}, nil
}

// WecomAuthQRQuery polls WeCom query_result (GET, source + scode).
func (*OpenClawChannelService) WecomAuthQRQuery(scode string) (*WecomAuthQRQueryResult, error) {
	scode = strings.TrimSpace(scode)
	if scode == "" {
		return nil, errs.New("error.wecom_auth_qr_scode_required")
	}

	u, err := url.Parse(wecomAuthQRQueryURL)
	if err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_request_failed", err)
	}
	q := u.Query()
	q.Set("source", wecomAuthQRSource)
	q.Set("scode", scode)
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_request_failed", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ChatClaw-OpenClaw/1.0")

	resp, err := wecomAuthQRHTTPClient.Do(req)
	if err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_request_failed", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_request_failed", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.Newf("error.wecom_auth_qr_http_status", map[string]any{"Code": resp.StatusCode})
	}

	var env wecomAuthQRQueryEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, errs.Wrap("error.wecom_auth_qr_invalid_response", err)
	}
	if env.Data == nil {
		return nil, errs.New("error.wecom_auth_qr_invalid_response")
	}
	out := &WecomAuthQRQueryResult{
		Status: strings.TrimSpace(env.Data.Status),
	}
	if len(env.Data.BotInfo) > 0 {
		out.BotInfo = env.Data.BotInfo
	}
	return out, nil
}
