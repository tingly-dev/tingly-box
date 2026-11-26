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

- **Host endpoints**: `netproxy` is the allowlist proxy, the exposed-port
  forwarders and the in-sandbox bridge, which any runtime without a network
  of its own reuses (see bwrap's `Start`).

- **Runtimes here**: `bwrap` (namespaces) and `microvm` (Cloud Hypervisor +
  virtiofsd; its guest init is this binary).
- **macOS**: `macvm` (Virtualization.framework through cgo; a Linux guest
  image, `macvm/image`; its calls are in `vz_darwin.go`, everything else is
  plain Go tested against a fake hypervisor) and `seatbelt` (sandbox-exec
  profiles; no internet level, no limits). `usernet` is the filtered
  user-space network a VM's card is attached to. Neither macOS runtime has
  been run on a real Mac: see `../.design/sandbox-macvm.md`.
- **VM guests**: `vmguest` is the host–guest protocol over vsock and the
  guest init every VM runtime shares; `vmguest/cmd/tb-guest` is that init
  built on its own, for a guest image.

Adding a runtime: a package here that implements `Runtime` (wrapping a command?
use `ExecCommand`), registers itself, and has
`func TestConformance(t *testing.T) { conformance.Run(t, Runtime{}) }`. Add
it to `.github/workflows/daily.yml`'s `test-sandbox` setup if it needs a tool
installed. Nothing outside this module changes but the one import.

Design (authoritative: security model, axes and levels, primitives, runtime
mapping, steps): `../.design/sandbox.md`. History and measurements:
`../projects/.design/thread-runtime.md`.
