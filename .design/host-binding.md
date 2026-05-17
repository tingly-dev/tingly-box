# Host Binding: Design and Decision

> Audience: tingly-box backend contributors and anyone adding "local listener / local URL" code.
> This document records the `127.0.0.1` vs `localhost` policy and its rationale, as a reference for future PRs.

---

## 1. Background

tingly-box is a **locally hosted** AI gateway:

- Server side (HTTP server, OAuth callback server) binds the loopback interface only; not reachable externally.
- Client side (Claude Code / Codex / OpenCode CLI, browser OAuth redirect) hits that gateway from the same machine.

We had long used the literal `127.0.0.1`. Commit `4569e8f` (PR #966) blanket-replaced
`127.0.0.1` with `localhost` codebase-wide, claiming "much more robust". The actual
result is **mixed**, which triggered this review (PR #972).

---

## 2. Key facts

### 2.1 Behavior of `net.Listen("tcp", "<host>:<port>")`

Go's `net.Listen` resolves the host but only ever binds a **single** address:

| host argument | actual behavior |
|---|---|
| `127.0.0.1` | always binds IPv4 loopback; deterministic |
| `::1` | always binds IPv6 loopback |
| `localhost` | depends on the **first** bindable address returned by the resolver. Dual-stack Linux usually returns `::1` first; macOS usually returns `127.0.0.1`; containers without a `/etc/hosts` entry fail outright |
| `""` / `0.0.0.0` | binds all IPv4 interfaces (**externally reachable**; wrong for a gateway) |

→ **Using `localhost` for bind is a regression**: on dual-stack hosts where the
server ends up bound to `::1` only, any client that explicitly hits
`http://127.0.0.1:PORT` is refused.

### 2.2 Safety of `localhost` on the client side

| caller | resolution strategy |
|---|---|
| Go `net/http` (default `Transport`) | `Dial` calls `LookupHost`, gets all addresses, tries them in order |
| Chrome / Firefox / Safari | RFC 6555 happy eyeballs — race v4/v6, first to connect wins |
| Most Node / Python HTTP clients | same as Go — multi-address fallback |

→ **Writing `localhost` on the client side is essentially safe**: even if the
server only binds v4, the client resolving `localhost` to both `::1` and
`127.0.0.1` will fall back to v4 after v6 refuses.

### 2.3 Helper functions like `getLocalIP()`

The signature implies "returns an IP string". Downstream may:

- splice it into a URL (`http://<ip>:port/...`; `localhost` happens to work but only by luck)
- compare against an IP field (`if ip == "127.0.0.1"`; `localhost` silently breaks the check)
- show it in logs / metrics ("localhost" reads like a bug)

→ Returning `"localhost"` from a fallback is a **contract violation**; must
return an IP literal.

---

## 3. Decision

| Use | Choice | Why |
|---|---|---|
| `net.Listen` server bind | **`127.0.0.1`** | deterministic, dual-stack safe |
| Test-side bind that probes port availability (e.g. `getAvailablePort`) | **`127.0.0.1`** | must match the real server bind it's reserving for, otherwise probe and reality diverge |
| Fallback of "returns IP" helpers like `getLocalIP()` | **`127.0.0.1`** | preserve the function contract |
| URLs written into user configs / docs / Swagger examples / prompts | **`localhost`** | aligns with happy eyeballs; survives a future IP-family switch on the server without code changes |
| Internally generated client URLs (`ANTHROPIC_BASE_URL`, `baseURL`, etc.) | **`localhost`** | same |
| Host comparisons in code ("is this local?") | **recognize both**: `host == "127.0.0.1" \|\| host == "localhost"` (also `::1` if relevant) | covering only one of them ships a latent bug |

In short: **"Bind writes an IP, URL writes a name."**

### 3.1 At a glance

```
   ┌──────────────────────────────────────────────────────────────┐
   │  SERVER                                                       │
   │                                                               │
   │     net.Listen("tcp", "127.0.0.1:12580")                      │
   │                       │                                       │
   │                       ▼                                       │
   │              ┌──────────────────┐                             │
   │              │ 127.0.0.1:12580  │  one socket, IPv4 loopback  │
   │              └──────────────────┘  deterministic              │
   └─────────────────────────┬─────────────────────────────────────┘
                             │
                             │ TCP
                             │
   ┌─────────────────────────┴─────────────────────────────────────┐
   │  CLIENT  (CC / Codex / OpenCode / browser / Go http.Client)   │
   │                                                               │
   │     URL: http://localhost:12580/tingly/...                    │
   │                       │                                       │
   │                       │ resolve "localhost"                   │
   │                       ▼                                       │
   │              ┌──────────────────┐                             │
   │              │ [::1, 127.0.0.1] │  multiple candidates        │
   │              └──────────────────┘                             │
   │                       │                                       │
   │                       │  happy eyeballs (RFC 6555)            │
   │                       ▼                                       │
   │       try ::1       ──► refused  (server not on v6)  ──┐      │
   │                                                         │     │
   │       fallback ──► try 127.0.0.1  ──► SUCCESS  ◄────────┘     │
   │                                                               │
   └───────────────────────────────────────────────────────────────┘
```

Contrast: binding the server side to `localhost` too (the state after
commit 4569e8f) is the anti-example —

```
   ┌──────────────────────────────────────────────────────────────┐
   │  SERVER (WRONG)                                               │
   │                                                               │
   │     net.Listen("tcp", "localhost:12580")                      │
   │                       │                                       │
   │                       │ Go binds the first bindable address   │
   │                       ▼                                       │
   │              ┌──────────────────┐                             │
   │              │     ::1:12580    │  IPv4 loopback NOT bound    │
   │              └──────────────────┘                             │
   └─────────────────────────┬─────────────────────────────────────┘
                             │
                             │ ✗ any client using 127.0.0.1 is refused
                             │ ✗ containers without /etc/hosts fail to bind at all
```

---

## 4. Anti-patterns

Reject these in review:

```go
// ❌ server bind using localhost
net.Listen("tcp", "localhost:8080")

// ❌ getLocalIP / IP fallback returning a name
func getLocalIP() string {
    ...
    return "localhost"
}

// ❌ binding 0.0.0.0 without an explicit external-reachability requirement
net.Listen("tcp", "0.0.0.0:8080")

// ❌ host comparison covering only one form
if host == "127.0.0.1" { /* misses localhost */ }
```

Correct:

```go
// ✅
net.Listen("tcp", "127.0.0.1:8080")

// ✅
func getLocalIP() string {
    ...
    return "127.0.0.1"
}

// ✅
func isLoopback(host string) bool {
    return host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// ✅ internally generated client URL
url := fmt.Sprintf("http://localhost:%d/tingly/codex", port)
```

---

## 5. Migrating existing configs

Older versions of tingly-box wrote URLs like `http://127.0.0.1:12580/tingly/...`
into user-owned files on disk:

- `~/.claude/settings.json` → `env.ANTHROPIC_BASE_URL`
- `~/.codex/config.toml` → `[model_providers.tingly-box].base_url`
- `~/.config/opencode/opencode.json` → `provider.tingly-box.options.baseURL`

Under this policy those fields **should be `localhost`** (they're client URLs;
happy eyeballs covers the v4/v6 question).

`internal/server/config/migration_localhost.go`'s `migrate20260517` rewrites
them once. Sniffing rule: host must be `127.0.0.1` **and** the path begins with
`/tingly/` (Claude Code) **or** the entry sits under the `tingly-box` provider
key (Codex / OpenCode). User-authored non-tingly URLs (e.g.
`socks5://127.0.0.1:7890` pointing at a local v2ray) are left alone.

---

## 6. Related PRs / commits

| ref | content |
|---|---|
| `4569e8f` (PR #966) | codebase-wide `127.0.0.1 → localhost`, including server bind (**partial regression**) |
| `28d80fe` (PR #972) | migration for already-deployed external configs |
| `3cc336e` (PR #972) | revert the regressed server binds back to `127.0.0.1`; client URLs stay `localhost` |
| `88c450b` (PR #972) | pencil graph added to this doc |
