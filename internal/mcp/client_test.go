package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// wire records what the client actually sent, so the tests assert the protocol
// contract instead of the client's internals.
type wire struct {
	mu       sync.Mutex
	methods  []string
	sessions []string
	accepts  []string
	auths    []string
}

func (log *wire) record(request *http.Request, method string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.methods = append(log.methods, method)
	log.sessions = append(log.sessions, request.Header.Get(sessionHeader))
	log.accepts = append(log.accepts, request.Header.Get("Accept"))
	log.auths = append(log.auths, request.Header.Get("Authorization"))
}

func (log *wire) count(method string) int {
	log.mu.Lock()
	defer log.mu.Unlock()
	total := 0
	for _, name := range log.methods {
		if name == method {
			total++
		}
	}
	return total
}

// call decodes the JSON-RPC method of one request.
func readMethod(t *testing.T, request *http.Request) string {
	t.Helper()
	var envelope struct {
		Method string `json:"method"`
	}
	if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return envelope.Method
}

func writeResult(t *testing.T, response http.ResponseWriter, result any) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result}); err != nil {
		t.Fatalf("write reply: %v", err)
	}
}

func toolContent(text string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
}

func TestHandshakeIsReusedAcrossCalls(t *testing.T) {
	log := &wire{}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		method := readMethod(t, request)
		log.record(request, method)
		switch method {
		case "initialize":
			response.Header().Set(sessionHeader, "session-1")
			writeResult(t, response, map[string]any{
				"protocolVersion": protocolVersion,
				"serverInfo":      map[string]any{"name": "golang-deepsearch-mcp"},
			})
		case "notifications/initialized":
			response.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeResult(t, response, map[string]any{
				"tools": []map[string]any{{"name": "magnet_search", "description": "搜索", "inputSchema": map[string]any{"type": "object"}}},
			})
		case "tools/call":
			writeResult(t, response, toolContent(`{"count":1,"items":[{"name":"ABP-001"}]}`))
		default:
			t.Errorf("unexpected method %q", method)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "token-1", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	tools, err := client.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "magnet_search" {
		t.Fatalf("tools = %+v", tools)
	}
	payload, err := client.Call(context.Background(), "magnet_search", map[string]any{"query": "ABP-001"})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"count":1,"items":[{"name":"ABP-001"}]}` {
		t.Fatalf("payload = %s", payload)
	}

	// One handshake serves both calls, and every call after it names the session.
	if got := log.count("initialize"); got != 1 {
		t.Fatalf("initialize ran %d times, want 1", got)
	}
	if got := log.count("tools/list"); got != 1 {
		t.Fatalf("tools/list ran %d times, want 1", got)
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.sessions[0] != "" {
		t.Fatalf("the handshake carried a session: %q", log.sessions[0])
	}
	for index, session := range log.sessions[1:] {
		if session != "session-1" {
			t.Fatalf("request %d carried session %q", index+1, session)
		}
	}
	for _, accept := range log.accepts {
		// Offering to accept a stream makes this server hold the call open.
		if accept != "application/json" {
			t.Fatalf("Accept = %q", accept)
		}
	}
	for _, auth := range log.auths {
		if auth != "Bearer token-1" {
			t.Fatalf("Authorization = %q", auth)
		}
	}
}

func TestHandshakeIsRedoneWhenTheServerForgetsTheSession(t *testing.T) {
	log := &wire{}
	forgotten := false
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		method := readMethod(t, request)
		log.record(request, method)
		switch {
		case method == "initialize":
			response.Header().Set(sessionHeader, fmt.Sprintf("session-%d", log.count("initialize")))
			writeResult(t, response, map[string]any{"protocolVersion": protocolVersion})
		case method == "notifications/initialized":
			response.WriteHeader(http.StatusAccepted)
		case !forgotten:
			// The first real call hits a session the server no longer knows.
			forgotten = true
			response.WriteHeader(http.StatusNotFound)
		default:
			writeResult(t, response, toolContent(`{"ok":true}`))
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := client.Call(context.Background(), "magnet_search", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"ok":true}` {
		t.Fatalf("payload = %s", payload)
	}
	if got := log.count("initialize"); got != 2 {
		t.Fatalf("initialize ran %d times, want 2", got)
	}
}

func TestToolErrorsAndProtocolErrorsBothSurface(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch readMethod(t, request) {
		case "initialize":
			writeResult(t, response, map[string]any{"protocolVersion": protocolVersion})
		case "notifications/initialized":
			response.WriteHeader(http.StatusAccepted)
		default:
			result := toolContent("磁力链接无法解析")
			result["isError"] = true
			writeResult(t, response, result)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(context.Background(), "magnet_preview", nil); err == nil ||
		!strings.Contains(err.Error(), "磁力链接无法解析") {
		t.Fatalf("a refused tool was accepted: %v", err)
	}
}

func TestStreamedReplyIsUnwrapped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch readMethod(t, request) {
		case "initialize":
			writeResult(t, response, map[string]any{"protocolVersion": protocolVersion})
		case "notifications/initialized":
			response.WriteHeader(http.StatusAccepted)
		default:
			response.Header().Set("Content-Type", "text/event-stream")
			body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": toolContent(`{"count":0}`)})
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(response, "event: message\ndata: %s\n\n", body)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := client.Call(context.Background(), "magnet_search", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"count":0}` {
		t.Fatalf("payload = %s", payload)
	}
}

func TestPlainTextToolOutputBecomesAJSONString(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch readMethod(t, request) {
		case "initialize":
			writeResult(t, response, map[string]any{"protocolVersion": protocolVersion})
		case "notifications/initialized":
			response.WriteHeader(http.StatusAccepted)
		default:
			writeResult(t, response, toolContent("no results"))
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := client.Call(context.Background(), "magnet_search", nil)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	if err := json.Unmarshal(payload, &text); err != nil || text != "no results" {
		t.Fatalf("payload = %s (%v)", payload, err)
	}
}

func TestARejectedTokenIsReportedAsAConfigurationProblem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := New(server.URL, "stale", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Tools(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "token") {
		t.Fatalf("a rejected token was not reported: %v", err)
	}
}

func TestInvalidEndpointsAreRefused(t *testing.T) {
	for _, endpoint := range []string{"", "   ", "ftp://example.com/mcp", "example.com/mcp", "https://user:pw@example.com/mcp"} {
		if _, err := New(endpoint, "token", nil); err == nil {
			t.Fatalf("endpoint %q was accepted", endpoint)
		}
	}
}
