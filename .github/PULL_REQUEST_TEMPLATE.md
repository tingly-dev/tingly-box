<!--
  Tingly-Box PR description template.
  Fill in the sections below. The HTML comments are guidance, not output —
  delete each comment as you fill in (or leave for the reviewer) its section.
  Adapt headings/order to the change; never emit an empty or irrelevant section.
-->

<!--
Title — set it in GitHub's title field, not here (this comment doesn't render).
Format: type(scope): description   e.g. fix(imagegen): stop invisible delete button
type = feat/fix/refactor/perf/test/docs/chore/ci/security; scope = area
(imagegen, server, cli…) — an area name is the scope, never the type.
-->

## Summary
<!-- 1–2 sentences: the motivating problem and the resolved outcome.
     Adds the *why* the title cannot carry; do not repeat the title here. -->

## Key Changes

- **<!-- Theme -->**: <!-- behavior change, in one clause -->
<!-- Theme = a concrete functional boundary (workflow, migration, safety
     guarantee, dev experience) — never a file/class name or "Misc"/"Minor".
     Add a second clause only if it adds a *why*/*before-after*; a third
     clause means split into two bullets. Example:
       - **Tool-call ID fidelity**: Responses→Anthropic tool-use now carries
         the upstream `call_id`, fixing multi-turn tool-result correlation. -->

<!--
Optional — include only when it helps evaluation; omit when empty, never pad:

## Minor
  Genuinely incidental (small cleanups/docs/tests) — never core behavior,
  migrations, or safety, even when small; those stay in Key Changes.

## Notes
  What reviewers/operators must act on or watch: limitations, follow-ups,
  rollout concerns. State the consequence + next action, don't echo Key Changes.

Other domain headings (Migration, Compatibility, Testing, Risks, Screenshots…)
when relevant.
-->

<!--
Brevity gate — hard caps, not suggestions; default to the floor.

| Diff size | Summary    | Key Changes | Optional          | Total |
|-----------|------------|-------------|--------------------|-------|
| Small     | 1 sentence | ≤ 3 bullets | none               | ≤ 8   |
| Medium    | ≤ 2 sent.  | ≤ 5 bullets | ≤ 1 section, ≤ 2   | ≤ 15  |
| Large     | ≤ 2 sent.  | ≤ 7 bullets | ≤ 2 sections, ≤ 3  | ≤ 25  |
  (Small < ~100 lines/1–2 commits; Large = multi-feature/migration.)

One bullet = one line (~15 words after the colon); 3+ lines means rewrite or
split. Over budget → cut the weakest bullets, don't compress wording. One
fact, one place — nothing repeats across title/Summary/bullets/Notes.
-->

<!--
No footer details from agent except session link
-->
