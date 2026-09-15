package agentrun

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/managedagent/gitrepo"
)

// GitAdapter exposes a gitrepo.Git as the Service's Git seam. It exists only
// to translate the diff type: gitrepo must not depend on the domain package.
type GitAdapter struct{ Git *gitrepo.Git }

var _ managedagent.Git = GitAdapter{}

func (a GitAdapter) IsRepo(ctx context.Context, dir string) bool { return a.Git.IsRepo(ctx, dir) }

func (a GitAdapter) Head(ctx context.Context, dir string) (string, error) {
	return a.Git.Head(ctx, dir)
}

func (a GitAdapter) ChangedFiles(ctx context.Context, dir, base string) (int, error) {
	return a.Git.ChangedFiles(ctx, dir, base)
}

func (a GitAdapter) Diff(ctx context.Context, dir, base string) (*managedagent.Diff, error) {
	d, err := a.Git.Diff(ctx, dir, base)
	if err != nil {
		return nil, err
	}
	return &managedagent.Diff{
		ChangedFiles: d.ChangedFiles, Stat: d.Stat, Patch: d.Patch,
		Untracked: d.Untracked, Truncated: d.Truncated,
	}, nil
}
