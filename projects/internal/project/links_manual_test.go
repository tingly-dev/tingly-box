package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestReadingsCarryAuthorUpdateTimeAndChecks(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	updated := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
	checks := &db.LinkChecks{Passing: 3, Failing: 1, Pending: 1, Attention: []db.LinkCheck{{Name: "lint", State: "failing", URL: "https://github.com/acme/widgets/runs/1"}, {Name: "e2e", State: "pending"}}}
	f.gh.set(l.Number, func(it *GitHubItem) { it.Author, it.UpdatedAt, it.Checks, it.CI = "octo", updated, checks, "failing" })
	f.advance(linkManualGap + time.Second)
	_, err := f.links.RefreshThread(context.Background(), f.worker.ID, f.ref(l))
	require.NoError(t, err)

	k := f.link(t, l.Number)
	assert.Equal(t, "octo", k.Author)
	require.NotNil(t, k.UpdatedAt)
	assert.True(t, k.UpdatedAt.Equal(updated))
	assert.Equal(t, checks, k.Checks)

	// A reading that does not know the author keeps it.
	f.gh.set(l.Number, func(it *GitHubItem) { it.Author = "" })
	f.advance(linkManualGap + time.Second)
	_, err = f.links.RefreshThread(context.Background(), f.worker.ID, f.ref(l))
	require.NoError(t, err)
	assert.Equal(t, "octo", f.link(t, l.Number).Author)
}

// ghByPath writes a gh executable that answers a check-runs, a status or a
// pull request read with the given JSON.
func ghByPath(t *testing.T, pull, runs, status string) string {
	t.Helper()
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		"*check-runs*) cat <<'JSON'\n" + runs + "\nJSON\n;;\n" +
		"*/status) cat <<'JSON'\n" + status + "\nJSON\n;;\n" +
		"*) cat <<'JSON'\n" + pull + "\nJSON\n;;\nesac\n"
	bin := filepath.Join(t.TempDir(), "gh")
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return bin
}

func TestGhCLIBreaksChecksDown(t *testing.T) {
	bin := ghByPath(t,
		`{"number":7,"html_url":"https://github.com/acme/widgets/pull/7","title":"T","state":"open","user":{"login":"octo"},"updated_at":"2026-11-13T09:30:00Z","head":{"ref":"tp/x","sha":"abc"},"base":{"ref":"main"},"mergeable_state":"clean"}`,
		`{"check_runs":[
 {"name":"build","status":"completed","conclusion":"success","html_url":"https://x/1"},
 {"name":"docs","status":"completed","conclusion":"skipped","html_url":"https://x/2"},
 {"name":"lint","status":"completed","conclusion":"failure","html_url":"https://x/3"},
 {"name":"e2e","status":"in_progress","conclusion":null,"html_url":"https://x/4"}]}`,
		`{"state":"failure","statuses":[{"context":"ci/legacy","state":"failure","target_url":"https://x/5"},{"context":"ci/ok","state":"success","target_url":"https://x/6"}]}`)
	it, err := ghCLI{bin: bin}.PR(context.Background(), GitHubRepo{Owner: "acme", Name: "widgets"}, 7)
	require.NoError(t, err)
	assert.Equal(t, "failing", it.CI)
	assert.Equal(t, "octo", it.Author)
	assert.Equal(t, time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC), it.UpdatedAt)
	require.NotNil(t, it.Checks)
	assert.Equal(t, 2, it.Checks.Passing)
	assert.Equal(t, 1, it.Checks.Skipped)
	assert.Equal(t, 2, it.Checks.Failing)
	assert.Equal(t, 1, it.Checks.Pending)
	assert.Equal(t, []db.LinkCheck{
		{Name: "lint", State: "failing", URL: "https://x/3"},
		{Name: "e2e", State: "pending", URL: "https://x/4"},
		{Name: "ci/legacy", State: "failing", URL: "https://x/5"},
	}, it.Checks.Attention)
	assert.Zero(t, it.Checks.More)
}

func TestChecksListAtMostFiveAndCountTheRest(t *testing.T) {
	var runs []string
	for i := 0; i < 8; i++ {
		runs = append(runs, `{"name":"job`+string(rune('a'+i))+`","status":"completed","conclusion":"failure","html_url":""}`)
	}
	bin := ghByPath(t,
		`{"number":7,"state":"open","head":{"sha":"abc"}}`,
		`{"check_runs":[`+strings.Join(runs, ",")+`]}`,
		`{"state":"pending","statuses":[]}`)
	it, err := ghCLI{bin: bin}.PR(context.Background(), GitHubRepo{Owner: "a", Name: "b"}, 7)
	require.NoError(t, err)
	assert.Len(t, it.Checks.Attention, db.LinkChecksListed)
	assert.Equal(t, 3, it.Checks.More)
	assert.Equal(t, 8, it.Checks.Failing)
}
