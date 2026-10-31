package peertransport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeHandlerPreservesHTTPRoutes(t *testing.T) {
	calls := 0
	handler := ProbeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusAccepted)
	}))
	for _, tc := range []struct {
		method, path, probe string
		status              int
		marker              string
	}{
		{http.MethodHead, "/prefix/tingly/test", Capability, http.StatusNoContent, Capability},
		{http.MethodHead, "/prefix/tingly/test", "", http.StatusAccepted, ""},
		{http.MethodPost, "/v1/messages", Capability, http.StatusAccepted, ""},
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set(ProbeHeader, tc.probe)
		handler.ServeHTTP(recorder, req)
		if recorder.Code != tc.status || recorder.Header().Get(CapabilityHeader) != tc.marker {
			t.Fatalf("%s %s: status %d marker %q", tc.method, tc.path, recorder.Code, recorder.Header().Get(CapabilityHeader))
		}
	}
	if calls != 2 {
		t.Fatalf("model handler called %d times", calls)
	}
}
