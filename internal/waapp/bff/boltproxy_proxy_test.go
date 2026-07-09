package bff

import (
	"strings"
	"testing"

	"github.com/byte-v-forge/wa-app/internal/waapp/rpc"
)

func baseBoltproxySettings() rpc.BoltproxySettings {
	return rpc.BoltproxySettings{Endpoint: "ip.boltproxy.org:1600", Username: "user-bolttest", Password: "s3cr3t", TTLMinutes: 20, Region: "NONE"}
}

func TestBoltproxySessionIDStableAndDistinct(t *testing.T) {
	a := boltproxySessionID("", "+15551230001")
	if got := boltproxySessionID("", "+15551230001"); got != a {
		t.Fatalf("session not stable for same phone: %s vs %s", a, got)
	}
	if boltproxySessionID("", "+15551230002") == a {
		t.Fatal("session collided across different phones")
	}
	if len(a) < boltproxySidLen {
		t.Fatalf("session too short: %d", len(a))
	}
	if boltproxySessionID("rotate2", "+15551230001") == a {
		t.Fatal("salt did not rotate the session")
	}
}

func TestBuildBoltproxyRouteStickyPerAccount(t *testing.T) {
	s := baseBoltproxySettings()
	r1, ok1 := buildBoltproxyRoute(s, "US", "+15551230001")
	r1b, _ := buildBoltproxyRoute(s, "US", "+15551230001")
	r2, ok2 := buildBoltproxyRoute(s, "US", "+15551230002")
	if !ok1 || !ok2 {
		t.Fatal("expected boltproxy route to resolve")
	}
	if r1.ProxyURL != r1b.ProxyURL {
		t.Fatal("same phone got a different proxy URL (not sticky)")
	}
	if r1.ProxyURL == r2.ProxyURL {
		t.Fatal("different phones got the same proxy URL")
	}
	// sid is exactly boltproxySidLen chars and the URL carries the boltproxy format.
	if len(r1.RouteID) != boltproxySidLen || r1.AccountID != r1.RouteID {
		t.Fatalf("unexpected sid: %q", r1.RouteID)
	}
	if !strings.Contains(r1.ProxyURL, "-session-"+r1.RouteID+"-sessTime-20") {
		t.Fatalf("proxy URL missing boltproxy sticky format: %s", r1.ProxyURL)
	}
	if !strings.HasPrefix(r1.ProxyURL, "http://user-bolttest-zone-res-") || !strings.Contains(r1.ProxyURL, "@ip.boltproxy.org:1600") {
		t.Fatalf("unexpected proxy URL shape: %s", r1.ProxyURL)
	}
	if r1.Source != waProxySourceBoltproxy || r1.PolicyMode != waProxyPolicyModeSticky {
		t.Fatalf("unexpected route metadata: %+v", r1)
	}
}

func TestBuildBoltproxyRouteCountry(t *testing.T) {
	// AUTO derives the (lowercase) country token from the phone's country code.
	s := baseBoltproxySettings()
	s.Region = "AUTO"
	r, _ := buildBoltproxyRoute(s, "US", "+15551230001")
	if !strings.Contains(r.ProxyURL, "-country-us-session-") {
		t.Fatalf("AUTO country not applied (lowercase): %s", r.ProxyURL)
	}
	// AUTO with a boltproxy-unsupported country (CN) falls back to US.
	rc, _ := buildBoltproxyRoute(s, "CN", "+8613800138000")
	if !strings.Contains(rc.ProxyURL, "-country-us-session-") {
		t.Fatalf("unsupported CN country should fall back to US: %s", rc.ProxyURL)
	}
	// AUTO with unknown country omits the country token (zone-res stays).
	r2, _ := buildBoltproxyRoute(s, "", "+15551230001")
	if strings.Contains(r2.ProxyURL, "-country-") || !strings.Contains(r2.ProxyURL, "-zone-res-") {
		t.Fatalf("country should be omitted (zone-res kept) when country unknown: %s", r2.ProxyURL)
	}
	// Explicit override forces the (lowercased) country.
	s.Region = "JP"
	r3, _ := buildBoltproxyRoute(s, "US", "+15551230001")
	if !strings.Contains(r3.ProxyURL, "-country-jp-session-") {
		t.Fatalf("explicit country override not applied: %s", r3.ProxyURL)
	}
	// NONE never adds the country token.
	s.Region = "NONE"
	r4, _ := buildBoltproxyRoute(s, "US", "+15551230001")
	if strings.Contains(r4.ProxyURL, "-country-") {
		t.Fatalf("country should be disabled: %s", r4.ProxyURL)
	}
}

func TestBuildBoltproxyRouteNotConfigured(t *testing.T) {
	full := baseBoltproxySettings()
	cases := []struct {
		name string
		s    rpc.BoltproxySettings
		e164 string
	}{
		{"empty endpoint", func() rpc.BoltproxySettings { c := full; c.Endpoint = ""; return c }(), "+15551230001"},
		{"empty username", func() rpc.BoltproxySettings { c := full; c.Username = ""; return c }(), "+15551230001"},
		{"empty password", func() rpc.BoltproxySettings { c := full; c.Password = ""; return c }(), "+15551230001"},
		{"empty phone", full, ""},
	}
	for _, c := range cases {
		if _, ok := buildBoltproxyRoute(c.s, "US", c.e164); ok {
			t.Fatalf("%s: expected not-ok (fall back to common/direct)", c.name)
		}
	}
}

func TestBuildBoltproxyRouteTTLDefault(t *testing.T) {
	s := baseBoltproxySettings()
	s.TTLMinutes = 0
	r, ok := buildBoltproxyRoute(s, "US", "+15551230001")
	if !ok || !strings.Contains(r.ProxyURL, "-sessTime-20") {
		t.Fatalf("TTL default not applied: ok=%v url=%s", ok, r.ProxyURL)
	}
}
