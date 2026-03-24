package openclawruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"chatclaw/internal/define"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	protocolVersion = 3
	clientID        = "gateway-client"
	clientMode      = "backend"
	clientRole      = "operator"
	deviceFamily    = "desktop"

	pingInterval = 20 * time.Second
	pongWait     = 40 * time.Second
)

type gatewayClientOptions struct {
	URL             string
	Token           string
	DeviceIdentity  *deviceIdentity
	StoredDeviceTok string
	Scopes          []string
	OnEvent         func(GatewayEventFrame)
	OnDisconnect    func(error)
}

type GatewayClient struct {
	opts     gatewayClientOptions
	conn     *websocket.Conn
	sendMu   sync.Mutex
	mu       sync.RWMutex
	pending  map[string]chan gatewayResponseFrame
	closed   bool
	stopPing chan struct{}
}

type GatewayRequestError struct {
	Code    string
	Message string
}

func (e *GatewayRequestError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type GatewayHelloOK struct {
	Auth *struct {
		DeviceToken string   `json:"deviceToken"`
		Role        string   `json:"role"`
		Scopes      []string `json:"scopes"`
	} `json:"auth,omitempty"`
}

type GatewayEventFrame struct {
	Type    string          `json:"type"`
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload"`
}

type gatewayRequestFrame struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type gatewayResponseFrame struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type gatewayEnvelope struct {
	Type string `json:"type"`
}

func NewGatewayClient(opts gatewayClientOptions) *GatewayClient {
	return &GatewayClient{
		opts:    opts,
		pending: map[string]chan gatewayResponseFrame{},
	}
}

func (c *GatewayClient) Connect(ctx context.Context) (*GatewayHelloOK, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}
	conn, _, err := dialer.DialContext(ctx, c.opts.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("dial gateway websocket: %w", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	nonce, err := c.readChallenge(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	requestID := uuid.NewString()
	connectParams, err := c.buildConnectParams(nonce)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.WriteJSON(gatewayRequestFrame{
		Type: "req", ID: requestID, Method: "connect", Params: connectParams,
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send connect request: %w", err)
	}

	hello, err := c.readConnectResponse(conn, requestID)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	c.mu.Lock()
	c.conn = conn
	c.closed = false
	c.pending = map[string]chan gatewayResponseFrame{}
	c.stopPing = make(chan struct{})
	c.mu.Unlock()

	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	go c.readLoop()
	go c.pingLoop(conn, c.stopPing)
	return hello, nil
}

func (c *GatewayClient) readChallenge(conn *websocket.Conn) (string, error) {
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("read connect challenge: %w", err)
		}
		var env gatewayEnvelope
		if json.Unmarshal(payload, &env) != nil || env.Type != "event" {
			continue
		}
		var event GatewayEventFrame
		if json.Unmarshal(payload, &event) != nil || event.Event != "connect.challenge" {
			continue
		}
		var ch struct{ Nonce string }
		if err := json.Unmarshal(event.Payload, &ch); err != nil {
			return "", fmt.Errorf("decode connect challenge: %w", err)
		}
		if ch.Nonce == "" {
			return "", fmt.Errorf("gateway connect challenge missing nonce")
		}
		return ch.Nonce, nil
	}
}

func (c *GatewayClient) readConnectResponse(conn *websocket.Conn, requestID string) (*GatewayHelloOK, error) {
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("read connect response: %w", err)
		}
		var env gatewayEnvelope
		if json.Unmarshal(payload, &env) != nil {
			continue
		}
		if env.Type == "event" {
			var event GatewayEventFrame
			if json.Unmarshal(payload, &event) == nil && c.opts.OnEvent != nil {
				c.opts.OnEvent(event)
			}
			continue
		}
		if env.Type != "res" {
			continue
		}
		var resp gatewayResponseFrame
		if json.Unmarshal(payload, &resp) != nil || resp.ID != requestID {
			continue
		}
		if !resp.OK {
			return nil, &GatewayRequestError{Code: resp.Error.Code, Message: resp.Error.Message}
		}
		var hello GatewayHelloOK
		if err := json.Unmarshal(resp.Payload, &hello); err != nil {
			return nil, fmt.Errorf("decode hello-ok: %w", err)
		}
		return &hello, nil
	}
}

func (c *GatewayClient) buildConnectParams(nonce string) (*gatewayConnectParams, error) {
	params := &gatewayConnectParams{
		MinProtocol: protocolVersion,
		MaxProtocol: protocolVersion,
		Role:        clientRole,
		Scopes:      append([]string(nil), c.opts.Scopes...),
		Caps:        []string{"tool-events", "thinking-events"},
	}
	params.Client.ID = clientID
	params.Client.DisplayName = define.AppDisplayName
	params.Client.Version = define.Version
	params.Client.Platform = runtime.GOOS
	params.Client.DeviceFamily = deviceFamily
	params.Client.Mode = clientMode

	if c.opts.Token != "" || c.opts.StoredDeviceTok != "" {
		params.Auth = &connectAuth{Token: c.opts.Token, DeviceToken: c.opts.StoredDeviceTok}
	}

	if c.opts.DeviceIdentity != nil {
		signedAt := time.Now().UnixMilli()
		publicKey, err := c.opts.DeviceIdentity.PublicKeyBase64URL()
		if err != nil {
			return nil, err
		}
		payload := buildDeviceAuthPayloadV3(
			c.opts.DeviceIdentity.DeviceID,
			clientID, clientMode, clientRole,
			c.opts.Scopes, signedAt,
			c.opts.Token, nonce,
			runtime.GOOS, deviceFamily,
		)
		signature, err := c.opts.DeviceIdentity.SignPayload(payload)
		if err != nil {
			return nil, err
		}
		params.Device = &connectDevice{
			ID: c.opts.DeviceIdentity.DeviceID, PublicKey: publicKey,
			Signature: signature, SignedAt: signedAt, Nonce: nonce,
		}
	}

	return params, nil
}

func (c *GatewayClient) Request(ctx context.Context, method string, params any, out any) error {
	c.mu.RLock()
	conn, closed := c.conn, c.closed
	c.mu.RUnlock()
	if conn == nil || closed {
		return errors.New("gateway websocket is not connected")
	}

	id := uuid.NewString()
	ch := make(chan gatewayResponseFrame, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.writeFrame(gatewayRequestFrame{Type: "req", ID: id, Method: method, Params: params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	case resp := <-ch:
		if !resp.OK {
			return &GatewayRequestError{Code: resp.Error.Code, Message: resp.Error.Message}
		}
		if out == nil || len(resp.Payload) == 0 {
			return nil
		}
		return json.Unmarshal(resp.Payload, out)
	}
}

func (c *GatewayClient) Close() error {
	conn := c.teardown()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

// teardown closes internal state, drains pending requests, returns the raw conn for the caller to close.
func (c *GatewayClient) teardown() *websocket.Conn {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	conn := c.conn
	c.conn = nil
	pending := c.pending
	c.pending = map[string]chan gatewayResponseFrame{}
	stop := c.stopPing
	c.stopPing = nil
	c.mu.Unlock()

	if stop != nil {
		close(stop)
	}
	for _, ch := range pending {
		ch <- gatewayResponseFrame{OK: false, Error: &struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{Code: "closed", Message: "gateway connection closed"}}
	}
	return conn
}

func (c *GatewayClient) writeFrame(frame gatewayRequestFrame) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return errors.New("gateway websocket is not connected")
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := conn.WriteJSON(frame)
	_ = conn.SetWriteDeadline(time.Time{})
	if err != nil {
		return fmt.Errorf("write gateway request: %w", err)
	}
	return nil
}

func (c *GatewayClient) pingLoop(conn *websocket.Conn, stop chan struct{}) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.sendMu.Lock()
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			c.sendMu.Unlock()
			if err != nil {
				return
			}
		case <-stop:
			return
		}
	}
}

func (c *GatewayClient) readLoop() {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return
	}
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			c.handleDisconnect(err)
			return
		}
		var env gatewayEnvelope
		if json.Unmarshal(payload, &env) != nil {
			continue
		}
		switch env.Type {
		case "res":
			var resp gatewayResponseFrame
			if json.Unmarshal(payload, &resp) != nil {
				continue
			}
			c.mu.Lock()
			ch := c.pending[resp.ID]
			delete(c.pending, resp.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- resp
			}
		case "event":
			var event GatewayEventFrame
			if json.Unmarshal(payload, &event) == nil && c.opts.OnEvent != nil {
				c.opts.OnEvent(event)
			}
		}
	}
}

func (c *GatewayClient) handleDisconnect(err error) {
	conn := c.teardown()
	if conn != nil {
		_ = conn.Close()
	}
	if c.opts.OnDisconnect != nil {
		c.opts.OnDisconnect(err)
	}
}

// --- Connect param types (private, only used for JSON serialization) ---

type connectAuth struct {
	Token       string `json:"token,omitempty"`
	DeviceToken string `json:"deviceToken,omitempty"`
}

type connectDevice struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
	SignedAt  int64  `json:"signedAt"`
	Nonce     string `json:"nonce"`
}

type gatewayConnectParams struct {
	MinProtocol int    `json:"minProtocol"`
	MaxProtocol int    `json:"maxProtocol"`
	Client      struct {
		ID           string `json:"id"`
		DisplayName  string `json:"displayName,omitempty"`
		Version      string `json:"version"`
		Platform     string `json:"platform"`
		DeviceFamily string `json:"deviceFamily,omitempty"`
		Mode         string `json:"mode"`
	} `json:"client"`
	Role   string         `json:"role,omitempty"`
	Scopes []string       `json:"scopes,omitempty"`
	Caps   []string       `json:"caps"`
	Auth   *connectAuth   `json:"auth,omitempty"`
	Device *connectDevice `json:"device,omitempty"`
}
