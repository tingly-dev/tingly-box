package debug

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler()
	r.GET("/debug/memstats", h.GetMemStats)
	r.GET("/debug/pprof/heap", h.GetHeapProfile)
	return r
}

func TestGetMemStats(t *testing.T) {
	r := newTestRouter()

	for _, gc := range []bool{false, true} {
		url := "/debug/memstats"
		if gc {
			url += "?gc=true"
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("gc=%v: status %d", gc, w.Code)
		}
		var resp MemStatsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("gc=%v: invalid JSON: %v", gc, err)
		}
		if resp.HeapAllocBytes == 0 || resp.TotalAllocBytes == 0 {
			t.Errorf("gc=%v: zero heap/total alloc: %+v", gc, resp)
		}
		if resp.NumGoroutine <= 0 {
			t.Errorf("gc=%v: NumGoroutine = %d", gc, resp.NumGoroutine)
		}
		if resp.GCForced != gc {
			t.Errorf("gc=%v: GCForced = %v", gc, resp.GCForced)
		}
	}
}

// TestForcedGCThrottle verifies that back-to-back gc=true calls do not force
// repeated GCs: the second call inside the throttle window still serves a
// snapshot but reports gc_forced=false.
func TestForcedGCThrottle(t *testing.T) {
	r := newTestRouter()

	sample := func() MemStatsResponse {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/debug/memstats?gc=true", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d", w.Code)
		}
		var resp MemStatsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		return resp
	}

	if first := sample(); !first.GCForced {
		t.Error("first gc=true call should force a GC")
	}
	if second := sample(); second.GCForced {
		t.Error("immediate second gc=true call should be throttled (gc_forced=false)")
	}

	// The heap-profile path shares the GC throttle (reported via header) and
	// additionally throttles profile serialization itself: a first profile
	// serves (with the GC skipped), an immediate second one is rejected.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/debug/pprof/heap?gc=true", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("profile status %d", w.Code)
	}
	if got := w.Header().Get("X-Debug-GC-Forced"); got != "false" {
		t.Errorf("X-Debug-GC-Forced = %q, want \"false\" within throttle window", got)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("immediate second profile: status %d, want 429", w.Code)
	}
}

func TestGetHeapProfile(t *testing.T) {
	r := newTestRouter()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/debug/pprof/heap?gc=true", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.Bytes()
	if len(body) == 0 {
		t.Fatal("empty profile body")
	}
	// pprof.WriteTo(w, 0) emits gzipped protobuf — check the gzip magic so a
	// silent format change (e.g. debug>0 text output) fails loudly.
	if body[0] != 0x1f || body[1] != 0x8b {
		t.Errorf("profile is not gzip data (first bytes: %x %x)", body[0], body[1])
	}
}
