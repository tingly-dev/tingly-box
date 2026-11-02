# Decision endpoints and dialects

Both `POST /tingly/:scenario[/v1]/decisions` and `.../systemone` share the
same authentication, team scope, rule routing, usage accounting and handler.
Both identify OpenAI Decisions (`input`, question array) and System One
(`state`, question object) independently of the inbound URL. Mixed shapes are
rejected. Legacy requests with neither identifying field remain opaque.

## Upstream selection

`internal/decision.ResolveTarget` is the single registry for the
known upstream endpoint and dialect. Exact host matching uses the resolved
OpenAI-compatible provider base, not a model or provider display name:

| Host | Upstream endpoint | Dialect |
| --- | --- | --- |
| `api.typesafe.ai` | `/v1/systemone` | System One |
| `openrouter.ai` | `/api/alpha/decisions` | System One |
| `api.openai.com` | `{base}/decisions` | OpenAI |
| `ai-gateway.vercel.sh` | `{base}/decisions` | OpenAI |

Unknown hosts and unrecognized OpenAI-compatible path prefixes keep `{base}/decisions` and opaque forwarding. A virtual provider
uses System One. The virtual server also converts incoming OpenAI bodies when
called directly. There is no UI override or automatic endpoint probing.

## Conversion

`internal/decision` performs request conversion only when both formats
are known and differ. A per-request context carries question order, original
names, choice values and score labels to convert the response. The model field
is replaced by the routed model upstream and echoed back as the caller's model.
Same-format and unknown-format requests continue as opaque passthrough.

The supported common subset is plain text, named or unnamed predicates/noul,
string choices with string descriptions, and scores with 2–10 string labels.
OpenAI score descriptions, boolean choices, structured input/instructions,
image assets, noul criteria, cache controls and other extra fields are rejected
before contacting an upstream. Question counts and choice limits are checked.
No probabilities, confidence or answer values are fabricated; incompatible,
refused, incomplete or unexpected upstream answers produce 502 after usage
accounting. Original model usage and provider metadata are retained, with
known token spelling aliases normalized and total calculated only when both
counts exist. Providers that omit token breakdowns cannot supply all OpenAI
usage-detail fields; the gateway does not invent them.

Decisions remain non-streaming, use content-free service selection, and skip
the chat transform chain. The endpoint does not perform automatic failover.
Conversion is based on the published OpenAI schema and TypeSafe examples;
provider-specific extensions and live credentials require separate verification.

References: [OpenAI Decisions](https://developers.openai.com/api/docs/guides/decisions)
and [TypeSafe OpenAPI](https://api.typesafe.ai/openapi.json). Both define score
as the expected value of zero-based ordered levels.
