package project

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gh "github.com/tingly-dev/tingly-box/mcphub/github"
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
	checks := &gh.Checks{Passing: 3, Failing: 1, Pending: 1, Failed: []gh.Check{{Name: "lint", URL: "https://github.com/acme/widgets/runs/1"}}}
	f.gh.set(l.Number, func(it *GitHubItem) { it.Checks, it.CI = checks, "failing" })
	f.advance(linkManualGap + time.Second)
	_, err := f.links.RefreshThread(context.Background(), f.worker.ID, f.ref(l))
	require.NoError(t, err)
	assert.Equal(t, dbChecks(checks), f.link(t, l.Number).Checks)
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
