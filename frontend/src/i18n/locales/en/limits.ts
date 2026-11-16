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
    "cancel": "Cancel"
  }
};
