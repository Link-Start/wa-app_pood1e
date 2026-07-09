package engine

const (
	AccountProbeFlowUnknown           = "unknown"
	AccountProbeFlowProbeFailed       = "probe_failed"
	AccountProbeFlowRegistered        = "registered"
	AccountProbeFlowNotRegistered     = "not_registered"
	AccountProbeFlowBlocked           = "blocked"
	AccountProbeFlowConsentBlocked    = "consent_blocked"
	AccountProbeFlowNotAllowed        = "not_allowed"
	AccountProbeFlowInvalidNumber     = "invalid_number"
	AccountProbeFlowRateLimited       = "rate_limited"
	AccountProbeFlowConsentRequired   = "consent_required"
	AccountProbeFlowChallengeRequired = "challenge_required"
)
