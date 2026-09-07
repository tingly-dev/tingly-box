package agentrun

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/managedagent/gitrepo"
)

// GitAdapter exposes a gitrepo.Git as the Service's Git seam.
type GitAdapter struct{ Git *gitrepo.Git }

var _ managedagent.Git = GitAdapter{}

func (a GitAdapter) Diff(ctx context.Context, ws *managedagent.Workspace) (*managedagent.Diff, error) {
	d, err := a.Git.Diff(ctx, ws.Path, ws.BaseRef)
	if err != nil {
		return nil, err
	}
	return &managedagent.Diff{
		ChangedFiles: d.ChangedFiles, Stat: d.Stat, Patch: d.Patch,
		Untracked: d.Untracked, Truncated: d.Truncated,
	}, nil
}

func (a GitAdapter) Push(ctx context.Context, ws *managedagent.Workspace, log func(string)) error {
	return a.Git.Push(ctx, ws.Path, ws.Branch, log)
}
