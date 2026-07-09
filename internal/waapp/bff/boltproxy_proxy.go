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
	waProxySourceBoltproxy  = "BOLTPROXY"
	waProxyModeBoltproxy    = "BOLTPROXY_STICKY"
	waProxyPolicyModeSticky = "STICKY"
	boltproxyDefaultTTLMin  = 20
	boltproxySidLen         = 8 // sticky session token = first 8 hex chars of the per-phone hash
)

// phoneE164FromPayload extracts the normalized E.164 number from an action
// payload; it seeds the stable per-account boltproxy sticky session.
func phoneE164FromPayload(payload map[string]any) string {
	return strings.TrimSpace(wamodel.NormalizePhone(phoneFromAction(payload)).GetE164Number())
}

// boltproxySessionID derives a stable, opaque session key for a phone. The raw
// phone number is never exposed (hashed); an optional salt rotates every
// account's IP at once. Only the first boltproxySidLen chars are used as the sid.
func boltproxySessionID(salt, e164 string) string {
	sum := sha256.Sum256([]byte(salt + "|" + e164))
	return hex.EncodeToString(sum[:])
}

// boltproxySidForAttempt derives the sticky sid for a rotation attempt. Attempt 0
// keeps the phone's stable deterministic sid (boltproxySessionID(salt, e164)) so a
// good first exit preserves the per-phone sticky IP; each later attempt varies the
// seed ("<e164>#<attempt>") so boltproxy hands out a DIFFERENT exit.
func boltproxySidForAttempt(salt, e164 string, attempt int) string {
	seed := e164
	if attempt > 0 {
		seed = e164 + "#" + strconv.Itoa(attempt)
	}
	return boltproxySessionID(salt, seed)[:boltproxySidLen]
}

func (g *actionGateway) resolveBoltproxyRoute(countryCode, e164 string) (wacore.WAProxyRoute, bool) {
	if g == nil || g.server == nil {
		return wacore.WAProxyRoute{}, false
	}
	return buildBoltproxyRoute(g.server.BoltproxySettings(), countryCode, e164)
}

// buildBoltproxyRoute builds the per-account sticky boltproxy route. The username
// follows boltproxy's sticky syntax
// "<base>-zone-res-country-<cc>-session-<sid8>-sessTime-<ttl>" (country omitted
// when unknown/disabled; zone-res selects the residential pool).
// The same account (phone) always maps to the same sid, so its whole
// registration (exist -> code -> register) exits through one sticky IP, and
// different accounts get different IPs. Server-free so it is unit-testable; the
// credential-bearing ProxyURL is never logged (callers log only the opaque sid
// via AccountID/RouteID).
func buildBoltproxyRoute(s rpc.BoltproxySettings, countryCode, e164 string) (wacore.WAProxyRoute, bool) {
	return buildBoltproxyRouteForAttempt(s, countryCode, e164, 0)
}

// buildBoltproxyRouteForAttempt builds the sticky route for a specific rotation
// attempt; attempt 0 is the deterministic per-phone route buildBoltproxyRoute
// returns. Only the sid varies across attempts — the region (and thus the exit
// country) stays fixed so the exit still matches the number's country.
func buildBoltproxyRouteForAttempt(s rpc.BoltproxySettings, countryCode, e164 string, attempt int) (wacore.WAProxyRoute, bool) {
	if strings.TrimSpace(e164) == "" {
		return wacore.WAProxyRoute{}, false
	}
	return buildBoltproxyRouteFromSid(s, countryCode, boltproxySidForAttempt(s.SessionSalt, e164, attempt))
}

// buildBoltproxyRouteFromSid builds the sticky route for an explicit sid. It is the
// shared core of buildBoltproxyRouteForAttempt and of the probe→registration exit
// pin path, which reuses the exact sid the probe already validated instead of
// re-deriving it from an attempt. The region (exit country) still comes from
// countryCode, so a pinned sid keeps the number's country.
func buildBoltproxyRouteFromSid(s rpc.BoltproxySettings, countryCode, sid string) (wacore.WAProxyRoute, bool) {
	endpoint := strings.TrimSpace(s.Endpoint)
	base := strings.TrimSpace(s.Username)
	sid = strings.TrimSpace(sid)
	if endpoint == "" || base == "" || strings.TrimSpace(s.Password) == "" || sid == "" {
		return wacore.WAProxyRoute{}, false
	}
	ttl := s.TTLMinutes
	if ttl < 1 {
		ttl = boltproxyDefaultTTLMin
	}
	username := base + "-zone-res"
	if country := boltproxyCountry(s.Region, countryCode); country != "" {
		username += "-country-" + country
	}
	username += "-session-" + sid + "-sessTime-" + strconv.Itoa(ttl)

	proxyURL := (&url.URL{Scheme: "http", User: url.UserPassword(username, s.Password), Host: endpoint}).String()
	return wacore.WAProxyRoute{
		ProxyURL:    proxyURL,
		ProxyMode:   waProxyModeBoltproxy,
		CountryCode: shared.FirstNonEmpty(countryCode, "UNKNOWN"),
		Source:      waProxySourceBoltproxy,
		PolicyMode:  waProxyPolicyModeSticky,
		AccountID:   sid,
		RouteID:     sid,
	}, true
}

// egressPinFromPayload reads the opaque exit pin the number probe returned to the
// caller (proxy.egress_pin, mirrored as a top-level egress_pin). The caller holds
// it transiently in the frontend and echoes it back on register so the whole
// registration reuses the exact boltproxy exit the probe already validated — no
// backend session storage. It is opaque: the caller never constructs it, and a
// stale/garbage value is simply re-checked and rotated away from.
func egressPinFromPayload(payload map[string]any) string {
	return strings.TrimSpace(shared.FirstNonEmpty(
		shared.TextField(payload, "egress_pin"),
		shared.TextField(shared.ObjectField(payload, "proxy"), "egress_pin"),
	))
}

// boltproxyFallbackCountry is used when the resolved country is one boltproxy has
// no exit pool for (see boltproxyUnsupportedCountries): rather than emit a country
// token boltproxy rejects, fall back to a country it does serve.
const boltproxyFallbackCountry = "US"

// boltproxyUnsupportedCountries lists countries boltproxy cannot provide an exit
// in, so an AUTO/explicit selection for them is substituted with the US fallback.
// Extend as more unsupported countries are found.
var boltproxyUnsupportedCountries = map[string]struct{}{
	"CN": {}, // boltproxy has no China residential exits
}

// boltproxySupportedCountry maps a resolved 2-letter country to one boltproxy can
// serve, substituting the US fallback for unsupported countries.
func boltproxySupportedCountry(cc string) string {
	if _, unsupported := boltproxyUnsupportedCountries[cc]; unsupported {
		return boltproxyFallbackCountry
	}
	return cc
}

// boltproxyCountry resolves the lowercase country token in the username: "NONE"
// disables it; "" / "AUTO" derives it from the phone's country code (so the exit
// IP country matches the number); an explicit value forces that country. Countries
// boltproxy can't serve (e.g. CN) fall back to US so the request still succeeds.
func boltproxyCountry(override, countryCode string) string {
	switch up := strings.ToUpper(strings.TrimSpace(override)); up {
	case "NONE":
		return ""
	case "", "AUTO":
		cc := strings.ToUpper(strings.TrimSpace(countryCode))
		if isAlpha2(cc) {
			return strings.ToLower(boltproxySupportedCountry(cc))
		}
		return ""
	default:
		return strings.ToLower(boltproxySupportedCountry(up))
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
