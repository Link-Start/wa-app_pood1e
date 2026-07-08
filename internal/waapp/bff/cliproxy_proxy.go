package bff

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"

	"github.com/byte-v-forge/wa-app/internal/waapp/rpc"
	"github.com/byte-v-forge/wa-app/internal/waapp/shared"
	"github.com/byte-v-forge/wa-app/internal/waapp/wacore"
	"github.com/byte-v-forge/wa-app/internal/waapp/wamodel"
)

const (
	waProxySourceCliproxy   = "CLIPROXY"
	waProxyModeCliproxy     = "CLIPROXY_STICKY"
	waProxyPolicyModeSticky = "STICKY"
	cliproxyDefaultTTLMin   = 20
	cliproxySidLen          = 8 // cliproxy sticky sid is the first 8 chars (matches the reference impl)
)

// phoneE164FromPayload extracts the normalized E.164 number from an action
// payload; it seeds the stable per-account cliproxy sticky session.
func phoneE164FromPayload(payload map[string]any) string {
	return strings.TrimSpace(wamodel.NormalizePhone(phoneFromAction(payload)).GetE164Number())
}

// cliproxySessionID derives a stable, opaque session key for a phone. The raw
// phone number is never exposed (hashed); an optional salt rotates every
// account's IP at once. Only the first cliproxySidLen chars are used as the sid.
func cliproxySessionID(salt, e164 string) string {
	sum := sha256.Sum256([]byte(salt + "|" + e164))
	return hex.EncodeToString(sum[:])
}

func (g *actionGateway) resolveCliproxyRoute(countryCode, e164 string) (wacore.WAProxyRoute, bool) {
	if g == nil || g.server == nil {
		return wacore.WAProxyRoute{}, false
	}
	return buildCliproxyRoute(g.server.CliproxySettings(), countryCode, e164)
}

// buildCliproxyRoute builds the per-account sticky cliproxy route. The username
// follows cliproxy's verified sticky syntax
// "<base>-region-<cc>-sid-<sid8>-t-<ttl>" (region omitted when unknown/disabled).
// The same account (phone) always maps to the same sid, so its whole
// registration (exist -> code -> register) exits through one sticky IP, and
// different accounts get different IPs. Server-free so it is unit-testable; the
// credential-bearing ProxyURL is never logged (callers log only the opaque sid
// via AccountID/RouteID).
func buildCliproxyRoute(s rpc.CliproxySettings, countryCode, e164 string) (wacore.WAProxyRoute, bool) {
	endpoint := strings.TrimSpace(s.Endpoint)
	base := strings.TrimSpace(s.Username)
	if endpoint == "" || base == "" || strings.TrimSpace(s.Password) == "" || strings.TrimSpace(e164) == "" {
		return wacore.WAProxyRoute{}, false
	}
	ttl := s.TTLMinutes
	if ttl < 1 {
		ttl = cliproxyDefaultTTLMin
	}
	sid := cliproxySessionID(s.SessionSalt, e164)[:cliproxySidLen]

	username := base
	if region := cliproxyRegion(s.Region, countryCode); region != "" {
		username += "-region-" + region
	}
	username += "-sid-" + sid + "-t-" + strconv.Itoa(ttl)

	proxyURL := (&url.URL{Scheme: "http", User: url.UserPassword(username, s.Password), Host: endpoint}).String()
	return wacore.WAProxyRoute{
		ProxyURL:    proxyURL,
		ProxyMode:   waProxyModeCliproxy,
		CountryCode: shared.FirstNonEmpty(countryCode, "UNKNOWN"),
		Source:      waProxySourceCliproxy,
		PolicyMode:  waProxyPolicyModeSticky,
		AccountID:   sid,
		RouteID:     sid,
	}, true
}

// cliproxyRegion resolves the cliproxy region token in the username: "NONE"
// disables it; "" / "AUTO" derives it from the phone's country code (so the exit
// IP country matches the number); an explicit value forces that region.
func cliproxyRegion(override, countryCode string) string {
	switch up := strings.ToUpper(strings.TrimSpace(override)); up {
	case "NONE":
		return ""
	case "", "AUTO":
		cc := strings.ToUpper(strings.TrimSpace(countryCode))
		if isAlpha2(cc) {
			return cc
		}
		return ""
	default:
		return up
	}
}

func isAlpha2(s string) bool {
	if len(s) != 2 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
