package vmodel

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func threeSteps(t *testing.T) *Sequence {
	t.Helper()
	cfg, err := ParseScript([]byte("id: s\nsteps:\n  - say: one\n  - say: two\n  - say: three"), "s")
	require.NoError(t, err)
	return NewSequence(cfg)
}

func TestNextFor_SessionsAreIndependentRuns(t *testing.T) {
	seq := threeSteps(t)

	assert.Equal(t, "one", seq.NextFor("a").Content)
	assert.Equal(t, "two", seq.NextFor("a").Content)
	assert.Equal(t, "one", seq.NextFor("b").Content, "b starts at the beginning, whatever a did")
	assert.Equal(t, "one", seq.Next().Content, "the default session is its own run too")
	assert.Equal(t, "three", seq.NextFor("a").Content, "a is unaffected by b and the default")
	assert.Equal(t, "two", seq.NextFor("b").Content)
}

func TestNextFor_ConcurrentSessionsEachSeeTheWholeProgramInOrder(t *testing.T) {
	seq := threeSteps(t)
	var wg sync.WaitGroup
	got := make([][]string, 8)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				got[i] = append(got[i], seq.NextFor(fmt.Sprintf("s%d", i)).Content)
			}
		}()
	}
	wg.Wait()
	for i, g := range got {
		assert.Equal(t, []string{"one", "two", "three"}, g, "session s%d", i)
	}
}

func TestNextFor_ToolIDsNameTheSession(t *testing.T) {
	cfg, err := ParseScript([]byte("id: f\nsteps:\n  - tool: {name: Read}"), "f")
	require.NoError(t, err)
	seq := NewSequence(cfg)
	assert.Equal(t, "toolu_f_1", seq.Next().Tool.ID)
	assert.Equal(t, "toolu_f_s1_1", seq.NextFor("s1").Tool.ID)
	assert.Equal(t, "toolu_f_s2_1", seq.NextFor("s2").Tool.ID)
}

func TestNextFor_SessionTableIsBounded(t *testing.T) {
	seq := threeSteps(t)
	seq.NextFor("keep")
	seq.NextFor("keep") // "keep" is at step 3 now
	for i := 0; i < MaxSessions+10; i++ {
		seq.NextFor(fmt.Sprintf("flood-%d", i))
		seq.NextFor("keep") // stays recently used, so it survives eviction
	}
	seq.mu.Lock()
	n := len(seq.sessions)
	seq.mu.Unlock()
	assert.LessOrEqual(t, n, MaxSessions)
	assert.Equal(t, "one", seq.NextFor("flood-0").Content, "an evicted session restarts")
}

func TestSplitSessionModel(t *testing.T) {
	cases := []struct {
		in, id, session string
		ok              bool
	}{
		{"flow@test-1", "flow", "test-1", true},
		{"flow", "flow", "", false},
		{"flow@", "flow@", "", false},
		{"@x", "@x", "", false},
		{"flow@has space", "flow@has space", "", false},
	}
	for _, c := range cases {
		id, session, ok := SplitSessionModel(c.in)
		assert.Equal(t, c.ok, ok, c.in)
		assert.Equal(t, c.id, id, c.in)
		assert.Equal(t, c.session, session, c.in)
	}
	_, _, ok := SplitSessionModel("flow@" + fmt.Sprintf("%065d", 0))
	assert.False(t, ok, "65-character session is too long")
}
