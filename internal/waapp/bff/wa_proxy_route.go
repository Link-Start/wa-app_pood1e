package bff

import (
	"strings"

	"github.com/byte-v-forge/wa-app/internal/waapp/shared"
)

func proxyCountryCodeFromPayload(payload map[string]any) string {
	phone := shared.ObjectField(payload, "phone")
	proxy := shared.ObjectField(payload, "proxy")
	value := shared.FirstNonEmpty(
		shared.TextField(payload, "proxy_country_code"),
		shared.TextField(proxy, "country_code"),
		shared.TextField(proxy, "proxy_country_code"),
		shared.TextField(payload, "country_iso2"),
		shared.TextField(payload, "country_region"),
		shared.TextField(payload, "region"),
		shared.TextField(phone, "country_iso2"),
	)
	if value != "" {
		return normalizeProxyCountryCode(value)
	}
	callingCode := shared.FirstNonEmpty(
		shared.TextField(payload, "country_calling_code"),
		shared.TextField(payload, "cc"),
		shared.TextField(payload, "country_code"),
		shared.TextField(phone, "country_calling_code"),
	)
	return normalizeProxyCountryCode(proxyCountryCodeFromCallingCode(callingCode))
}

// callingCodeToISO2 maps ITU-T E.164 country calling codes (1–3 digits) to
// ISO-3166 alpha-2 country codes. It drives cliproxy AUTO region selection so the
// sticky exit IP lands in the same country as the number being registered.
// Calling codes are a prefix-free code, so a longest-prefix lookup resolves
// correctly whether the input is a bare calling code or a full number. Shared
// codes resolve to their dominant member: NANP (+1) -> US, and +7 -> RU.
var callingCodeToISO2 = map[string]string{
	// Zone 1 — North American Numbering Plan
	"1": "US",
	// Zone 2 — Africa
	"20": "EG", "211": "SS", "212": "MA", "213": "DZ", "216": "TN",
	"218": "LY", "220": "GM", "221": "SN", "222": "MR", "223": "ML",
	"224": "GN", "225": "CI", "226": "BF", "227": "NE", "228": "TG",
	"229": "BJ", "230": "MU", "231": "LR", "232": "SL", "233": "GH",
	"234": "NG", "235": "TD", "236": "CF", "237": "CM", "238": "CV",
	"239": "ST", "240": "GQ", "241": "GA", "242": "CG", "243": "CD",
	"244": "AO", "245": "GW", "248": "SC", "249": "SD", "250": "RW",
	"251": "ET", "252": "SO", "253": "DJ", "254": "KE", "255": "TZ",
	"256": "UG", "257": "BI", "258": "MZ", "260": "ZM", "261": "MG",
	"263": "ZW", "264": "NA", "265": "MW", "266": "LS", "267": "BW",
	"268": "SZ", "269": "KM", "27": "ZA", "291": "ER",
	// Zone 3/4 — Europe
	"30": "GR", "31": "NL", "32": "BE", "33": "FR", "34": "ES",
	"350": "GI", "351": "PT", "352": "LU", "353": "IE", "354": "IS",
	"355": "AL", "356": "MT", "357": "CY", "358": "FI", "359": "BG",
	"36": "HU", "370": "LT", "371": "LV", "372": "EE", "373": "MD",
	"374": "AM", "375": "BY", "376": "AD", "377": "MC", "378": "SM",
	"380": "UA", "381": "RS", "382": "ME", "383": "XK", "385": "HR",
	"386": "SI", "387": "BA", "389": "MK", "39": "IT",
	"40": "RO", "41": "CH", "420": "CZ", "421": "SK", "423": "LI",
	"43": "AT", "44": "GB", "45": "DK", "46": "SE", "47": "NO",
	"48": "PL", "49": "DE",
	// Zone 5 — Central and South America
	"500": "FK", "501": "BZ", "502": "GT", "503": "SV", "504": "HN",
	"505": "NI", "506": "CR", "507": "PA", "509": "HT", "51": "PE",
	"52": "MX", "53": "CU", "54": "AR", "55": "BR", "56": "CL",
	"57": "CO", "58": "VE", "591": "BO", "592": "GY", "593": "EC",
	"595": "PY", "597": "SR", "598": "UY",
	// Zone 6 — Southeast Asia and Oceania
	"60": "MY", "61": "AU", "62": "ID", "63": "PH", "64": "NZ",
	"65": "SG", "66": "TH", "670": "TL", "673": "BN", "675": "PG",
	"679": "FJ",
	// Zone 7 — Russia and Kazakhstan
	"7": "RU",
	// Zone 8 — East Asia
	"81": "JP", "82": "KR", "84": "VN", "86": "CN", "852": "HK",
	"853": "MO", "855": "KH", "856": "LA", "880": "BD", "886": "TW",
	// Zone 9 — West/South/Central Asia and the Middle East
	"90": "TR", "91": "IN", "92": "PK", "93": "AF", "94": "LK",
	"95": "MM", "960": "MV", "961": "LB", "962": "JO", "963": "SY",
	"964": "IQ", "965": "KW", "966": "SA", "967": "YE", "968": "OM",
	"970": "PS", "971": "AE", "972": "IL", "973": "BH", "974": "QA",
	"975": "BT", "976": "MN", "977": "NP", "98": "IR", "992": "TJ",
	"993": "TM", "994": "AZ", "995": "GE", "996": "KG", "998": "UZ",
}

// proxyCountryCodeFromCallingCode resolves a dialing code (e.g. "+44", "44", or a
// full number that starts with the code) to an ISO-3166 alpha-2 country. Codes are
// prefix-free, so it matches the longest code prefix first (3 digits down to 1).
func proxyCountryCodeFromCallingCode(value string) string {
	digits := leadingDigits(value)
	for n := 3; n >= 1; n-- {
		if len(digits) >= n {
			if iso2, ok := callingCodeToISO2[digits[:n]]; ok {
				return iso2
			}
		}
	}
	return ""
}

// leadingDigits returns the run of ASCII digits at the start of value, ignoring a
// leading "+" and surrounding whitespace, so "+44", "44", and "+44 20 …" all yield
// the calling-code digits.
func leadingDigits(value string) string {
	s := strings.TrimPrefix(strings.TrimSpace(value), "+")
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s[:i]
		}
	}
	return s
}

func normalizeProxyCountryCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "+")
	switch value {
	case "", "1", "USA", "UNITEDSTATES", "UNITED_STATES":
		return "US"
	default:
		return value
	}
}
