// Package debug exposes runtime memory diagnostics for a running instance:
// a memstats snapshot and a pprof heap profile. They exist so memory can be
// observed per process over the same HTTP surface real deployments run —
// the duo harness samples both of its tingly-box instances through these
// endpoints, and the same endpoints serve live incident diagnosis.
package debug

import (
	"net/http"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// minForcedGCInterval caps how often callers can force a full GC. A forced
// double GC is the one genuinely expensive operation here (stop-the-world on
// the live heap), and it is exposed to whoever holds the user token — the
// throttle bounds the worst case (a polling client) to one extra GC per
// second instead of a GC storm, while leaving normal diagnostic cadence
// (samples seconds apart) untouched.
const minForcedGCInterval = time.Second

// Handler serves the runtime memory diagnostics endpoints.
type Handler struct {
	mu          sync.Mutex
	lastForceGC time.Time
}

// NewHandler returns a Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// tryForceGC runs two GC cycles (so finalizer-revived objects are collected
// too, giving a stable post-GC retained set) unless a forced GC ran within
// minForcedGCInterval. Returns whether the GC actually ran; a throttled call
// still serves its snapshot, just without forcing.
func (h *Handler) tryForceGC() bool {
	h.mu.Lock()
	if time.Since(h.lastForceGC) < minForcedGCInterval {
		h.mu.Unlock()
		return false
	}
	h.lastForceGC = time.Now()
	h.mu.Unlock()
	runtime.GC()
	runtime.GC()
	return true
}

// GetMemStats returns a runtime.MemStats snapshot. With ?gc=true it forces
// a full GC first (subject to the throttle), so heap_alloc_bytes is the
// post-GC retained set; gc_forced reports whether the GC actually ran.
func (h *Handler) GetMemStats(c *gin.Context) {
	gcForced := c.Query("gc") == "true" && h.tryForceGC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	c.JSON(http.StatusOK, MemStatsResponse{
		HeapAllocBytes:  m.HeapAlloc,
		HeapInuseBytes:  m.HeapInuse,
		HeapSysBytes:    m.HeapSys,
		HeapObjects:     m.HeapObjects,
		TotalAllocBytes: m.TotalAlloc,
		NumGC:           m.NumGC,
		NumGoroutine:    runtime.NumGoroutine(),
		GCForced:        gcForced,
	})
}

// GetHeapProfile streams a pprof heap profile (gzipped protobuf, the format
// `go tool pprof` reads). With ?gc=true it forces a full GC first (subject
// to the throttle; the X-Debug-GC-Forced header reports whether it ran) so
// the profile reflects retained memory rather than garbage awaiting
// collection.
func (h *Handler) GetHeapProfile(c *gin.Context) {
	if c.Query("gc") == "true" {
		c.Header("X-Debug-GC-Forced", boolStr(h.tryForceGC()))
	}
	profile := pprof.Lookup("heap")
	if profile == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "heap profile unavailable"})
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", `attachment; filename="heap.pb.gz"`)
	if err := profile.WriteTo(c.Writer, 0); err != nil {
		// Headers are already sent; nothing better to do than log via gin.
		_ = c.Error(err)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
