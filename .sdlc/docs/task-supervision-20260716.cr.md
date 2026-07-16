# Task Supervision Code Review

**Status:** PASS
**Date:** 2026-07-16
**Target:** Task supervision branch changes from `543b7bb14^` through `01a26437c`
**Blocking issues:** 0

## Review results

- **Correctness:** bounded Task and TaskRun states remain separate; live controls block only the current native run and finalization clears pending state.
- **Concurrency:** the one-shot broker synchronizes claim/delivery/timeout paths; targeted race tests pass.
- **Persistence:** memory and SQLite stores implement the same control, event, filtering, and recovery behavior.
- **API:** control responses validate task/run/control ownership and expose conflict semantics for stale delivery.
- **Frontend:** startup authority and tool scope are separate axes; current attention, concrete command input, resume command, and per-run outcomes are visible without navigating to another surface.
- **Performance:** task-list attention uses one active-status Run query rather than an N+1 query or a scan of arbitrary recent history.
- **Maintainability:** API models remain backend-first and the experimental frontend client is explicitly marked as a codegen placeholder.

## Issues found and resolved during review

1. **Major — durable events retained full approval input.** Resolved by retaining only the event summary after the pending request is cleared.
2. **Major — a fixed recent-run limit could omit an older still-active approval.** Resolved by adding Store-level Run status filtering.
3. **Minor — API classified invalid decisions by matching error strings.** Resolved with `ErrInvalidControlDecision` and `errors.Is`.

No unresolved critical or major findings remain in the reviewed scope.
