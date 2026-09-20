package netx

import (
	"net/http"
	"testing"
	"time"
)

func TestRestyClientResolvesProxyPerRequest(t *testing.T) {
	manager, err := NewProxyManager(ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	client := NewRestyClient(manager, RestyOptions{Timeout: time.Second, ResponseHeaderTimeout: 2 * time.Second})
	transport := client.GetClient().Transport.(*http.Transport)
	if transport.ResponseHeaderTimeout != 2*time.Second || client.GetClient().Timeout != time.Second {
		t.Fatalf("timeouts were not applied: %+v", transport)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if proxy, _ := transport.Proxy(request); proxy != nil {
		t.Fatalf("direct client resolved proxy %v", proxy)
	}
	if err := manager.Update(ProxyConfig{Enabled: true, URL: "socks5://127.0.0.1:1080"}); err != nil {
		t.Fatal(err)
	}
	if proxy, _ := transport.Proxy(request); proxy == nil || proxy.Scheme != "socks5" {
		t.Fatalf("updated proxy was not picked up: %v", proxy)
	}
}

func TestDirectRestyClientIgnoresManager(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("HTTP_PROXY", "")
	client := NewDirectRestyClient(RestyOptions{})
	transport := client.GetClient().Transport.(*http.Transport)
	request, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if proxy, _ := transport.Proxy(request); proxy != nil {
		t.Fatalf("direct client resolved proxy %v", proxy)
	}
}

func TestFingerprintClientBuildsWithAndWithoutProxy(t *testing.T) {
	_, proxy, err := Normalize(ProxyConfig{Enabled: true, URL: "http://127.0.0.1:7890"})
	if err != nil {
		t.Fatal(err)
	}
	for _, options := range []FingerprintOptions{{Timeout: time.Second}, {Timeout: time.Second, Proxy: proxy, CookieJar: true}} {
		client, err := NewFingerprintClient(options)
		if err != nil {
			t.Fatalf("NewFingerprintClient(%+v): %v", options, err)
		}
		client.CloseIdleConnections()
	}
}
