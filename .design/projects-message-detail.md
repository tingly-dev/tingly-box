# Projects message detail

The conversation's display preference has two values, **Detailed / Concise**
(**详细 / 简洁**). Detailed is the default and uses the existing rendering,
including its normal folded tools/reports. Tool expansion is still a separate
inspection action, not a third detail level.

- Project worker threads and folder threads: Concise hides ordinary tool-call
  rows and results, not structured cards. Agent cards keep their header, prompt,
  report, focus action, expansion and lazy paging. User/assistant messages,
  thoughts, errors, checklists and working status remain. Approval/question cards
  stay actionable; settled decisions stay as request rows when their normal tool
  row is hidden. In Concise, Agent cards start open while a request is pending,
  so nested interactions remain discoverable without replacing their card.
- Project main thread (the manager/coordinator conversation): Concise keeps all
  human input (Web and IM), the manager model's own replies and **all cards and
  controls**. Conversational text with `source=thread` is filtered, whether
  its event kind is `user` or `assistant`: role is not provenance. Redundant
  `thread_update` notifications with payload `kind=finished` are also hidden;
  these are not the thread cards. Thread lifecycle cards (including ready,
  resolved and failed threads), needs-input/failed report rows, suggestions and
  error alerts remain, with their original expansion and actions.
  The composer, model/switch controls, Overview and waiting signals are unchanged.
  This main-thread renderer does not have approval/ask response controls; worker
  controls remain in the shared transcript.

Concise is not text truncation or generated summarization. Structured cards are
never treated as conversational text, even when they contain text or originate
from a thread. Unknown structured event kinds are not filtered preemptively.

Both use `MessageDetailControl` next to the composer. A tooltip explains the
surface-specific meaning and that this is display only (UX P3/P8/P9). The two
surfaces are remembered separately, avoiding surprising cross-surface changes
(P12), with the existing guarded browser storage:

- `tingly-project.conversation.messageDetail`
- `tingly-project.thread.messageDetail`

Values are `detailed` / `concise`. Missing or invalid settings default to
Detailed. With no new conversation setting, legacy
`tingly-project.conversation.threadReports = 0` maps to Concise; all other old
values map to Detailed. The new setting takes precedence. `thread.expandTools`
is preserved and is not interpreted as visibility. Mounted panes and browser
tabs synchronize the corresponding preference; navigation/reload reads it back.

Filtering happens after event conversion and call/result/request pairing, not
in the event cache, transport, or execution path. Block indexes remain stable
for trajectory anchors. Explicit “show in chat” for a hidden call switches to
Detailed before revealing it. Trajectory remains a full inspection view, with
its existing independent inspection filters; the detail picker controls the
conversation, not the trajectory. Request draft/submission storage is unchanged,
so switching detail levels cannot repeat an accepted decision.

No API/schema or backend setting is needed for this per-browser convenience.
