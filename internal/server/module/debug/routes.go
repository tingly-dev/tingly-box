package debug

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"

	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers the runtime memory diagnostics routes under the
// given /api/v1 group. Auth middleware is attached per-route via
// WithMiddleware so this registration does not mutate the parent group's
// middleware chain.
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router, authMiddleware := rt.V1, rt.UserAuth
	router.GET("/debug/memstats", h.GetMemStats,
		swagger.WithTags("debug"),
		swagger.WithDescription("Runtime memory statistics snapshot. Pass gc=true to force a full GC first so heap_alloc_bytes is the post-GC retained set; forced GCs are throttled (min 1s apart) and gc_forced reports whether one actually ran."),
		swagger.WithResponseModel(MemStatsResponse{}),
		swagger.WithMiddleware(authMiddleware),
	)
	router.GET("/debug/pprof/heap", h.GetHeapProfile,
		swagger.WithTags("debug"),
		swagger.WithDescription("pprof heap profile (gzipped protobuf for `go tool pprof`). Pass gc=true to force a full GC first so the profile reflects retained memory; forced GCs are throttled (min 1s apart), reported via the X-Debug-GC-Forced header. Profile serialization itself is also throttled (min 1s apart, 429 with Retry-After when exceeded)."),
		swagger.WithMiddleware(authMiddleware),
	)
}
