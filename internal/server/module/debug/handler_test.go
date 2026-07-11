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
