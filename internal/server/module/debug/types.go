package debug

// MemStatsResponse is the response body of GET /debug/memstats. Byte fields
// come straight from runtime.MemStats so external observers (the duo memory
// harness, incident diagnosis against a live instance) read the same numbers
// an in-process runtime.ReadMemStats would.
type MemStatsResponse struct {
	// HeapAllocBytes is the live heap (bytes of allocated, not yet freed
	// objects). Sampled after a forced GC (gc=true) it is the post-GC
	// retained set — the number retention-slope measurements diff.
	HeapAllocBytes uint64 `json:"heap_alloc_bytes" example:"10485760"`
	HeapInuseBytes uint64 `json:"heap_inuse_bytes" example:"12582912"`
	HeapSysBytes   uint64 `json:"heap_sys_bytes" example:"16777216"`
	HeapObjects    uint64 `json:"heap_objects" example:"52000"`
	// TotalAllocBytes is cumulative bytes allocated since process start
	// (monotonic); diffs of it measure allocation churn.
	TotalAllocBytes uint64 `json:"total_alloc_bytes" example:"104857600"`
	NumGC           uint32 `json:"num_gc" example:"12"`
	NumGoroutine    int    `json:"num_goroutine" example:"42"`
	// GCForced reports whether this sample ran a forced GC first (gc=true).
	GCForced bool `json:"gc_forced" example:"true"`
}
