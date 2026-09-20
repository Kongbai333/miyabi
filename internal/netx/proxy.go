package netx

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

// ErrInvalidProxy wraps every configuration validation failure so callers can
// map it to a client error without inspecting the message.
var ErrInvalidProxy = errors.New("代理配置无效")

// ProxyConfig is the process-wide upstream proxy configuration. A disabled
// proxy keeps its URL so it can be enabled again without re-entering it.
type ProxyConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

type proxyState struct {
	config ProxyConfig
	proxy  *url.URL
}

// ProxyManager provides a concurrency-safe proxy configuration and a small
// notification channel for clients that need to rebuild transports.
type ProxyManager struct {
	current atomic.Pointer[proxyState]

	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

// NewProxyManager validates the initial configuration before publishing it.
func NewProxyManager(initial ProxyConfig) (*ProxyManager, error) {
	state, err := makeProxyState(initial)
	if err != nil {
		return nil, err
	}
	manager := &ProxyManager{subs: make(map[chan struct{}]struct{})}
	manager.current.Store(state)
	return manager, nil
}

// Normalize validates a configuration and returns the value stored internally
// together with the proxy URL that would be used, or nil when direct.
func Normalize(config ProxyConfig) (ProxyConfig, *url.URL, error) {
	state, err := makeProxyState(config)
	if err != nil {
		return ProxyConfig{}, nil, err
	}
	return state.config, state.resolve(), nil
}

// Config returns the current configuration by value.
func (manager *ProxyManager) Config() ProxyConfig {
	return manager.current.Load().config
}

// Resolve returns a copy of the active proxy URL, or nil when the proxy is
// disabled. The returned URL can be modified by the caller safely.
func (manager *ProxyManager) Resolve() *url.URL {
	return manager.current.Load().resolve()
}

// Update validates and publishes a new configuration. Subscribers are
// notified after the new value becomes visible; a full channel is coalesced
// into the notification already waiting for the subscriber.
func (manager *ProxyManager) Update(next ProxyConfig) error {
	state, err := makeProxyState(next)
	if err != nil {
		return err
	}
	manager.current.Store(state)

	manager.mu.Lock()
	defer manager.mu.Unlock()
	for subscriber := range manager.subs {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
	return nil
}

// Subscribe returns a buffered channel that receives a signal after each
// successful update. Notifications are deliberately coalesced.
func (manager *ProxyManager) Subscribe() <-chan struct{} {
	subscriber := make(chan struct{}, 1)
	manager.mu.Lock()
	manager.subs[subscriber] = struct{}{}
	manager.mu.Unlock()
	return subscriber
}

// Unsubscribe removes a previously returned subscription channel.
func (manager *ProxyManager) Unsubscribe(subscription <-chan struct{}) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	for candidate := range manager.subs {
		if candidate == subscription {
			delete(manager.subs, candidate)
			return
		}
	}
}

func (state *proxyState) resolve() *url.URL {
	if !state.config.Enabled || state.proxy == nil {
		return nil
	}
	proxy := *state.proxy
	return &proxy
}

func makeProxyState(config ProxyConfig) (*proxyState, error) {
	config.URL = strings.TrimSpace(config.URL)
	proxy, err := parseProxyURL(config.URL)
	if err != nil {
		return nil, err
	}
	return &proxyState{config: config, proxy: proxy}, nil
}

func parseProxyURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	proxy, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: 代理地址格式错误", ErrInvalidProxy)
	}
	if proxy.Scheme == "" || proxy.Hostname() == "" {
		return nil, fmt.Errorf("%w: 代理地址必须包含协议（如 http://）与主机地址", ErrInvalidProxy)
	}
	proxy.Scheme = strings.ToLower(proxy.Scheme)
	switch proxy.Scheme {
	case "http", "https", "socks5":
		return proxy, nil
	}
	return nil, fmt.Errorf("%w: 代理协议仅支持 http://、https:// 或 socks5://", ErrInvalidProxy)
}
