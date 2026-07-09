package engine

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewProxyHTTPClient builds a standard *http.Client whose requests egress through
// the given proxy URL (http/https/socks5, with optional embedded credentials).
// It reuses parseOutboundProxyURL so proxy parsing stays consistent, and relies
// on the stdlib's native http.ProxyURL support rather than hand-rolling a dialer.
// This is for simple auxiliary calls (e.g. the cliproxy exit pre-check), NOT the
// WA registration protocol, which uses the TLS-fingerprinted native client.
func NewProxyHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	parsed, err := parseOutboundProxyURL(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, fmt.Errorf("proxy URL is required")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	transport := &http.Transport{
		Proxy:               http.ProxyURL(parsed),
		ForceAttemptHTTP2:   false,
		MaxIdleConns:        1,
		IdleConnTimeout:     timeout,
		TLSHandshakeTimeout: timeout,
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

func (e *engineCore) httpForProxy() (*nativeHTTPClient, error) {
	if _, err := e.proxyURL(); err != nil {
		return nil, err
	}
	return e.http, nil
}

func (e *engineCore) proxyURL() (string, error) {
	if proxyURL := strings.TrimSpace(e.activeProxyURL); proxyURL != "" {
		return normalizeProxyURLString(proxyURL)
	}
	return "", nil
}

func normalizeProxyURLString(value string) (string, error) {
	parsed, err := parseOutboundProxyURL(value)
	if err != nil || parsed == nil {
		return "", err
	}
	return parsed.String(), nil
}

func parseOutboundProxyURL(value string) (*url.URL, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if !strings.Contains(value, "://") {
		value = "socks5://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL")
	}
	parsed.Scheme = normalizeProxyScheme(parsed.Scheme)
	if parsed.Scheme == "" {
		return nil, fmt.Errorf("invalid proxy URL scheme")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, fmt.Errorf("proxy host is required")
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func normalizeProxyScheme(scheme string) string {
	scheme = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(scheme)), "://")
	switch scheme {
	case "http", "https", "socks5", "socks5h":
		return scheme
	case "socks", "socks5-proxy":
		return "socks5"
	default:
		return ""
	}
}
