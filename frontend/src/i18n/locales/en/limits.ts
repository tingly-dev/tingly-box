// Account/service concurrency & usage limits editor (see .design/routing-limits.md)
export default {
  "limits": {
    "title": "Account limits",
    "titleService": "Limits for this service",
    "hintAccount": "Caps apply to every rule and scope that uses this account. When an account is at its limit, requests go to another account of the rule; if none is left the rule's overflow policy decides.",
    "hintService": "This rule's share of the account (for a Team: the Team's share). The account's own limits still apply on top.",
    "maxConcurrent": "Max concurrent requests",
    "maxConcurrentHint": "In-flight requests at once. Empty = unlimited.",
    "window": "Usage window",
    "window1h": "1 hour",
    "window5h": "5 hours",
    "window24h": "24 hours",
    "window7d": "7 days",
    "maxRequests": "Max requests",
    "maxTokens": "Max tokens",
    "maxTokensHint": "Input + output tokens. Counted when a request finishes.",
    "softPercent": "Steer traffic away at (%)",
    "softPercentHint": "Prefer other accounts once any limit reaches this share. Empty = only stop at the hard limit.",
    "quotaPercent": "Stop at upstream quota (%)",
    "quotaPercentHint": "Skip this account once its provider quota (e.g. 5h/7d window) is used up to this share. Empty = ignore quota.",
    "live": "Right now",
    "liveInflight": "{{count}} in flight",
    "liveWindow": "{{requests}} requests · {{tokens}} tokens in window",
    "state": {
      "ok": "OK",
      "hot": "Near limit",
      "blocked": "At limit"
    },
    "clear": "Clear limits",
    "button": "Limits",
    "buttonTooltip": "Concurrency & usage limits",
    "save": "Save",
    "cancel": "Cancel",
    "saved": "Limits saved",
    "saveFailed": "Failed to save limits",
    "inheritedFromTeam": "No personal limit is set. This person currently follows the team's per-member default.",
    "overflow": {
      "label": "When the concurrency limit is hit",
      "hint": "A used-up usage budget always answers 429 with Retry-After.",
      "queue": "Wait in line, then 429",
      "reject": "Reject with 429 immediately",
      "timeout": "Max wait (seconds)"
    },
    "team": {
      "title": "Limits for {{team}}",
      "explain": "Limits apply on top of account and rule limits and before a request is routed. Each cap is checked independently; the strictest one decides.",
      "whole": "Whole team",
      "wholeHint": "Everyone in the team shares this budget.",
      "member": "Each member",
      "memberHint": "Every member gets their own counter with these limits. A personal limit on a key overrides it.",
      "button": "Limits",
      "buttonTooltip": "Team and per-member limits"
    },
  "person": {
      "title": "Limits for {{name}}",
      "hint": "Caps what this key's user may use, across every account and model. The team's limits still apply."
    },
  "summary": {
      "concurrent": "{{n}} concurrent",
      "requests": "{{n}} requests / {{window}}",
      "tokens": "{{n}} tokens / {{window}}",
      "steer": "steer away at {{n}}%",
      "quota": "stop at {{n}}% of upstream quota",
      "service": "This rule's share",
      "account": "Account",
      "indicator": "Limits are set"
    }
  }
};
