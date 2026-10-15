package provider

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/catalog"
	"github.com/tingly-dev/tingly-box/internal/config"
)

// The browser must not hold a cached list longer than the server would: the
// expiry follows the upstream fetch time, not the response time.
func TestModelListExpiry_FollowsFetchTime(t *testing.T) {
	fetched := time.Now().Add(-50 * time.Minute)
	got := modelListExpiry(config.ResolvedModels{Source: config.ModelListSourceCache, LastUpdatedAt: fetched})
	if want := fetched.Add(catalog.ModelCacheTTL); !got.Equal(want) {
		t.Fatalf("expiry = %v, want %v", got, want)
	}

	// An already-expired (stale) list gets a short grace, not another full TTL.
	stale := modelListExpiry(config.ResolvedModels{Source: config.ModelListSourceCache, LastUpdatedAt: time.Now().Add(-3 * time.Hour)})
	if stale.After(time.Now().Add(2 * time.Minute)) {
		t.Fatalf("stale expiry too far out: %v", stale)
	}
}
