package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestFirstChunkGate_BufferCaptureBeforeCommit(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)

	g.WriteHeader(500)
	if _, err := g.WriteString(`{"error":"boom"}`); err != nil {
		t.Fatal(err)
	}

	if g.Committed() {
		t.Fatal("gate must not be committed before an explicit commit signal")
	}
	if g.Status() != 500 {
		t.Fatalf("Status() = %d, want 500", g.Status())
	}
	if g.Size() == 0 {
		t.Fatal("Size() = 0 after write")
	}
	if rec.Code != 200 {
		t.Fatalf("real recorder code = %d, want 200 (nothing flushed yet)", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("real recorder body should be empty before commit, got %q", rec.Body.String())
	}
}

func TestFirstChunkGate_CommitFirstChunkFlushesThenPassesThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)

	g.Header().Set("Content-Type", "text/event-stream")
	if _, err := g.WriteString("event: first\n\n"); err != nil {
		t.Fatal(err)
	}

	// Producer signals the first real chunk arrived.
	g.CommitFirstChunk()

	if !g.Committed() {
		t.Fatal("gate must be committed after CommitFirstChunk")
	}
	if rec.Code != 200 {
		t.Fatalf("committed code = %d, want 200 (default)", rec.Code)
	}
	if rec.Body.String() != "event: first\n\n" {
		t.Fatalf("flushed body = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("flushed Content-Type missing: %v", rec.Header())
	}

	// Subsequent writes pass straight through to the real writer.
	if _, err := g.WriteString("event: second\n\n"); err != nil {
		t.Fatal(err)
	}
	g.Flush()
	if rec.Body.String() != "event: first\n\nevent: second\n\n" {
		t.Fatalf("pass-through body = %q", rec.Body.String())
	}
}

func TestFirstChunkGate_CommitIfBufferedTerminalError(t *testing.T) {
	// The orchestrator's deferred terminal flush: an uncommitted buffered
	// error (the last failure after retries) reaches the wire.
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	g.Header().Set("X-Test", "yes")
	g.WriteHeader(503)
	_, _ = g.WriteString("oh no")

	if g.Committed() {
		t.Fatal("503 must not commit on its own")
	}
	g.CommitIfBuffered()

	if rec.Code != 503 {
		t.Fatalf("committed code = %d, want 503", rec.Code)
	}
	if rec.Body.String() != "oh no" {
		t.Fatalf("committed body = %q, want %q", rec.Body.String(), "oh no")
	}
	if rec.Header().Get("X-Test") != "yes" {
		t.Fatalf("committed header missing: %v", rec.Header())
	}
}

func TestFirstChunkGate_CommitIfBufferedNoopWhenUntouched(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)

	g.CommitIfBuffered() // nothing buffered → must be a no-op

	if g.Committed() {
		t.Fatal("untouched gate must not commit")
	}
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("untouched gate leaked to wire: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestFirstChunkGate_CommitIfBufferedNoopAfterCommit(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	_, _ = g.WriteString("chunk")
	g.CommitFirstChunk()

	// A second terminal flush must not double-write.
	g.CommitIfBuffered()
	if rec.Body.String() != "chunk" {
		t.Fatalf("double-write detected: %q", rec.Body.String())
	}
}

func TestFirstChunkGate_DiscardResetsThenRetry(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	g.Header().Set("X-Try-1", "yes")
	g.WriteHeader(429)
	_, _ = g.WriteString("rate limited")
	g.Discard()

	if g.Status() != 0 {
		t.Fatalf("after Discard Status() = %d, want 0 (reset/untouched)", g.Status())
	}
	if g.Size() != 0 {
		t.Fatalf("after Discard Size() = %d, want 0", g.Size())
	}
	if got := g.Header().Get("X-Try-1"); got != "" {
		t.Fatalf("after Discard header still present: %q", got)
	}

	// Next attempt succeeds and commits a fresh response.
	_, _ = g.WriteString(`{"ok":true}`)
	g.CommitFirstChunk()

	if rec.Code != 200 {
		t.Fatalf("final code = %d, want 200", rec.Code)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("final body = %q, want fresh ok body", rec.Body.String())
	}
	if rec.Header().Get("X-Try-1") != "" {
		t.Fatalf("stale header leaked through Discard")
	}
}

func TestFirstChunkGate_DiscardNoopAfterCommit(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	_, _ = g.WriteString("hello")
	g.CommitFirstChunk()

	g.Discard() // committed → no-op, bytes already on the wire
	if rec.Body.String() != "hello" {
		t.Fatalf("Discard after commit must not affect wire: got %q", rec.Body.String())
	}
}

func TestFirstChunkGate_CommitFirstChunkIdempotent(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	_, _ = g.WriteString("once")
	g.CommitFirstChunk()
	g.CommitFirstChunk() // second call must not re-flush the buffer

	if rec.Body.String() != "once" {
		t.Fatalf("idempotency broken, body = %q", rec.Body.String())
	}
}

func TestFirstChunkGate_StatusDefaults(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)

	if g.Status() != 0 {
		t.Fatalf("untouched Status() = %d, want 0", g.Status())
	}
	_, _ = g.WriteString("body")
	if g.Status() != 200 {
		t.Fatalf("buffered Status() = %d, want 200 default", g.Status())
	}
}

func TestFirstChunkGate_FlushNoopUntilCommitted(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	_, _ = g.WriteString("buffered")
	g.Flush() // must not leak buffered bytes before commit

	if rec.Body.Len() != 0 {
		t.Fatalf("Flush leaked buffered bytes before commit: %q", rec.Body.String())
	}

	g.CommitFirstChunk()
	if rec.Body.String() != "buffered" {
		t.Fatalf("post-commit body = %q", rec.Body.String())
	}
}

func TestIsRetryableStatus(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{0, false},   // writer never written → terminal, no retry
		{200, false}, // 2xx → success
		{201, false},
		{400, false}, // 4xx (not 429) → client error, don't retry
		{401, false},
		{403, false},
		{404, false},
		{422, false},
		{429, true}, // rate limit
		{500, true}, // includes our own SendErrorResponse on forwarding errors
		{502, true},
		{503, true},
		{504, true},
		// The whole 5xx range is retryable: error forwarding propagates the
		// upstream's status verbatim, so provider-specific codes must not
		// slip through the failover net. 529 is Anthropic's overloaded_error
		// (the original escape); 52x also covers Cloudflare-fronted providers.
		{529, true},
		{520, true},
		{599, true},
		{501, true}, // heterogeneous fallback may well implement what this tier didn't
	}
	for _, tc := range cases {
		if got := isRetryableStatus(tc.code); got != tc.want {
			t.Errorf("isRetryableStatus(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestFirstChunkGate_HeaderOnlyResponseCommits(t *testing.T) {
	// A status-only response (e.g. 204) still flushes via CommitIfBuffered.
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	g := newFirstChunkGate(c.Writer)
	g.WriteHeader(http.StatusNoContent)

	if g.Status() != http.StatusNoContent {
		t.Fatalf("Status() = %d, want %d", g.Status(), http.StatusNoContent)
	}
	g.CommitIfBuffered()
	if rec.Code != http.StatusNoContent {
		t.Fatalf("committed code = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestSelectFallbackService_RespectsTierOrdering(t *testing.T) {
	// Test that TierTactic.SelectService respects tier ordering.
	// This indirectly validates that selectFallbackService will use
	// tier-aware selection when the rule has tier-based routing.
	//
	// This test verifies the fix for the bug where selectFallbackService
	// was using the generic load balancer instead of tier-aware selection,
	// causing lower-tier services to be selected before higher-tier ones.

	// Create a tier tactic with random within-tier selection
	tierTactic := typ.NewTierTactic(loadbalance.TacticRandom)

	// Create a rule with tier-based routing and multiple providers per tier
	// T0: 2 providers (provider-t0a, provider-t0b)
	// T1: 1 provider  (provider-t1)
	// T2: 2 providers (provider-t2a, provider-t2b)
	rule := &typ.Rule{
		UUID: "test-tier-rule",
		LBTactic: typ.Tactic{
			Type: loadbalance.TacticTier,
			Params: &typ.TierParams{
				WithinTierTactic: loadbalance.TacticRandom,
			},
		},
		Services: []*loadbalance.Service{
			{Provider: "provider-t0a", Model: "gpt-4", Active: true, Tier: 0},
			{Provider: "provider-t0b", Model: "gpt-4", Active: true, Tier: 0},
			{Provider: "provider-t1", Model: "gpt-4", Active: true, Tier: 1},
			{Provider: "provider-t2a", Model: "gpt-4", Active: true, Tier: 2},
			{Provider: "provider-t2b", Model: "gpt-4", Active: true, Tier: 2},
		},
	}

	// Test 1: All services available - should pick from T0 (highest priority, tier=0)
	service := tierTactic.SelectService(rule)
	if service == nil {
		t.Fatal("TierTactic.SelectService returned nil with all services available")
	}
	if service.Tier != 0 {
		t.Fatalf("with all services available, picked tier %d, want T0 (0)", service.Tier)
	}
	if service.Provider != "provider-t0a" && service.Provider != "provider-t0b" {
		t.Fatalf("with all services available, picked provider %s, want one of T0 providers", service.Provider)
	}

	// Test 2: All T0 providers disabled - should pick from T1 (tier=1)
	rule.Services[0].Active = false
	rule.Services[1].Active = false
	service = tierTactic.SelectService(rule)
	if service == nil {
		t.Fatal("TierTactic.SelectService returned nil with T0 disabled")
	}
	if service.Tier != 1 {
		t.Fatalf("with T0 disabled, picked tier %d, want T1 (1)", service.Tier)
	}
	if service.Provider != "provider-t1" {
		t.Fatalf("with T0 disabled, picked provider %s, want provider-t1", service.Provider)
	}

	// Test 3: T0 and T1 disabled - should pick from T2 (tier=2)
	rule.Services[2].Active = false
	service = tierTactic.SelectService(rule)
	if service == nil {
		t.Fatal("TierTactic.SelectService returned nil with T0,T1 disabled")
	}
	if service.Tier != 2 {
		t.Fatalf("with T0,T1 disabled, picked tier %d, want T2 (2)", service.Tier)
	}
	if service.Provider != "provider-t2a" && service.Provider != "provider-t2b" {
		t.Fatalf("with T0,T1 disabled, picked provider %s, want one of T2 providers", service.Provider)
	}

	// Test 4: One T2 provider disabled - should still pick from T2 (the remaining one)
	rule.Services[3].Active = false
	service = tierTactic.SelectService(rule)
	if service == nil {
		t.Fatal("TierTactic.SelectService returned nil with only one T2 provider")
	}
	if service.Tier != 2 {
		t.Fatalf("with only one T2 provider, picked tier %d, want T2 (2)", service.Tier)
	}
	if service.Provider != "provider-t2b" {
		t.Fatalf("with only T2b available, picked provider %s, want provider-t2b", service.Provider)
	}

	// Test 5: All services disabled - should return nil (no active services)
	rule.Services[4].Active = false
	service = tierTactic.SelectService(rule)
	if service != nil {
		t.Fatalf("with all services disabled, expected nil, got %v", service)
	}
}

func TestSelectFallbackService_TierVsRandomTactic(t *testing.T) {
	// Compare tier-based vs random selection to ensure they behave differently.
	// This validates that selectFallbackService uses the correct tactic type.
	// Test with multiple providers per tier to ensure within-tier selection works.

	services := []*loadbalance.Service{
		{Provider: "provider-a1", Model: "gpt-4", Active: true, Tier: 1}, // Lower priority (tier 1)
		{Provider: "provider-a2", Model: "gpt-4", Active: true, Tier: 1}, // Also tier 1
		{Provider: "provider-b1", Model: "gpt-4", Active: true, Tier: 0}, // Higher priority (tier 0)
		{Provider: "provider-b2", Model: "gpt-4", Active: true, Tier: 0}, // Also tier 0
	}

	tierRule := &typ.Rule{
		UUID: "test-tier",
		LBTactic: typ.Tactic{
			Type: loadbalance.TacticTier,
			Params: &typ.TierParams{
				WithinTierTactic: loadbalance.TacticRandom,
			},
		},
		Services: services,
	}

	randomRule := &typ.Rule{
		UUID: "test-random",
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: &typ.RandomParams{},
		},
		Services: services,
	}

	// Tier tactic should prefer lower tier (tier 0 = provider-b1 or provider-b2)
	tierTactic := typ.NewTierTactic(loadbalance.TacticRandom)
	tierService := tierTactic.SelectService(tierRule)
	if tierService == nil {
		t.Fatal("TierTactic.SelectService returned nil")
	}
	// Tier 0 should be selected over tier 1
	if tierService.Tier != 0 {
		t.Errorf("TierTactic picked tier %d, expected tier 0 (higher priority)", tierService.Tier)
	}
	if tierService.Provider != "provider-b1" && tierService.Provider != "provider-b2" {
		t.Errorf("TierTactic picked provider %s, expected one of tier 0 providers (b1/b2)", tierService.Provider)
	}

	// Random tactic could pick any of the 4 providers
	randomTactic := typ.NewRandomTactic()
	randomService := randomTactic.SelectService(randomRule)
	if randomService == nil {
		t.Fatal("RandomTactic.SelectService returned nil")
	}
	// Random tactic doesn't guarantee tier ordering
	validProviders := []string{"provider-a1", "provider-a2", "provider-b1", "provider-b2"}
	found := false
	for _, valid := range validProviders {
		if randomService.Provider == valid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RandomTactic picked unexpected provider: %s", randomService.Provider)
	}

	// The key assertion: tier tactic respects ordering (always picks tier 0)
	// Run multiple times to verify consistency
	for i := 0; i < 10; i++ {
		tierService = tierTactic.SelectService(tierRule)
		if tierService.Tier != 0 {
			t.Errorf("TierTactic iteration %d: picked tier %d instead of 0", i, tierService.Tier)
		}
	}
}
