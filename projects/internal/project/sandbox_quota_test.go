package project

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/sandbox"
)

func TestThreadResourcesFollowTheProjectsQuotas(t *testing.T) {
	r := threadResources(&db.Project{})
	assert.Equal(t, sandbox.Resources{Level: sandbox.ResourcesUnlimited}, r, "no quota set: unlimited")

	r = threadResources(&db.Project{ThreadTasks: 200})
	assert.Equal(t, sandbox.Resources{Level: sandbox.ResourcesLimited, Tasks: 200}, r, "one limit set, the others stay unlimited")

	r = threadResources(&db.Project{ThreadMemoryBytes: 2 << 30, ThreadTasks: 512, ThreadCPU: 1500})
	assert.Equal(t, sandbox.Resources{Level: sandbox.ResourcesLimited, MemoryBytes: 2 << 30, Tasks: 512, CPU: 1500}, r)
	assert.NoError(t, threadPolicy("/h", nil, nil, nil, r).WellFormed(), "a policy with quotas is well formed")
}

func TestFailureTextSaysWhenTheSandboxRanOutOfMemory(t *testing.T) {
	f := newFixture(t, nil)
	f.project.ThreadMemoryBytes = 2 << 30
	if err := f.store.SaveProject(f.project); err != nil {
		t.Fatal(err)
	}
	th := &db.Thread{ProjectID: f.project.ID}
	oom := fmt.Errorf("turn: %w", &sandbox.ExitError{Code: 137, Reason: sandbox.EndOOM})
	got := f.threads.failureText(th, oom)
	assert.Contains(t, got, "ran out of memory")
	assert.Contains(t, got, "limit 2 GiB")

	other := &sandbox.ExitError{Code: 1, Reason: sandbox.EndExited}
	assert.Equal(t, other.Error(), f.threads.failureText(th, other))
	assert.Equal(t, "boom", f.threads.failureText(th, errors.New("boom")))
}

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "512 MiB", FormatBytes(512<<20))
	assert.Equal(t, "2 GiB", FormatBytes(2<<30))
	assert.Equal(t, "1500 bytes", FormatBytes(1500))
}
