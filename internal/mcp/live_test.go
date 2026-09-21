package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestLiveServer runs the client against a real deployment. It is skipped
// unless both the endpoint and the token are supplied:
//
//	MIYABI_MCP_LIVE_URL=https://magnet.kiteyuan.info/api/v1/mcp \
//	MIYABI_MCP_LIVE_TOKEN=... go test ./internal/mcp/ -run Live -v
//
// The token is never printed, and the search asks for a single result so the
// call stays cheap.
func TestLiveServer(t *testing.T) {
	endpoint := os.Getenv("MIYABI_MCP_LIVE_URL")
	token := os.Getenv("MIYABI_MCP_LIVE_TOKEN")
	if endpoint == "" || token == "" {
		t.Skip("MIYABI_MCP_LIVE_URL and MIYABI_MCP_LIVE_TOKEN are not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := New(endpoint, token, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := client.Tools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	t.Logf("tools: %v", names)
	for _, required := range []string{"magnet_search", "magnet_preview", "magnet_files", "favorite_collections_list"} {
		found := false
		for _, name := range names {
			if name == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("the server does not offer %s", required)
		}
	}

	payload, err := client.Call(ctx, "magnet_search", map[string]any{"query": "ABP-001", "limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Count int `json:"count"`
		Items []struct {
			Name      string `json:"name"`
			MagnetURL string `json:"magnet_url"`
			Size      string `json:"size"`
		} `json:"items"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.Count == 0 || len(result.Items) == 0 || result.Items[0].MagnetURL == "" {
		t.Fatalf("search returned %s", payload)
	}
	t.Logf("search: %d result(s), first = %s (%s)", result.Count, result.Items[0].Name, result.Items[0].Size)

	// A second call must reuse the session established above.
	if _, err := client.Call(ctx, "favorite_collections_list", map[string]any{}); err != nil {
		t.Fatal(err)
	}
}
