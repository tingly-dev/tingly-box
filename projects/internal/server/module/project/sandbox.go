package project

import (
	"context"
	"time"

	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/sandbox"
)

// sandboxRuntimes lists the runtimes this build ships, the host first, with
// whether each can run here and what a project's threads get in it.
func sandboxRuntimes(ctx context.Context) []SandboxRuntime {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := []SandboxRuntime{}
	for _, r := range sandbox.List() {
		info := r.Info()
		s := SandboxRuntime{Name: info.Name, Isolation: string(info.Isolation), Available: true, Levels: map[string]string{}}
		if err := r.Check(ctx); err != nil {
			s.Available, s.Reason = false, err.Error()
		}
		for axis, level := range rt.ThreadPolicyLevels(info.Name) {
			s.Levels[string(axis)] = string(level)
		}
		if info.Name == sandbox.HostName {
			out = append([]SandboxRuntime{s}, out...)
		} else {
			out = append(out, s)
		}
	}
	return out
}
