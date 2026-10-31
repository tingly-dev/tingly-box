package peertransport

import "net/http"

// ProbeHandler identifies a TB peer on the existing HTTP listener. A matching
// probe confirms the request reached TB; TLS and HTTP/3 may terminate upstream.
func ProbeHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.Header.Get(ProbeHeader) != Capability {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set(CapabilityHeader, Capability)
		w.WriteHeader(http.StatusNoContent)
	})
}
