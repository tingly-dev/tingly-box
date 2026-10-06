package capture

import (
	"net/http"
	"strings"
)

// credentialHeaders are masked at capture time so recordings never become a
// credential leak (.design/recording.md §3).
var credentialHeaders = map[string]bool{
	"authorization":       true,
	"proxy-authorization": true,
	"x-api-key":           true,
	"api-key":             true,
	"x-goog-api-key":      true,
	"cookie":              true,
	"set-cookie":          true,
}

// redactHeaders flattens h to first values and masks credential headers,
// keeping a short prefix and suffix so recordings still tell keys apart.
func redactHeaders(h map[string][]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) == 0 {
			continue
		}
		val := v[0]
		if credentialHeaders[strings.ToLower(k)] {
			val = mask(val)
		}
		out[http.CanonicalHeaderKey(k)] = val
	}
	return out
}

func mask(v string) string {
	if len(v) <= 12 {
		return "***"
	}
	return v[:6] + "***" + v[len(v)-4:]
}
