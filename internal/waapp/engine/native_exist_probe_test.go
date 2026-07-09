package engine

import (
	"testing"

	waappv1 "github.com/byte-v-forge/wa-app/gen/go/byte/v/forge/waapp/v1"
)

// TestParseExistProbeResultReasonClassification locks the /v2/exist reason ->
// account-flow mapping. The crux: a literal ban is the only "blocked" verdict;
// the age/parental-consent hard-blocks and the not-allowed/limited-release
// disallows each get their own terminal flow and never set Blocked, so the
// dashboard cannot render them as a ban.
func TestParseExistProbeResultReasonClassification(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		reason      string
		wantFlow    string
		wantBlocked bool
		wantStatus  waappv1.AccountProbeStatus
	}{
		{"ban_reason", "", "blocked", AccountProbeFlowBlocked, true, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"ban_status", "blocked", "", AccountProbeFlowBlocked, true, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"consent_underage_block", "", "consent_underage_block", AccountProbeFlowConsentBlocked, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"consent_impossible_age", "", "consent_impossible_age", AccountProbeFlowConsentBlocked, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"consent_parent_block", "", "consent_parent_block", AccountProbeFlowConsentBlocked, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"consent_parent_linking_ineligible", "", "consent_parent_linking_ineligible", AccountProbeFlowConsentBlocked, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"not_allowed", "", "not_allowed", AccountProbeFlowNotAllowed, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"biz_not_allowed", "", "biz_not_allowed", AccountProbeFlowNotAllowed, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"limited_release", "", "limited_release", AccountProbeFlowNotAllowed, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"consent_required_is_not_blocked", "", "consent", AccountProbeFlowConsentRequired, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_REACHABLE},
		{"challenge_required_is_not_blocked", "", "challenge", AccountProbeFlowChallengeRequired, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_REACHABLE},
		{"invalid_number", "", "length_short", AccountProbeFlowInvalidNumber, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"rate_limited", "", "too_many", AccountProbeFlowRateLimited, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_UNREACHABLE},
		{"not_registered", "", "incorrect", AccountProbeFlowNotRegistered, false, waappv1.AccountProbeStatus_ACCOUNT_PROBE_STATUS_REACHABLE},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseExistProbeResult(map[string]any{"status": tc.status, "reason": tc.reason})
			if result.AccountFlow != tc.wantFlow {
				t.Errorf("AccountFlow = %q, want %q", result.AccountFlow, tc.wantFlow)
			}
			if result.Blocked != tc.wantBlocked {
				t.Errorf("Blocked = %v, want %v", result.Blocked, tc.wantBlocked)
			}
			if result.Status != tc.wantStatus {
				t.Errorf("Status = %v, want %v", result.Status, tc.wantStatus)
			}
		})
	}
}

func TestExistConsentBlockedReason(t *testing.T) {
	blocked := []string{"consent_underage_block", "consent_impossible_age", "consent_parent_block", "consent_parent_linking_ineligible"}
	for _, reason := range blocked {
		if !existConsentBlockedReason(reason) {
			t.Errorf("existConsentBlockedReason(%q) = false, want true", reason)
		}
	}
	notBlocked := []string{"consent", "consent_minor", "app_store_age", "blocked", "not_allowed", ""}
	for _, reason := range notBlocked {
		if existConsentBlockedReason(reason) {
			t.Errorf("existConsentBlockedReason(%q) = true, want false", reason)
		}
	}
}

func TestExistNotAllowedReason(t *testing.T) {
	notAllowed := []string{"not_allowed", "biz_not_allowed", "limited_release"}
	for _, reason := range notAllowed {
		if !existNotAllowedReason(reason) {
			t.Errorf("existNotAllowedReason(%q) = false, want true", reason)
		}
	}
	allowed := []string{"blocked", "consent", "consent_underage_block", "incorrect", ""}
	for _, reason := range allowed {
		if existNotAllowedReason(reason) {
			t.Errorf("existNotAllowedReason(%q) = true, want false", reason)
		}
	}
}
