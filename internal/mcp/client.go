// Package mcp speaks the Model Context Protocol over HTTP to a remote tool
// server. Only the part this app needs is implemented: the handshake, the
// session the server hands back, listing tools and calling one.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/ppxb/miyabi/internal/domain"
)

const (
	// protocolVersion is the revision this client asks for. The server answers
	// with the revision it will actually speak.
	protocolVersion = "2025-06-18"
	// sessionHeader carries the session created during the handshake.
	sessionHeader = "Mcp-Session-Id"
	// maxResponseBytes bounds one response. A torrent listing repeats its tree,
	// so the ceiling has to be generous.
	maxResponseBytes = 8 << 20
	// maxMessage bounds a message that came from the remote server.
	maxMessage = 200
)

// errSessionExpired marks a call the server rejected because it forgot the
// session, which the caller answers by handshaking again.
var errSessionExpired = errors.New("mcp session expired")

// Tool is one entry of the server's tool list.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Client is safe for concurrent use. It keeps the session so a settings page,
// a search and a listing share one handshake.
type Client struct {
	endpoint string
	token    string
	http     *http.Client

	mu      sync.Mutex
	session string
}

// New validates the endpoint and returns a client. The caller owns the
// http.Client so it can carry the app's proxy and timeout settings.
func New(endpoint, token string, client *http.Client) (*Client, error) {
	address, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return nil, domain.E(domain.KindInvalid, "磁力服务地址无效", err)
	}
	if (address.Scheme != "http" && address.Scheme != "https") || address.Host == "" || address.User != nil {
		return nil, domain.E(domain.KindInvalid, "磁力服务地址必须是 http 或 https 地址", nil)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{endpoint: address.String(), token: strings.TrimSpace(token), http: client}, nil
}

// Tools lists what the server offers.
func (client *Client) Tools(ctx context.Context) ([]Tool, error) {
	payload, err := client.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("decode magnet tool list: %w", err)
	}
	return result.Tools, nil
}

// Call runs one tool and returns the JSON its first text block carries. A tool
// that answers with plain text yields that text as a JSON string.
func (client *Client) Call(ctx context.Context, name string, arguments any) (json.RawMessage, error) {
	if strings.TrimSpace(name) == "" {
		return nil, domain.E(domain.KindInvalid, "缺少工具名称", nil)
	}
	payload, err := client.call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return nil, err
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("decode %s result: %w", name, err)
	}
	if len(result.Content) == 0 {
		return nil, domain.E(domain.KindUpstream, "磁力服务没有返回内容", nil)
	}
	text := strings.TrimSpace(result.Content[0].Text)
	if result.IsError {
		return nil, domain.E(domain.KindUpstream, clip("磁力服务拒绝了该请求", text), nil)
	}
	if json.Valid([]byte(text)) {
		return json.RawMessage(text), nil
	}
	encoded, err := json.Marshal(text)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// call handshakes when needed and retries once if the server dropped the
// session between two calls.
func (client *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := client.ensure(ctx); err != nil {
		return nil, err
	}
	payload, _, err := client.rpc(ctx, client.currentSession(), method, params)
	if err == nil {
		return payload, nil
	}
	if !errors.Is(err, errSessionExpired) {
		return nil, err
	}
	client.mu.Lock()
	client.session = ""
	client.mu.Unlock()
	if err := client.ensure(ctx); err != nil {
		return nil, err
	}
	payload, _, err = client.rpc(ctx, client.currentSession(), method, params)
	return payload, err
}

func (client *Client) ensure(ctx context.Context) error {
	client.mu.Lock()
	established := client.session != ""
	client.mu.Unlock()
	if established {
		return nil
	}
	payload, session, err := client.rpc(ctx, "", "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "miyabi", "version": "1.0.0"},
	})
	if err != nil {
		return err
	}
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return fmt.Errorf("decode magnet service handshake: %w", err)
	}
	client.mu.Lock()
	client.session = session
	client.mu.Unlock()
	// A notification has no reply, and a server that dislikes it still answers
	// the calls that follow, so its result is not worth failing the handshake.
	_, _, _ = client.rpc(ctx, session, "notifications/initialized", nil)
	return nil
}

func (client *Client) currentSession() string {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.session
}

// rpc performs one round trip and reports the session the server named, if any.
func (client *Client) rpc(ctx context.Context, session, method string, params any) (json.RawMessage, string, error) {
	request := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		request["params"] = params
	}
	notification := strings.HasPrefix(method, "notifications/")
	if !notification {
		request["id"] = 1
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, "", err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", domain.E(domain.KindInvalid, "磁力服务地址无效", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	// This server streams only when the caller offers to accept a stream, and
	// holds the connection open when it does. Plain JSON keeps one call one reply.
	httpRequest.Header.Set("Accept", "application/json")
	if client.token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+client.token)
	}
	if session != "" {
		httpRequest.Header.Set(sessionHeader, session)
	}
	response, err := client.http.Do(httpRequest)
	if err != nil {
		return nil, "", domain.E(domain.KindUpstream, "无法连接磁力服务", err)
	}
	defer response.Body.Close()
	named := strings.TrimSpace(response.Header.Get(sessionHeader))
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return nil, named, domain.E(domain.KindInvalid, "磁力服务的 token 无效或已过期", nil)
	case response.StatusCode == http.StatusNotFound && session != "":
		return nil, named, errSessionExpired
	case response.StatusCode < 200 || response.StatusCode > 299:
		return nil, named, domain.E(domain.KindUpstream,
			fmt.Sprintf("磁力服务返回 HTTP %d", response.StatusCode), nil)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, named, fmt.Errorf("read magnet service response: %w", err)
	}
	if int64(len(payload)) > maxResponseBytes {
		return nil, named, domain.E(domain.KindUpstream, "磁力服务返回内容过大", nil)
	}
	// A notification or an accepted-only reply carries no envelope.
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, named, nil
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(unwrapStream(payload), &envelope); err != nil {
		return nil, named, fmt.Errorf("decode magnet service reply: %w", err)
	}
	if envelope.Error != nil {
		return nil, named, domain.E(domain.KindUpstream,
			clip("磁力服务返回错误", envelope.Error.Message), nil)
	}
	return envelope.Result, named, nil
}

// unwrapStream returns the JSON a body carries, unwrapping a server-sent event
// envelope when the server answered with a stream instead of a document.
func unwrapStream(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if !bytes.HasPrefix(trimmed, []byte("event:")) && !bytes.HasPrefix(trimmed, []byte("data:")) {
		return trimmed
	}
	var payload []byte
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		if data, found := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:")); found {
			payload = bytes.TrimSpace(data)
		}
	}
	if len(payload) == 0 {
		return trimmed
	}
	return payload
}

// clip keeps a message from the remote server short enough to show.
func clip(prefix, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return prefix
	}
	if len(message) > maxMessage {
		message = message[:maxMessage] + "…"
	}
	return prefix + "：" + message
}
