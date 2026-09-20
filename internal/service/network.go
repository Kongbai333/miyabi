package service

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/ppxb/miyabi/internal/ent"
	"github.com/ppxb/miyabi/internal/javdb"
	"github.com/ppxb/miyabi/internal/netx"
)

const networkProxySetting = "network.proxy"

// NetworkProbeResult reports the test status and latency of an upstream target.
type NetworkProbeResult struct {
	Available bool   `json:"available"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NetworkTestResponse contains probe results for both JavDB and JavBus.
type NetworkTestResponse struct {
	JavDB  NetworkProbeResult `json:"javdb"`
	JavBus NetworkProbeResult `json:"javbus"`
}

// NetworkService owns the persisted upstream network configuration. The
// manager is shared with clients so a settings update takes effect in-process.
type NetworkService struct {
	database *ent.Client
	proxy    *netx.ProxyManager
}

func NewNetworkService(ctx context.Context, database *ent.Client) (*NetworkService, error) {
	config, found, err := loadSetting[netx.ProxyConfig](ctx, database, networkProxySetting)
	if err != nil {
		return nil, err
	}
	if !found {
		config = netx.ProxyConfig{}
	}
	proxy, err := netx.NewProxyManager(config)
	if err != nil {
		return nil, fmt.Errorf("load network proxy setting: %w", err)
	}
	return &NetworkService{database: database, proxy: proxy}, nil
}

func (service *NetworkService) ProxyManager() *netx.ProxyManager {
	return service.proxy
}

func (service *NetworkService) Network(context.Context) (netx.ProxyConfig, error) {
	return service.proxy.Config(), nil
}

func (service *NetworkService) UpdateNetwork(ctx context.Context, config netx.ProxyConfig) error {
	current := service.proxy.Config()
	config.URL = netx.RestoreMaskedPassword(config.URL, current.URL)
	normalized, err := netx.Normalize(config)
	if err != nil {
		return err
	}
	if err := saveSetting(ctx, service.database, networkProxySetting, normalized); err != nil {
		return err
	}
	return service.proxy.Update(normalized)
}

// TestNetwork probes JavDB and JavBus concurrently with candidate or active settings.
func (service *NetworkService) TestNetwork(ctx context.Context, config netx.ProxyConfig) (NetworkTestResponse, error) {
	current := service.proxy.Config()
	config.URL = netx.RestoreMaskedPassword(config.URL, current.URL)
	normalized, err := netx.Normalize(config)
	if err != nil {
		return NetworkTestResponse{}, err
	}

	var proxyURL *url.URL
	if normalized.Enabled && normalized.URL != "" {
		parsed, err := url.Parse(normalized.URL)
		if err == nil {
			proxyURL = parsed
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var resp NetworkTestResponse
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		dur, err := javdb.Probe(probeCtx, proxyURL)
		if err != nil {
			resp.JavDB = NetworkProbeResult{Available: false, Error: err.Error()}
		} else {
			resp.JavDB = NetworkProbeResult{Available: true, LatencyMS: dur.Milliseconds()}
		}
	}()

	go func() {
		defer wg.Done()
		dur, err := probeJavBus(probeCtx, proxyURL)
		if err != nil {
			resp.JavBus = NetworkProbeResult{Available: false, Error: err.Error()}
		} else {
			resp.JavBus = NetworkProbeResult{Available: true, LatencyMS: dur.Milliseconds()}
		}
	}()

	wg.Wait()
	return resp, nil
}

func probeJavBus(ctx context.Context, proxy *url.URL) (time.Duration, error) {
	clientOptions := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(8),
		tlsclient.WithClientProfile(profiles.Chrome_120),
		tlsclient.WithNotFollowRedirects(),
	}
	if proxy != nil {
		clientOptions = append(clientOptions, tlsclient.WithProxyUrl(proxy.String()))
	}
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), clientOptions...)
	if err != nil {
		return 0, err
	}
	defer client.CloseIdleConnections()

	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.javbus.com", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", "dv=1")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 512)

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return time.Since(started), nil
	}
	return 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
}
