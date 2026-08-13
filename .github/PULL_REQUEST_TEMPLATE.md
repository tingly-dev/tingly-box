<!--
  Tingly-Box PR description template.
  Fill in the sections below. The HTML comments are guidance, not output —
  delete each comment as you fill in (or leave for the reviewer) its section.
  Adapt headings/order to the change; never emit an empty or irrelevant section.
-->

## Summary
<!-- 1–2 sentences: the motivating problem and the resolved outcome.
     Adds the *why* the title cannot carry; do not repeat the title here. -->
-->

## Key Changes

- **<!-- Theme -->**: <!-- behavior change, in one clause -->
<!-- Theme rules:
     - Themes are concrete functional boundaries: a user workflow, lifecycle,
       compatibility boundary, migration, safety guarantee, or developer experience.
     - Never use file names, class names, or vague labels ("Improvements",
       "Miscellaneous", "Major", "Minor") as themes.
     - Add a second clause only when it changes the meaning (a *why*,
       a *so-what*, or a *before/after*). If a third clause is needed, split
       into two bullets. Example:
         - **Tool-call ID fidelity**: Responses→Anthropic tool-use now carries
           the upstream `call_id`, fixing multi-turn tool-result correlation.
-->

<!--
Optional sections — include only when they make the PR easier to evaluate.
Never invent or demote content to fill them; omit when empty.

## Minor
  Genuinely incidental work done along the way (small cleanups, docs, tests).
  Never core behavior, migrations, compatibility guarantees, or safety
  protections — those stay in Key Changes even when small.

## Notes
  Only what reviewers/operators must act on or watch: limitations, follow-ups,
  rollout concerns. State the consequence and next action. Never echo Key Changes.

Other domain headings (Migration, Compatibility, Testing, Rollout, Risks,
Screenshots, …) when relevant.
-->

<!--
────────────────────────────────────────────────────────────────────────────
Brevity gate — check before submitting.

Length budget — hard caps, not suggestions. Default to the floor of each range;
only approach the cap when the diff genuinely demands it. When over budget,
cut bullets, don't compress wording — merging two points into one dense line
still fails the budget.

| Diff size | Summary    | Key Changes | Optional sections          | Total   |
|-----------|------------|-------------|----------------------------|---------|
| Small     | 1 sentence | ≤ 3 bullets | none                       | ≤ 8     |
| Medium    | ≤ 2 sent.  | ≤ 5 bullets | ≤ 1 section, ≤ 2 bullets   | ≤ 15    |
| Large     | ≤ 2 sent.  | ≤ 7 bullets | ≤ 2 sections, ≤ 3 each     | ≤ 25    |
  (Small < ~100 lines or 1–2 commits; Large = multi-feature / migration.)

- One bullet = one line after wrapping (~15 words after the colon). A bullet
  that wraps to 3+ lines is a paragraph — rewrite or split.
- The budget is the default output. Only exceed it when the reviewer
  explicitly asks for a detailed description.

Final checks:
- Within budget. Count lines; if over, delete the weakest bullets until it fits.
- One fact, one place. Nothing repeats across title / Summary / bullets / Notes.
- Each bullet earns its line. No re-explanations, no "this is safe" reassurances,
  no hedges; two bullets must not say the same thing at different altitudes.
- Pick the shorter phrasing when two carry equal information.
- Scannable. Reading only the bold themes reveals the shape of the PR.
────────────────────────────────────────────────────────────────────────────
-->
