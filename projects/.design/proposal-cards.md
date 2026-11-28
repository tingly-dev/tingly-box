# Proposal cards (suggested threads)

> Status: **built**. A proposal card is the `suggestions` event of a project's
> conversation (`projects/internal/project/coordinator.go`), drawn by
> `SuggestionsCard` in `frontend/packages/projects/src/components/project/ConversationPane.tsx`.
> Related: [thread-coordinator-channel.md](./thread-coordinator-channel.md) (`propose_threads`,
> the card as the confirmation), [mcp-tools.md](./mcp-tools.md).

## Why

The card used to be a one-off: one Start button per proposal, no way to say no,
and after a new instruction it still offered Start forever. It also differed
from the decision card (row padding, gaps, button order). A card is an offer;
an offer has to be refusable, can go stale, and should read like the other cards.

## Payload

`db.ThreadEvent{Kind: "suggestions"}` carries JSON:

```jsonc
{
  "title": "Release checklist",        // optional; the frontend's "Suggested threads" otherwise
  "intent": "Cut 2.4 after the PR.",   // optional body line
  "suggestions": [
    {
      "title": "Prepare the 2.4 release",
      "task": "Collect merged PRs, write the changelog, tag.",
      "repo": "", "base": "", "branch": "",  // start_thread settings
      "thread_id": "",                      // set once started
      "dismissed": false,                   // user declined
      "superseded": false                   // a newer instruction made it stale
    }
  ],
  "actions": [                              // optional; omitted keeps the defaults
    { "id": "start", "style": "primary", "scope": "item" },
    { "id": "dismiss", "style": "default", "scope": "item" },
    { "id": "start_all", "style": "primary", "scope": "card" },
    { "id": "dismiss_all", "style": "default", "scope": "card" }
  ]
}
```

The frontend renders the payload generically: `scope: "item"` actions sit in
each proposal row, `scope: "card"` actions in the footer. A built-in action with
no `label` gets the frontend's own words (localized); a service action always
brings its `label`. `style` is `primary` (contained), `default` (outlined,
inherit) or `danger` (outlined, error). An explicit empty `actions` array means
no actions; omitted means the four defaults above.

## State model

| state | set by | the card shows | start? |
|---|---|---|---|
| proposed | the card is posted | full row, actions | yes |
| started | `StartSuggestions` (thread_id set) | the thread's live row (`ThreadCard`) | — |
| dismissed | the user declines | nothing (row hidden); the record keeps the flag | no |
| superseded | a newer instruction / a newer card | row at 55% opacity + "Expired" chip | yes |

- `DismissSuggestions` writes `dismissed: true` on the picked proposals (empty
  indexes = all not started) and updates the event, so a reload or another
  device keeps them gone. Dismissing creates no thread and no input.
- `SupersedeSuggestions` writes `superseded: true` on every still-proposed
  proposal of older cards. A superseded proposal stays viewable and startable
  (the plan may still be wanted); only the card is weakened, and it can still be
  dismissed. This is preferred over deleting the card: the conversation keeps
  an honest record, and the user can still act on it.
- Dismissing clears `superseded` on that proposal; starting it sets `thread_id`
  and the state becomes started.

### When a card is superseded

- Any new user message to the coordinator's main thread: a queued message in
  `Coordinator.post`, or one inserted into a running turn in
  `coordinatorSessions.insert`. Answering a decision card is a user message too,
  so it supersedes older proposal cards.
- A newer proposal card supersedes the older ones (`ProposeCard`).

The rule is deliberately blunt: any new instruction means the coordinator is
re-planning, so the older plan is stale. A reply that answers a card through a
chat goes to `StartSuggestions`/`Choose` directly and does not pass through
`post`, so it does not expire the card it answers.

## Actions endpoint

`POST /api/v1/threads/{id}/events/{eid}/action` with
`{"action": "start|dismiss|start_all|dismiss_all|<custom>", "indexes": [0]}`.

- Built-ins map to `StartSuggestions` / `DismissSuggestions`.
- A custom id must be declared on the card **and** have a handler registered
  with `Coordinator.RegisterCardAction`; otherwise the request is refused
  (400), so a stale or forged button cannot run an arbitrary service action.
- The handler gets `(ctx, project, eventID, indexes, src)` and updates the
  event itself. The endpoint answers with the event as it is afterwards.
- `POST .../start` is kept for IM clients that predate the action endpoint.

## Service-driven proposals

A service can post its own card with
`Coordinator.ProposeCard(ctx, project, ProposalCard{...})` (same validation as
`propose_threads`, supersedes older cards, tells the chats). A custom action is
one line in `actions` plus one `RegisterCardAction`.

Deferred (not built, smallest useful next step): an HTTP endpoint for other
services to post cards (`POST /projects/{id}/proposal-cards` with the payload
above). `ProposeCard` plus `RegisterCardAction` are the Go seam; an HTTP route
would need an authorization story (project scope, like the MCP tools) before it
is safe to expose, so it is not part of this change.

## Chats

A proposal card reaches the project's chats too (`Relay.suggestions`): one
button per proposal, Start all, and Dismiss all, or the reply forms
`start 1,3` / `start all` / `dismiss 1,3` / `dismiss all`. Every state change
calls `Coordinator.OnCardSettled` → `Relay.CardSettled`, which rewrites the
chat prompt with what happened ("Started/Dismissed/Expired … on the web|in a
chat") and takes its buttons away, and forgets the request so the chat's next
message is no longer taken as an answer. `DismissSuggestions` publishes the
updated event, which is what settles the web card when the change came from a
chat — the same state everywhere. Proposals still waiting after a partial
change are listed in the settled line ("start it in the web UI") because a
restated prompt cannot carry new buttons.

## Mistake protection

Dismissing asks once in a `ConfirmDialog` (title + which proposal/all), then
commits. Undo was considered and rejected for now: it needs a restore action
and a place to keep the last dismissed set per card, while the confirmation
covers the actual risk. Starting is not destructive and has no dialog.

## Layout

`SuggestionsCard` uses the decision card's metrics: `Paper` outlined
`radius.card` with `p: 1.5`, an overline heading, rows `p: 1` `radius.row` on
`action.hover`, gap `0.75`, title `fontSizes.base`/600, body2 secondary. Card
actions wrap (`flex-wrap: wrap`); item actions stay beside the row with
`flex-shrink: 0`, so a narrow card wraps the footer and never widens.
`ConversationPane.test.tsx` asserts the card/row metrics against a decision card
and the footer's wrap.
