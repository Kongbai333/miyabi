package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ppxb/miyabi/internal/database"
)

// These payloads were captured from the magnet server and are kept verbatim, so
// a change in the remote shape fails the decode instead of yielding zeroes.
const (
	magnetSearchPayload = `{"count":2,"items":[` +
		`{"name":"ABP-001","magnet_url":"magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC","link":"magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC","size":"1.08 GB","date":"2026-03-22","score":147},` +
		`{"name":"(無修正-流出) ABP-001","magnet_url":"magnet:?xt=urn:btih:90769A8FBA67E539071AA343ADF0A11B0A57C4AF","link":"magnet:?xt=urn:btih:90769A8FBA67E539071AA343ADF0A11B0A57C4AF","size":"4.56 GB","date":"2026-07-09","score":147}` +
		`],"local_best_score":147,"query":"ABP-001","source":"local"}`

	magnetPreviewPayload = `{"magnet_link":"magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC","size":1154835496,` +
		`"data":{"count":2,"error":"","file_type":"folder","name":"ABP-001","size":1154835496,"type":"FOLDER",` +
		`"screenshots":[{"screenshot":"https://whatslink.info/image/bb58bbb62e6f9a59c7c12c0659731fa5","time":0},` +
		`{"screenshot":"https://whatslink.info/image/1e5e0967b49639ec3d0d50805f290164","time":0}]}}`

	// raw_data repeats the whole tree; the decode must ignore it.
	magnetFilesPayload = `{"magnet_link":"magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC","list_id":"1k0RLj0B-Wzsu3vs5BchhxjXWJvtkv6i",` +
		`"total_size":1154835496,"file_count":1,"files":[{"id":"OarMFcYC5CI_hC-bTAox8MNQ1Pg.0","name":"ABP-001","file_size":"1154835496","file_size_bytes":1154835496,"file_count":2,"file_index":0,"is_dir":true,"parent_id":"OarMFcYC5CI_hC-bTAox8MNQ1Pg","resolver":"DIRECT",` +
		`"meta":{"bt_create_time":"1473369913","file_category":"","hash":"","icon":"https://backstage-img-ssl.a.88cdn.com/3c70412ed9baf44edc11ba49fe51fefd5d4bee5c","mime_type":"","status":"1","url":"magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC","url_tag":"video","video_resolution":""},` +
		`"sub_files":[{"id":"OarMFcYC5CI_hC-bTAox8MNQ1Pg.0.0","name":"abp001.mp4","file_size":"1154637593","file_size_bytes":1154637593,"file_count":1,"is_dir":false,"meta":{"hash":"71BDD7AFDD3DF4CA6ED290DCCEBD4372D74C0D1B","icon":"https://backstage-img-ssl.a.88cdn.com/29ff67798744a75f489a5af476846187e709fe30","mime_type":"video/mp4","url_tag":""}},` +
		`{"id":"OarMFcYC5CI_hC-bTAox8MNQ1Pg.0.1","name":"Nn3CGRY.jpg","file_size_bytes":197903,"is_dir":false,"meta":{"mime_type":"image/jpeg"}}]}],` +
		`"raw_data":{"state":0,"list_id":"1k0RLj0B-Wzsu3vs5BchhxjXWJvtkv6i","list":{"page_size":2000,"resources":[]}}}`

	collectionsPayload = `{"collections":[{"deletable":false,"is_default":true,"key":"default","label":"默认合集"}],"count":1}`

	collectionDetailPayload = `{"collection":{"is_default":true,"key":"default","label":"默认合集"},"count":1,"items":[` +
		`{"name":"ABP-001","tagsText":["无码","中字"],"magnet_url":"magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC","added_at":1789996460000,"collection":"default"}]}`

	shareEnablePayload = `{"code":"u63ky5vmvg46","rss_url":"/api/v1/favorites/share/u63ky5vmvg46/rss","share_url":"/share/u63ky5vmvg46"}`

	shareDetailPayload = `{"code":"u63ky5vmvg46","count":0,"items":[],"label":"默认合集","owner_id":956,"rss_url":"/api/v1/favorites/share/u63ky5vmvg46/rss","share_url":"/share/u63ky5vmvg46","updated_at":"2026-09-21T21:37:34+08:00"}`
)

// mcpStub answers the protocol and replays a payload per tool.
type mcpStub struct {
	payloads map[string]string

	mu    sync.Mutex
	calls []stubCall
}

type stubCall struct {
	Tool      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (stub *mcpStub) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(response http.ResponseWriter, request *http.Request) {
		var envelope struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		response.Header().Set("Content-Type", "application/json")
		switch envelope.Method {
		case "initialize":
			response.Header().Set("Mcp-Session-Id", "session-1")
			writeMCPResult(t, response, map[string]any{"protocolVersion": "2025-06-18"})
		case "notifications/initialized":
			response.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeMCPResult(t, response, map[string]any{"tools": []map[string]any{{"name": "magnet_search"}}})
		case "tools/call":
			var call stubCall
			if err := json.Unmarshal(envelope.Params, &call); err != nil {
				t.Fatalf("decode call: %v", err)
			}
			stub.mu.Lock()
			stub.calls = append(stub.calls, call)
			stub.mu.Unlock()
			payload, found := stub.payloads[call.Tool]
			if !found {
				t.Fatalf("no payload for tool %q", call.Tool)
			}
			writeMCPResult(t, response, map[string]any{
				"content": []map[string]any{{"type": "text", "text": payload}},
			})
		default:
			t.Fatalf("unexpected method %q", envelope.Method)
		}
	}
}

func (stub *mcpStub) lastCall(t *testing.T) stubCall {
	t.Helper()
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.calls) == 0 {
		t.Fatal("the server was never called")
	}
	return stub.calls[len(stub.calls)-1]
}

func writeMCPResult(t *testing.T, response http.ResponseWriter, result any) {
	t.Helper()
	if err := json.NewEncoder(response).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result}); err != nil {
		t.Fatalf("write reply: %v", err)
	}
}

// mcpFixture wires a service to a stub server.
func mcpFixture(t *testing.T, payloads map[string]string) (*MCPService, *mcpStub) {
	t.Helper()
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	stub := &mcpStub{payloads: payloads}
	server := httptest.NewServer(stub.handler(t))
	t.Cleanup(server.Close)

	service, err := NewMCPService(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateSettings(t.Context(), MCPSettings{URL: server.URL, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	return service, stub
}

func TestMagnetSearchDecodesTheServerPayload(t *testing.T) {
	service, stub := mcpFixture(t, map[string]string{"magnet_search": magnetSearchPayload})

	result, err := service.MagnetSearch(t.Context(), "  ABP-001  ", 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 2 || len(result.Items) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if result.Items[0].Name != "ABP-001" || result.Items[0].Size != "1.08 GB" || result.Items[0].Score != 147 {
		t.Fatalf("first item = %+v", result.Items[0])
	}
	if result.Items[1].Date != "2026-07-09" {
		t.Fatalf("second item = %+v", result.Items[1])
	}
	if result.Query != "ABP-001" || result.Source != "local" {
		t.Fatalf("result = %+v", result)
	}
	// The query is trimmed and the limit falls back to the documented default.
	var arguments struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(stub.lastCall(t).Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.Query != "ABP-001" || arguments.Limit != mcpSearchDefault {
		t.Fatalf("arguments = %+v", arguments)
	}
}

func TestMagnetSearchClampsTheLimit(t *testing.T) {
	service, stub := mcpFixture(t, map[string]string{"magnet_search": magnetSearchPayload})
	if _, err := service.MagnetSearch(t.Context(), "ABP-001", 500); err != nil {
		t.Fatal(err)
	}
	var arguments struct {
		Limit int `json:"limit"`
	}
	if err := json.Unmarshal(stub.lastCall(t).Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.Limit != mcpSearchMaximum {
		t.Fatalf("limit = %d", arguments.Limit)
	}
}

func TestMagnetPreviewDecodesScreenshotsAndBytes(t *testing.T) {
	service, _ := mcpFixture(t, map[string]string{"magnet_preview": magnetPreviewPayload})

	result, err := service.MagnetPreview(t.Context(), "1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC")
	if err != nil {
		t.Fatal(err)
	}
	// A bare info hash is turned into a magnet URI before it is sent.
	if result.MagnetLink != "magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC" {
		t.Fatalf("magnet = %q", result.MagnetLink)
	}
	if result.Size != 1154835496 || result.Data.Name != "ABP-001" || result.Data.FileType != "folder" {
		t.Fatalf("preview = %+v", result)
	}
	if len(result.Data.Screenshots) != 2 ||
		!strings.HasPrefix(result.Data.Screenshots[0].URL, "https://whatslink.info/image/") {
		t.Fatalf("screenshots = %+v", result.Data.Screenshots)
	}
}

func TestMagnetFilesDecodesTheTreeAndIgnoresTheDuplicate(t *testing.T) {
	service, _ := mcpFixture(t, map[string]string{"magnet_files": magnetFilesPayload})

	result, err := service.MagnetFiles(t.Context(), "magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC")
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalSize != 1154835496 || result.FileCount != 1 || len(result.Files) != 1 {
		t.Fatalf("files = %+v", result)
	}
	root := result.Files[0]
	if !root.IsDir || root.Name != "ABP-001" || root.Meta.URLTag != "video" {
		t.Fatalf("root = %+v", root)
	}
	if len(root.SubFiles) != 2 || root.SubFiles[0].Name != "abp001.mp4" ||
		root.SubFiles[0].Meta.MimeType != "video/mp4" {
		t.Fatalf("sub files = %+v", root.SubFiles)
	}
}

func TestCollectionsDecode(t *testing.T) {
	service, _ := mcpFixture(t, map[string]string{
		"favorite_collections_list":  collectionsPayload,
		"favorite_collection_detail": collectionDetailPayload,
		"favorite_collection_create": `{"ok":true}`,
		"favorite_collection_rename": `{"ok":true}`,
		"favorite_collection_delete": `{"ok":true}`,
		"favorite_share_import":      `{"ok":true}`,
	})

	collections, err := service.Collections(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(collections) != 1 || collections[0].Key != "default" ||
		collections[0].Label != "默认合集" || !collections[0].IsDefault || collections[0].Deletable {
		t.Fatalf("collections = %+v", collections)
	}

	detail, err := service.Collection(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Count != 1 || len(detail.Items) != 1 {
		t.Fatalf("detail = %+v", detail)
	}
	item := detail.Items[0]
	if item.Name != "ABP-001" || item.MagnetURL == "" || item.AddedAt != 1789996460000 || item.Collection != "default" {
		t.Fatalf("item = %+v", item)
	}
	if len(item.Tags) != 2 || item.Tags[1] != "中字" {
		t.Fatalf("tags = %+v", item.Tags)
	}

	for _, mutate := range []func() error{
		func() error { return service.CreateCollection(t.Context(), " 无码精选 ") },
		func() error { return service.RenameCollection(t.Context(), "default", "改名") },
		func() error { return service.DeleteCollection(t.Context(), "gone") },
		func() error { return service.ImportShare(t.Context(), "u63ky5vmvg46", "") },
	} {
		if err := mutate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestShareDecodes(t *testing.T) {
	service, _ := mcpFixture(t, map[string]string{
		"favorite_share_enable":  shareEnablePayload,
		"favorite_share_detail":  shareDetailPayload,
		"favorite_share_disable": `{"ok":true}`,
	})

	share, err := service.EnableShare(t.Context(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if share.Code != "u63ky5vmvg46" || share.ShareURL != "/share/u63ky5vmvg46" {
		t.Fatalf("share = %+v", share)
	}

	detail, err := service.ShareDetail(t.Context(), "u63ky5vmvg46")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Label != "默认合集" || detail.OwnerID != 956 || detail.UpdatedAt == "" {
		t.Fatalf("share detail = %+v", detail)
	}
	if detail.Items == nil {
		t.Fatal("an empty share should decode to an empty list, not nil")
	}

	if err := service.DisableShare(t.Context(), "default"); err != nil {
		t.Fatal(err)
	}
}

func TestInputsAreValidatedBeforeAnyCall(t *testing.T) {
	service, stub := mcpFixture(t, map[string]string{"magnet_search": magnetSearchPayload})

	if _, err := service.MagnetSearch(t.Context(), "   ", 0); err == nil {
		t.Fatal("an empty query was accepted")
	}
	if _, err := service.MagnetPreview(t.Context(), ""); err == nil {
		t.Fatal("an empty magnet was accepted")
	}
	if _, err := service.Collection(t.Context(), ""); err == nil {
		t.Fatal("an empty collection key was accepted")
	}
	if err := service.CreateCollection(t.Context(), "  "); err == nil {
		t.Fatal("an empty label was accepted")
	}
	if _, err := service.ShareDetail(t.Context(), ""); err == nil {
		t.Fatal("an empty share code was accepted")
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.calls) != 0 {
		t.Fatalf("invalid input still reached the server: %+v", stub.calls)
	}
}

func TestMagnetLinksAreNormalized(t *testing.T) {
	const hash = "1f5f629f420fe6fa53fcddb10f1daf030e7b62ec"
	for input, want := range map[string]string{
		hash:                                    "magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC",
		strings.ToUpper(hash):                   "magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC",
		"magnet:?xt=urn:btih:" + hash:           "magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC",
		"magnet:?xt=urn:btih:" + hash + "&dn=x": "magnet:?xt=urn:btih:1F5F629F420FE6FA53FCDDB10F1DAF030E7B62EC",
	} {
		got, err := magnetLink(input)
		if err != nil {
			t.Fatalf("%q: %v", input, err)
		}
		if got != want {
			t.Fatalf("%q -> %q, want %q", input, got, want)
		}
	}
	// Anything that is not a hash is passed through so the server can judge it.
	if got, err := magnetLink("https://example.com/x.torrent"); err != nil || got != "https://example.com/x.torrent" {
		t.Fatalf("passthrough = %q (%v)", got, err)
	}
}

func TestMissingTokenIsReportedWithoutCallingTheServer(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service, err := NewMCPService(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.MagnetSearch(t.Context(), "ABP-001", 0); !errors.Is(err, ErrMagnetTokenRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestSettingsPersistAndTheDefaultServerFillsIn(t *testing.T) {
	store, err := database.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	service, err := NewMCPService(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := service.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.URL != DefaultMCPServer || settings.Token != "" {
		t.Fatalf("default settings = %+v", settings)
	}

	saved, err := service.UpdateSettings(t.Context(), MCPSettings{Token: "  mcp_abc  "})
	if err != nil {
		t.Fatal(err)
	}
	if saved.URL != DefaultMCPServer || saved.Token != "mcp_abc" {
		t.Fatalf("saved settings = %+v", saved)
	}

	// A restart reads the same configuration back.
	restarted, err := NewMCPService(t.Context(), store.Client)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := restarted.Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if reloaded != saved {
		t.Fatalf("reloaded = %+v, want %+v", reloaded, saved)
	}

	if _, err := service.UpdateSettings(t.Context(), MCPSettings{URL: "ftp://example.com/mcp", Token: "x"}); err == nil {
		t.Fatal("an invalid endpoint was saved")
	}
}

func TestTestReportsTheTools(t *testing.T) {
	service, _ := mcpFixture(t, nil)
	result, err := service.Test(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 1 || result.Tools[0] != "magnet_search" {
		t.Fatalf("result = %+v", result)
	}
}
