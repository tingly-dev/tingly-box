package protocoltest

// Regression tests over the Duo two-process environment — topology and
// route matrix are documented on the engine in duo.go. TestDuoFunctional
// covers every route; TestDuoMemoryRegression guards the #1255 class of leak
// per instance (numbers with the threshold below); TestDuoBackpressure runs
// the slow-stream + slow-reader variant. Heap profiles are written when
// OOM_PROFILE_DIR is set:
//
//	OOM_PROFILE_DIR=/tmp go test ./internal/protocoltest/ -run TestDuo -v
//
// TestMain re-executes this test binary as the duo child servers (see
// MaybeRunDuoServe), so each tb runs as a real, separately-observable
// process booted through server.Start.

import (
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	MaybeRunDuoServe()
	os.Exit(m.Run())
}

// The #1255 leak measured 823 KB/request on the gateway instance. Healthy
// builds measure ~0.5 KB/request; 32 KB leaves generous headroom against GC
// noise while still catching any per-request pin of a request-body-sized
// buffer. Applied to each instance separately: tb2 guards the
// gateway/conversion path, tb1 the vmodel serving path.
const duoMaxSlopeKB = 32.0

func assertDuoSlopes(t *testing.T, report *DuoMemoryReport) {
	t.Helper()
	for _, im := range report.Instances() {
		t.Logf("%s | route %s | baseline %.2f MB | slope %.1f KB/request | churn %.2f MB/request | burst peak %.2f MB (post-GC %+.2f MB) | goroutines %d→%d",
			im.Instance, report.Route, im.BaselineHeapMB, im.SlopeKBPerRequest,
			im.ChurnMBPerRequest, im.PeakHeapMB, im.PostBurstDeltaMB,
			im.BaselineGoroutines, im.FinalGoroutines)
		if im.BaselineProfile != "" {
			t.Logf("%s profiles: %s %s", im.Instance, im.BaselineProfile, im.FinalProfile)
		}
		if im.SlopeKBPerRequest > duoMaxSlopeKB {
			t.Errorf("%s post-GC retention slope %.1f KB/request exceeds %.0f KB/request — a per-request memory pin (see #1255)",
				im.Instance, im.SlopeKBPerRequest, duoMaxSlopeKB)
		}
	}
}

func TestDuoFunctional(t *testing.T) {
	if testing.Short() {
		t.Skip("duo e2e is not a -short test")
	}
	env, err := NewDuoEnv(DuoEnvConfig{})
	if err != nil {
		t.Fatalf("boot duo env: %v", err)
	}
	defer env.Close()

	for _, route := range AllDuoRoutes() {
		route := route
		t.Run(route.Name, func(t *testing.T) {
			checks := env.RunFunctionalChecks(route, 256*1024)
			if len(checks) == 0 {
				t.Fatal("no functional checks ran")
			}
			for _, c := range checks {
				if !c.Pass {
					t.Errorf("check %s failed: %s", c.Name, c.Detail)
				} else {
					t.Logf("check %s ok %s", c.Name, c.Detail)
				}
			}
		})
	}
}

func TestDuoMemoryRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("duo e2e is not a -short test")
	}
	env, err := NewDuoEnv(DuoEnvConfig{})
	if err != nil {
		t.Fatalf("boot duo env: %v", err)
	}
	defer env.Close()

	report, err := env.RunMemoryPhase(DuoMemoryConfig{
		ProfileDir: os.Getenv("OOM_PROFILE_DIR"),
		Progress:   t.Logf,
	})
	if err != nil {
		t.Fatalf("memory phase: %v", err)
	}
	assertDuoSlopes(t, report)
}

// TestDuoBackpressure runs the slow-stream + slow-reader variant of the
// Claude Code hot path: tb1 streams a large response slowly while the client
// reads slowly, so buffering under real TCP backpressure is on the measured
// path. Parameters are kept small to bound test wall time.
func TestDuoBackpressure(t *testing.T) {
	if testing.Short() {
		t.Skip("duo e2e is not a -short test")
	}
	env, err := NewDuoEnv(DuoEnvConfig{StreamKB: 64, StreamMS: 150})
	if err != nil {
		t.Fatalf("boot duo env: %v", err)
	}
	defer env.Close()

	route := DuoDefaultRoute.SlowVariant()
	report, err := env.RunMemoryPhase(DuoMemoryConfig{
		Route:      &route,
		BodyBytes:  512 * 1024,
		Batch:      6,
		Workers:    3,
		PerWorker:  3,
		ReadDelay:  10 * time.Millisecond,
		ProfileDir: os.Getenv("OOM_PROFILE_DIR"),
		Progress:   t.Logf,
	})
	if err != nil {
		t.Fatalf("memory phase: %v", err)
	}
	assertDuoSlopes(t, report)
}
