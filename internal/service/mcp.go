package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/mcp"
)

const (
	mcpServerSetting = "mcp.server"
	// DefaultMCPServer is the magnet tool server the settings page offers.
	DefaultMCPServer = "https://magnet.kiteyuan.info/api/v1/mcp"
	// A search may crawl several engines before it answers.
	mcpCallTimeout = 90 * time.Second
	// The server documents these bounds for magnet_search.
	mcpSearchDefault = 7
	mcpSearchMaximum = 20
)

// ErrMagnetTokenRequired is returned before any call when no token is saved.
var ErrMagnetTokenRequired = domain.E(domain.KindInvalid, "请先在设置中填写磁力服务的 token", nil)

// magnetHash matches the info hash of a magnet URI.
var magnetHash = regexp.MustCompile(`xt=urn:btih:([0-9a-fA-F]{40})`)

// MCPSettings is the persisted configuration. The token is stored as written
// and handed back to the settings page unchanged, because the app runs for one
// operator behind its own access password.
type MCPSettings struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// MCPTestResult reports what the server offered when the settings were tested.
type MCPTestResult struct {
	Tools []string `json:"tools"`
}

// MagnetItem is one search hit. Size arrives formatted, unlike the preview and
// the file listing, which report bytes.
type MagnetItem struct {
	Name      string `json:"name"`
	MagnetURL string `json:"magnet_url"`
	Link      string `json:"link"`
	Size      string `json:"size"`
	Date      string `json:"date"`
	Score     int    `json:"score"`
}

type MagnetSearchResult struct {
	Query          string       `json:"query"`
	Source         string       `json:"source"`
	Count          int          `json:"count"`
	LocalBestScore int          `json:"local_best_score"`
	Items          []MagnetItem `json:"items"`
}

type MagnetScreenshot struct {
	URL  string `json:"screenshot"`
	Time int    `json:"time"`
}

type MagnetPreviewData struct {
	Count       int                `json:"count"`
	Error       string             `json:"error"`
	Name        string             `json:"name"`
	FileType    string             `json:"file_type"`
	Type        string             `json:"type"`
	Size        int64              `json:"size"`
	Screenshots []MagnetScreenshot `json:"screenshots"`
}

type MagnetPreview struct {
	MagnetLink string            `json:"magnet_link"`
	Size       int64             `json:"size"`
	Data       MagnetPreviewData `json:"data"`
}

type MagnetFileMeta struct {
	Icon            string `json:"icon"`
	MimeType        string `json:"mime_type"`
	Hash            string `json:"hash"`
	URLTag          string `json:"url_tag"`
	VideoResolution string `json:"video_resolution"`
}

type MagnetFile struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	SizeBytes int64          `json:"file_size_bytes"`
	FileCount int            `json:"file_count"`
	IsDir     bool           `json:"is_dir"`
	Meta      MagnetFileMeta `json:"meta"`
	SubFiles  []MagnetFile   `json:"sub_files"`
}

type MagnetFiles struct {
	MagnetLink string       `json:"magnet_link"`
	ListID     string       `json:"list_id"`
	FileCount  int          `json:"file_count"`
	TotalSize  int64        `json:"total_size"`
	Files      []MagnetFile `json:"files"`
}

type MagnetCollection struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Deletable bool   `json:"deletable"`
	IsDefault bool   `json:"is_default"`
}

type MagnetCollectionItem struct {
	Name       string   `json:"name"`
	Tags       []string `json:"tagsText"`
	MagnetURL  string   `json:"magnet_url"`
	AddedAt    int64    `json:"added_at"`
	Collection string   `json:"collection"`
}

type MagnetCollectionDetail struct {
	Collection MagnetCollection       `json:"collection"`
	Count      int                    `json:"count"`
	Items      []MagnetCollectionItem `json:"items"`
}

// MagnetShare is what the server answers when a collection starts sharing.
type MagnetShare struct {
	Code     string `json:"code"`
	RSSURL   string `json:"rss_url"`
	ShareURL string `json:"share_url"`
}

// MagnetShareDetail is the public view of a share code.
type MagnetShareDetail struct {
	Code      string                 `json:"code"`
	Label     string                 `json:"label"`
	Count     int                    `json:"count"`
	Items     []MagnetCollectionItem `json:"items"`
	OwnerID   int64                  `json:"owner_id"`
	RSSURL    string                 `json:"rss_url"`
	ShareURL  string                 `json:"share_url"`
	UpdatedAt string                 `json:"updated_at"`
}

// MCPService owns the persisted magnet server configuration and the session
// that talks to it. A settings update drops the client so the next call
// handshakes with the new endpoint.
type MCPService struct {
	database *ent.Client

	mu       sync.Mutex
	settings MCPSettings
	client   *mcp.Client
}

func NewMCPService(ctx context.Context, database *ent.Client) (*MCPService, error) {
	stored, _, err := loadSetting[MCPSettings](ctx, database, mcpServerSetting)
	if err != nil {
		return nil, err
	}
	service := &MCPService{database: database}
	service.settings = normalizeMCPSettings(stored)
	return service, nil
}

// Settings returns the saved configuration, token included.
func (service *MCPService) Settings(context.Context) (MCPSettings, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.settings, nil
}

// UpdateSettings validates and persists the configuration. The URL is optional
// and falls back to the default server, so a blank field is not an error.
func (service *MCPService) UpdateSettings(ctx context.Context, settings MCPSettings) (MCPSettings, error) {
	next := normalizeMCPSettings(settings)
	if _, err := mcp.New(next.URL, next.Token, nil); err != nil {
		return MCPSettings{}, err
	}
	if err := saveSetting(ctx, service.database, mcpServerSetting, next); err != nil {
		return MCPSettings{}, err
	}
	service.mu.Lock()
	service.settings = next
	// The old session belonged to the old endpoint and the old token.
	service.client = nil
	service.mu.Unlock()
	return next, nil
}

// Test handshakes and lists the tools, which is what a settings page needs to
// tell a wrong token from a wrong address.
func (service *MCPService) Test(ctx context.Context) (MCPTestResult, error) {
	client, err := service.session()
	if err != nil {
		return MCPTestResult{}, err
	}
	tools, err := client.Tools(ctx)
	if err != nil {
		return MCPTestResult{}, err
	}
	result := MCPTestResult{Tools: make([]string, 0, len(tools))}
	for _, tool := range tools {
		result.Tools = append(result.Tools, tool.Name)
	}
	return result, nil
}

func (service *MCPService) MagnetSearch(ctx context.Context, query string, limit int) (MagnetSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return MagnetSearchResult{}, domain.E(domain.KindInvalid, "请输入搜索关键词", nil)
	}
	if limit <= 0 {
		limit = mcpSearchDefault
	}
	if limit > mcpSearchMaximum {
		limit = mcpSearchMaximum
	}
	payload, err := service.invoke(ctx, "magnet_search", map[string]any{"query": query, "limit": limit})
	if err != nil {
		return MagnetSearchResult{}, err
	}
	var result MagnetSearchResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return MagnetSearchResult{}, fmt.Errorf("decode magnet search: %w", err)
	}
	if result.Items == nil {
		result.Items = []MagnetItem{}
	}
	return result, nil
}

func (service *MCPService) MagnetPreview(ctx context.Context, link string) (MagnetPreview, error) {
	link, err := magnetLink(link)
	if err != nil {
		return MagnetPreview{}, err
	}
	payload, err := service.invoke(ctx, "magnet_preview", map[string]any{"magnet_link": link})
	if err != nil {
		return MagnetPreview{}, err
	}
	var result MagnetPreview
	if err := json.Unmarshal(payload, &result); err != nil {
		return MagnetPreview{}, fmt.Errorf("decode magnet preview: %w", err)
	}
	return result, nil
}

func (service *MCPService) MagnetFiles(ctx context.Context, link string) (MagnetFiles, error) {
	link, err := magnetLink(link)
	if err != nil {
		return MagnetFiles{}, err
	}
	payload, err := service.invoke(ctx, "magnet_files", map[string]any{"magnet_link": link})
	if err != nil {
		return MagnetFiles{}, err
	}
	var result MagnetFiles
	if err := json.Unmarshal(payload, &result); err != nil {
		return MagnetFiles{}, fmt.Errorf("decode magnet files: %w", err)
	}
	return result, nil
}

func (service *MCPService) Collections(ctx context.Context) ([]MagnetCollection, error) {
	payload, err := service.invoke(ctx, "favorite_collections_list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Collections []MagnetCollection `json:"collections"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, fmt.Errorf("decode magnet collections: %w", err)
	}
	if result.Collections == nil {
		result.Collections = []MagnetCollection{}
	}
	return result.Collections, nil
}

func (service *MCPService) Collection(ctx context.Context, key string) (MagnetCollectionDetail, error) {
	key, err := collectionKey(key)
	if err != nil {
		return MagnetCollectionDetail{}, err
	}
	payload, err := service.invoke(ctx, "favorite_collection_detail", map[string]any{"collection_key": key})
	if err != nil {
		return MagnetCollectionDetail{}, err
	}
	var result MagnetCollectionDetail
	if err := json.Unmarshal(payload, &result); err != nil {
		return MagnetCollectionDetail{}, fmt.Errorf("decode magnet collection: %w", err)
	}
	if result.Items == nil {
		result.Items = []MagnetCollectionItem{}
	}
	return result, nil
}

// Mutations answer with shapes the server does not document, and the caller
// re-reads the list afterwards, so only the failure matters here.
func (service *MCPService) CreateCollection(ctx context.Context, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return domain.E(domain.KindInvalid, "请输入合集名称", nil)
	}
	_, err := service.invoke(ctx, "favorite_collection_create", map[string]any{"label": label})
	return err
}

func (service *MCPService) RenameCollection(ctx context.Context, key, label string) error {
	key, err := collectionKey(key)
	if err != nil {
		return err
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return domain.E(domain.KindInvalid, "请输入合集名称", nil)
	}
	_, err = service.invoke(ctx, "favorite_collection_rename", map[string]any{"collection_key": key, "label": label})
	return err
}

func (service *MCPService) DeleteCollection(ctx context.Context, key string) error {
	key, err := collectionKey(key)
	if err != nil {
		return err
	}
	_, err = service.invoke(ctx, "favorite_collection_delete", map[string]any{"collection_key": key})
	return err
}

func (service *MCPService) EnableShare(ctx context.Context, key string) (MagnetShare, error) {
	key, err := collectionKey(key)
	if err != nil {
		return MagnetShare{}, err
	}
	payload, err := service.invoke(ctx, "favorite_share_enable", map[string]any{"collection_key": key})
	if err != nil {
		return MagnetShare{}, err
	}
	var share MagnetShare
	if err := json.Unmarshal(payload, &share); err != nil {
		return MagnetShare{}, fmt.Errorf("decode magnet share: %w", err)
	}
	return share, nil
}

func (service *MCPService) DisableShare(ctx context.Context, key string) error {
	key, err := collectionKey(key)
	if err != nil {
		return err
	}
	_, err = service.invoke(ctx, "favorite_share_disable", map[string]any{"collection_key": key})
	return err
}

func (service *MCPService) ShareDetail(ctx context.Context, code string) (MagnetShareDetail, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return MagnetShareDetail{}, domain.E(domain.KindInvalid, "请输入分享码", nil)
	}
	payload, err := service.invoke(ctx, "favorite_share_detail", map[string]any{"code": code})
	if err != nil {
		return MagnetShareDetail{}, err
	}
	var result MagnetShareDetail
	if err := json.Unmarshal(payload, &result); err != nil {
		return MagnetShareDetail{}, fmt.Errorf("decode magnet share detail: %w", err)
	}
	if result.Items == nil {
		result.Items = []MagnetCollectionItem{}
	}
	return result, nil
}

func (service *MCPService) ImportShare(ctx context.Context, code, label string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return domain.E(domain.KindInvalid, "请输入分享码", nil)
	}
	arguments := map[string]any{"code": code}
	if label = strings.TrimSpace(label); label != "" {
		arguments["label"] = label
	}
	_, err := service.invoke(ctx, "favorite_share_import", arguments)
	return err
}

func (service *MCPService) invoke(ctx context.Context, tool string, arguments any) (json.RawMessage, error) {
	client, err := service.session()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, mcpCallTimeout)
	defer cancel()
	return client.Call(ctx, tool, arguments)
}

// session returns the shared client, building it on first use. The magnet
// server answers domestic traffic directly, so it does not take the upstream
// proxy the JavDB and JavBus clients use.
func (service *MCPService) session() (*mcp.Client, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.client != nil {
		return service.client, nil
	}
	if strings.TrimSpace(service.settings.Token) == "" {
		return nil, ErrMagnetTokenRequired
	}
	client, err := mcp.New(service.settings.URL, service.settings.Token, &http.Client{Timeout: mcpCallTimeout})
	if err != nil {
		return nil, err
	}
	service.client = client
	return client, nil
}

// normalizeMCPSettings trims the saved values and fills in the default server.
func normalizeMCPSettings(settings MCPSettings) MCPSettings {
	settings.URL = strings.TrimSpace(settings.URL)
	if settings.URL == "" {
		settings.URL = DefaultMCPServer
	}
	settings.Token = strings.TrimSpace(settings.Token)
	return settings
}

// magnetLink accepts a magnet URI or a bare info hash, the way the magnet
// server's own web client does.
func magnetLink(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", domain.E(domain.KindInvalid, "缺少磁力链接", nil)
	}
	if match := magnetHash.FindStringSubmatch(trimmed); match != nil {
		return "magnet:?xt=urn:btih:" + strings.ToUpper(match[1]), nil
	}
	if len(trimmed) == 40 && isHex(trimmed) {
		return "magnet:?xt=urn:btih:" + strings.ToUpper(trimmed), nil
	}
	return trimmed, nil
}

func isHex(value string) bool {
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9',
			character >= 'a' && character <= 'f',
			character >= 'A' && character <= 'F':
		default:
			return false
		}
	}
	return true
}

func collectionKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", domain.E(domain.KindInvalid, "缺少合集标识", nil)
	}
	return key, nil
}
