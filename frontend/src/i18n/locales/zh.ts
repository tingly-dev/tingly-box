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
    "zen": "禅",
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
      "useOpenAI": "OpenAI SDK",
      "useAnthropic": "Anthropic SDK",
      "useCodex": "Codex",
      "useClaudeCode": "Claude Code",
      "useOpenCode": "OpenCode",
      "useXcode": "Xcode",
      "useVSCode": "VS Code",
      "useEmbed": "Embed 嵌入",
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
      "sunlit": "日光",
      "zenMode": "禅模式",
      "enterZenMode": "进入禅模式：",
      "more": "更多",
      "exitZen": "退出",
      "click": "点击"
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
    "accessControl": "访问控制",
    "status": "状态",
    "system": "系统",
    "experimental": "实验功能",
    "logs": "日志",
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
    "later": "稍后"
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
      "button": "添加您的第一个 API 密钥"
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
    "addButton": "添加 API 密钥",
    "emptyCardTitle": "未配置模型 API 密钥",
    "emptyCardSubtitle": "添加您的第一个 API 令牌或密钥以开始使用",
    "emptyCardButton": "添加您的第一个提供商",
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
    "addTitle": "添加新的 API 密钥",
    "addDescription": "选择提供商并输入您的 API 密钥以连接 AI 服务。支持多种协议的提供商可以启用多个协议。",
    "editTitle": "编辑 API 密钥",
    "addButton": "添加 API 密钥",
    "apiStyle": {
      "label": "API 风格",
      "placeholder": "选择 API 风格...",
      "helperOpenAI": "支持来自 OpenAI、Google 和许多其他 OpenAI 兼容提供商的模型",
      "helperAnthropic": "用于 Anthropic 兼容的 AI 提供商，通常与 Claude Code 一起使用。",
      "openAI": "OpenAI 兼容",
      "anthropic": "Anthropic 兼容",
      "switchWarning": "API 风格已更改。基础 URL 已重置。请选择兼容的提供商。"
    },
    "provider": {
      "label": "提供商或自定义基础 URL",
      "placeholder": "选择提供商或输入自定义基础 URL"
    },
    "protocol": {
      "label": "协议"
    },
    "keyName": {
      "label": "API 密钥名称",
      "placeholder": "例如：OpenAI API Key",
      "autoFill": "{{title}} API 密钥"
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
        "useGlobal": "使用全局代理（{{url}}）",
        "useGlobalNotSet": "使用全局代理（未配置 — 请在系统设置中配置）"
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
    "menu": {
      "refreshModels": "刷新模型",
      "deleteProvider": "删除提供商",
      "deleteSmartRule": "删除智能规则"
    },
    "tooltips": {
      "addProviderFirst": "添加提供商以启用请求转发",
      "addProviderSecond": "添加另一个提供商（有 2+ 个提供商时，将根据策略启用负载平衡）",
      "addProviderMore": "添加另一个提供商（请求将在所有提供商之间负载平衡）",
      "addFirstProvider": "添加您的第一个提供商"
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
      "deleteTooltip": "删除智能规则"
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
      "loading": "加载中..."
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
        "label": "全局代理",
        "helper": "所有提供商和 OAuth 的兜底代理，每个提供商的专属代理优先级更高。",
        "saveSuccess": "全局代理 URL 已保存",
        "saveFailed": "保存全局代理 URL 失败"
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
      "fusion": "融合 Provider",
      "enableIdeSkills": "启用 IDE Skills 功能，用于管理来自 IDE 的代码片段和技能",
      "enableGuardrails": "启用 Guardrails - 阻止有风险的工具调用并过滤敏感输出",
      "enableMCP": "启用 MCP Tools - 配置 MCP（Model Context Protocol）工具，如网页搜索和网页获取",
      "enableFusion": "允许单个 Provider 条目同时暴露 OpenAI 和 Anthropic 兼容的 base URL，入站请求按协议原生路由，无需协议转换。",
      "on": "On",
      "off": "Off",
      "enabled": "已启用",
      "disabled": "已禁用 - 点击启用",
      "guardrailsEnabledInfo": "Guardrails 已启用。侧边栏中提供了「Guardrails」页面用于规则管理。",
      "mcpEnabledInfo": "MCP Tools 已启用。侧边栏 System 下方提供了「MCP Tools」页面进行配置。",
      "fusionEnabledInfo": "融合 Provider 已启用。Provider 表单现在允许在同一条目中配置 OpenAI 与 Anthropic 的 URL。"
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
    "configButton": "快速配置",
    "quickApply": "快速应用",
    "quickApplyWithStatusLine": "快速应用和状态行",
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
      "separate": "分离",
      "unifiedDescription": "所有模型使用相同的路由规则",
      "separateDescription": "每个模型使用自己的路由规则",
      "modeUpdated": "配置文件模式已更新为 {{mode}}",
      "modeUpdateFailed": "配置文件模式更新失败"
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
      "selectProvider": "选择此提供商"
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
  }
};
