export default {
  "common": {
    "add": "添加",
    "cancel": "取消",
    "save": "保存",
    "delete": "删除",
    "edit": "编辑",
    "confirm": "确认",
    "loading": "加载中...",
    "enabled": "已启用",
    "disabled": "已禁用",
    "active": "活动",
    "inactive": "非活动",
    "close": "关闭",
    "copy": "复制",
    "refresh": "刷新",
    "verify": "验证",
    "saveChanges": "保存更改",
    "success": "成功",
    "error": "错误",
    "warning": "警告",
    "info": "信息",
    "on": "On",
    "off": "Off",
    "direct": "Direct",
    "theme": "主题",
    "openClaw": "OpenClaw",
    "prompt": "提示词"
  },
  "layout": {
    "appTitle": "Tingly Box",
    "slogan": "智能，尽在掌握。",
    "version": "版本<br/>{{version}}",
    "settings": "设置",
    "nav": {
      "home": "智能应用",
      "settings": "设置",
      // Scenario names intentionally kept in English — do not translate.
      "useOpenAI": "OpenAI SDK",
      "useAnthropic": "Anthropic SDK",
      "useCodex": "Codex",
      "useClaudeCode": "Claude Code",
      "useClaudeDesktop": "Claude Desktop",
      "useOpenCode": "OpenCode",
      "useXcode": "Xcode",
      "useVSCode": "VS Code",
      "useEmbed": "Embedding",
      "useImageGen": "Image Gen",
      "useTeam": "Team",
      "playground": "Playground",
      "apiKeys": "API 密钥",
      "oauth": "OAuth 凭证",
      "credential": "凭证",
      "prompt": "提示词"
    },
    "sidebar": {
      "newProfile": "新建配置文件",
      "profileName": "配置文件名称",
      "mode": "模式",
      "modeUnified": "统一：所有模型使用相同配置",
      "modeSeparate": "分离：每个模型单独配置",
      "separate": "分离",
      "unified": "统一",
      "createProfileTooltip": "创建一个新的 Claude Code 配置文件，自定义设置",
      "sloganTooltip": "致，所有独立开发者、开发团队和智能应用。"
    },
    "activityBar": {
      "disconnected": "已断开",
      "disconnectedDebug": "已断开（调试）",
      "devMode": "开发",
      "newVersionAvailable": "更新",
      "error": "错误",
      "theme": "主题",
      "light": "浅色",
      "dark": "深色",
      "system": "跟随系统",
      "sunlit": "日光",
      "claude": "Claude",
      "click": "点击",
      "feedback": "反馈",
      "feedbackTooltip": "发送反馈（跳转到 GitHub Issues）"
    },
    "themeMenu": {
      "switchTo": "切换到：",
      "theme": "主题："
    },
    "easterEgg": "Hi，我是 Tingly-Box，为您掌控智能",
    "dashboard": "仪表盘",
    "usage": "用量",
    "heatmap": "热力图",
    "today": "今天",
    "yesterday": "昨天",
    "days": "天",
    "remote": "远程访问",
    "overview": "概览",
    "platforms": {
      "weixin": "微信",
      "wecom": "企业微信",
      "telegram": "Telegram",
      "feishu": "飞书",
      "lark": "Lark",
      "dingtalk": "钉钉"
    },
    "guardrails": "防护栏",
    "policyGroups": "策略组",
    "policies": "策略",
    "guardrailsHistory": "历史",
    "mcp": "MCP",
    "sources": "来源",
    "localMode": "本地模式",
    "modelKey": "模型密钥",
    "tinglyBox": "共享分发",
    "tinglyBoxTooltip": "不暴露你的 Provider 凭据即可分发模型访问。每个分发令牌独立计量，可以随时单独吊销而不影响其他令牌。",
    "virtualModels": "虚拟模型",
    "virtualModelsTooltip": "内置的合成模型 Provider，用于上手演示与本地试跑——无需联网即在进程内返回响应。",
    "accessControl": "访问控制",
    "status": "状态",
    "system": "系统",
    "general": "通用",
    "experimental": "实验功能",
    "logs": "错误检查",
    "userRequest": "用户请求",
    "skills": "技能",
    "addProfile": "添加配置文件",
    "default": "默认",
    "onboarding": "快速添加提供商",
    "onboardingHint": "浏览或粘贴配置",
    "onboardingShort": "入门"
  },
  "health": {
    "connected": "已连接",
    "disconnected": "已断开",
    "checking": "检查中...",
    "lastChecked": "最后检查：{{time}}",
    "never": "从未",
    "retry": "重试",
    "disconnectMessage": "与服务器的连接已丢失。请检查服务器是否正在运行。",
    "disconnectTitle": "连接已丢失"
  },
  "update": {
    "newVersionAvailable": "新版本可用",
    "versionAvailable": "最新：{{latest}}（您当前使用 {{current}}）",
    "download": "下载",
    "close": "关闭",
    "checking": "正在检查更新...",
    "message": "GitHub 上有新版本可用。您想现在下载吗？",
    "later": "稍后",
    "check": "检查更新",
    "checkUpdates": "手动检查更新",
    "upToDate": "您使用的是最新版本",
    "updateAvailable": "有新版本可用",
    "currentVersion": "当前版本：{{version}}",
    "versionComparison": "{{latest}} 可用（您当前使用 {{current}}）",
    "releaseNotes": "查看更新说明",
    "updateMethods": "更新方式",
    "copy": "复制",
    "copied": "已复制！",
    "error": "检查更新失败",
    "methods": {
      "npx": {
        "title": "快速更新（npx）",
        "description": "使用最新版本运行一次"
      },
      "bundle": {
        "title": "离线包（npx）",
        "description": "下载包含内置二进制文件的离线包，解决网络问题"
      },
      "docker": {
        "title": "Docker 镜像",
        "description": "从 GitHub Container Registry 拉取镜像"
      }
    }
  },
  "login": {
    "title": "Tingly Box",
    "subtitle": "智能，尽在掌握",
    "tokenLabel": "身份验证令牌",
    "tokenHelper": "输入您的用户身份验证令牌以访问 UI 和管理功能",
    "loginButton": "登录",
    "validating": "验证中...",
    "generateTokenButton": "生成新令牌",
    "errors": {
      "invalidToken": "令牌无效。请检查您的令牌后重试。",
      "validationFailed": "令牌验证失败。请检查您的连接后重试。",
      "enterValidToken": "请输入有效的令牌"
    },
    "success": {
      "loginSuccess": "登录成功！正在重定向..."
    }
  },
  "home": {
    "tabs": {
      "useOpenAI": "使用 OpenAI",
      "useAnthropic": "使用 Anthropic",
      "useClaudeCode": "使用 Claude Code"
    },
    "emptyState": {
      "title": "没有可用的 API 密钥",
      "description": "添加您的第一个 AI API 密钥以开始使用服务。",
      "button": "连接您的第一个 AI"
    },
    "token": {
      "generated": "{{label}} 已复制到剪贴板！",
      "copyFailed": "复制到剪贴板失败",
      "generationFailed": "生成令牌失败：{{error}}",
      "refresh": {
        "title": "确认刷新令牌",
        "alert": "重要提醒",
        "description": "修改令牌将导致已配置的工具无法使用。您确定要继续生成新令牌吗？",
        "button": "确认刷新"
      }
    },
    "notifications": {
      "providerAdded": "提供商添加成功！",
      "providerAddFailed": "添加提供商失败：{{error}}"
    }
  },
  "provider": {
    "pageTitle": "凭证",
    "subtitleWithCount": "正在管理 {{count}} 个提供商和 API 密钥",
    "subtitleEmpty": "尚未配置 API 密钥",
    "addButton": "连接 AI",
    "emptyCardTitle": "未配置模型 API 密钥",
    "emptyCardSubtitle": "连接您的第一个 AI 提供商以开始使用",
    "emptyCardButton": "连接您的第一个提供商",
    "emptyCardContent": "配置您的 API 令牌和密钥以访问 AI 服务",
    "notifications": {
      "loadFailed": "加载提供商失败：{{error}}",
      "added": "提供商添加成功！",
      "updated": "提供商更新成功！",
      "deleted": "提供商删除成功！",
      "addFailed": "添加提供商失败：{{error}}",
      "updateFailed": "更新提供商失败：{{error}}",
      "deleteFailed": "删除提供商失败：{{error}}",
      "toggleFailed": "切换提供商失败：{{error}}",
      "loadDetailFailed": "加载提供商详情失败：{{error}}"
    }
  },
  "providerDialog": {
    "addTitle": "连接 AI",
    "addDescription": "选择提供商并输入您的 API 密钥以连接 AI 服务。支持多种协议的提供商可以启用多个协议。",
    "editTitle": "修改连接",
    "addButton": "连接",
    "protocol": {
      "label": "协议",
      "openAILabel": "OpenAI 兼容",
      "anthropicLabel": "Anthropic 兼容",
      "helperOpenAI": "支持来自 OpenAI、Google 和许多其他 OpenAI 兼容提供商的模型",
      "helperAnthropic": "用于 Anthropic 兼容的 AI 提供商，通常与 Claude Code 一起使用",
      "fromTemplate": "来自模板",
      "recommendedBadge": "推荐"
    },
    "candidates": {
      "title": "匹配的提供商 — 点击填充 URL"
    },
    "keyName": {
      "label": "名称",
      "placeholder": "例如：OpenAI",
      "default": "默认供应商",
      "helper": "留空将使用上方自动生成的名称，创建后可随时重命名。",
      "editAction": "编辑名称"
    },
    "providerOrUrl": {
      "label": "提供商或自定义基础 URL",
      "placeholder": "选择提供商或输入自定义 URL"
    },
    "apiKey": {
      "label": "API 密钥",
      "placeholderAdd": "您的 API 密钥",
      "placeholderEdit": "留空以保留当前密钥",
      "helperEdit": "留空以保留当前密钥"
    },
    "enabled": "已启用",
    "advanced": {
      "title": "高级",
      "proxyUrl": {
        "label": "HTTP/SOCKS 代理 URL（可选）",
        "placeholder": "http://127.0.0.1:7890 或 socks5://127.0.0.1:7890",
        "helper": "可选：使用代理绕过区域限制。将保存以供将来使用。",
        "useGlobal": "使用常用代理（{{url}}）",
        "useGlobalNotSet": "使用常用代理（未配置 — 请在系统设置中配置）"
      }
    },
    "verification": {
      "verifying": "验证中...",
      "verifyButton": "验证",
      "missingFields": "请填写所有必填字段（API 风格、名称、API 基础 URL、API 密钥）",
      "failed": "连接检查失败",
      "networkError": "无法连接。请检查您的网络和代理设置。",
      "failureHint": "如果您确定配置正确，仍可以通过'仍要添加'按钮添加此提供商。",
      "responseTime": "响应时间：{{time}}ms",
      "modelsAvailable": "可用 {{count}} 个模型",
      "testResult": "测试结果：{{result}}"
    },
    "forceAdd": {
      "title": "仍要添加提供商？",
      "providerInfo": "请确认您的提供商配置：",
      "message": "连接检查失败。这可能是由于网络问题、API 密钥错误或提供商不支持标准验证方法。",
      "explanation": "某些提供商可能无法通过标准检查，但仍能正常工作。",
      "whyFailed": "连接检查失败：",
      "errorDetails": "错误详情",
      "noKey": "未提供",
      "confirmNoteTitle": "您确定要继续吗？",
      "confirmNote": "添加前请验证您的基础 URL 和 API 密钥是否正确。您仍可以添加此提供商，但如果配置不正确，可能无法正常工作。",
      "cancel": "返回",
      "confirm": "确认添加"
    }
  },
  "providerTable": {
    "columns": {
      "name": "名称",
      "apiKey": "API 密钥",
      "apiBase": "API 基础",
      "apiStyle": "API 风格",
      "actions": "操作",
      "status": "状态"
    },
    "status": {
      "enabled": "已启用",
      "disabled": "已禁用"
    },
    "token": {
      "notSet": "未设置",
      "view": "查看令牌",
      "viewTooltip": "查看令牌"
    },
    "deleteModal": {
      "title": "删除提供商",
      "description": "您确定要删除提供商 \"{{name}}\" 吗？此操作无法撤销。",
      "cancelButton": "取消",
      "confirmButton": "删除"
    },
    "tokenModal": {
      "title": "API 密钥 - {{providerName}}",
      "loading": "正在加载 API 密钥...",
      "failedToLoad": "加载令牌失败",
      "copyButton": "复制令牌",
      "loadingTooltip": "加载中...",
      "closeTooltip": "关闭"
    }
  },
  "templateActions": {
    "troubleshoot": "错误检查",
    "collapseAllRules": "收起所有规则",
    "expandAllRules": "展开所有规则",
    "connectAI": "连接 AI",
    "newRule": "新建规则",
    "createNewRule": "创建新的路由规则",
    "howRoutingWorks": "路由原理"
  },
  "probe": {
    "quickTest": "快速测试（流式）",
    "testAll": "测试全部",
    "testAllHint": "对所有激活规则运行一次流式快测",
    "viewDetails": "查看详情",
    "dismiss": "关闭",
    "testRule": "测试规则",
    "testProvider": "测试服务",
    "shape": "请求",
    "nonstream": "非流式",
    "stream": "流式",
    "scope": "范围",
    "throughTB": "经过 TB",
    "direct": "直连上游",
    "scopeHint": "直连上游会绕过 Tingly-Box 的路由与中间件,用于判断故障在上游还是 TB 内部。",
    "run": "运行测试",
    "running": "测试中…",
    "runHint": "选择请求类型后点击「运行测试」",
    "rerun": "重新测试",
    "copyResponse": "复制响应",
    "copied": "已复制!",
    "success": "成功",
    "failed": "失败",
    "journey": "请求旅程",
    "response": "响应",
    "rawJson": "原始 JSON",
    "rawJsonHide": "收起原始 JSON",
    "noText": "（无法提取文本,见原始 JSON）",
    "pending": "— 待补",
    "flagsNone": "（无）",
    "directValue": "直连上游（已绕过 TB）",
    "row": {
      "rule": "规则",
      "flags": "Flags",
      "routing": "路由",
      "provider": "服务商",
      "endpoint": "端点",
      "upstreamUrl": "上游 URL",
      "requestUrl": "请求 URL"
    }
  },
  "rule": {
    "pageTitle": "高级代理配置",
    "subtitle": "配置本地模型以将请求转发到远程提供商",
    "addButton": "添加转发规则",
    "emptyState": {
      "title": "未配置规则",
      "description": "点击「添加规则」创建您的第一个规则"
    },
    "card": {
      "unspecifiedModel": "请指定模型名称",
      "useKey": "使用 {{count}} 个 {{key}}",
      "key": "密钥",
      "keys": "密钥",
      "responseAs": "响应为 {{model}}"
    },
    "graph": {
      "title": "请求代理可视化",
      "requestLocalModel": "请求模型名称",
      "responseModel": "响应模型",
      "requestLocalTooltip": "客户端用来发出请求的模型名称。这将与传入的 API 调用进行匹配。",
      "responseTooltip": "返回给客户端的模型名称。来自上游提供商的响应将被转换以显示此模型名称。",
      "forwardingToProviders": "转发到提供商",
      "addProvider": "添加提供商",
      "noProviders": "未配置提供商",
      "legend": "• 点击提供商节点选择提供商和模型",
      "selectProvider": "选择提供商",
      "selectModel": "选择模型"
    },
    "service": {
      "providerNotFound": "找不到该提供商，请刷新或重新导入。",
      "selectProvider": "选择提供商",
      "selectModel": "选择模型",
      "testService": "测试服务",
      "editProvider": "编辑服务商凭证",
      "deleteService": "删除服务"
    },
    "tier": {
      "invalidInput": "请输入有效的数字。",
      "tooltipSet": "层级 {{tier}}（数值越小越优先）。点击修改。",
      "tooltipUnset": "未设置层级（与其他 T0 模型负载均衡）。点击分配。",
      "ariaLabel": "层级 {{tier}}",
      "ariaUnset": "未设置层级",
      "editTitle": "设置层级",
      "adjustTier": "调整层级",
      "helpHigher": "数值越小越优先（T0 最先尝试），同一层级内的模型将负载均衡。",
      "helpZero": "设为 0 即 T0——第一层级。",
      "tierLabel": "T{{index}}",
      "tierBalanced": "均衡",
      "dividerHelp": "编号越小的层级越优先。只有当该层级所有模型均不可用（熔断）时，流量才会降级到下一层级。同一层级内的模型之间负载均衡。",
      "tooltip": "T0 最先尝试，T1 为备用，依此类推。同一层级内的模型负载均衡。",
      "addTierTooltip": "添加新的后备层级",
      "nodeTooltipPrimaryTitle": "T0 — 最高优先级",
      "nodeTooltipPrimaryBody": "每次请求优先尝试，同层级内模型负载均衡。",
      "nodeTooltipFallbackTitle": "T{{tier}} — 后备层级",
      "nodeTooltipFallbackBody": "仅当更高优先级的层级（编号越小越优先）全部不可用时才启用，同层级内模型负载均衡。",
      "nodeMoveHint": "↑ / ↓  拖动模型卡片可移动到其他层级",
      "nodeTooltipLearnMore": "查看层级图解 →",
      "guideButtonAriaLabel": "查看层级图解",
      "guide": {
        "title": "了解层级",
        "subtitle": "步骤 {{current}} / {{total}}",
        "previous": "上一步",
        "next": "下一步",
        "gotIt": "明白了！",
        "close": "关闭",
        "firstRunHint": "💡 您刚刚添加了第二个提供商。配置层级以设置主备路由！",
        "dontShowAgain": "不再显示",
        "hoverHint": "操作按钮已显示 - 尝试悬停节点查看！",
        "steps": {
          "1": {
            "title": "什么是层级？",
            "content": "层级按优先级组织您的模型。T0（零层）是最高优先级层级——这里的模型会在每次请求时首先被尝试。层级编号越小，优先级越高。",
            "annotation": {
              "tier": "T0 — 最高优先级层级",
              "service": "您的模型卡片，包含模型和提供商信息"
            }
          },
          "2": {
            "title": "同一层级的多个模型",
            "content": "当同一层级（如 T0）中有多个模型时，它们会共享传入的流量。这称为负载均衡——请求会在该层级的所有模型之间分配。",
            "annotation": {
              "loadBalance": "同一层级 = 负载均衡",
              "multiple": "多个模型共享流量"
            }
          },
          "3": {
            "title": "设置主备模型",
            "content": "使用模型卡片上的 ↑/↓ 按钮在不同层级之间移动模型。T0 中的模型是您的首选。T1、T2 等层级中的模型作为备选——只有在所有更高优先级层级都失败时才会运行。",
            "annotation": {
              "primary": "T0 — 主模型（首先尝试）",
              "fallback": "T1 — 备选模型（T0 失败时使用）",
              "actionButtons": "↑/↓ 按钮在不同层级间移动模型"
            }
          },
          "4": {
            "title": "自动故障转移",
            "content": "当一个层级中的所有模型都失败（熔断器打开）时，流量会自动降级到下一个层级。一旦层级恢复（熔断器关闭），流量会自动返回。您无需做任何操作——一切都是自动的。",
            "annotation": {
              "circuitBreaker": "熔断器监控模型健康状态",
              "automaticFailover": "自动降级到下一层级"
            }
          },
          "5": {
            "title": "多层级故障转移链",
            "content": "您可以根据需要创建任意数量的层级。T0 → T1 → T2 → ... 流量会级联下降，直到找到正常工作的层级。这可用于成本优化（先用便宜的，贵的作为备选）或区域故障转移（先用本地的，远程作为备选）。",
            "annotation": {
              "priority": "编号越小 = 优先级越高",
              "cascade": "流量在层级间级联下降"
            }
          }
        }
      }
    },
    "routing": {
      "directTooltipTitle": "直接路由",
      "directTooltipBody": "按层级顺序在所有服务间负载均衡。简单可预测。",
      "smartTooltipTitle": "智能路由",
      "smartTooltipBody": "基于自定义条件路由，如模型名称、令牌数量或用户组。",
      "tooltipHint": "点击按钮切换模式",
      "viewDirectGuide": "查看直接路由图解 →",
      "viewSmartGuide": "查看智能路由图解 →",
      "guide": {
        "directTitle": "直接路由指南",
        "smartTitle": "智能路由指南",
        "subtitle": "步骤 {{current}} / {{total}}",
        "previous": "上一步",
        "next": "下一步",
        "gotIt": "明白了！",
        "close": "关闭",
        "hoverHint": "操作按钮已显示 - 尝试悬停节点查看！",
        "toolbarLabel": "页面工具栏",
        "clickHere": "点这里",
        "steps": {
          "connectAI": {
            "title": "1. 连接一个 AI 提供商",
            "content": "路由需要至少一个可转发的 AI 服务。点击页面工具栏的「Connect AI」添加提供商——粘贴 API 密钥、用 OAuth 登录，或指向自托管服务器。在此之前，规则没有可路由的目标。",
            "annotation": {
              "toolbar": "Connect AI 在页面工具栏",
              "empty": "空规则还没有模型"
            }
          },
          "addModel": {
            "title": "2. 添加第一个模型",
            "content": "每条规则把一个请求模型映射到一个或多个模型。在空规则里点击「＋ 添加模型」，选择已连接的提供商和模型。需要为另一个请求模型单独建一条规则？用工具栏的「New Rule」。",
            "annotation": {
              "addModel": "＋ 添加模型——选提供商 + 模型",
              "newRule": "New Rule 新增一条请求模型映射"
            }
          },
          "editModel": {
            "title": "3. 更换或移除模型",
            "content": "点击任意模型卡片即可编辑——换成别的模型、切换提供商，或移动到其他层级。悬停卡片会显示操作按钮；垃圾桶图标可把它从规则中移除。",
            "annotation": {
              "click": "点击卡片编辑 / 换模型",
              "remove": "悬停 → 垃圾桶图标移除"
            }
          },
          "loadBalance": {
            "title": "4. 层级内负载均衡",
            "content": "当多个模型处于同一层级（T0）时，传入流量会在它们之间均匀分配，从而平衡负载、避免任何单个模型过载。",
            "annotation": {
              "sameTier": "同一层级 = 负载均衡",
              "services": "多个模型共享流量"
            }
          },
          "tierFallback": {
            "title": "5. 基于层级的故障转移链",
            "content": "层级越低越先尝试：T0 是主选，若所有 T0 模型都失败，流量会级联到 T1，再到 T2，依此类推。用卡片上的上/下操作在层级间移动它，搭建故障转移链。",
            "annotation": {
              "primary": "T0 — 主模型（首先尝试）",
              "fallback": "T1 — 备选模型（T0 失败时使用）"
            }
          },
          "smartIntro": {
            "title": "什么是智能路由？",
            "content": "智能路由让你定义自定义条件来控制哪个模型处理每个请求。可按模型名称、令牌数量、用户组或任意请求参数路由——精细控制，无需管理复杂的层级配置。",
            "annotation": {
              "smartButton": "用入口开关切换到 Smart",
              "conditional": "基于规则的条件路由"
            }
          },
          "smartConditions": {
            "title": "智能路由条件",
            "content": "每个智能规则都有一个决定何时生效的条件——例如模型名称「包含 claude」，或令牌数量「大于 4000」用于大上下文。规则自上而下评估，第一个匹配的获胜。",
            "annotation": {
              "modelBased": "按模型名称路由",
              "tokenBased": "按令牌数量路由"
            }
          },
          "smartAdvanced": {
            "title": "高级智能路由",
            "content": "把多个智能规则叠加成更丰富的策略：Claude 请求走一条路、大上下文走另一条、高级用户走第三条。不匹配任何规则的请求会落到默认服务。",
            "annotation": {
              "defaultRoute": "不匹配请求的默认路由",
              "claudeRoute": "Claude 模型的路由",
              "largeContext": "大上下文窗口的路由"
            }
          }
        }
      }
    },
    "menu": {
      "refreshModels": "刷新模型",
      "deleteProvider": "删除提供商",
      "deleteService": "删除服务",
      "deleteSmartRule": "删除智能规则"
    },
    "tooltips": {
      "addProviderFirst": "添加提供商以启用请求转发",
      "addProviderSecond": "添加另一个提供商（有 2+ 个提供商时，将根据策略启用负载平衡）",
      "addProviderMore": "添加另一个提供商（请求将在所有提供商之间负载平衡）",
      "addFirstProvider": "添加您的第一个提供商",
      "addServiceFirst": "添加模型以启用请求转发",
      "addServiceSecond": "添加另一个模型（将启用负载均衡）"
    },
    "notifications": {
      "loadFailed": "加载数据失败",
      "requestModelRequired": "请求模型名称是必填项",
      "modelRequired": "请为提供商 {{name}} 选择模型",
      "saved": "规则 \"{{model}}\" 保存成功",
      "saveFailed": "保存规则失败：{{error}}",
      "saveError": "保存规则时出错：{{error}}",
      "reset": "规则已重置为最新保存状态",
      "modelsRefreshed": "成功刷新 {{name}} 的模型",
      "modelsRefreshFailed": "刷新模型失败：{{error}}",
      "modelsRefreshError": "刷新模型失败：{{error}}"
    },
    "deleteDialog": {
      "title": "删除规则",
      "description": "您确定要删除此规则吗？此操作无法撤销。",
      "cancelButton": "取消",
      "confirmButton": "删除"
    },
    "status": {
      "clickToActivate": "点击以激活",
      "clickToDeactivate": "点击以停用",
      "cannotToggle": "无法切换"
    },
    "smart": {
      "untitledRule": "未命名的智能规则",
      "noOperation": "无操作",
      "noValue": "无值",
      "deleteTooltip": "删除智能规则",
      "unconditional": "无条件，跳过"
    }
  },
  "system": {
    "pageTitle": "服务器状态",
    "status": {
      "running": "运行中",
      "stopped": "已停止",
      "server": "服务器",
      "keys": "密钥",
      "connected": "已连接",
      "uptime": "运行时间",
      "lastUpdated": "最后更新：{{time}}",
      "loading": "加载中...",
      "unavailable": "不可用"
    },
    "prompts": {
      "enterPort": "输入服务器端口：",
      "enterClientId": "输入客户端 ID（web）："
    },
    "confirmations": {
      "stopServer": "您确定要停止服务器吗？"
    },
    "notifications": {
      "startSuccess": "{{message}}",
      "stopSuccess": "{{message}}",
      "restartSuccess": "{{message}}",
      "startFailed": "{{error}}",
      "stopFailed": "{{error}}",
      "restartFailed": "{{error}}",
      "tokenGenerated": "令牌生成成功",
      "tokenGenerateFailed": "{{error}}"
    },
    "proxy": {
      "title": "代理设置",
      "label": "代理",
      "respectEnvProxy": {
        "label": "遵循环境代理",
        "helper": "启用后，没有显式代理配置的提供商将使用系统代理设置（HTTP_PROXY、HTTPS_PROXY、macOS 系统代理、Clash 等）"
      },
      "globalProxyUrl": {
        "label": "常用代理",
        "description": "保存一个常用的代理，配置 Provider 或 OAuth 时可一键复用；如果 Provider 单独设置了代理，会以 Provider 的为准。",
        "helper": "可在 Provider 和 OAuth 中一键复用，Provider 单独设置的代理优先级更高。",
        "saveSuccess": "常用代理已保存",
        "saveFailed": "保存常用代理失败"
      },
      "notifications": {
        "updateSuccess": "代理设置更新成功",
        "updateFailed": "更新代理设置失败：{{error}}"
      }
    },
    "accessControl": {
      "userToken": "用户令牌（控制面板）",
      "modelToken": "模型令牌（API 代理）",
      "userTokenDesc": "此令牌保护对 Web 控制面板的访问。请与 API 用户分享模型令牌。",
      "modelTokenDesc": "与需要 API 访问权限的用户分享此令牌。",
      "copy": "复制",
      "copied": "已复制！",
      "resetToken": "重置令牌",
      "resetting": "重置中...",
      "viewFullToken": "查看完整令牌",
      "fullTokenWarning": "请保护好您的令牌。任何拥有此令牌的人都可以访问您的控制面板。",
      "secure": "令牌是安全的（随机生成）",
      "warning": {
        "default": "您正在使用默认的用户令牌。这是一个安全风险！请重置为安全的随机令牌。",
        "resetNow": "立即重置"
      },
      "reset": {
        "title": "重置用户令牌",
        "confirm": "您确定要重置您的用户令牌吗？",
        "points": {
          "new": "将生成一个新的随机令牌",
          "session": "您当前的会话将自动更新",
          "other": "任何其他浏览器/设备都需要重新登录",
          "stop": "旧令牌将立即停止工作"
        },
        "warning": "重置前请确保您可以访问此设备。",
        "cancel": "取消"
      },
      "success": {
        "title": "令牌重置成功",
        "message": "您的新用户令牌已生成并保存到您的会话中。",
        "saved": "我已经保存了我的令牌"
      }
    },
    "language": {
      "title": "语言",
      "description": "选择界面显示语言",
      "en": "English",
      "zh": "中文",
      "current": "当前语言",
      "saveSuccess": "语言设置已更新",
      "saveFailed": "语言设置更新失败"
    },
    "experimentalFeatures": {
      "title": "实验性功能",
      "description": "这些实验性功能适用于所有场景。各个场景可以覆盖这些设置。",
      "skills": "Skills",
      "guardrails": "Guardrails",
      "mcp": "MCP",
      "enableIdeSkills": "启用 IDE Skills 功能，用于管理来自 IDE 的代码片段和技能",
      "enableGuardrails": "启用 Guardrails - 阻止有风险的工具调用并过滤敏感输出",
      "enableMCP": "启用 MCP Tools - 配置 MCP（Model Context Protocol）工具，如网页搜索和网页获取",
      "on": "On",
      "off": "Off",
      "enabled": "已启用",
      "disabled": "已禁用 - 点击启用",
      "guardrailsEnabledInfo": "Guardrails 已启用。侧边栏中提供了「Guardrails」页面用于规则管理。",
      "mcpEnabledInfo": "MCP Tools 已启用。侧边栏 System 下方提供了「MCP Tools」页面进行配置。"
    },
    "about": {
      "title": "关于",
      "version": "版本",
      "license": "许可证",
      "github": "GitHub",
      "devMode": "开发模式",
      "available": "可用"
    },
    "serverStatus": {
      "title": "服务器状态",
      "server": "服务器",
      "forceLogout": "强制登出",
      "refreshStatus": "刷新状态"
    }
  },
  "serverInfo": {
    "title": "API 端点",
    "openAI": {
      "label": "OpenAI 基础 URL",
      "copyTooltip": "复制 OpenAI 基础 URL",
      "copyCurlTooltip": "复制 OpenAI cURL 示例"
    },
    "anthropic": {
      "label": "Anthropic 基础 URL",
      "copyTooltip": "复制 Anthropic 基础 URL",
      "copyCurlTooltip": "复制 Anthropic cURL 示例"
    },
    "docker": {
      "tooltip": "Docker 模式。要从容器访问，请配置网络：Linux 上使用 --network=host，或在 Docker Desktop（Mac/Windows）上使用 host.docker.internal"
    },
    "authentication": {
      "title": "身份验证",
      "apiKeyLabel": "API 密钥",
      "showTokenTooltip": "显示令牌",
      "hideTokenTooltip": "隐藏令牌",
      "copyTokenTooltip": "复制令牌",
      "generateTooltip": "生成新令牌"
    },
    "notifications": {
      "copied": "{{label}} 已复制到剪贴板！",
      "copyFailed": "复制到剪贴板失败",
      "generateFailed": "生成令牌失败：{{error}}"
    }
  },
  "apiKeyModal": {
    "title": "API 密钥",
    "description": "您的身份验证令牌：",
    "clickToCopy": "点击复制令牌",
    "copyButton": "复制令牌"
  },
  "history": {
    "pageTitle": "活动日志和历史",
    "subtitle": "{{count}} 条最近的活动记录"
  },
  "claudeCode": {
    "configPath": "将环境配置添加到 Claude Code 配置文件",
    "copyConfig": "配置",
    "oneClickScript": "一键脚本",
    "jsonConfig": "JSON 配置",
    "step1": "1. 配置模型",
    "step2": "2. 跳过入门 - 让 Claude Code 直接可用",
    "step3": "3. 状态行集成（可选）",
    "unifiedConfig": "统一配置",
    "separateConfig": "分离配置",
    "switchToSeparate": "切换到分离",
    "switchToUnified": "切换到统一",
    "configButton": "自动配置",
    "quickApply": "自动配置",
    "quickApplyWithStatusLine": "自动配置和状态行",
    "statusLine": {
      "description": "安装状态行集成以在您的终端中显示实时请求信息。",
      "jsonDescription": "配置状态行集成以在您的终端提示符中显示实时请求信息。",
      "addToSettingsJson": "将 statusLine 部分添加到 ~/.claude/settings.json（与 env 部分一起）：",
      "manualSetup": "或手动下载并安装状态行脚本：",
      "downloadLink": "下载状态行脚本"
    },
    "modal": {
      "title": "Claude Code 配置指南",
      "subtitle": "按照以下步骤配置 Claude Code 以使用 Tingly Box 作为您的 AI 代理",
      "dontRemindAgain": "不再提醒"
    },
    "profile": {
      "renameProfile": "重命名配置文件",
      "deleteProfile": "删除配置文件",
      "quickStart": "快速开始",
      "switchToGlobal": "切换到全局命令",
      "switchToNpm": "切换到 npm 命令",
      "copyCommand": "复制命令",
      "clickToCopy": "点击复制命令",
      "renameTitle": "重命名配置文件",
      "profileName": "配置文件名称",
      "save": "保存",
      "deleteTitle": "删除配置文件",
      "deleteConfirm": "您确定要删除配置文件 {{name}} 吗？",
      "deleteWarning": "这将删除配置文件及其所有关联的规则和标志。此操作无法撤销。",
      "profileRenamed": "配置文件已重命名",
      "profileDeleted": "配置文件已删除",
      "renameFailed": "重命名配置文件失败",
      "deleteFailed": "删除配置文件失败",
      "mode": "模式",
      "unified": "统一",
      "separate": "分离"
    }
  },
  "prompt": {
    "menu": "提示词",
    "user": {
      "title": "用户录制",
      "subtitle": "浏览和管理您的 IDE 录制",
      "filters": "筛选",
      "searchPlaceholder": "搜索录制...",
      "userFilter": "用户",
      "allUsers": "所有用户",
      "projectFilter": "项目",
      "allProjects": "所有项目",
      "typeFilter": "类型",
      "allTypes": "所有类型",
      "recordingsFound": "找到 {{count}} 条录制",
      "recordingsFor": "{{date}} 的录制",
      "noRecordings": "此日期没有找到录制",
      "actions": {
        "play": "播放",
        "viewDetails": "查看详情",
        "delete": "删除"
      },
      "types": {
        "code-review": "代码审查",
        "debug": "调试",
        "refactor": "重构",
        "test": "测试",
        "custom": "自定义"
      }
    },
    "skill": {
      "title": "技能",
      "subtitle": "从您的 IDE 目录管理技能",
      "addPath": "添加路径",
      "autoDiscover": "自动发现",
      "refreshAll": "全部刷新",
      "adapterConfig": "适配器配置",
      "locations": "位置",
      "selectLocation": "选择位置以查看技能",
      "noLocations": "未添加技能位置",
      "noSkills": "此位置没有找到技能",
      "skillsCount": "{{count}} 个技能",
      "searchPlaceholder": "搜索技能...",
      "ideFilter": "IDE 来源",
      "allIdes": "所有 IDE",
      "openAll": "全部打开",
      "openFolder": "打开文件夹",
      "actions": {
        "refresh": "刷新",
        "remove": "移除",
        "open": "打开"
      },
      "ides": {
        "claude-code": "Claude Code",
        "opencode": "OpenCode",
        "vscode": "VS Code",
        "cursor": "Cursor",
        "codex": "Codex",
        "antigravity": "Antigravity",
        "amp": "Amp",
        "kilo-code": "Kilo Code",
        "roo-code": "Roo Code",
        "goose": "Goose",
        "gemini-cli": "Gemini CLI",
        "github-copilot": "GitHub Copilot",
        "clawdbot": "Clawdbot",
        "droid": "Droid",
        "windsurf": "Windsurf",
        "custom": "自定义"
      },
      "dialog": {
        "title": "添加技能路径",
        "nameLabel": "显示名称",
        "namePlaceholder": "例如：我的 Claude Code 技能",
        "pathLabel": "路径",
        "pathPlaceholder": "/path/to/skills",
        "ideSourceLabel": "IDE 来源",
        "cancel": "取消",
        "add": "添加"
      },
      "discoveryDialog": {
        "title": "发现 IDE 技能",
        "description": "扫描您的主目录以查找支持的 IDE 并导入其技能。",
        "scanning": "正在扫描已安装的 IDE...",
        "foundIdes": "找到 {{count}} 个 IDE",
        "foundWithSkills": "找到 {{ides}} 个 IDE，共 {{skills}} 个技能",
        "noIdesFound": "未找到支持的 IDE。手动添加技能路径。",
        "selectToImport": "选择要导入技能的 IDE",
        "selectAll": "全选",
        "deselectAll": "取消全选",
        "importSelected": "导入选中的（{{count}}）",
        "importButton": "导入选中的"
      },
      "detailDialog": {
        "title": "技能详情",
        "path": "路径",
        "fileType": "文件类型",
        "size": "大小",
        "modified": "最后修改时间",
        "contentHash": "内容哈希",
        "description": "描述",
        "preview": "预览",
        "openInEditor": "在编辑器中打开",
        "unknownSize": "未知",
        "unknownDate": "未知",
        "loadError": "加载技能内容失败"
      }
    },
    "command": {
      "title": "命令",
      "comingSoon": "命令管理功能即将推出..."
    }
  },
  "accessControl": {
    "pageTitle": "访问控制",
    "pageDescription": "管理您的控制面板和 API 访问的身份验证令牌。",
    "userToken": {
      "title": "用户令牌（控制面板）",
      "description": "此令牌保护对 Web 控制面板的访问。请保持安全，不要与 API 用户分享。",
      "resetToken": "重置用户令牌",
      "resetTitle": "重置用户令牌",
      "resetConfirm": "您确定要重置您的用户令牌吗？",
      "resetPoints": {
        "new": "将生成一个新的随机令牌",
        "session": "您当前的会话将自动更新",
        "other": "任何其他浏览器/设备都需要重新登录",
        "stop": "旧令牌将立即停止工作"
      },
      "resetWarning": "重置前请确保您可以访问此设备。",
      "resetCancel": "取消",
      "resetConfirmButton": "重置",
      "resetSuccess": "用户令牌重置成功",
      "resetSuccessMessage": "您的新用户令牌已生成并保存到您的会话中。",
      "saved": "我已经保存了我的令牌",
      "pullToken": "从服务器拉取最新令牌"
    },
    "modelToken": {
      "title": "模型令牌（API 代理）",
      "description": "与需要 LLM 端点 API 访问权限的用户分享此令牌。",
      "sharing": "与需要访问 LLM API 的 API 用户分享模型令牌（上方）。请保密用户令牌。",
      "resetToken": "重置模型令牌",
      "resetTitle": "重置模型令牌",
      "resetConfirm": "您确定要重置模型令牌吗？",
      "resetPoints": {
        "new": "将生成一个新的随机令牌",
        "stop": "旧令牌将立即停止工作 - 所有 API 客户端都需要更新"
      },
      "resetWarning": "重置前请确保已通知所有 API 客户端。",
      "resetCancel": "取消",
      "resetConfirmButton": "重置",
      "resetSuccess": "模型令牌重置成功",
      "resetSuccessMessage": "您的新模型令牌已生成。请确保更新您的 API 客户端。",
      "saved": "我已经更新了我的客户端",
      "pullToken": "从服务器拉取最新令牌"
    },
    "securityInfo": {
      "title": "令牌安全",
      "description": "了解用户令牌和模型令牌的区别：",
      "point1": "用户令牌：保护 Web 控制面板和管理功能",
      "point2": "模型令牌：API 客户端用于访问 LLM 端点（/openai/*、/anthropic/*、/tingly/*）",
      "point3": "与 API 用户分享模型令牌，但绝不分享用户令牌"
    },
    "copy": "复制",
    "copied": "已复制！",
    "resetting": "重置中...",
    "viewFullToken": "查看完整令牌",
    "fullTokenWarning": "请保护好您的令牌。任何拥有此令牌的人都可以访问您的控制面板。",
    "secure": "令牌是安全的（随机生成）",
    "warning": {
      "default": "您正在使用默认的用户令牌。这是一个安全风险！",
      "description": "默认令牌是公开的，应替换为安全的随机令牌。",
      "resetNow": "立即重置"
    },
    "success": {
      "title": "令牌重置成功",
      "message": "您的新用户令牌已生成并保存到您的会话中。请确保安全保存。",
      "saved": "我已经保存了我的令牌"
    }
  },
  "dashboard": {
    "agentNav": {
      "title": "快速开始",
      "description": "开启智能应用"
    }
  },
  "mcp": {
    "pageTitle": "MCP 工具",
    "info": "配置 MCP（模型上下文协议）工具以启用网页搜索和网页获取功能。MCP 服务器作为本地 stdio 子进程运行或连接到远程 HTTP 端点。",
    "connection": {
      "title": "连接设置",
      "endpoint": "MCP 服务器端点",
      "endpointPlaceholder": "http://localhost:3000",
      "endpointHelp": "MCP 服务器的 HTTP 端点（例如 npx @modelcontextprotocol/server-filesystem）",
      "command": "命令",
      "commandPlaceholder": "python3",
      "scriptPath": "脚本路径",
      "scriptPathPlaceholder": "builtin",
      "scriptPathHelp": "MCP 服务器脚本的路径（或 Go 工具的 'builtin'）",
      "workingDir": "工作目录",
      "timeout": "请求超时（秒）",
      "timeoutHelp": "MCP 工具调用的超时时间",
      "transportHttp": "使用 HTTP 传输（取消选中以使用 stdio）",
      "transportStdio": "使用 Stdio 传输"
    },
    "tools": {
      "title": "工具配置",
      "description": "选择要启用的 MCP 工具：",
      "webSearch": "网页搜索",
      "webSearchDesc": "使用 Serper API 搜索网页。需要 SERPER_API_KEY 环境变量。",
      "webFetch": "网页获取",
      "webFetchDesc": "通过 Jina Reader 获取 URL 并转换为 markdown。可选 JINA_API_KEY。"
    },
    "proxy": {
      "title": "代理设置",
      "useGlobal": "使用全局代理配置",
      "useGlobalHelp": "启用后，MCP 服务器将从系统继承 HTTP_PROXY、HTTPS_PROXY 和 NO_PROXY 环境变量。"
    },
    "actions": {
      "save": "保存配置",
      "reset": "重置为默认值",
      "reload": "重新加载",
      "docs": "MCP 协议文档",
      "saving": "保存中...",
      "savedSuccess": "MCP 配置保存成功",
      "savedError": "保存 MCP 配置失败"
    },
    "currentConfig": "当前配置"
  },
  "onboarding": {
    "title": "欢迎使用 Tingly Box",
    "subtitle": "添加你的第一个 AI 提供商。可以从清单里挑一个，也可以粘贴一段配置文本让系统自动识别。",
    "hint": "识别完全在本地完成，粘贴的内容不会发送到任何第三方。",
    "tab": {
      "browse": "浏览提供商",
      "paste": "粘贴并识别"
    },
    "browse": {
      "searchPlaceholder": "搜索提供商",
      "empty": "没有匹配的提供商。",
      "selectProvider": "选择此提供商",
      "customProvider": "自定义提供商",
      "customProviderHint": "使用自定义接入地址",
      "section": {
        "global": "国际",
        "china": "中国大陆",
        "custom": "自定义"
      }
    },
    "paste": {
      "detectButton": "识别",
      "manualFill": "手动填写",
      "noMatch": "没有识别到 URL 或 API Key，可以手动填写。",
      "pickHint": "选择想用的 URL 和 Token，然后点击「使用所选」。",
      "urlsTitle": "识别到的 URL",
      "tokensTitle": "识别到的 Token",
      "noURL": "未识别到 URL。",
      "noToken": "未识别到 Token。",
      "useSelected": "使用所选"
    },
    "quickLinks": "快速链接",
    "goToDashboard": "控制台",
    "goToHelp": "帮助与文档"
  },
  "scenarioOverview": {
    "title": "智能应用",
    "subtitle": "选择要配置的场景，未使用的可以隐藏以保持侧边栏整洁。",
    "showInSidebar": "在侧边栏显示",
    "hidden": "已隐藏",
    "editTooltip": "管理可见的智能应用",
    // Scenario descriptions intentionally omitted — falls back to English. Do not add Chinese translations here.
    "descriptions": {}
  }
};
