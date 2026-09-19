# Feishu/Lark: IM Service Built Directly, Not via the Fat SDK Client

## Problem

`imbot/platform/feishu.Bot` only ever calls three Lark API surfaces:
`Im.Message.Create`, `Im.Message.Patch`, `Im.MessageReaction.Create`, plus
the tenant-access-token check in `Connect()`. It got them by constructing
`lark.NewClient(appID, appSecret, ...)` — the SDK's top-level client, whose
`initService()` unconditionally builds all ~65 Lark product-line services
(`Corehr`, `Hire`, `Payroll`, `Okr`, `Attendance`, `Mail`, `Bitable`,
`Sheets`, …), none of which tingly-box ever touches. `lark.Client`'s field
list and `initService`'s body are the exhaustive evidence: every field is
constructed unconditionally in one function, with no lazy/opt-in path.

## Decision

Build the IM v1 service directly against a hand-assembled `*larkcore.Config`
instead of going through `lark.NewClient`:

```go
config := &larkcore.Config{
    BaseUrl: baseURL, AppId: clientID, AppSecret: clientSecret,
    EnableTokenCache: true, AppType: larkcore.AppTypeSelfBuilt,
}
larkcore.NewLogger(config)
larkcore.NewCache(config)
larkcore.NewSerialization(config)
larkcore.NewHttpClient(config)
client := larkim.New(config) // im/v1, same type client.Im.V1 pointed to
```

This is not a reimplementation — it's the literal same construction path
`lark.NewClient` runs before `initService`, verified line-for-line against
the vendored SDK source (`client.go`): same `Config` field values (checked
against `WithOpenBaseUrl`/`WithEnableTokenCache`'s effect), same four
bootstrap calls, same underlying type (`im.NewService(config)` is exactly
`&Service{V1: v1.New(config), V2: v2.New(config)}` — pure struct
composition, no side effects, so skipping the unused `V2` changes nothing).
`Connect()`'s auth check (`GetTenantAccessTokenBySelfBuiltApp`) is likewise
copied from the SDK method body byte-for-byte, taking `*larkcore.Config`
directly instead of `*lark.Client`.

WebSocket receiving (`StartReceiving`) is untouched: `larkws.NewClient`
always took `appID`/`appSecret` directly, never routed through
`lark.Client`.

## Evidence

Measured with a real linked-binary comparison (`git stash` / `go build -o` /
`ls -la`, not `tingly-go weight`'s compiled-archive estimate — see
[FFengIll/tingly-go#12](https://github.com/FFengIll/tingly-go/issues/12) for
why that number is not a reliable proxy for actual savings):

| | size |
|---|---|
| before | 151.0 MB |
| after | 134.3 MB |
| **saved** | **16.7 MB (11.1%)** |

## Known limitation: `event/dispatcher` still links per-service event types

The ~16.7MB saved is real but far smaller than the ~252MB of "unused" SDK
code `tingly-go weight`'s per-package archive sizes suggested was on the
table. The gap is a second, independent bloat source this change does not
touch: `imbot/platform/feishu` also uses
`github.com/larksuite/oapi-sdk-go/v3/event/dispatcher` for WebSocket event
routing (`OnP2MessageReceiveV1`, `OnP2CardActionTrigger`) — needed
regardless. That package ships one file per service
(`corehr_v1_event_dispatch.go`, `corehr_v2_event_dispatch.go`, …), each
adding dozens of `OnP2<Service>Xxx(handler)` methods to the *same*
`*EventDispatcher` type we call two methods on. `go tool nm` on the final
binary confirms those unrelated handler types (e.g.
`P2ContractCreatedV1Handler.Handle`) are still linked even though nothing
calls the methods that would register them, for reasons not fully
root-caused here (see the tingly-go issue for the working theory).

## Non-goals

- Not fixed here: eliminating the `event/dispatcher` bloat would mean
  replacing the SDK's monolithic dispatcher with a minimal one that parses
  only the two event types this bot handles — a materially larger,
  higher-risk change than this one, deferred pending a decision on whether
  ~100+ MB is worth that rewrite.
- `imbot/platform/lark`'s 44-line wrapper (`lark.NewBot` →
  `feishu.NewBot(config, feishu.DomainLark)`) needed no change — it only
  selects `Domain`, never touches `Bot`'s internals.
