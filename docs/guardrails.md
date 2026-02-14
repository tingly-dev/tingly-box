# Guardrails

This module provides a flexible, standalone guardrails engine that can be configured via JSON/YAML and unit-tested independently. It supports rule-based filters (e.g., text/regex) and model-judge rules for more flexible safety checks.

## Content-aware filtering

Content is split into three parts, and rules can target each part independently:

- `text`: the single model response message
- `messages`: full message history
- `command`: function-calling payloads

Two knobs control targeting:

- `scope.content_types`: gate whether a rule should run for an input
- `params.targets`: define which content parts a rule should evaluate

If `targets` is omitted, rules evaluate all available content by default.

## Example configuration

See `docs/examples/guardrails.yaml` for a complete sample. A minimal rule-based filter looks like:

```yaml
rules:
  - id: "dangerous-command"
    name: "Block Dangerous Command"
    type: "text_match"
    enabled: true
    scope:
      content_types: ["command"]
    params:
      patterns: ["rm -rf"]
      targets: ["command"]
      verdict: "block"
```

## Rule types

### text_match

Matches patterns against content. Supports literal or regex matching, case sensitivity, and minimum match counts.

Common params:

- `patterns` (required)
- `targets` (optional): `text`, `messages`, `command`
- `use_regex` (optional)
- `case_sensitive` (optional)
- `verdict` (optional)
- `reason` (optional)

### model_judge

Delegates to an external judge model/service. The judge implementation is injected and can use the filtered content.

Common params:

- `model` (optional)
- `prompt` (optional)
- `targets` (optional)
- `verdict_on_error` (optional)
- `verdict_on_refuse` (optional)

## Loading config

You can load YAML/JSON from disk with `guardrails.LoadConfig`, then build an engine:

```go
cfg, err := guardrails.LoadConfig("guardrails.yaml")
if err != nil {
    // handle error
}
engine, err := guardrails.BuildEngine(cfg, guardrails.Dependencies{})
```
