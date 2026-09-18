package visionproxy

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Three distinct images; the cache key hashes the base64 text, so any
// distinct strings will do for the fake client.
const (
	imgOld = "T0xE" // "OLD"
	imgMid = "TUlE" // "MID"
	imgNew = "TkVX" // "NEW"
)

// TestVisionProxy_DescribeLimit_ConvergesOverTurns is the contract behind
// bounding instead of describing everything: each turn spends its slots on
// the newest cache misses, and because every described image is cached for
// the session, a fully uncached history is covered within a few turns —
// after which the request costs nothing and its text is byte-stable.
func TestVisionProxy_DescribeLimit_ConvergesOverTurns(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("desc new", "desc mid", "desc old")
	p := mkProcessor(t, fake, prov)
	p.describeLimit = 1
	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	session := typ.SessionID{Value: "session-a"}

	history := func() *anthropic.BetaMessageNewParams {
		return betaReqWithMessages(
			betaMessage(anthropic.BetaMessageParamRoleUser, "turn 1", imgOld),
			betaMessage(anthropic.BetaMessageParamRoleUser, "turn 2", imgMid),
			betaMessage(anthropic.BetaMessageParamRoleUser, "turn 3", imgNew),
		)
	}

	// Turn A: proxy just enabled on a conversation with three uncached
	// images. One slot → the newest is described, the two older deferred.
	reqA := history()
	require.NoError(t, p.Process(context.Background(), reqA, svcs, session))
	require.Equal(t, 1, fake.callCount())
	require.Contains(t, blockText(reqA.Messages[2]), "desc new")
	require.Contains(t, blockText(reqA.Messages[1]), imageOverLimitText)
	require.Contains(t, blockText(reqA.Messages[0]), imageOverLimitText)

	// Turn B: newest hits the cache and costs no slot; the slot goes to the
	// next-newest miss.
	reqB := history()
	require.NoError(t, p.Process(context.Background(), reqB, svcs, session))
	require.Equal(t, 2, fake.callCount())
	require.Contains(t, blockText(reqB.Messages[2]), "desc new")
	require.Contains(t, blockText(reqB.Messages[1]), "desc mid")
	require.Contains(t, blockText(reqB.Messages[0]), imageOverLimitText)

	// Turn C: the last miss is described; history is now fully covered.
	reqC := history()
	require.NoError(t, p.Process(context.Background(), reqC, svcs, session))
	require.Equal(t, 3, fake.callCount())
	require.Contains(t, blockText(reqC.Messages[0]), "desc old")

	// Turn D: converged — no upstream call, identical text to turn C.
	reqD := history()
	require.NoError(t, p.Process(context.Background(), reqD, svcs, session))
	require.Equal(t, 3, fake.callCount(), "fully cached history costs nothing")
	require.Equal(t, collectText(reqC), collectText(reqD), "text is byte-stable once converged")
}

// TestVisionProxy_DescribeLimit_CacheHitsDoNotConsumeSlots: only misses
// count against the limit, so a request full of already-described images
// still has all its slots for genuinely new ones.
func TestVisionProxy_DescribeLimit_CacheHitsDoNotConsumeSlots(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("desc old", "desc mid", "desc new")
	p := mkProcessor(t, fake, prov)
	p.describeLimit = 1
	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	session := typ.SessionID{Value: "session-a"}

	require.NoError(t, p.Process(context.Background(),
		betaReqWithMessages(betaMessage(anthropic.BetaMessageParamRoleUser, "t1", imgOld)), svcs, session))
	require.NoError(t, p.Process(context.Background(),
		betaReqWithMessages(betaMessage(anthropic.BetaMessageParamRoleUser, "t2", imgMid)), svcs, session))
	require.Equal(t, 2, fake.callCount())

	req := betaReqWithMessages(
		betaMessage(anthropic.BetaMessageParamRoleUser, "t1", imgOld),
		betaMessage(anthropic.BetaMessageParamRoleUser, "t2", imgMid),
		betaMessage(anthropic.BetaMessageParamRoleUser, "t3", imgNew),
	)
	require.NoError(t, p.Process(context.Background(), req, svcs, session))
	require.Equal(t, 3, fake.callCount(), "two hits, one miss: the miss gets the single slot")
	require.NotContains(t, collectText(req), imageOverLimitText)
}

// TestBoundNewestFirst pins the ranking itself, independent of the
// concurrent fan-out: the newest misses are kept, in newest-first order,
// so they are dispatched first; everything older is deferred.
func TestBoundNewestFirst(t *testing.T) {
	mk := func(names ...string) []imageRef {
		out := make([]imageRef, 0, len(names))
		for _, n := range names {
			out = append(out, imageRef{b64: n, cacheKey: visionCacheKey{content: n}, splice: func(string) {}})
		}
		return out
	}
	b64s := func(refs []imageRef) []string {
		out := make([]string, 0, len(refs))
		for _, r := range refs {
			out = append(out, r.b64)
		}
		return out
	}

	keep, deferred := boundNewestFirst(mk("a", "b", "c", "d"), 2)
	require.Equal(t, []string{"d", "c"}, b64s(keep), "newest two, newest first")
	require.Equal(t, []string{"b", "a"}, b64s(deferred), "older two deferred")

	keep, deferred = boundNewestFirst(mk("a", "b"), 8)
	require.Equal(t, []string{"b", "a"}, b64s(keep), "under the limit everything is kept, still newest first")
	require.Empty(t, deferred)

	keep, deferred = boundNewestFirst(nil, 8)
	require.Empty(t, keep)
	require.Empty(t, deferred)
}

func TestDescribeLimitFor(t *testing.T) {
	p := &VisionProxyProcessor{}

	t.Setenv(describeLimitEnv, "")
	require.Equal(t, defaultDescribeLimit, p.describeLimitFor())

	t.Setenv(describeLimitEnv, " 16\n")
	require.Equal(t, 16, p.describeLimitFor())

	for _, bad := range []string{"nonsense", "0", "-1", "2.5"} {
		t.Setenv(describeLimitEnv, bad)
		require.Equal(t, defaultDescribeLimit, p.describeLimitFor(), "bad value %q must fall back", bad)
	}

	t.Setenv(describeLimitEnv, "16")
	p.describeLimit = 3
	require.Equal(t, 3, p.describeLimitFor(), "an explicit processor value wins over the environment")
}

// TestVisionProxy_NoUsableService_StripsAllUniformly: without a vision
// service the limit does not apply — every image is a fail-strip, none is
// told it is merely "not yet described".
func TestVisionProxy_NoUsableService_StripsAllUniformly(t *testing.T) {
	fake := newFakeVisionClient("never called")
	p := mkProcessor(t, fake) // no providers → nothing resolves
	p.describeLimit = 1

	req := betaReqWithMessages(
		betaMessage(anthropic.BetaMessageParamRoleUser, "t1", imgOld),
		betaMessage(anthropic.BetaMessageParamRoleUser, "t2", imgMid),
		betaMessage(anthropic.BetaMessageParamRoleUser, "t3", imgNew),
	)
	svcs := []*loadbalance.Service{mkService("missing-provider", true)}
	require.NoError(t, p.Process(context.Background(), req, svcs, typ.SessionID{Value: "s"}))
	require.Equal(t, 0, fake.callCount())
	require.Equal(t, 0, countImages(req))
	text := collectText(req)
	require.NotContains(t, text, imageOverLimitText, "nothing is deferred when nothing can be described")
	require.Equal(t, 3, strings.Count(text, imageUnavailableText), "every image is fail-stripped")
}

// TestVisionProxy_DescribeLimit_FailedImageDoesNotHoldSlot is the starvation
// case the negative cache exists for: with one slot and a newest image that
// always fails, the older image would never be reached. After the first
// failure the newest is fail-stripped from the negative cache and the slot
// goes to the older one.
func TestVisionProxy_DescribeLimit_FailedImageDoesNotHoldSlot(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("", "desc old")
	fake.failCall(0, errors.New("dead link"))
	p := mkProcessor(t, fake, prov)
	p.describeLimit = 1
	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	session := typ.SessionID{Value: "session-a"}

	history := func() *anthropic.BetaMessageNewParams {
		return betaReqWithMessages(
			betaMessage(anthropic.BetaMessageParamRoleUser, "t1", imgOld),
			betaMessage(anthropic.BetaMessageParamRoleUser, "t2", imgNew),
		)
	}

	reqA := history()
	require.NoError(t, p.Process(context.Background(), reqA, svcs, session))
	require.Equal(t, 1, fake.callCount(), "the newest takes the slot and fails")
	require.Contains(t, blockText(reqA.Messages[1]), imageUnavailableText)
	require.Contains(t, blockText(reqA.Messages[0]), imageOverLimitText)

	reqB := history()
	require.NoError(t, p.Process(context.Background(), reqB, svcs, session))
	require.Equal(t, 2, fake.callCount(), "the failed newest no longer holds the slot; the older is described")
	require.Contains(t, blockText(reqB.Messages[1]), imageUnavailableText)
	require.Contains(t, blockText(reqB.Messages[0]), "desc old")
}

// TestVisionProxy_DuplicateImageInOneRequest_DescribedOnce: the same bytes
// in two positions of one request are described once and both positions
// get the identical text — so neither changes on the next turn.
func TestVisionProxy_DuplicateImageInOneRequest_DescribedOnce(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	fake := newFakeVisionClient("the one description")
	p := mkProcessor(t, fake, prov)
	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	session := typ.SessionID{Value: "session-a"}

	req := betaReqWithMessages(
		betaMessage(anthropic.BetaMessageParamRoleUser, "screenshot A", imgNew),
		betaMessage(anthropic.BetaMessageParamRoleUser, "same screenshot again", imgNew),
	)
	require.NoError(t, p.Process(context.Background(), req, svcs, session))
	require.Equal(t, 1, fake.callCount(), "one describe for one distinct image")
	require.Equal(t, 0, countImages(req))
	require.Equal(t, blockText(req.Messages[0]), strings.Replace(blockText(req.Messages[1]), "same screenshot again", "screenshot A", 1),
		"both positions carry the identical replacement text")

	// Next turn: both positions hit the cache, nothing changes.
	again := betaReqWithMessages(
		betaMessage(anthropic.BetaMessageParamRoleUser, "screenshot A", imgNew),
		betaMessage(anthropic.BetaMessageParamRoleUser, "same screenshot again", imgNew),
	)
	require.NoError(t, p.Process(context.Background(), again, svcs, session))
	require.Equal(t, 1, fake.callCount())
	require.Equal(t, collectText(req), collectText(again))
}

// TestVisionProxy_DescribeTimeout_StripsAndNegativeCaches: a hung vision
// upstream is cut off at the per-call timeout, the image is fail-stripped,
// and the failure is remembered so the next turn does not wait again.
func TestVisionProxy_DescribeTimeout_StripsAndNegativeCaches(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	hung := &hangingVisionClient{}
	p := mkProcessor(t, hung, prov)
	p.describeTimeout = 20 * time.Millisecond
	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	session := typ.SessionID{Value: "session-a"}

	req := betaReqWithImages("describe", imgNew)
	start := time.Now()
	require.NoError(t, p.Process(context.Background(), req, svcs, session))
	require.Less(t, time.Since(start), 2*time.Second, "the request must not wait on the hung upstream")
	require.Equal(t, 1, hung.calls())
	require.Contains(t, collectText(req), imageUnavailableText)

	again := betaReqWithImages("describe", imgNew)
	require.NoError(t, p.Process(context.Background(), again, svcs, session))
	require.Equal(t, 1, hung.calls(), "timed-out image is negative-cached; no second wait")
}

// hangingVisionClient blocks until the context is cancelled.
type hangingVisionClient struct {
	mu sync.Mutex
	n  int
}

func (h *hangingVisionClient) Describe(ctx context.Context, _ *loadbalance.Service, _, _, _ string) (string, error) {
	h.mu.Lock()
	h.n++
	h.mu.Unlock()
	<-ctx.Done()
	return "", ctx.Err()
}

func (h *hangingVisionClient) calls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.n
}

func TestDescribeTimeoutFor(t *testing.T) {
	p := &VisionProxyProcessor{}
	t.Setenv(describeTimeoutEnv, "")
	require.Equal(t, defaultDescribeTimeout, p.describeTimeoutFor())
	t.Setenv(describeTimeoutEnv, "90s")
	require.Equal(t, 90*time.Second, p.describeTimeoutFor())
	t.Setenv(describeTimeoutEnv, "45")
	require.Equal(t, 45*time.Second, p.describeTimeoutFor(), "a bare number is seconds")
	for _, bad := range []string{"nonsense", "0", "-5s"} {
		t.Setenv(describeTimeoutEnv, bad)
		require.Equal(t, defaultDescribeTimeout, p.describeTimeoutFor(), "bad value %q must fall back", bad)
	}
	p.describeTimeout = time.Second
	require.Equal(t, time.Second, p.describeTimeoutFor(), "an explicit processor value wins")
}

// TestVisionProxy_CallerCancel_IsNotNegativeCached: a describe cut short by
// the caller's own context (user abort, client retry) is not a failure of
// the image, so the resend must try upstream again instead of reporting a
// proxy error for the TTL.
func TestVisionProxy_CallerCancel_IsNotNegativeCached(t *testing.T) {
	prov := mkProvider("anthropic-vision")
	hung := &hangingVisionClient{}
	p := mkProcessor(t, hung, prov)
	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	session := typ.SessionID{Value: "session-a"}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	req := betaReqWithImages("describe", imgNew)
	require.NoError(t, p.Process(ctx, req, svcs, session))
	require.Equal(t, 1, hung.calls())
	require.Contains(t, collectText(req), imageUnavailableText, "this request still strips the image")

	// The resend, with a live context, goes upstream again.
	ctx2, cancel2 := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel2() }()
	again := betaReqWithImages("describe", imgNew)
	require.NoError(t, p.Process(ctx2, again, svcs, session))
	require.Equal(t, 2, hung.calls(), "a caller-cancelled describe is not remembered as a failure")
}
