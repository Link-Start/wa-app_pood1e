package bff

import "testing"

func TestProxyCountryCodeFromCallingCode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Bare calling codes, with and without the "+".
		{"+1", "US"}, {"1", "US"},
		{"+44", "GB"}, {"44", "GB"},
		{"+49", "DE"}, {"+33", "FR"}, {"+34", "ES"}, {"+39", "IT"},
		{"+31", "NL"}, {"+91", "IN"}, {"+62", "ID"}, {"+55", "BR"},
		{"+52", "MX"}, {"+7", "RU"}, {"+90", "TR"}, {"+20", "EG"},
		{"+234", "NG"}, {"+92", "PK"}, {"+880", "BD"}, {"+966", "SA"},
		{"+971", "AE"}, {"+63", "PH"}, {"+84", "VN"}, {"+66", "TH"},
		{"+60", "MY"}, {"+48", "PL"}, {"+57", "CO"}, {"+54", "AR"},
		{"+86", "CN"},
		// Longest-prefix wins: 3-digit codes are not shadowed by shorter ones.
		{"+212", "MA"}, {"+351", "PT"}, {"+254", "KE"}, {"+998", "UZ"},
		// Full E.164 numbers resolve by their leading calling code.
		{"+8613800138000", "CN"}, {"+14155550123", "US"}, {"+442079460000", "GB"},
		{"+2348012345678", "NG"}, {"+551199998888", "BR"},
		// Whitespace-separated input keeps the calling-code digits.
		{"+44 20 7946 0000", "GB"},
		// Unknown / unmappable input yields empty (region omitted downstream).
		{"", ""}, {"+", ""}, {"US", ""}, {"+999", ""}, {"0", ""},
	}
	for _, c := range cases {
		if got := proxyCountryCodeFromCallingCode(c.in); got != c.want {
			t.Errorf("proxyCountryCodeFromCallingCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestProxyCountryCodeFromPayloadCallingCode(t *testing.T) {
	// An arbitrary country now resolves end-to-end through the payload path
	// (previously only ~6 codes were mapped, so most numbers fell back to US).
	cases := []struct {
		field string
		value string
		want  string
	}{
		{"country_calling_code", "+49", "DE"},
		{"cc", "91", "IN"},
		{"country_code", "+55", "BR"},
	}
	for _, c := range cases {
		got := proxyCountryCodeFromPayload(map[string]any{c.field: c.value})
		if got != c.want {
			t.Errorf("payload %s=%q resolved to %q, want %q", c.field, c.value, got, c.want)
		}
	}
}
