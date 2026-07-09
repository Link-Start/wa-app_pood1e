package bff

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/byte-v-forge/wa-app/internal/waapp/engine"
	"github.com/byte-v-forge/wa-app/internal/waapp/rpc"
	"github.com/byte-v-forge/wa-app/internal/waapp/shared"
	"github.com/byte-v-forge/wa-app/internal/waapp/wacore"
)

// The exit pre-check endpoint and classifier live together here so the intel
// source and the good/bad rule are adjusted in one place. Default endpoint is
// ip-api.com's free JSON API (no key), restricted to the fields we classify on.
const (
	boltproxyPrecheckDefaultEndpoint    = "http://ip-api.com/json/?fields=status,message,countryCode,isp,org,as,mobile,proxy,hosting,query"
	boltproxyPrecheckDefaultMaxAttempts = 5
	boltproxyPrecheckDefaultTimeoutSecs = 10
	boltproxyPrecheckResponseSizeLimit  = 1 << 16
	// boltproxyPinnedExitAttempt is the attempt sentinel logged when the pre-check
	// is re-verifying the exit a prior probe pinned (rather than a 0..N rotation
	// attempt), so the reuse is visible in logs without exposing the sid meaning.
	boltproxyPinnedExitAttempt = -1
)

// boltproxyExitMeta is the value-safe summary of a candidate exit: never the
// credential-bearing proxy URL, only the observable exit attributes.
type boltproxyExitMeta struct {
	Country string
	ISP     string
	Mobile  bool
	Hosting bool
}

// ipAPIResponse mirrors the ip-api.com JSON fields the pre-check requests.
type ipAPIResponse struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	CountryCode string `json:"countryCode"`
	ISP         string `json:"isp"`
	Org         string `json:"org"`
	AS          string `json:"as"`
	Mobile      bool   `json:"mobile"`
	Proxy       bool   `json:"proxy"`
	Hosting     bool   `json:"hosting"`
	Query       string `json:"query"`
}

// classifyBoltproxyExit turns a raw ip-api JSON body into (ok, meta). ok requires
// both connectivity (status == "success") and a NON-datacenter IP type
// (hosting == false, i.e. ISP / residential / mobile — the only types wanted).
// A proxy==true flag alone does NOT disqualify: a residential-proxy exit is still
// a residential IP. mobile==true is a good (mobile carrier) exit.
func classifyBoltproxyExit(body []byte) (bool, boltproxyExitMeta, error) {
	var resp ipAPIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, boltproxyExitMeta{}, err
	}
	meta := boltproxyExitMeta{Country: resp.CountryCode, ISP: resp.ISP, Mobile: resp.Mobile, Hosting: resp.Hosting}
	if resp.Status != "success" {
		return false, meta, nil
	}
	return !resp.Hosting, meta, nil
}

// boltproxyExitChecker pre-checks a single candidate exit given its proxy URL. It
// is a function so tests can inject a stub and drive rotation without real HTTP.
type boltproxyExitChecker func(ctx context.Context, proxyURL string) (bool, boltproxyExitMeta, error)

// boltproxyExitPrecheck returns the real checker: ONE HTTP GET through the given
// proxy to the intel endpoint, then classifyBoltproxyExit on the body.
func boltproxyExitPrecheck(endpoint string, timeout time.Duration) boltproxyExitChecker {
	return func(ctx context.Context, proxyURL string) (bool, boltproxyExitMeta, error) {
		client, err := engine.NewProxyHTTPClient(proxyURL, timeout)
		if err != nil {
			return false, boltproxyExitMeta{}, err
		}
		defer client.CloseIdleConnections()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return false, boltproxyExitMeta{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return false, boltproxyExitMeta{}, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, boltproxyPrecheckResponseSizeLimit))
		if err != nil {
			return false, boltproxyExitMeta{}, err
		}
		return classifyBoltproxyExit(body)
	}
}

// boltproxyPrecheckConfig is the resolved (defaults-applied) pre-check policy.
type boltproxyPrecheckConfig struct {
	Enabled     bool
	MaxAttempts int
	Endpoint    string
	Timeout     time.Duration
}

// boltproxyPrecheckConfigFromSettings resolves the pre-check policy, applying the
// package defaults for any unset knob so the defaults live in one place.
func boltproxyPrecheckConfigFromSettings(s rpc.BoltproxySettings) boltproxyPrecheckConfig {
	cfg := boltproxyPrecheckConfig{
		Enabled:     s.Precheck,
		MaxAttempts: s.PrecheckMaxAttempts,
		Endpoint:    s.PrecheckEndpoint,
		Timeout:     time.Duration(s.PrecheckTimeoutSeconds) * time.Second,
	}
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = boltproxyPrecheckDefaultMaxAttempts
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = boltproxyPrecheckDefaultEndpoint
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = boltproxyPrecheckDefaultTimeoutSecs * time.Second
	}
	return cfg
}

// resolveBoltproxyRoutePrechecked resolves the sticky boltproxy route for a
// registration, pre-flighting the exit when pre-check is enabled. With pre-check
// off it is exactly the deterministic buildBoltproxyRoute path (no check, no
// rotation). Not-ok means boltproxy is not configured so the caller falls back to
// the common proxy / direct.
func (g *actionGateway) resolveBoltproxyRoutePrechecked(ctx context.Context, countryCode, e164, pinnedSid string) (wacore.WAProxyRoute, bool) {
	if g == nil || g.server == nil {
		return wacore.WAProxyRoute{}, false
	}
	s := g.server.BoltproxySettings()
	cfg := boltproxyPrecheckConfigFromSettings(s)
	if !cfg.Enabled {
		// With pre-check off every flow uses the deterministic attempt-0 sid, so a
		// probe and its registration already resolve the identical exit — no pin.
		return buildBoltproxyRoute(s, countryCode, e164)
	}
	return rotateBoltproxyExit(ctx, s, countryCode, e164, pinnedSid, cfg, boltproxyExitPrecheck(cfg.Endpoint, cfg.Timeout))
}

// rotateBoltproxyExit tries attempts 0..MaxAttempts-1, building the route for each
// attempt (attempt 0 = the phone's stable deterministic sid, later attempts a
// varied sid) and returning the FIRST route whose exit the checker approves
// (reachable AND non-datacenter). If none qualifies it logs a warning and falls
// back to the attempt-0 deterministic route so registration still proceeds — a
// working-but-datacenter exit beats no registration. The checker is injected so
// the rotation is unit-testable without real network calls.
//
// When pinnedSid is set (a prior probe's validated exit, echoed back by the
// caller) it is re-checked FIRST: still-good means the registration reuses the
// exact IP the probe validated; degraded/expired means it falls through to a
// fresh 0..N rotation. The pinned sid is skipped inside the loop so it is never
// re-checked twice.
func rotateBoltproxyExit(ctx context.Context, s rpc.BoltproxySettings, countryCode, e164, pinnedSid string, cfg boltproxyPrecheckConfig, check boltproxyExitChecker) (wacore.WAProxyRoute, bool) {
	baseRoute, ok := buildBoltproxyRouteForAttempt(s, countryCode, e164, 0)
	if !ok {
		return wacore.WAProxyRoute{}, false
	}
	pinnedSid = strings.TrimSpace(pinnedSid)
	if pinnedSid != "" {
		if pinned, ok := buildBoltproxyRouteFromSid(s, countryCode, pinnedSid); ok {
			good, meta, err := check(ctx, pinned.ProxyURL)
			logBoltproxyExitPrecheck(pinned, boltproxyPinnedExitAttempt, good, meta, err)
			if good {
				return pinned, true
			}
		}
	}
	attempts := cfg.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		route, ok := buildBoltproxyRouteForAttempt(s, countryCode, e164, attempt)
		if !ok {
			break
		}
		if route.RouteID == pinnedSid {
			continue // already re-checked above as the pinned exit
		}
		good, meta, err := check(ctx, route.ProxyURL)
		logBoltproxyExitPrecheck(route, attempt, good, meta, err)
		if good {
			return route, true
		}
	}
	log.Printf(
		"wa_boltproxy_exit_precheck_fallback sid=%s attempts=%d reason=no_good_exit note=using_deterministic_exit_quality_unverified",
		shared.ProbeLogValue(baseRoute.AccountID),
		attempts,
	)
	return baseRoute, true
}

func logBoltproxyExitPrecheck(route wacore.WAProxyRoute, attempt int, ok bool, meta boltproxyExitMeta, err error) {
	log.Printf(
		"wa_boltproxy_exit_precheck sid=%s attempt=%d ok=%t exit_country=%s isp=%s mobile=%t hosting=%t error=%s",
		shared.ProbeLogValue(route.AccountID),
		attempt,
		ok,
		shared.ProbeLogValue(meta.Country),
		shared.ProbeLogValue(meta.ISP),
		meta.Mobile,
		meta.Hosting,
		boltproxyPrecheckErrorReason(err),
	)
}

// boltproxyPrecheckErrorReason maps a pre-check error to a coarse, value-safe
// reason token; it never emits the raw error text, so a credential-bearing proxy
// URL can never leak into logs even if an underlying error mentioned it.
func boltproxyPrecheckErrorReason(err error) string {
	if err == nil {
		return ""
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	return "unreachable"
}
