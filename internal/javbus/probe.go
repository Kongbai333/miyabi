package javbus

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/ppxb/miyabi/internal/netx"
)

const (
	baseURL   = "https://www.javbus.com"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// Probe measures reachability of the JavBus home page through the given
// proxy, or directly when proxy is nil.
func Probe(ctx context.Context, proxy *url.URL, timeout time.Duration) (time.Duration, error) {
	client, err := netx.NewFingerprintClient(netx.FingerprintOptions{Timeout: timeout, Proxy: proxy})
	if err != nil {
		return 0, err
	}
	defer client.CloseIdleConnections()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("User-Agent", userAgent)
	// The age gate is a cookie check; without it the home page redirects.
	request.Header.Set("Cookie", "dv=1")

	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, 512)
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return 0, fmt.Errorf("JavBus returned HTTP %d", response.StatusCode)
	}
	return time.Since(started), nil
}
