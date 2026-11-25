# Sandbox

The one place that knows how an agent process is isolated. Its own Go module
(`github.com/tingly-dev/tingly-box/sandbox`); it imports no other module of
this repository, and agentboot, Projects and Tingly Box use it.

- **Behaviour model**: `Policy`, five axes (storage, network, exec,
  credentials, resources), each at a `Level`. Callers say what, never how.
- **Interface**: `Runtime` (`Info` with the levels it supports per axis,
  `Check`, `Start`). `sandbox.Start` validates every axis and refuses what a
  runtime cannot give (`ErrUnavailable`); nothing falls back to the host.
- **Registry**: a runtime package registers itself in `init`; a build ships
  the runtimes it imports (Projects: `projects/internal/project/sandbox.go`).

- **Allowlist network**: `netproxy` is the host proxy and in-sandbox bridge
  any runtime without a network of its own can reuse (see bwrap's `Start`).

Adding a runtime: a package here that implements `Runtime` (wrapping a command?
use `ExecCommand`), registers itself, and has
`func TestConformance(t *testing.T) { conformance.Run(t, Runtime{}) }`. Add
it to `.github/workflows/daily.yml`'s `test-sandbox` setup if it needs a tool
installed. Nothing outside this module changes but the one import.

Design: `../projects/.design/thread-runtime.md` §11.
