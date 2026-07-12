# Harness Duo — two-process memory & protocol verification

> For contributors working with `internal/protocoltest/duo*.go`,
> `cli/harness/duo.go`, the `/api/v1/debug` diagnostics module, or anyone
> investigating a gateway memory leak / OOM report.
>
> Related: the protocol-transform matrix (Tier A) lives in
> [`harness-matrix.md`](./harness-matrix.md); the load-balancing simulator
> (Tier LB) in [`tier-routing.md`](./tier-routing.md).

---

## 1. Why duo exists

Duo was born from the #1255 OOM investigation: Claude-Code-shaped traffic
(multi-MB conversation per turn, streaming) leaked ~823 KB **per request**
inside the gateway's conversion path. Reproducing that class of bug needs
three things a unit test doesn't give you:

1. **The real pipeline end-to-end** — routing, transform, client pool,
   transports, real HTTP between gateway and upstream.
2. **A leak signal that separates retention from churn** — allocation volume
   is huge and healthy; what matters is the **post-GC retention slope**
   across request batches.
3. **Attribution** — tingly-box appears on *both* sides of a deployment
   (gateway, and upstream-ish serving surfaces). "The process grew" is not a
   diagnosis; *which role leaked* is.

## 2. Topology

```
                 harness (parent process — drives requests, never measured)
                    │
                    │ Anthropic v1/beta stream          per-instance sampling
                    ▼                                   /api/v1/debug/{memstats,pprof/heap}
   ┌──────────────────────────────┐    real HTTP    ┌──────────────────────────────┐
   │  tb2  (gateway under test)   ├────────────────▶│  tb1  (vmodel upstream)      │
   │  own process, server.Start() │                 │  own process, server.Start() │
   └──────────────────────────────┘                 └──────────────────────────────┘
```

Design decisions, and why:

- **Two real processes, not two in-process httptest servers.** A shared Go
  heap makes per-instance attribution impossible (both instances run the same
  code, so even heap-profile stacks can't always tell them apart) and lets
  the harness client's own allocations pollute the numbers. Separate
  processes give each instance its own runtime: tb2's slope is the
  gateway/conversion verdict, tb1's slope is the vmodel-serving verdict, and
  the parent is never measured.

- **Full `server.Start()`, not `GetRouter()` + httptest.** Children boot the
  production lifecycle: token refreshers, quota auto-refresh, config watcher,
  remote-coder autostart, and the production `http.Server` timeouts. Leaks in
  or interacting with those components are on the measured path.

- **Re-execution instead of a separate server binary.** The parent spawns
  `os.Executable()` with the `TINGLY_DUO_*` env contract;
  `MaybeRunDuoServe()` (called first thing in `cli/harness` `main()` and in
  `duo_test.go`'s `TestMain`) intercepts and runs the server, never
  returning. The same mechanism therefore works for the CLI binary *and* the
  `go test` binary — CI needs no extra build step.

- **Children self-seed their own config dirs.** tb2 receives tb1's URL/token
  via env and writes its own providers + rules at boot. The parent never
  opens an instance's SQLite store (no cross-process DB access); its only
  reads are each child's `config.json` (tokens) and HTTP.

## 3. Observation: the `/api/v1/debug` module

`internal/server/module/debug` is a **production management API**, not test
scaffolding (UX principle: *diagnostics must traverse the real path*). The
duo harness and a live incident use the same two endpoints (user-token auth):

| Endpoint | Semantics |
|----------|-----------|
| `GET /api/v1/debug/memstats?gc=true` | `runtime.MemStats` snapshot; `gc=true` forces a double GC first so `heap_alloc_bytes` is the post-GC **retained set**. Also reports goroutine count (a second leak axis). |
| `GET /api/v1/debug/pprof/heap?gc=true` | pprof heap profile (gzipped protobuf for `go tool pprof`), post-GC when `gc=true` (`X-Debug-GC-Forced` header reports whether it ran). |

These complement the CLI-level `--pprof` flag (side server on `:6060`,
unauthenticated, opt-in at start): the debug module is on the main port,
authenticated, and available on any running instance without a restart.

**Cost & control.** Mounting the routes costs nothing at runtime. The two
expensive operations are both throttled to one per second per instance: the
opt-in forced GC (stop-the-world on the live heap; a throttled call degrades
to an un-forced snapshot, `gc_forced: false`) and heap-profile serialization
(CPU proportional to live objects; a throttled call gets a 429 with
Retry-After, since a profile has no cheap degraded form). Plain memstats
reads are microsecond-scale and unthrottled. The endpoints stay mounted by
default rather than being gated behind a debug flag deliberately: their
primary use is incident diagnosis on an already-running instance, and
restarting with a flag to enable them would destroy the leaked heap being
diagnosed. The duo harness retries `MemStats(gc=true)` and profile fetches
past the throttle windows so slope samples are always genuinely post-GC.

**Relation to pprof.** The heap endpoint is deliberately a *narrowed*
re-exposure of `runtime/pprof` (the same `Lookup("heap").WriteTo` primitive
`net/http/pprof` wraps), not a reimplementation of the suite. Mounting
`net/http/pprof` wholesale was rejected: its import registers unauthenticated
handlers on `http.DefaultServeMux` as a side effect (a footgun for every
binary embedding the server), it exposes strictly more than needed
(`cmdline` leaks argv, goroutine dumps can leak pprof labels, CPU profiling
is a compute lever), and its `?gc=1` cannot be throttled without forking the
handler. Information-wise these endpoints expose allocation-site stacks and
counters, never heap contents — strictly less than pprof — behind the same
user token that already guards far more valuable targets (provider keys,
config apply). The full pprof suite remains available via the CLI's
explicit `--pprof` side server on `:6060`; the split is intentional: main
port = narrow, contract-shaped memory observation; `:6060` = full pprof,
opt-in at start.

Registration note: routes are registered in `server_control.go`
(`UseUIEndpoints`) **and** in `swagger.go` (`registerAllAPIRoutes`) — the
offline OpenAPI generator does not execute `UseUIEndpoints`, so a module
registered in only one place either misses the running server or misses
`openapi.json`. (Known drift: the `virtualmodel` module is currently
runtime-only.)

## 4. The memory phase

Per route, per instance (`DuoMemoryReport{TB1, TB2}`):

- **Retention slope** — warmup, baseline (post-GC), two sequential batches of
  N requests, post-GC sample after each. Slope = (after2 − after1) / N. A
  per-request pin shows as a positive slope; transient spikes and warm-up
  growth (caches, pools) land in batch 1's delta and cancel out of the slope.
  Threshold: 32 KB/request per instance (`duo_test.go`; #1255 measured 823,
  healthy tb2 measures ~0.5).
- **Allocation churn** — `total_alloc` delta per request; context for slope,
  not a failure signal.
- **Concurrent burst peak** — workers×per-worker requests while a sampler
  polls both instances' live heap (no GC) every 100 ms; catches
  amplification that sequential batches hide.
- **Goroutine counts** — baseline vs final, both instances; a goroutine leak
  is a memory leak with a different unit.
- **Heap profiles** — `duo-<route>-<tb>-{baseline,final}.pb.gz` fetched per
  instance when `--profile-dir` is set.

## 5. Backpressure (`-slow` routes)

The builtin vmodels answer instantly with ~130 bytes, which hides every
buffering behaviour that only appears when data is in flight. Each duo route
has a `-slow` variant that changes both ends:

- **tb1 side** — `duo-serve` registers `duo-slow-gpt` / `duo-slow-claude`
  (see `registerDuoStreamModels`): `--stream-kb` of content in ~2 KB chunks,
  streamed over roughly 2×`--stream-ms` (the vmodel `Delay` is applied once
  as TTFT by the virtualserver handler and spread again across chunks by the
  mock's stream loop).
- **client side** — the harness reads the SSE body through a `slowReader`
  (8 KB read window + `--read-delay-ms` pause), building real TCP
  backpressure against tb2 the way a slow consumer does.

Default memory routes are `beta-chat,beta-chat-slow`: the Claude Code hot
path fast *and* under backpressure, in one run.

## 6. Fidelity ledger

What matches production, and what still doesn't:

| Aspect | Status |
|--------|--------|
| Request pipeline (routing → transform → client pool → HTTP) | ✅ identical code path; `/tingly/anthropic/v1/messages` shares the `/tingly/:scenario/v1` handler with `claude_code` |
| Server lifecycle & background components | ✅ full `server.Start()` |
| Process isolation / per-role memory | ✅ separate processes |
| Slow upstream / slow client | ✅ `-slow` routes |
| TLS to upstream | ❌ loopback HTTP (production upstreams are HTTPS; TLS record buffers not exercised) |
| Request shape variety | ⚠️ alternating text messages only — no tool_use/thinking/image blocks in the conversation body |
| Real provider latency/response distribution | ❌ vmodel responses are deterministic |

Extend the request-shape axis in `BuildConversationBody` if a leak is ever
suspected in a block-type-specific conversion branch.

## 7. Entry points

```bash
./harness duo                          # functional all routes; memory fast + backpressure
./harness duo --mem-routes all         # slope on every fast route
./harness duo --skip-func --profile-dir /tmp -v
go test ./internal/protocoltest/ -run 'TestDuoFunctional|TestDuoMemoryRegression|TestDuoBackpressure' -v
```

Code map: `internal/protocoltest/duo.go` (parent: routes, spawning, request
driving) · `duo_serve.go` (child: boot + seeding, env contract) ·
`duo_checks.go` (functional phase) · `duo_memory.go` (per-instance memory
phase) · `internal/server/module/debug/` (observation endpoints) ·
`cli/harness/duo.go` (CLI + side-by-side report).
