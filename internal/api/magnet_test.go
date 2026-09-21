package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ppxb/miyabi/internal/service"
)

// magnetStub records what reached the service so the tests can assert the
// handlers forward their input instead of reshaping it.
type magnetStub struct {
	MagnetManager

	calls    []string
	settings service.MCPSettings
	query    string
	limit    int
	link     string
	key      string
	code     string
	label    string
}

func (stub *magnetStub) note(call string) { stub.calls = append(stub.calls, call) }

func (stub *magnetStub) Settings(context.Context) (service.MCPSettings, error) {
	stub.note("settings")
	return stub.settings, nil
}

func (stub *magnetStub) UpdateSettings(_ context.Context, settings service.MCPSettings) (service.MCPSettings, error) {
	stub.note("update-settings")
	stub.settings = settings
	return settings, nil
}

func (stub *magnetStub) Test(context.Context) (service.MCPTestResult, error) {
	stub.note("test")
	return service.MCPTestResult{Tools: []string{"magnet_search"}}, nil
}

func (stub *magnetStub) MagnetSearch(_ context.Context, query string, limit int) (service.MagnetSearchResult, error) {
	stub.note("search")
	stub.query, stub.limit = query, limit
	return service.MagnetSearchResult{Query: query, Count: 1, Items: []service.MagnetItem{
		{Name: "ABP-001", MagnetURL: "magnet:?xt=urn:btih:ABC", Size: "1.08 GB", Date: "2026-03-22", Score: 147},
	}}, nil
}

func (stub *magnetStub) MagnetPreview(_ context.Context, link string) (service.MagnetPreview, error) {
	stub.note("preview")
	stub.link = link
	return service.MagnetPreview{MagnetLink: link, Size: 1, Data: service.MagnetPreviewData{
		Name: "ABP-001", Screenshots: []service.MagnetScreenshot{{URL: "https://example.com/a.jpg", Time: 10}},
	}}, nil
}

func (stub *magnetStub) MagnetFiles(_ context.Context, link string) (service.MagnetFiles, error) {
	stub.note("files")
	stub.link = link
	return service.MagnetFiles{MagnetLink: link, FileCount: 1, TotalSize: 2}, nil
}

func (stub *magnetStub) Collections(context.Context) ([]service.MagnetCollection, error) {
	stub.note("collections")
	return []service.MagnetCollection{{Key: "default", Label: "默认合集", IsDefault: true}}, nil
}

func (stub *magnetStub) Collection(_ context.Context, key string) (service.MagnetCollectionDetail, error) {
	stub.note("collection")
	stub.key = key
	return service.MagnetCollectionDetail{Collection: service.MagnetCollection{Key: key}, Items: []service.MagnetCollectionItem{}}, nil
}

func (stub *magnetStub) CreateCollection(_ context.Context, label string) error {
	stub.note("create-collection")
	stub.label = label
	return nil
}

func (stub *magnetStub) RenameCollection(_ context.Context, key, label string) error {
	stub.note("rename-collection")
	stub.key, stub.label = key, label
	return nil
}

func (stub *magnetStub) DeleteCollection(_ context.Context, key string) error {
	stub.note("delete-collection")
	stub.key = key
	return nil
}

func (stub *magnetStub) EnableShare(_ context.Context, key string) (service.MagnetShare, error) {
	stub.note("enable-share")
	stub.key = key
	return service.MagnetShare{Code: "u63ky5vmvg46", ShareURL: "/share/u63ky5vmvg46"}, nil
}

func (stub *magnetStub) DisableShare(_ context.Context, key string) error {
	stub.note("disable-share")
	stub.key = key
	return nil
}

func (stub *magnetStub) ShareDetail(_ context.Context, code string) (service.MagnetShareDetail, error) {
	stub.note("share-detail")
	stub.code = code
	return service.MagnetShareDetail{Code: code, Items: []service.MagnetCollectionItem{}}, nil
}

func (stub *magnetStub) ImportShare(_ context.Context, code, label string) error {
	stub.note("import-share")
	stub.code, stub.label = code, label
	return nil
}

func magnetRouter(stub *magnetStub) http.Handler {
	return NewRouter(Dependencies{Magnet: stub, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func call(t *testing.T, stub *magnetStub, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	response := httptest.NewRecorder()
	magnetRouter(stub).ServeHTTP(response, httptest.NewRequest(method, path, reader))
	return response
}

func TestMagnetSettingsRoundTrip(t *testing.T) {
	stub := &magnetStub{settings: service.MCPSettings{URL: "https://example.com/mcp", Token: "mcp_secret"}}
	response := call(t, stub, http.MethodGet, "/api/magnet/settings", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
	// The token is handed back as stored, which is why the route is no-store.
	if !strings.Contains(response.Body.String(), "mcp_secret") {
		t.Fatalf("the token was not returned: %s", response.Body)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("a response carrying the token is cacheable: %q", response.Header().Get("Cache-Control"))
	}

	response = call(t, stub, http.MethodPut, "/api/magnet/settings", `{"url":"","token":"  mcp_new  "}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
	if stub.settings.Token != "  mcp_new  " {
		t.Fatalf("the handler reshaped the token: %q", stub.settings.Token)
	}

	response = call(t, stub, http.MethodPost, "/api/magnet/test", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "magnet_search") {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
}

func TestMagnetSearchForwardsTheQueryAndLimit(t *testing.T) {
	stub := &magnetStub{}
	response := call(t, stub, http.MethodPost, "/api/magnet/search", `{"query":"ABP-001","limit":3}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
	if stub.query != "ABP-001" || stub.limit != 3 {
		t.Fatalf("forwarded query=%q limit=%d", stub.query, stub.limit)
	}
	var result struct {
		Items []struct {
			Name string `json:"name"`
			Size string `json:"size"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Size != "1.08 GB" {
		t.Fatalf("items = %+v", result.Items)
	}
}

func TestMagnetHandlersRejectUnusableInput(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path, body string
	}{
		{"search without a query", http.MethodPost, "/api/magnet/search", `{}`},
		// A blank query is not rejected here: the service trims and refuses it
		// before reaching the server, and owns that rule for every caller.
		{"search above the documented maximum", http.MethodPost, "/api/magnet/search", `{"query":"x","limit":50}`},
		{"preview without a magnet", http.MethodPost, "/api/magnet/preview", `{}`},
		{"files without a magnet", http.MethodPost, "/api/magnet/files", `{}`},
		{"collection create without a label", http.MethodPost, "/api/magnet/collections", `{"label":""}`},
		{"share import without a code", http.MethodPost, "/api/magnet/shares/import", `{}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &magnetStub{}
			response := call(t, stub, scenario.method, scenario.path, scenario.body)
			if response.Code != http.StatusBadRequest || len(stub.calls) != 0 {
				t.Fatalf("status = %d calls = %v body = %s", response.Code, stub.calls, response.Body)
			}
		})
	}
}

func TestMagnetCollectionRoutesBindTheirKeys(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path, body string
		status                   int
		call                     string
	}{
		{"detail", http.MethodGet, "/api/magnet/collections/default", "", http.StatusOK, "collection"},
		{"rename", http.MethodPut, "/api/magnet/collections/default", `{"label":"改名"}`, http.StatusOK, "rename-collection"},
		{"delete", http.MethodDelete, "/api/magnet/collections/default", "", http.StatusOK, "delete-collection"},
		{"share on", http.MethodPost, "/api/magnet/collections/default/share", "", http.StatusOK, "enable-share"},
		{"share off", http.MethodDelete, "/api/magnet/collections/default/share", "", http.StatusOK, "disable-share"},
		{"share detail", http.MethodGet, "/api/magnet/shares/u63ky5vmvg46", "", http.StatusOK, "share-detail"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &magnetStub{}
			response := call(t, stub, scenario.method, scenario.path, scenario.body)
			if response.Code != scenario.status {
				t.Fatalf("status = %d body = %s", response.Code, response.Body)
			}
			if len(stub.calls) != 1 || stub.calls[0] != scenario.call {
				t.Fatalf("calls = %v", stub.calls)
			}
		})
	}
	// The key reaches the service rather than being dropped by the binder.
	stub := &magnetStub{}
	call(t, stub, http.MethodGet, "/api/magnet/collections/%E7%A0%81", "")
	if stub.key != "码" {
		t.Fatalf("key = %q", stub.key)
	}
}

func TestMagnetCreateAndImportAnswerWithTheNewList(t *testing.T) {
	stub := &magnetStub{}
	response := call(t, stub, http.MethodPost, "/api/magnet/collections", `{"label":"无码精选"}`)
	if response.Code != http.StatusCreated || stub.label != "无码精选" {
		t.Fatalf("status = %d label = %q body = %s", response.Code, stub.label, response.Body)
	}
	if !strings.Contains(response.Body.String(), "默认合集") {
		t.Fatalf("the created list was not returned: %s", response.Body)
	}

	stub = &magnetStub{}
	response = call(t, stub, http.MethodPost, "/api/magnet/shares/import", `{"code":"u63ky5vmvg46","label":"导入"}`)
	if response.Code != http.StatusCreated || stub.code != "u63ky5vmvg46" || stub.label != "导入" {
		t.Fatalf("status = %d code = %q label = %q body = %s", response.Code, stub.code, stub.label, response.Body)
	}
}

func TestMagnetRoutesRequireTheAccessPassword(t *testing.T) {
	// The gate only protects routes when it is wired in, so this checks the
	// routes live inside the guarded group rather than outside it.
	gate := service.NewAccessGateService("secret")
	router := NewRouter(Dependencies{
		Magnet: &magnetStub{},
		Access: gate,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	for _, path := range []string{"/api/magnet/settings", "/api/magnet/collections"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s answered %d without a session", path, response.Code)
		}
	}
}
