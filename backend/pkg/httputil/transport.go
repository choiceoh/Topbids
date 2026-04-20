// Package httputil provides shared HTTP transports and client factories.
// Ported from Deneb gateway.
package httputil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

var version string

func SetVersion(v string) { version = v }

var sharedTransport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:        64,
	MaxIdleConnsPerHost: 8,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 5 * time.Second,
	ForceAttemptHTTP2:   true,
}

var sharedRoundTripper http.RoundTripper = &uaTransport{base: sharedTransport}

type uaTransport struct{ base http.RoundTripper }

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		ua := "Topworks"
		if version != "" {
			ua += "/" + version
		}
		req.Header.Set("User-Agent", ua)
	}
	return t.base.RoundTrip(req)
}

func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: sharedRoundTripper}
}

func CloseIdle() {
	sharedTransport.CloseIdleConnections()
}

func WaitForHealth(ctx context.Context, url string, interval time.Duration) error {
	client := NewClient(3 * time.Second)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	probe := func() (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return false, fmt.Errorf("health check at %s: %w", url, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()
		return resp.StatusCode < 500, nil
	}

	if ok, err := probe(); err != nil {
		return err
	} else if ok {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check at %s: %w", url, ctx.Err())
		case <-ticker.C:
			if ok, err := probe(); err != nil {
				return err
			} else if ok {
				return nil
			}
		}
	}
}
