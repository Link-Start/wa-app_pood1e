package bff

import (
	"context"
	"strings"
	"testing"

	"github.com/byte-v-forge/wa-app/internal/waapp/rpc"
)

func TestClassifyBoltproxyExit(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantOK   bool
		wantMeta boltproxyExitMeta
	}{
		{
			name:     "residential ISP is good",
			body:     `{"status":"success","countryCode":"US","isp":"Comcast Cable","hosting":false,"mobile":false,"proxy":false}`,
			wantOK:   true,
			wantMeta: boltproxyExitMeta{Country: "US", ISP: "Comcast Cable", Mobile: false, Hosting: false},
		},
		{
			name:     "datacenter/hosting is bad",
			body:     `{"status":"success","countryCode":"DE","isp":"Hetzner Online","hosting":true,"mobile":false,"proxy":true}`,
			wantOK:   false,
			wantMeta: boltproxyExitMeta{Country: "DE", ISP: "Hetzner Online", Mobile: false, Hosting: true},
		},
		{
			name:     "mobile carrier is good",
			body:     `{"status":"success","countryCode":"GB","isp":"EE Mobile","hosting":false,"mobile":true,"proxy":true}`,
			wantOK:   true,
			wantMeta: boltproxyExitMeta{Country: "GB", ISP: "EE Mobile", Mobile: true, Hosting: false},
		},
		{
			name:     "residential proxy flag alone is still good",
			body:     `{"status":"success","countryCode":"FR","isp":"Orange","hosting":false,"mobile":false,"proxy":true}`,
			wantOK:   true,
			wantMeta: boltproxyExitMeta{Country: "FR", ISP: "Orange", Mobile: false, Hosting: false},
		},
		{
			name:     "status not success is bad (unreachable)",
			body:     `{"status":"fail","message":"reserved range","hosting":false}`,
			wantOK:   false,
			wantMeta: boltproxyExitMeta{Country: "", ISP: "", Mobile: false, Hosting: false},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, meta, err := classifyBoltproxyExit([]byte(c.body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if meta != c.wantMeta {
				t.Fatalf("meta = %+v, want %+v", meta, c.wantMeta)
			}
		})
	}
}

func TestClassifyBoltproxyExitInvalidJSON(t *testing.T) {
	if _, _, err := classifyBoltproxyExit([]byte("not json")); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func precheckTestSettings() rpc.BoltproxySettings {
	s := baseBoltproxySettings()
	s.Precheck = true
	return s
}

// sidForAttempt exposes the sid a given rotation attempt would produce, so the
// rotation tests can assert exactly which attempt's exit was pinned.
func sidForAttempt(s rpc.BoltproxySettings, e164 string, attempt int) string {
	route, ok := buildBoltproxyRouteForAttempt(s, "US", e164, attempt)
	if !ok {
		panic("route did not build")
	}
	return route.AccountID
}

func TestRotateBoltproxyExitPinsAttemptZeroWhenApprovedImmediately(t *testing.T) {
	s := precheckTestSettings()
	e164 := "+15551230001"
	cfg := boltproxyPrecheckConfigFromSettings(s)

	calls := 0
	check := func(_ context.Context, _ string) (bool, boltproxyExitMeta, error) {
		calls++
		return true, boltproxyExitMeta{Country: "US", ISP: "Comcast", Hosting: false}, nil
	}

	route, ok := rotateBoltproxyExit(context.Background(), s, "US", e164, "", cfg, check)
	if !ok {
		t.Fatal("expected a route")
	}
	if calls != 1 {
		t.Fatalf("expected exactly one pre-check when attempt 0 is good, got %d", calls)
	}
	want := sidForAttempt(s, e164, 0)
	if route.AccountID != want {
		t.Fatalf("expected attempt-0 sid %q (stable per-phone IP), got %q", want, route.AccountID)
	}
	if route.AccountID != boltproxySessionID(s.SessionSalt, e164)[:boltproxySidLen] {
		t.Fatal("attempt-0 sid must equal the deterministic per-phone sid")
	}
}

func TestRotateBoltproxyExitReusesPinnedExitWhenStillGood(t *testing.T) {
	// The probe pinned attempt-1's exit; at registration it is re-checked FIRST and,
	// still good, reused verbatim — the probe and registration share one sticky IP.
	s := precheckTestSettings()
	e164 := "+15551230001"
	cfg := boltproxyPrecheckConfigFromSettings(s)
	pinned := sidForAttempt(s, e164, 1)

	var checked []string
	check := func(_ context.Context, proxyURL string) (bool, boltproxyExitMeta, error) {
		checked = append(checked, proxyURL)
		return true, boltproxyExitMeta{Country: "US", ISP: "Comcast", Hosting: false}, nil
	}

	route, ok := rotateBoltproxyExit(context.Background(), s, "US", e164, pinned, cfg, check)
	if !ok {
		t.Fatal("expected a route")
	}
	if route.AccountID != pinned {
		t.Fatalf("expected the pinned sid %q to be reused, got %q", pinned, route.AccountID)
	}
	if len(checked) != 1 || !strings.Contains(checked[0], "-session-"+pinned+"-") {
		t.Fatalf("expected exactly one pre-check of the pinned exit, got %v", checked)
	}
}

func TestRotateBoltproxyExitRotatesFreshWhenPinnedExitDegraded(t *testing.T) {
	// The pinned exit went bad since the probe; it must fall through to a fresh
	// rotation and never re-check the pinned sid twice.
	s := precheckTestSettings()
	e164 := "+15551230001"
	cfg := boltproxyPrecheckConfigFromSettings(s)
	pinned := sidForAttempt(s, e164, 0) // pin the deterministic attempt-0 exit
	good := sidForAttempt(s, e164, 1)

	pinnedChecks := 0
	check := func(_ context.Context, proxyURL string) (bool, boltproxyExitMeta, error) {
		if strings.Contains(proxyURL, "-session-"+pinned+"-") {
			pinnedChecks++
			return false, boltproxyExitMeta{Country: "US", ISP: "Hetzner", Hosting: true}, nil
		}
		return true, boltproxyExitMeta{Country: "US", ISP: "Comcast", Hosting: false}, nil
	}

	route, ok := rotateBoltproxyExit(context.Background(), s, "US", e164, pinned, cfg, check)
	if !ok {
		t.Fatal("expected a route")
	}
	if route.AccountID != good {
		t.Fatalf("expected fresh rotation to attempt-1 %q after pinned exit degraded, got %q", good, route.AccountID)
	}
	if pinnedChecks != 1 {
		t.Fatalf("pinned exit must be checked exactly once (not re-checked in the loop), got %d", pinnedChecks)
	}
}

func TestRotateBoltproxyExitRotatesUntilGood(t *testing.T) {
	s := precheckTestSettings()
	e164 := "+15551230001"
	cfg := boltproxyPrecheckConfigFromSettings(s)

	sid0 := sidForAttempt(s, e164, 0)
	sid1 := sidForAttempt(s, e164, 1)
	if sid0 == sid1 {
		t.Fatal("rotation must vary the sid across attempts")
	}

	// Reject attempt 0 (datacenter), approve the first different exit (attempt 1).
	check := func(_ context.Context, proxyURL string) (bool, boltproxyExitMeta, error) {
		if strings.Contains(proxyURL, "-session-"+sid0+"-") {
			return false, boltproxyExitMeta{Country: "US", ISP: "Hetzner", Hosting: true}, nil
		}
		return true, boltproxyExitMeta{Country: "US", ISP: "Comcast", Hosting: false}, nil
	}

	route, ok := rotateBoltproxyExit(context.Background(), s, "US", e164, "", cfg, check)
	if !ok {
		t.Fatal("expected a route")
	}
	if route.AccountID != sid1 {
		t.Fatalf("expected attempt-1 sid %q after attempt 0 rejected, got %q", sid1, route.AccountID)
	}
}

func TestRotateBoltproxyExitFallsBackToAttemptZeroWhenNoneGood(t *testing.T) {
	s := precheckTestSettings()
	s.PrecheckMaxAttempts = 3
	e164 := "+15551230001"
	cfg := boltproxyPrecheckConfigFromSettings(s)

	calls := 0
	// Every exit is a datacenter — no good exit within maxAttempts.
	check := func(_ context.Context, _ string) (bool, boltproxyExitMeta, error) {
		calls++
		return false, boltproxyExitMeta{Country: "US", ISP: "Hetzner", Hosting: true}, nil
	}

	route, ok := rotateBoltproxyExit(context.Background(), s, "US", e164, "", cfg, check)
	if !ok {
		t.Fatal("fallback must still yield a route (do not hard-fail registration)")
	}
	if calls != 3 {
		t.Fatalf("expected maxAttempts (3) pre-checks, got %d", calls)
	}
	if want := sidForAttempt(s, e164, 0); route.AccountID != want {
		t.Fatalf("fallback must use attempt-0 deterministic sid %q, got %q", want, route.AccountID)
	}
}

func TestRotateBoltproxyExitNotConfigured(t *testing.T) {
	s := precheckTestSettings()
	s.Endpoint = "" // boltproxy not configured
	cfg := boltproxyPrecheckConfigFromSettings(s)

	check := func(_ context.Context, _ string) (bool, boltproxyExitMeta, error) {
		t.Fatal("checker must not run when boltproxy is unconfigured")
		return false, boltproxyExitMeta{}, nil
	}
	if _, ok := rotateBoltproxyExit(context.Background(), s, "US", "+15551230001", "", cfg, check); ok {
		t.Fatal("expected not-ok so the caller falls back to common/direct")
	}
}

func TestBoltproxyPrecheckConfigDefaults(t *testing.T) {
	cfg := boltproxyPrecheckConfigFromSettings(rpc.BoltproxySettings{Precheck: true})
	if cfg.MaxAttempts != boltproxyPrecheckDefaultMaxAttempts {
		t.Fatalf("MaxAttempts default = %d, want %d", cfg.MaxAttempts, boltproxyPrecheckDefaultMaxAttempts)
	}
	if cfg.Endpoint != boltproxyPrecheckDefaultEndpoint {
		t.Fatalf("Endpoint default = %q, want %q", cfg.Endpoint, boltproxyPrecheckDefaultEndpoint)
	}
	if cfg.Timeout.Seconds() != boltproxyPrecheckDefaultTimeoutSecs {
		t.Fatalf("Timeout default = %v, want %ds", cfg.Timeout, boltproxyPrecheckDefaultTimeoutSecs)
	}
}

// Guard against ever logging the credential-bearing proxy URL: the error-reason
// helper only ever returns coarse, non-leaking tokens.
func TestBoltproxyPrecheckErrorReasonNeverLeaks(t *testing.T) {
	if got := boltproxyPrecheckErrorReason(nil); got != "" {
		t.Fatalf("nil error reason = %q, want empty", got)
	}
	if got := boltproxyPrecheckErrorReason(context.DeadlineExceeded); got != "timeout" {
		t.Fatalf("deadline reason = %q, want timeout", got)
	}
	leak := "http://user:p4ss@ip.boltproxy.org:1600"
	if got := boltproxyPrecheckErrorReason(&urlErrorStub{msg: leak}); strings.Contains(got, "p4ss") || strings.Contains(got, "boltproxy.org") {
		t.Fatalf("error reason leaked proxy details: %q", got)
	}
}

type urlErrorStub struct{ msg string }

func (e *urlErrorStub) Error() string { return e.msg }
