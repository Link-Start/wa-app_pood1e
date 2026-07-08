package bff

import (
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
// else the shared WA_COMMON_PROXY, else a direct connection.
func (g *actionGateway) resolveWAProxyRoute(req waProxyResolveRequest) (wacore.WAProxyRoute, bool) {
	countryCode := normalizeProxyCountryCode(shared.FirstNonEmpty(req.CountryCode, proxyCountryCodeFromPayload(req.Payload)))
	if route, ok := g.resolveCliproxyRoute(countryCode, phoneE164FromPayload(req.Payload)); ok {
		return route, true
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
