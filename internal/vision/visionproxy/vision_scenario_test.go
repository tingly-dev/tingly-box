package visionproxy

// Scenario tests: the vision proxy driven end to end through Service.Apply
// (rule/scenario resolution included) with the message shape Claude Code
// actually sends, across several turns. Each test is one row of the
// verification matrix in .design/vision-proxy.md §11; keep the two in sync.
//
// The fake vision client returns a different wording on every call, so any
// re-describe shows up as a text change — the property under test is that
// the text a position carries never changes once described.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// claudeCodeSession builds the request Claude Code sends on turn N of a
// tool loop: the prompt, a system reminder, then for every screenshot so
// far an assistant tool_use, a user tool_result carrying the image, and a
// trailing system reminder. images are oldest first.
func claudeCodeSession(images ...string) *anthropic.BetaMessageNewParams {
	sys := func(text string) anthropic.BetaMessageParam {
		return anthropic.BetaMessageParam{
			Role:    anthropic.BetaMessageParamRoleSystem,
			Content: []anthropic.BetaContentBlockParamUnion{{OfText: &anthropic.BetaTextBlockParam{Text: text}}},
		}
	}
	msgs := []anthropic.BetaMessageParam{
		{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: "fix the layout bug"}},
		}},
		sys("<system-reminder>context</system-reminder>"),
	}
	for i, b64 := range images {
		id := fmt.Sprintf("toolu_%d", i)
		msgs = append(msgs,
			anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{
				{OfToolUse: &anthropic.BetaToolUseBlockParam{ID: id, Name: "Screenshot"}},
			}},
			anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{
				{OfToolResult: &anthropic.BetaToolResultBlockParam{
					ToolUseID: id,
					Content: []anthropic.BetaToolResultBlockParamContentUnion{
						{OfImage: &anthropic.BetaImageBlockParam{
							Source: anthropic.BetaImageBlockParamSourceUnion{
								OfBase64: &anthropic.BetaBase64ImageSourceParam{
									Data:      b64,
									MediaType: anthropic.BetaBase64ImageSourceMediaType(tinyPNGMediaType),
								},
							},
						}},
					},
				}},
			}},
			sys("<system-reminder>tokens left</system-reminder>"),
		)
	}
	return &anthropic.BetaMessageNewParams{Model: anthropic.Model("claude-sonnet-4"), Messages: msgs}
}

// toolResultText returns the text spliced into the i-th screenshot's
// tool_result (oldest first) in a claudeCodeSession request.
func toolResultText(req *anthropic.BetaMessageNewParams, i int) string {
	m := req.Messages[2+3*i+1]
	tr := m.Content[0].OfToolResult
	if tr == nil || len(tr.Content) == 0 || tr.Content[0].OfText == nil {
		return "<no text>"
	}
	return tr.Content[0].OfText.Text
}

// wordingVisionClient answers every call with a distinct wording, so a
// re-describe is visible as a text change. It can be told to fail.
type wordingVisionClient struct {
	mu    sync.Mutex
	n     int
	fail  error
	calls []string // model per call
}

func (w *wordingVisionClient) Describe(_ context.Context, svc *loadbalance.Service, _, b64, _ string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.n++
	w.calls = append(w.calls, svc.Model)
	if w.fail != nil {
		return "", w.fail
	}
	return fmt.Sprintf("wording #%d of %s by %s", w.n, b64, svc.Model), nil
}

func (w *wordingVisionClient) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.n
}

func (w *wordingVisionClient) setFail(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fail = err
}

// scenarioService wires a Service the way server boot does — scenario-level
// vision service, real Resolve — over the given store (nil = in-process).
func scenarioService(client VisionClient, store DescribeStore, limit int) *Service {
	return NewService(&VisionProxyProcessor{
		Client:        client,
		Resolver:      alwaysResolvingProvider{},
		cache:         newDescribeCache(store),
		describeLimit: limit,
	})
}

const claudeCodeSessionID = "user_abc_account_def_session_0001"

func apply(t *testing.T, s *Service, model string, req *anthropic.BetaMessageNewParams) {
	t.Helper()
	cfg := testConfig("claude_code", scenarioVisionExt("p-vision", model))
	s.Apply(context.Background(), cfg, "claude_code", &typ.Rule{}, req,
		typ.SessionID{Source: typ.SessionSourceUser, Value: claudeCodeSessionID})
	require.Equal(t, 0, countImages(req), "no image block may reach the downstream model")
}

// S1 — tool loop. Each turn adds one screenshot; earlier screenshots keep
// the exact text they got when first described, and the vision upstream is
// called exactly once per screenshot over the whole conversation.
func TestScenario_ToolLoop_HistoryTextIsStable(t *testing.T) {
	client := &wordingVisionClient{}
	s := scenarioService(client, nil, 0)
	images := []string{"c2NyZWVuMQ==", "c2NyZWVuMg==", "c2NyZWVuMw==", "c2NyZWVuNA=="}

	var seen []string
	for turn := 1; turn <= len(images); turn++ {
		req := claudeCodeSession(images[:turn]...)
		apply(t, s, "vision-a", req)
		require.Equal(t, turn, client.count(), "turn %d: exactly one new describe", turn)

		for i := 0; i < turn-1; i++ {
			require.Equal(t, seen[i], toolResultText(req, i), "turn %d: screenshot %d must keep its turn-%d text", turn, i, i+1)
		}
		latest := toolResultText(req, turn-1)
		require.Contains(t, latest, "wording #"+fmt.Sprint(turn))
		seen = append(seen, latest)
	}
}

// S2 — gateway restart. A new Service over the same database (new process)
// answers the next turn with zero vision calls and byte-identical text.
func TestScenario_Restart_NoRedescribe(t *testing.T) {
	conn := openTestDB(t)
	images := []string{"c2NyZWVuMQ==", "c2NyZWVuMg==", "c2NyZWVuMw=="}

	before := &wordingVisionClient{}
	s1 := scenarioService(before, newTestStore(t, conn), 0)
	last := claudeCodeSession(images...)
	for turn := 1; turn <= len(images); turn++ {
		last = claudeCodeSession(images[:turn]...)
		apply(t, s1, "vision-a", last)
	}
	require.Equal(t, len(images), before.count())

	after := &wordingVisionClient{}
	s2 := scenarioService(after, newTestStore(t, conn), 0)
	next := claudeCodeSession(images...)
	apply(t, s2, "vision-a", next)
	require.Equal(t, 0, after.count(), "after restart the whole history comes from the store")
	require.Equal(t, collectText(last), collectText(next), "text is byte-identical across the restart")
}

// S3 — vision model switched mid-conversation. Old descriptions are not
// reused under the new model; the history is re-described newest first at
// `limit` per turn and settles once every image has been re-described.
func TestScenario_ModelSwitch_ReconvergesBounded(t *testing.T) {
	client := &wordingVisionClient{}
	s := scenarioService(client, nil, 1)
	images := []string{"c2NyZWVuMQ==", "c2NyZWVuMg==", "c2NyZWVuMw=="}

	for turn := 1; turn <= len(images); turn++ {
		apply(t, s, "vision-a", claudeCodeSession(images[:turn]...))
	}
	require.Equal(t, 3, client.count())

	// Switch: three turns to re-describe three images with one slot, newest first.
	var prev string
	for turn := 1; turn <= len(images); turn++ {
		req := claudeCodeSession(images...)
		apply(t, s, "vision-b", req)
		require.Equal(t, 3+turn, client.count(), "switch turn %d: one re-describe", turn)
		require.Equal(t, "vision-b", client.calls[len(client.calls)-1])
		newest := toolResultText(req, len(images)-1)
		require.Contains(t, newest, "by vision-b", "the newest image is re-described first")
		require.NotContains(t, collectText(req), "by vision-a", "nothing from the old model is ever reused")
		prev = collectText(req)
	}
	settled := claudeCodeSession(images...)
	apply(t, s, "vision-b", settled)
	require.Equal(t, 6, client.count(), "fully re-described: no further calls")
	require.Equal(t, prev, collectText(settled))
}

// S4 — proxy enabled on a conversation that already carries screenshots.
// Nothing is cached; the history converges to fully described at `limit`
// per turn, newest first, and the deferred ones are marked, not lost.
func TestScenario_EnabledMidConversation_Converges(t *testing.T) {
	client := &wordingVisionClient{}
	s := scenarioService(client, nil, 2)
	images := []string{"c2NyZWVuMQ==", "c2NyZWVuMg==", "c2NyZWVuMw==", "c2NyZWVuNA==", "c2NyZWVuNQ=="}

	turn1 := claudeCodeSession(images...)
	apply(t, s, "vision-a", turn1)
	require.Equal(t, 2, client.count())
	require.Contains(t, toolResultText(turn1, 4), "wording #")
	require.Contains(t, toolResultText(turn1, 3), "wording #")
	for i := 0; i < 3; i++ {
		require.Equal(t, imageOverLimitText, toolResultText(turn1, i), "older screenshot %d is deferred, not dropped", i)
	}

	turn2 := claudeCodeSession(images...)
	apply(t, s, "vision-a", turn2)
	require.Equal(t, 4, client.count())
	require.Equal(t, toolResultText(turn1, 4), toolResultText(turn2, 4), "already described text is stable")
	require.Equal(t, imageOverLimitText, toolResultText(turn2, 0))

	turn3 := claudeCodeSession(images...)
	apply(t, s, "vision-a", turn3)
	require.Equal(t, 5, client.count(), "converged after ceil(5/2) turns")

	turn4 := claudeCodeSession(images...)
	apply(t, s, "vision-a", turn4)
	require.Equal(t, 5, client.count(), "steady state: zero calls")
	require.Equal(t, collectText(turn3), collectText(turn4))
}

// S5 — vision upstream outage, then recovery. During the outage the image
// is fail-stripped with a constant marker and, within the negative-cache
// TTL, not retried; after the TTL it is described once and stays stable.
func TestScenario_UpstreamOutage_ThenRecovery(t *testing.T) {
	client := &wordingVisionClient{}
	s := scenarioService(client, nil, 0)
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	s.Processor.cache.now = func() time.Time { return now }
	images := []string{"c2NyZWVuMQ=="}

	client.setFail(errors.New("upstream 503"))
	t1 := claudeCodeSession(images...)
	apply(t, s, "vision-a", t1)
	require.Equal(t, 1, client.count())
	require.Equal(t, imageUnavailableText, toolResultText(t1, 0))

	now = now.Add(time.Minute)
	t2 := claudeCodeSession(images...)
	apply(t, s, "vision-a", t2)
	require.Equal(t, 1, client.count(), "within the TTL the failure is not retried")
	require.Equal(t, toolResultText(t1, 0), toolResultText(t2, 0), "the marker is constant while it lasts")

	client.setFail(nil)
	now = now.Add(describeFailureTTL)
	t3 := claudeCodeSession(images...)
	apply(t, s, "vision-a", t3)
	require.Equal(t, 2, client.count(), "after the TTL the image is retried")
	require.Contains(t, toolResultText(t3, 0), "wording #2")

	t4 := claudeCodeSession(images...)
	apply(t, s, "vision-a", t4)
	require.Equal(t, 2, client.count())
	require.Equal(t, toolResultText(t3, 0), toolResultText(t4, 0), "recovered text is stable")
}

// S6 — the same screenshot in two tool results of one request is described
// once and both positions carry identical text, this turn and the next.
func TestScenario_SameScreenshotTwice_OneDescribe(t *testing.T) {
	client := &wordingVisionClient{}
	s := scenarioService(client, nil, 0)
	images := []string{"c2NyZWVuMQ==", "c2NyZWVuMQ=="}

	t1 := claudeCodeSession(images...)
	apply(t, s, "vision-a", t1)
	require.Equal(t, 1, client.count())
	require.Equal(t, toolResultText(t1, 0), toolResultText(t1, 1))

	t2 := claudeCodeSession(images...)
	apply(t, s, "vision-a", t2)
	require.Equal(t, 1, client.count())
	require.Equal(t, collectText(t1), collectText(t2))
}

// S7 — a conversation whose history includes a screenshot that can never
// be described does not starve the ones behind it: the bad one takes a
// slot once, is negative-cached, and the slot moves on next turn.
func TestScenario_PermanentlyBadImage_DoesNotStarveHistory(t *testing.T) {
	bad := "YmFk" // "bad"
	client := &selectiveFailClient{badB64: bad}
	s := scenarioService(client, nil, 1)
	images := []string{"c2NyZWVuMQ==", bad}

	t1 := claudeCodeSession(images...)
	apply(t, s, "vision-a", t1)
	require.Equal(t, imageUnavailableText, toolResultText(t1, 1), "newest (bad) takes the slot and fails")
	require.Equal(t, imageOverLimitText, toolResultText(t1, 0))

	t2 := claudeCodeSession(images...)
	apply(t, s, "vision-a", t2)
	require.Equal(t, imageUnavailableText, toolResultText(t2, 1), "bad image is remembered, not retried")
	require.True(t, strings.HasPrefix(toolResultText(t2, 0), "Here is an [image]"), "the slot moved to the good image")
}

// selectiveFailClient fails only for one image's bytes.
type selectiveFailClient struct {
	badB64 string
	n      int
	mu     sync.Mutex
}

func (c *selectiveFailClient) Describe(_ context.Context, _ *loadbalance.Service, _, b64, _ string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	if b64 == c.badB64 {
		return "", errors.New("unsupported image")
	}
	return fmt.Sprintf("good #%d", c.n), nil
}
