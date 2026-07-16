# Task Supervision Security Review

**Status:** PASSED
**Date:** 2026-07-16
**Scope:** Changes from `543b7bb14^` through `01a26437c`

## Findings

- **Authentication:** Task routes, including control responses, are registered under the authenticated `/api/v1` group.
- **Control binding:** A response must match task ID, run ID, durable control ID, and the process-local broker waiter. Stale or restarted requests return conflict.
- **Replay protection:** Each waiter can be claimed once; duplicate responses are rejected.
- **Input validation:** Approval/question actions use a typed validation error; empty question answers and unsupported actions are rejected.
- **Sensitive data:** pending tool input is capped at 16 KiB and sensitive key names are redacted. Full tool input is not retained in permanent Run events after the decision; history retains a summary.
- **Command/path handling:** resume commands shell-quote the workspace and session ID. Agent execution continues to use argument arrays rather than shell interpolation.
- **Recovery:** restart clears pending controls and marks their runs interrupted, preventing UI approval of a dead native process.
- **Secrets scan:** no credible credential/private-key patterns were found in the feature diff. One filename-only false positive came from `task-...` text matching a broad `sk-` pattern.
- **Dependencies:** no dependency or lockfile changes were made, so dependency auditing was not expanded for this feature.

## Hardening applied

The review removed full control payloads from durable events, added active-status filtering to prevent history records hiding a live request, added typed decision validation, and added redaction/filter tests.
