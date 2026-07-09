package bff

import (
	"context"
	"strings"

	"github.com/byte-v-forge/wa-app/internal/waapp/shared"
	"github.com/byte-v-forge/wa-app/internal/waapp/wacore"
)

const (
	waProxySourceSystemCommon = "SYSTEM_COMMON"
	waProxySourceDirect       = "DIRECT"

	waProxyModeDirect = "DIRECT"
	waProxyModeCommon = "COMMON_PROXY"
)

type waProxyResolveRequest struct {
	Payload     map[string]any
	CountryCode string
}

// resolveWAProxyRoute resolves the egress route for a WA registration/probe
// request: a per-account cliproxy sticky route (WA_CLIPROXY_GATEWAY) when
// configured — one stable exit IP per account across exist/code/register —
// else the shared WA_COMMON_PROXY, else a direct connection. This is the plain
// deterministic path (no exit pre-check); probes use it.
func (g *actionGateway) resolveWAProxyRoute(req waProxyResolveRequest) (wacore.WAProxyRoute, bool) {
	countryCode := proxyRouteCountryCode(req)
	route, ok := g.resolveCliproxyRoute(countryCode, phoneE164FromPayload(req.Payload))
	return g.finishWAProxyRoute(countryCode, route, ok)
}

// resolveWAProxyRoutePrechecked mirrors resolveWAProxyRoute but pre-flights the
// cliproxy exit (rotating the sticky sid until a non-datacenter exit is found and
// pinning it) so the registration uses a good, quality-verified exit. Common
// proxy / direct fallbacks are identical.
func (g *actionGateway) resolveWAProxyRoutePrechecked(ctx context.Context, req waProxyResolveRequest) (wacore.WAProxyRoute, bool) {
	countryCode := proxyRouteCountryCode(req)
	route, ok := g.resolveCliproxyRoutePrechecked(ctx, countryCode, phoneE164FromPayload(req.Payload))
	return g.finishWAProxyRoute(countryCode, route, ok)
}

func proxyRouteCountryCode(req waProxyResolveRequest) string {
	return normalizeProxyCountryCode(shared.FirstNonEmpty(req.CountryCode, proxyCountryCodeFromPayload(req.Payload)))
}

// finishWAProxyRoute applies the shared fallback chain once the cliproxy
// candidate has been resolved: use it when present, else the common proxy, else
// direct.
func (g *actionGateway) finishWAProxyRoute(countryCode string, cliproxyRoute wacore.WAProxyRoute, cliproxyOK bool) (wacore.WAProxyRoute, bool) {
	if cliproxyOK {
		return cliproxyRoute, true
	}
	if route, ok := g.resolveSystemCommonProxyRoute(countryCode); ok {
		return route, true
	}
	return wacore.WAProxyRoute{ProxyMode: waProxyModeDirect, CountryCode: "LOCAL", Source: waProxySourceDirect, PolicyMode: waProxyModeDirect}, false
}

func (g *actionGateway) resolveSystemCommonProxyRoute(countryCode string) (wacore.WAProxyRoute, bool) {
	commonProxyURL := ""
	if g != nil && g.server != nil {
		commonProxyURL = g.server.CommonProxyURL()
	}
	if strings.TrimSpace(commonProxyURL) == "" {
		return wacore.WAProxyRoute{}, false
	}
	route := staticProxyRoute("common", commonProxyURL, staticCommonProxyMode)
	route.CountryCode = countryCode
	route.Source = waProxySourceSystemCommon
	route.PolicyMode = waProxyModeCommon
	return route, true
}

func waProxySummary(route wacore.WAProxyRoute, useProxy bool) map[string]any {
	if !useProxy {
		return map[string]any{"success": true, "accepted": true, "proxy_mode": waProxyModeDirect, "country_code": "LOCAL", "source": waProxySourceDirect}
	}
	result := map[string]any{
		"success":      true,
		"accepted":     true,
		"proxy_mode":   shared.FirstNonEmpty(route.ProxyMode, "PROXY"),
		"country_code": shared.FirstNonEmpty(route.CountryCode, "UNKNOWN"),
	}
	if route.Source != "" {
		result["source"] = route.Source
	}
	if route.PolicyMode != "" {
		result["policy_mode"] = route.PolicyMode
	}
	if route.AccountID != "" {
		result["account_id"] = route.AccountID
	}
	if route.RouteID != "" {
		result["route_id"] = route.RouteID
	}
	return result
}
