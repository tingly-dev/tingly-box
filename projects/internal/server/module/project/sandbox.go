package project

import (
	"context"
	"time"

	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/sandbox"
)

// sandboxRuntimes lists the runtimes this build ships, the host first, with
// whether a project's threads can run in each here and what they get in it.
func sandboxRuntimes(ctx context.Context) []SandboxRuntime {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out := []SandboxRuntime{}
	for _, r := range sandbox.List() {
		info := r.Info()
		s := SandboxRuntime{Name: info.Name, Isolation: string(info.Isolation), Available: true, Levels: map[string]string{}}
		if err := rt.CheckThreadSandbox(ctx, info.Name); err != nil {
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
