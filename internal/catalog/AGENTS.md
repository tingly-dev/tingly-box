# internal/catalog

Static model data and its loaders. Read `.design/model-data.md` before editing data.

- `providers.json`: who serves which models (endpoints, limits).
- `claude.models.json`: per-model Claude thinking capabilities (`dialects`, `mandatory`, `supported_efforts`).
- `claude.models.ref.json`: reference mirror of Anthropic `/v1/models`; not embedded, not read by code.
- `catalog_check.go`: validity rules, shared by tests and `harness catalog check`.

## Rules

- New Claude model: add it to `claude.models.json` first, then `providers.json`.
- `claude.models.json` allows only `id` and `reasoning`; dialects `budget|adaptive`, efforts `low|medium|high|xhigh|max`.
- `providers.json`: map key == `id`; non-OAuth, non-cloud entries need a base URL; OAuth entries need `oauth_provider`.
- `context` / `max_output` are decimal (1M=1000000), never powers of two.
- `sources` / `note` / `last_updated` are human-only metadata; Go drops them.
- Omit values you can't source.

## Verify

```bash
go test ./internal/catalog/
go run ./cli/harness catalog check
```

CI (`harness-pr.yml`) runs `catalog check` on `internal/catalog/**` changes.

## New check rules

Add them to `catalog_check.go` with tests for both embedded-data-passes and bad-data-rejected. Don't duplicate validation in `cli/harness`.
