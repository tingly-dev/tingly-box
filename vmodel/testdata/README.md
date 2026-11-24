# VModel examples

Every model definition in this directory uses the same outer structure:

```yaml
id: example
kind: mock
mock:
  content: Hello.
expect:
  http_status: 200
  content_equals: Hello.
```

`id` and `kind` identify the model and its behavior. The behavior block depends
on the kind; `expect`, `protocols`, `name` and `description` are optional common
fields. The shared loader accepts both JSON and YAML.

| File | Kind | Behavior block |
| --- | --- | --- |
| `model.yaml` | `mock` | `mock` |
| `unified.yaml` | `mock` | `mock` |
| `recorded.yaml` | `recording` | `responses`, keyed by protocol |
| `sequence.yaml` | `sequence` | `sequence` |
| `decision.yaml` | `decision` | `decision` |

The former legacy fixtures duplicated `model.yaml` and `recorded.yaml` and
have been removed. Every definition must follow the explicit `id`/`kind`
contract; there is no legacy-import path. See
[the definition documentation](../spec/README.md).

Captured client requests and golden wire outputs in other testdata directories
are protocol inputs/evidence, not VModel definitions, and retain their native
wire format.
