package bff

import (
	"strings"
	"testing"

	"github.com/byte-v-forge/wa-app/internal/waapp/rpc"
)

func baseCliproxySettings() rpc.CliproxySettings {
	return rpc.CliproxySettings{Endpoint: "us.cliproxy.io:3010", Username: "7ou81208230", Password: "s3cr3t", TTLMinutes: 20, Region: "NONE"}
}

func TestCliproxySessionIDStableAndDistinct(t *testing.T) {
	a := cliproxySessionID("", "+15551230001")
	if got := cliproxySessionID("", "+15551230001"); got != a {
		t.Fatalf("session not stable for same phone: %s vs %s", a, got)
	}
	if cliproxySessionID("", "+15551230002") == a {
		t.Fatal("session collided across different phones")
	}
	if len(a) < cliproxySidLen {
		t.Fatalf("session too short: %d", len(a))
	}
	if cliproxySessionID("rotate2", "+15551230001") == a {
		t.Fatal("salt did not rotate the session")
	}
}

func TestBuildCliproxyRouteStickyPerAccount(t *testing.T) {
	s := baseCliproxySettings()
	r1, ok1 := buildCliproxyRoute(s, "US", "+15551230001")
	r1b, _ := buildCliproxyRoute(s, "US", "+15551230001")
	r2, ok2 := buildCliproxyRoute(s, "US", "+15551230002")
	if !ok1 || !ok2 {
		t.Fatal("expected cliproxy route to resolve")
	}
	if r1.ProxyURL != r1b.ProxyURL {
		t.Fatal("same phone got a different proxy URL (not sticky)")
	}
	if r1.ProxyURL == r2.ProxyURL {
		t.Fatal("different phones got the same proxy URL")
	}
	// sid is exactly cliproxySidLen chars and the URL carries the verified format.
	if len(r1.RouteID) != cliproxySidLen || r1.AccountID != r1.RouteID {
		t.Fatalf("unexpected sid: %q", r1.RouteID)
	}
	if !strings.Contains(r1.ProxyURL, "-sid-"+r1.RouteID+"-t-20") {
		t.Fatalf("proxy URL missing verified sticky format: %s", r1.ProxyURL)
	}
	if !strings.HasPrefix(r1.ProxyURL, "http://7ou81208230-") || !strings.Contains(r1.ProxyURL, "@us.cliproxy.io:3010") {
		t.Fatalf("unexpected proxy URL shape: %s", r1.ProxyURL)
	}
	if r1.Source != waProxySourceCliproxy || r1.PolicyMode != waProxyPolicyModeSticky {
		t.Fatalf("unexpected route metadata: %+v", r1)
	}
}

func TestBuildCliproxyRouteRegion(t *testing.T) {
	// AUTO region derives from the phone's country code.
	s := baseCliproxySettings()
	s.Region = "AUTO"
	r, _ := buildCliproxyRoute(s, "US", "+15551230001")
	if !strings.Contains(r.ProxyURL, "-region-US-sid-") {
		t.Fatalf("AUTO region not applied: %s", r.ProxyURL)
	}
	// AUTO with unknown country omits region.
	r2, _ := buildCliproxyRoute(s, "", "+15551230001")
	if strings.Contains(r2.ProxyURL, "-region-") {
		t.Fatalf("region should be omitted when country unknown: %s", r2.ProxyURL)
	}
	// Explicit override forces the region.
	s.Region = "JP"
	r3, _ := buildCliproxyRoute(s, "US", "+15551230001")
	if !strings.Contains(r3.ProxyURL, "-region-JP-sid-") {
		t.Fatalf("explicit region override not applied: %s", r3.ProxyURL)
	}
	// NONE never adds region.
	s.Region = "NONE"
	r4, _ := buildCliproxyRoute(s, "US", "+15551230001")
	if strings.Contains(r4.ProxyURL, "-region-") {
		t.Fatalf("region should be disabled: %s", r4.ProxyURL)
	}
}

func TestBuildCliproxyRouteNotConfigured(t *testing.T) {
	full := baseCliproxySettings()
	cases := []struct {
		name string
		s    rpc.CliproxySettings
		e164 string
	}{
		{"empty endpoint", func() rpc.CliproxySettings { c := full; c.Endpoint = ""; return c }(), "+15551230001"},
		{"empty username", func() rpc.CliproxySettings { c := full; c.Username = ""; return c }(), "+15551230001"},
		{"empty password", func() rpc.CliproxySettings { c := full; c.Password = ""; return c }(), "+15551230001"},
		{"empty phone", full, ""},
	}
	for _, c := range cases {
		if _, ok := buildCliproxyRoute(c.s, "US", c.e164); ok {
			t.Fatalf("%s: expected not-ok (fall back to common/direct)", c.name)
		}
	}
}

func TestBuildCliproxyRouteTTLDefault(t *testing.T) {
	s := baseCliproxySettings()
	s.TTLMinutes = 0
	r, ok := buildCliproxyRoute(s, "US", "+15551230001")
	if !ok || !strings.Contains(r.ProxyURL, "-t-20") {
		t.Fatalf("TTL default not applied: ok=%v url=%s", ok, r.ProxyURL)
	}
}
