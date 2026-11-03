// Account/service concurrency & usage limits editor (see .design/routing-limits.md)
export default {
  "limits": {
    "title": "账号限额",
    "titleService": "此服务的限额",
    "hintAccount": "限额对所有使用该账号的规则和作用域生效。账号达到上限时，请求会转到规则里的其他账号；全部达到上限时由规则的溢出策略决定。",
    "hintService": "这条规则（团队场景下即该团队）可使用的账号份额；账号自身的限额仍然叠加生效。",
    "maxConcurrent": "最大并发请求数",
    "maxConcurrentHint": "同时进行中的请求数，留空表示不限。",
    "window": "用量窗口",
    "window1h": "1 小时",
    "window5h": "5 小时",
    "window24h": "24 小时",
    "window7d": "7 天",
    "maxRequests": "最大请求数",
    "maxTokens": "最大 Token 数",
    "maxTokensHint": "输入 + 输出 Token，在请求结束时计入。",
    "softPercent": "提前分流阈值（%）",
    "softPercentHint": "任一限额达到该比例后，优先使用其他账号；留空表示只在达到硬上限时才停用。",
    "quotaPercent": "上游额度停用阈值（%）",
    "quotaPercentHint": "当该账号的上游额度（如 5h/7d 窗口）用到该比例时跳过该账号；留空表示不看额度。",
    "live": "当前状态",
    "liveInflight": "{{count}} 个请求进行中",
    "liveWindow": "窗口内 {{requests}} 次请求 · {{tokens}} Token",
    "state": {
      "ok": "正常",
      "hot": "接近上限",
      "blocked": "已达上限"
    },
    "clear": "清除限额",
    "button": "限额",
    "buttonTooltip": "并发与用量限额",
    "save": "保存",
    "cancel": "取消"
  }
};
