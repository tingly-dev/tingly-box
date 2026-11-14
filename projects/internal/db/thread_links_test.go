package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func linkFixture(t *testing.T) (*Store, *Project, *Thread) {
	t.Helper()
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	p := &Project{Name: "P"}
	require.NoError(t, s.CreateProject(p))
	th := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode}
	require.NoError(t, s.CreateThread(th))
	return s, p, th
}

func TestUpsertThreadLinkKeepsOneRowAndTheStrongerRole(t *testing.T) {
	s, p, th := linkFixture(t)
	l := &ThreadLink{ThreadID: th.ID, ProjectID: p.ID, Kind: LinkPR, Repo: "acme/w", Number: 3, Role: LinkOpened, State: LinkStateOpen, Title: "one"}
	require.NoError(t, s.UpsertThreadLink(l))
	created := l.CreatedAt

	// A later comment on the same PR updates it but does not demote "opened".
	again := &ThreadLink{ThreadID: th.ID, ProjectID: p.ID, Kind: LinkPR, Repo: "acme/w", Number: 3, Role: LinkCommented, State: LinkStateClosed, Title: "two"}
	require.NoError(t, s.UpsertThreadLink(again))

	links, err := s.ThreadLinks(th.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, LinkOpened, links[0].Role)
	assert.Equal(t, "two", links[0].Title)
	assert.Equal(t, LinkStateClosed, links[0].State)
	assert.True(t, links[0].CreatedAt.Equal(created))

	// The same number as an issue, or in another repository, is another row.
	require.NoError(t, s.UpsertThreadLink(&ThreadLink{ThreadID: th.ID, ProjectID: p.ID, Kind: LinkIssue, Repo: "acme/w", Number: 3, Role: LinkOpened, State: LinkStateOpen}))
	require.NoError(t, s.UpsertThreadLink(&ThreadLink{ThreadID: th.ID, ProjectID: p.ID, Kind: LinkPR, Repo: "acme/other", Number: 3, Role: LinkOpened, State: LinkStateOpen}))
	links, err = s.ThreadLinks(th.ID)
	require.NoError(t, err)
	assert.Len(t, links, 3)
}

func TestOpenThreadLinksSkipsFinishedItemsAndArchivedProjects(t *testing.T) {
	s, p, th := linkFixture(t)
	for n, state := range map[int]string{1: LinkStateOpen, 2: LinkStateMerged, 3: LinkStateClosed} {
		require.NoError(t, s.UpsertThreadLink(&ThreadLink{ThreadID: th.ID, ProjectID: p.ID, Kind: LinkPR, Repo: "a/b", Number: n, State: state}))
	}
	open, err := s.OpenThreadLinks()
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, 1, open[0].Number)

	now := time.Now()
	p.ArchivedAt = &now
	require.NoError(t, s.SaveProject(p))
	open, err = s.OpenThreadLinks()
	require.NoError(t, err)
	assert.Empty(t, open)
}

func TestSaveThreadLinkReadingChangesOnlyTheReading(t *testing.T) {
	s, p, th := linkFixture(t)
	l := &ThreadLink{ThreadID: th.ID, ProjectID: p.ID, Kind: LinkPR, Repo: "a/b", Number: 1, Role: LinkOpened, State: LinkStateOpen, CI: "failing", Title: "T"}
	require.NoError(t, s.UpsertThreadLink(l))
	l.CI, l.Failures, l.LastError, l.Role = "passing", 2, "boom", "hacked"
	require.NoError(t, s.SaveThreadLinkReading(l))
	got, err := s.FindThreadLink(th.ID, "a/b", 1)
	require.NoError(t, err)
	assert.Equal(t, "passing", got.CI)
	assert.Equal(t, 2, got.Failures)
	assert.Equal(t, "boom", got.LastError)
	assert.Equal(t, LinkOpened, got.Role)
}

func TestDeletingAProjectDeletesItsLinks(t *testing.T) {
	s, p, th := linkFixture(t)
	require.NoError(t, s.UpsertThreadLink(&ThreadLink{ThreadID: th.ID, ProjectID: p.ID, Kind: LinkIssue, Repo: "a/b", Number: 1}))
	require.NoError(t, s.DeleteProjectCascade(p.ID))
	by, err := s.ThreadLinksOf([]string{th.ID})
	require.NoError(t, err)
	assert.Empty(t, by)
}
