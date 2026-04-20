// Origin validation for CORS/browser security.
// Ported from Deneb auth/origin.go (a8f633aff3).
package middleware

import (
	"net"
	"net/url"
	"strings"
)

type OriginCheckResult struct {
	OK        bool   `json:"ok"`
	MatchedBy string `json:"matchedBy,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func CheckBrowserOrigin(requestHost, origin string, allowedOrigins []string, isLocalClient bool) OriginCheckResult {
	parsed := parseOrigin(origin)
	if parsed == nil {
		return OriginCheckResult{OK: false, Reason: "origin missing or invalid"}
	}

	allowlist := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(strings.ToLower(o))
		if o != "" {
			allowlist[o] = true
		}
	}
	if allowlist["*"] || allowlist[parsed.origin] {
		return OriginCheckResult{OK: true, MatchedBy: "allowlist"}
	}

	normalizedHost := normalizeHostHeader(requestHost)
	if normalizedHost != "" && parsed.host == normalizedHost {
		return OriginCheckResult{OK: true, MatchedBy: "host-header-fallback"}
	}

	if isLocalClient && isLoopbackHost(parsed.hostname) {
		return OriginCheckResult{OK: true, MatchedBy: "local-loopback"}
	}

	return OriginCheckResult{OK: false, Reason: "origin not allowed"}
}

type parsedOrigin struct {
	origin   string
	host     string
	hostname string
}

func parseOrigin(raw string) *parsedOrigin {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return nil
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	hostname := strings.ToLower(u.Hostname())
	return &parsedOrigin{origin: scheme + "://" + host, host: host, hostname: hostname}
}

func normalizeHostHeader(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if strings.HasSuffix(host, ":80") || strings.HasSuffix(host, ":443") {
		h, _, err := net.SplitHostPort(host)
		if err == nil {
			return h
		}
	}
	return host
}

func isLoopbackHost(hostname string) bool {
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" || hostname == "[::1]" {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}
