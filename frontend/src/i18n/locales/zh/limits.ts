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
    "windowCustom": "自定义…",
    "windowCustomTitle": "自定义统计窗口",
    "windowCustomHelp": "整数分钟，1 到 {{max}}（最长 30 天）。",
    "windowMinutes": "分钟",
    "windowCustomInvalid": "请输入 1 到 {{max}} 之间的整数分钟。",
    "windowApply": "应用",
    "windowCancel": "取消",
    "maxRequests": "最大请求数",
    "maxTokens": "最大 Token 数",
    "maxTokensHint": "在请求结束时计入。可写 K、M，如 500K、2M。",
    "maxTokensInfoLabel": "Token 计算口径",
    "maxTokensInfo": "按每个已完成请求的 输入 + 输出 Token 计算。\n缓存写入计入，缓存读取不计入。\n推理（reasoning）Token 仅在上游已将其计入输出时才计入。",
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
    "cancel": "取消",
    "saved": "限额已保存",
    "saveFailed": "保存限额失败",
    "inheritedFromTeam": "此人没有单独限额，目前沿用 Team 的“每位成员默认限额”。",
    "overflow": {
      "label": "达到并发上限时",
      "hint": "用量额度用完时一律返回 429 并带 Retry-After。",
      "queue": "排队等待，超时后返回 429",
      "reject": "立即返回 429",
      "timeout": "最长等待（秒）"
    },
    "team": {
      "title": "{{team}} 的限额",
      "explain": "限额叠加在账号和规则限额之上，并且在请求被路由之前检查。各项上限独立判断，以最严格的为准。",
      "whole": "整个 Team",
      "wholeHint": "Team 内所有人共用这份额度。",
      "member": "每位成员",
      "memberHint": "每位成员各自独立计数，使用这里的限额；单个 Key 上设置的个人限额会覆盖它。",
      "button": "限额",
      "buttonTooltip": "Team 与成员限额"
    },
  "person": {
      "title": "{{name}} 的限额",
      "hint": "限制此 Key 的使用者在所有账号和模型上的总用量；Team 的限额仍然同时生效。"
    },
  "summary": {
      "concurrent": "并发 {{n}}",
      "requests": "{{n}} 次请求 / {{window}}",
      "tokens": "{{n}} Token / {{window}}",
      "steer": "达到 {{n}}% 时提前分流",
      "quota": "上游额度用到 {{n}}% 时停用",
      "service": "此规则的份额",
      "account": "账号",
      "indicator": "已设置限额",
      "clickToEdit": "点击编辑"
    }
  }
};
