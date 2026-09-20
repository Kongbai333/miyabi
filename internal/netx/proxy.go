package netx

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

var errUnsupportedProxyScheme = errors.New("proxy URL scheme must be http, https or socks5")

// MaskedPassword is the replacement placeholder displayed when proxy credentials are sent to clients.
const MaskedPassword = "******"

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

// Validate checks a configuration without creating a long-lived manager.
func Validate(config ProxyConfig) error {
	_, err := makeProxyState(config)
	return err
}

// Normalize validates a configuration and returns the value used internally.
func Normalize(config ProxyConfig) (ProxyConfig, error) {
	state, err := makeProxyState(config)
	if err != nil {
		return ProxyConfig{}, err
	}
	return state.config, nil
}

// Config returns the current configuration by value.
func (manager *ProxyManager) Config() ProxyConfig {
	return manager.current.Load().config
}

// Resolve returns a copy of the active proxy URL, or nil when the proxy is
// disabled. The returned URL can be modified by the caller safely.
func (manager *ProxyManager) Resolve() *url.URL {
	state := manager.current.Load()
	if !state.config.Enabled || state.proxy == nil {
		return nil
	}
	proxy := *state.proxy
	return &proxy
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
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}
	if proxy.Scheme == "" || proxy.Host == "" || proxy.Hostname() == "" {
		return nil, errors.New("proxy URL must include a scheme and host")
	}
	switch strings.ToLower(proxy.Scheme) {
	case "http", "https", "socks5":
		proxy.Scheme = strings.ToLower(proxy.Scheme)
	default:
		return nil, errUnsupportedProxyScheme
	}
	return proxy, nil
}

// RestoreMaskedPassword checks whether the candidate proxy URL contains the
// masked placeholder password ("******"). If so, and if the scheme, host, and
// username match the currently stored proxy URL, it restores the real password
// from the existing URL so that the credentials are not destroyed.
func RestoreMaskedPassword(candidate string, existing string) string {
	if candidate == "" || existing == "" {
		return candidate
	}
	candURL, err := url.Parse(candidate)
	if err != nil || candURL.User == nil {
		return candidate
	}
	pass, hasPass := candURL.User.Password()
	if !hasPass || pass != MaskedPassword {
		return candidate
	}
	existURL, err := url.Parse(existing)
	if err != nil || existURL.User == nil {
		return candidate
	}
	existPass, existHasPass := existURL.User.Password()
	if !existHasPass {
		return candidate
	}
	if strings.EqualFold(candURL.Scheme, existURL.Scheme) &&
		strings.EqualFold(candURL.Host, existURL.Host) &&
		candURL.User.Username() == existURL.User.Username() {
		candURL.User = url.UserPassword(candURL.User.Username(), existPass)
		return candURL.String()
	}
	return candidate
}
