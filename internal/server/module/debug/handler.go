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

	"github.com/gin-gonic/gin"
)

// Handler serves the runtime memory diagnostics endpoints.
type Handler struct{}

// NewHandler returns a Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// forceGC runs two GC cycles so finalizer-revived objects are collected too,
// giving a stable post-GC retained set (the same double-GC that in-process
// retention measurements use).
func forceGC() {
	runtime.GC()
	runtime.GC()
}

// GetMemStats returns a runtime.MemStats snapshot. With ?gc=true it forces
// a full GC first, so heap_alloc_bytes is the post-GC retained set.
func (h *Handler) GetMemStats(c *gin.Context) {
	gcForced := c.Query("gc") == "true"
	if gcForced {
		forceGC()
	}
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
// `go tool pprof` reads). With ?gc=true it forces a full GC first so the
// profile reflects retained memory rather than garbage awaiting collection.
func (h *Handler) GetHeapProfile(c *gin.Context) {
	if c.Query("gc") == "true" {
		forceGC()
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
