package netx

import (
	"errors"
	"testing"
	"time"
)

func TestNewProxyManagerDefaultsToDirect(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Resolve(); got != nil {
		t.Fatalf("Resolve() = %v, want nil", got)
	}
	if got := manager.Config(); got != (ProxyConfig{}) {
		t.Fatalf("Config() = %+v, want zero config", got)
	}
}

func TestProxyManagerValidatesAndNormalizesURL(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{Enabled: true, URL: " HTTP://user:pass@127.0.0.1:7890 "})
	if err != nil {
		t.Fatal(err)
	}
	proxy := manager.Resolve()
	if proxy == nil || proxy.Scheme != "http" || proxy.Host != "127.0.0.1:7890" || proxy.User.String() != "user:pass" {
		t.Fatalf("Resolve() = %v", proxy)
	}
	if got := manager.Config(); got.URL != "HTTP://user:pass@127.0.0.1:7890" || !got.Enabled {
		t.Fatalf("Config() = %+v", got)
	}
}

func TestProxyManagerAllowsDisabledURL(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{URL: "socks5://127.0.0.1:1080"})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Resolve(); got != nil {
		t.Fatalf("Resolve() = %v, want nil while disabled", got)
	}
	if err := manager.Update(ProxyConfig{Enabled: true, URL: "socks5://127.0.0.1:1080"}); err != nil {
		t.Fatal(err)
	}
	if got := manager.Resolve(); got == nil || got.Scheme != "socks5" {
		t.Fatalf("Resolve() after enabling = %v", got)
	}
}

func TestProxyManagerRejectsInvalidConfigurations(t *testing.T) {
	for _, test := range []struct {
		name   string
		config ProxyConfig
	}{
		{name: "missing scheme", config: ProxyConfig{URL: "127.0.0.1:7890"}},
		{name: "missing host", config: ProxyConfig{URL: "http://"}},
		{name: "unsupported scheme", config: ProxyConfig{URL: "ftp://127.0.0.1:21"}},
		{name: "malformed URL", config: ProxyConfig{URL: "http://[::1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewProxyManager(test.config); !errors.Is(err, ErrInvalidProxy) {
				t.Fatalf("NewProxyManager() error = %v, want ErrInvalidProxy", err)
			}
		})
	}
}

func TestProxyManagerEnabledWithoutURLDefaultsToDirect(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{Enabled: true, URL: ""})
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Resolve(); got != nil {
		t.Fatalf("Resolve() = %v, want nil for empty URL", got)
	}
}

func TestProxyManagerUpdateNotifiesSubscribers(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	subscriber := manager.Subscribe()
	if err := manager.Update(ProxyConfig{Enabled: true, URL: "https://127.0.0.1:7890"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscriber:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not notified")
	}
	if got := manager.Resolve(); got == nil || got.Scheme != "https" {
		t.Fatalf("Resolve() = %v after update", got)
	}
	select {
	case <-subscriber:
		t.Fatal("notification was not coalesced")
	default:
	}
	if err := manager.Update(ProxyConfig{Enabled: false, URL: "https://127.0.0.1:7890"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-subscriber:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not notified after disabling")
	}
}

func TestProxyManagerResolveReturnsCopy(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7890"})
	if err != nil {
		t.Fatal(err)
	}
	first := manager.Resolve()
	first.Host = "127.0.0.1:1"
	second := manager.Resolve()
	if second == nil || second.Host != "127.0.0.1:7890" {
		t.Fatalf("Resolve() was mutated through returned URL: %v", second)
	}
}

func TestNormalizeTrimsAndResolves(t *testing.T) {
	config, proxy, err := Normalize(ProxyConfig{Enabled: true, URL: " http://127.0.0.1:7890 "})
	if err != nil || config.URL != "http://127.0.0.1:7890" || proxy == nil || proxy.Host != "127.0.0.1:7890" {
		t.Fatalf("Normalize() = %+v, %v, %v", config, proxy, err)
	}
	if _, proxy, err := Normalize(ProxyConfig{URL: "http://127.0.0.1:7890"}); err != nil || proxy != nil {
		t.Fatalf("Normalize(disabled) proxy = %v, err = %v", proxy, err)
	}
}
