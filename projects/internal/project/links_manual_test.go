package project

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func (f *linksFixture) ref(l *db.ThreadLink) *LinkRef {
	return &LinkRef{Repo: l.Repo, Kind: l.Kind, Number: l.Number}
}

func TestManualRefreshReadsNowAndAlwaysPublishes(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	ctx := context.Background()
	f.advance(linkManualGap + time.Second)

	got, err := f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, RefreshDone, got[0].Status)
	assert.Equal(t, 1, f.reads())
	assert.Equal(t, f.clock.Unix(), f.link(t, l.Number).RefreshedAt.Unix(), "the reading's time moved")
	assert.Equal(t, []string{f.worker.ID}, f.events, "an unchanged reading is published too: the age is what the person asked to see")
}

func TestManualRefreshHasACooldownAndJoinsAReadInFlight(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	ctx := context.Background()

	got, err := f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	require.NoError(t, err)
	assert.Equal(t, RefreshRecent, got[0].Status, "read a moment ago")
	assert.Equal(t, 0, f.reads())
	assert.Empty(t, f.events)

	f.advance(linkManualGap - time.Second)
	got, _ = f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	assert.Equal(t, RefreshRecent, got[0].Status)
	f.advance(2 * time.Second)

	f.gh.block = make(chan struct{})
	var wg sync.WaitGroup
	results := make([]string, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
			if err == nil {
				results[i] = r[0].Status
			}
		}()
	}
	// One of the two is in GitHub's hands; the other has already answered recent.
	require.Eventually(t, func() bool { return f.reads() == 1 }, time.Second, time.Millisecond)
	close(f.gh.block)
	wg.Wait()
	assert.ElementsMatch(t, []string{RefreshDone, RefreshRecent}, results)
	assert.Equal(t, 1, f.reads(), "two requests, one read")
}

func TestManualRefreshReadsFinishedItemsToo(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	f.gh.set(l.Number, func(it *GitHubItem) { it.State = "closed" })
	f.advance(linkInterval)
	f.links.Sweep(context.Background())
	require.Equal(t, db.LinkStateClosed, f.link(t, l.Number).State)
	f.advance(24 * time.Hour)
	f.links.Sweep(context.Background())
	require.Equal(t, 1, f.reads(), "the sweep never reads a closed item again")

	f.gh.set(l.Number, func(it *GitHubItem) { it.State = "open" })
	got, err := f.links.RefreshThread(context.Background(), f.worker.ID, f.ref(l))
	require.NoError(t, err)
	assert.Equal(t, RefreshDone, got[0].Status)
	assert.Equal(t, db.LinkStateOpen, f.link(t, l.Number).State, "a reopened PR is seen")
}

func TestManualFailureSaysWhyAndDoesNotFeedTheBackoff(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	ctx := context.Background()
	f.gh.set(l.Number, func(it *GitHubItem) { it.CI = "failing" })
	f.gh.fail = errors.New("GitHub is not available: gh is missing ghp_abcdefghijklmnopqrstuvwxyz0123456789")
	f.advance(linkManualGap + time.Second)

	got, err := f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	require.NoError(t, err, "a failed read is a result, not an error")
	assert.Equal(t, RefreshFailed, got[0].Status)
	assert.Contains(t, got[0].Error, "not available")
	assert.NotContains(t, got[0].Error, "ghp_abcdef")
	k := f.link(t, l.Number)
	assert.Equal(t, 0, k.Failures, "pressing refresh while signed out must not push the sweep's wait up")
	assert.Equal(t, got[0].Error, k.LastError)
	assert.Equal(t, "pending", k.CI, "the last good reading stays")
	assert.NotEmpty(t, f.events, "the page shows 'could not refresh'")

	f.gh.fail = nil
	f.advance(linkManualGap + time.Second)
	got, _ = f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	assert.Equal(t, RefreshDone, got[0].Status)
	k = f.link(t, l.Number)
	assert.Empty(t, k.LastError)
	assert.Equal(t, "failing", k.CI)
}

func TestManualRefreshOfOneOrAllOfAThread(t *testing.T) {
	f := newLinksFixture(t)
	a, b := f.openedPR(t), f.openedPR(t)
	f.advance(linkManualGap + time.Second)
	ctx := context.Background()

	got, err := f.links.RefreshThread(ctx, f.worker.ID, f.ref(b))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, b.Number, got[0].Number)
	assert.Equal(t, 1, f.reads())

	f.advance(linkManualGap + time.Second)
	got, err = f.links.RefreshThread(ctx, f.worker.ID, nil)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, 3, f.reads())

	_, err = f.links.RefreshThread(ctx, f.worker.ID, &LinkRef{Repo: "acme/widgets", Number: 999})
	assert.ErrorIs(t, err, db.ErrNotFound)
	_, err = f.links.RefreshThread(ctx, f.worker.ID, &LinkRef{Repo: "other/repo", Number: a.Number})
	assert.ErrorIs(t, err, db.ErrNotFound, "the same number in another repository is another item")
}

func TestProjectRefreshReadsOnlyOpenItemsOldestFirstAndIsBounded(t *testing.T) {
	f := newLinksFixture(t)
	for i := 0; i < linkSweepMax+3; i++ {
		f.openedPR(t)
	}
	done := f.openedPR(t)
	f.gh.set(done.Number, func(it *GitHubItem) { it.State = "merged" })
	f.advance(linkManualGap + time.Second)
	_, err := f.links.RefreshThread(context.Background(), f.worker.ID, f.ref(done))
	require.NoError(t, err)
	require.Equal(t, db.LinkStateMerged, f.link(t, done.Number).State)
	before := f.reads()
	f.advance(linkManualGap + time.Second)

	got, err := f.links.RefreshProject(context.Background(), f.p.ID)
	require.NoError(t, err)
	assert.Len(t, got, linkSweepMax, "bounded like the sweep")
	assert.Equal(t, before+linkSweepMax, f.reads())
	for _, r := range got {
		assert.NotEqual(t, done.Number, r.Number, "a merged item is left to a row's own refresh")
		assert.Equal(t, RefreshDone, r.Status)
	}
}

func TestReadingsCarryTheChecksBreakdown(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	checks := &db.LinkChecks{Passing: 3, Failing: 1, Pending: 1, Failed: []db.LinkCheck{{Name: "lint", URL: "https://github.com/acme/widgets/runs/1"}}}
	f.gh.set(l.Number, func(it *GitHubItem) { it.Checks, it.CI = checks, "failing" })
	f.advance(linkManualGap + time.Second)
	_, err := f.links.RefreshThread(context.Background(), f.worker.ID, f.ref(l))
	require.NoError(t, err)
	assert.Equal(t, checks, f.link(t, l.Number).Checks)
}

func TestProjectRefreshOfTheSameProjectWaitsAMinute(t *testing.T) {
	f := newLinksFixture(t)
	f.openedPR(t)
	f.openedPR(t)
	ctx := context.Background()
	f.advance(linkManualGap + time.Second)

	got, err := f.links.RefreshProject(ctx, f.p.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, 2, f.reads())

	// Past each item's own gap but inside the project's: nothing is asked.
	f.advance(linkManualGap + time.Second)
	got, err = f.links.RefreshProject(ctx, f.p.ID)
	require.NoError(t, err)
	for _, r := range got {
		assert.Equal(t, RefreshRecent, r.Status)
	}
	assert.Equal(t, 2, f.reads(), "a button pressed over and over is one round of requests a minute")

	f.advance(linkProjectGap)
	got, _ = f.links.RefreshProject(ctx, f.p.ID)
	assert.Equal(t, RefreshDone, got[0].Status)
	assert.Equal(t, 4, f.reads())
}

func TestReadsStopWhileGitHubsRequestBudgetIsNearlyUsedUp(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	ctx := context.Background()
	f.advance(linkInterval)
	f.gh.rate = GitHubRate{Remaining: rateFloor - 1, ResetAt: f.clock.Add(20 * time.Minute)}
	f.links.Sweep(ctx)
	require.Equal(t, 1, f.reads(), "the answer that says the budget is low is still an answer")

	// Neither the sweep nor a person's refresh asks again until the window resets.
	f.advance(linkInterval)
	f.links.Sweep(ctx)
	got, err := f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	require.NoError(t, err)
	assert.Equal(t, RefreshLimited, got[0].Status)
	assert.Contains(t, got[0].Error, "reads resume at")
	assert.Equal(t, 1, f.reads())
	assert.Empty(t, f.link(t, l.Number).LastError, "it is not this item's failure")

	f.advance(20 * time.Minute)
	f.gh.rate = GitHubRate{Remaining: 4000, ResetAt: f.clock.Add(time.Hour)}
	got, _ = f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	assert.Equal(t, RefreshDone, got[0].Status)
	f.links.Sweep(ctx)
	assert.GreaterOrEqual(t, f.reads(), 2)
}

func TestAnErrorThatNamesTheRateLimitPausesReads(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	ctx := context.Background()
	f.gh.fail = errors.New("GitHub POST graphql: API rate limit exceeded")
	f.advance(linkManualGap + time.Second)
	got, _ := f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	assert.Equal(t, RefreshFailed, got[0].Status)
	reads := f.reads()

	f.gh.fail = nil
	f.advance(linkManualGap + time.Second)
	got, _ = f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	assert.Equal(t, RefreshLimited, got[0].Status, "GitHub gave no reset time: wait a while rather than ask again")
	f.links.Sweep(ctx)
	assert.Equal(t, reads, f.reads())

	f.advance(rateLimitedFor + time.Second)
	got, _ = f.links.RefreshThread(ctx, f.worker.ID, f.ref(l))
	assert.Equal(t, RefreshDone, got[0].Status)
}

// ghGraphQL writes a gh executable that answers a read with the given JSON.
func ghGraphQL(t *testing.T, answer string) (bin, logFile string) {
	t.Helper()
	return fakeGH(t, answer, 0)
}

func TestGhCLIReadsAPRInOneRequestAndFoldsItsChecks(t *testing.T) {
	bin, logFile := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":4321,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{
 "__typename":"PullRequest","number":7,"url":"https://github.com/acme/widgets/pull/7","title":"T","body":"B","state":"OPEN","isDraft":false,
 "headRefName":"tp/x","baseRefName":"main","mergeable":"MERGEABLE","mergeStateStatus":"BLOCKED",
 "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"totalCount":5,"nodes":[
  {"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":"https://x/1"},
  {"__typename":"CheckRun","name":"docs","status":"COMPLETED","conclusion":"SKIPPED","detailsUrl":"https://x/2"},
  {"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://x/3"},
  {"__typename":"CheckRun","name":"e2e","status":"IN_PROGRESS","conclusion":null,"detailsUrl":"https://x/4"},
  {"__typename":"StatusContext","context":"ci/legacy","state":"ERROR","targetUrl":"https://x/5"}]}}}}]}}}}}`)
	it, err := ghCLI{bin: bin}.Read(context.Background(), GitHubRepo{Owner: "acme", Name: "widgets"}, 7)
	require.NoError(t, err)

	assert.True(t, it.IsPR)
	assert.Equal(t, "open", it.State)
	assert.Equal(t, "blocked", it.MergeState)
	assert.Equal(t, "tp/x", it.Head)
	assert.Equal(t, "failing", it.CI)
	assert.Equal(t, 4321, it.Rate.Remaining)
	assert.Equal(t, time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC), it.Rate.ResetAt)
	require.NotNil(t, it.Checks)
	assert.Equal(t, 2, it.Checks.Passing, "a skipped check is not a failure")
	assert.Equal(t, 2, it.Checks.Failing)
	assert.Equal(t, 1, it.Checks.Pending)
	assert.Equal(t, []db.LinkCheck{{Name: "lint", URL: "https://x/3"}, {Name: "ci/legacy", URL: "https://x/5"}}, it.Checks.Failed, "only what failed is named")

	b, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(b), "api -X POST --input - graphql"), "one request for the whole reading")
	assert.Contains(t, string(b), `"number":7`)
	assert.Contains(t, string(b), `"owner":"acme"`)
}

func TestGhCLIReadsAnIssueAndAMergedPRWithoutChecks(t *testing.T) {
	bin, _ := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{"__typename":"Issue","number":3,"url":"https://github.com/acme/widgets/issues/3","title":"I","body":"","state":"CLOSED"}}}}`)
	it, err := ghCLI{bin: bin}.Read(context.Background(), GitHubRepo{Owner: "acme", Name: "widgets"}, 3)
	require.NoError(t, err)
	assert.False(t, it.IsPR)
	assert.Equal(t, "closed", it.State)
	assert.Empty(t, it.CI)

	bin, _ = ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{"__typename":"PullRequest","number":4,"url":"u","title":"P","body":"","state":"MERGED","isDraft":false,"headRefName":"a","baseRefName":"main","mergeable":"UNKNOWN","mergeStateStatus":"UNKNOWN","commits":{"nodes":[]}}}}}`)
	it, err = ghCLI{bin: bin}.Read(context.Background(), GitHubRepo{Owner: "acme", Name: "widgets"}, 4)
	require.NoError(t, err)
	assert.Equal(t, "merged", it.State)
	assert.Empty(t, it.MergeState, "a merged PR has no merge state")
	assert.Empty(t, it.CI)
}

func TestGhCLIReadOfAMissingNumberSaysSo(t *testing.T) {
	bin, _ := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":null}}}`)
	_, err := ghCLI{bin: bin}.Read(context.Background(), GitHubRepo{Owner: "acme", Name: "widgets"}, 99)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no issue or PR #99")
}

func TestChecksNameAtMostFiveFailuresAndCountTheRest(t *testing.T) {
	var nodes []string
	for i := 0; i < 8; i++ {
		nodes = append(nodes, `{"__typename":"CheckRun","name":"job`+string(rune('a'+i))+`","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":""}`)
	}
	bin, _ := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{"__typename":"PullRequest","number":7,"url":"u","title":"P","body":"","state":"OPEN","headRefName":"a","baseRefName":"main","mergeStateStatus":"CLEAN","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"totalCount":8,"nodes":[`+strings.Join(nodes, ",")+`]}}}}]}}}}}`)
	it, err := ghCLI{bin: bin}.Read(context.Background(), GitHubRepo{Owner: "a", Name: "b"}, 7)
	require.NoError(t, err)
	assert.Len(t, it.Checks.Failed, db.LinkChecksListed)
	assert.Equal(t, 3, it.Checks.More)
	assert.Equal(t, 8, it.Checks.Failing)
}

func TestAFailureBeyondTheHundredChecksReadStillShows(t *testing.T) {
	bin, _ := ghGraphQL(t, `{"data":{"rateLimit":{"remaining":10,"resetAt":"2026-11-15T09:00:00Z"},"repository":{"issueOrPullRequest":{"__typename":"PullRequest","number":7,"url":"u","title":"P","body":"","state":"OPEN","headRefName":"a","baseRefName":"main","mergeStateStatus":"CLEAN","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"FAILURE","contexts":{"totalCount":130,"nodes":[{"__typename":"CheckRun","name":"ok","status":"COMPLETED","conclusion":"SUCCESS","detailsUrl":""}]}}}}]}}}}}`)
	it, err := ghCLI{bin: bin}.Read(context.Background(), GitHubRepo{Owner: "a", Name: "b"}, 7)
	require.NoError(t, err)
	assert.Equal(t, "failing", it.CI, "the rollup has the last word when not every check was read")
}
