// App shell and shared copy: navigation, system settings, help, login, onboarding
export default {
  "common": {
    "back": "返回",
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
    "done": "完成",
    "applying": "应用中...",
    "copy": "复制",
    "copied": "已复制",
    "copyModelName": "复制模型名",
    "modelNameCopied": "已复制",
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
    "prompt": "提示词",
    "and": "和",
    "clear": "清除选择",
    "dismiss": "忽略",
    "moveDown": "下移",
    "moveUp": "上移",
    "saved": "已保存"
  },
  "layout": {
    "workingFolders": "工作目录",
    "noWorkingFolders": "还没有工作目录",
    "workingFolderWaiting": "{{count}} 个会话在等你",
    "desk": "工作台",
    "projects": "项目",
    "projectsWaiting": "有线程在等你处理",
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
      "usePi": "Pi",
      "useDsh": "DeepSeek",
      "useXcode": "Xcode",
      "useVSCode": "VS Code",
      "useCursor": "Cursor",
      "useEmbed": "Embedding",
      "useDecisions": "Decisions",
      "useImageGen": "Image API",
      "useTeam": "Team",
      "useCustom": "Custom",
      "apiKeys": "API 密钥",
      "oauth": "OAuth 凭证",
      "credential": "凭证",
      "prompt": "提示词"
    },
    "sidebar": {
      "pinToTop": "置顶",
      "unpin": "取消置顶",
      "newProfile": "新建方案",
      "newTeam": "新建 Team",
      "profileName": "方案名称",
      "mode": "模式",
      "modeUnified": "统一：所有模型使用相同配置",
      "modeSeparate": "分离：每个模型单独配置",
      "separate": "分离",
      "unified": "统一",
      "createProfileTooltip": "创建一个新的 Claude Code 配置方案，自定义设置",
      "createTeamTooltip": "创建一个独立鉴权的 Team 工作区",
      "sloganTooltip": "致，所有独立开发者、开发团队和智能应用。",
      "collapse": "收起侧边栏",
      "doneEditing": "完成",
      "expand": "展开侧边栏"
    },
    "coachMarks": {
      "gotIt": "知道了",
      "agentVisibility": {
        "title": "选择显示哪些 Agent",
        "text": "点这个眼睛，就能显示或隐藏列表里的 Agent。隐藏的不会丢，随时可以在这里恢复。"
      },
      "powerUps": {
        "title": "扩展功能的开关移到了这里",
        "text": "Team、Image、Remote、Bench 等可选项，现在在这个菜单的「扩展」里开启或关闭。"
      }
    },
    "activityBar": {
      "disconnected": "已断开",
      "disconnectedDebug": "已断开（调试）",
      "offline": "离线",
      "devMode": "开发",
      "newVersionAvailable": "更新",
      "error": "错误",
      "theme": "主题",
      "light": "浅色",
      "dark": "深色",
      "system": "跟随系统",
      "sunlit": "日光",
      "claude": "Claude",
      "ds": "DeepSeek",
      "click": "点击",
      "feedback": "反馈",
      "feedbackTooltip": "发送反馈（跳转到 GitHub Issues）",
      "preferences": "偏好设置",
      "allSettings": "全部设置"
    },
    "themeMenu": {
      "switchTo": "切换到：",
      "theme": "主题："
    },
    "easterEgg": "Hi，我是 Tingly-Box，为您掌控智能",
    "githubStar": {
      "text": "喜欢 Tingly-Box 吗？点个 GitHub Star 让更多人发现它吧～",
      "cta": "去 GitHub 点 Star",
      "short": "Star"
    },
    "dashboard": "仪表盘",
    // 只用于一级入口（Activity Rail）这一处的 label；侧栏行和 Team 页面自己的
    // 标题继续用 nav.useTeam（全局统一保留英文 "Team"，见 .design/team.md §3）。
    "team": "团队",
    "usage": "用量",
    "bench": "测试台",
    "userUsage": "团队用量",
    "quotaHistory": "Quota 历史",
    "heatmap": "热力图",
    "today": "今天",
    "yesterday": "昨天",
    "days": "天",
    "remote": "远程",
    "image": "Image",
    "imagePlayground": "Playground",
    "remoteControl": "远程控制",
    "notify": "IM 通知",
    "bots": "机器人",
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
    "protectedCredentials": "密钥保护",
    "mcp": "MCP",
    "sources": "来源",
    "localMode": "本地模式",
    "credentials": "凭证",
    "virtualModels": "虚拟模型",
    "virtualModelsNavLabel": "VModel",
    "virtualModelsTooltip": "内置的合成模型 Provider，用于上手演示与本地试跑——无需联网即在进程内返回响应。",
    "accessControl": "访问控制",
    "status": "状态",
    "system": "系统",
    "general": "通用",
    "experimental": "实验功能",
    "develop": "开发者",
    "logs": "日志",
    "userRequest": "用户请求",
    "skills": "技能",
    "addProfile": "新建方案",
    "addTeam": "添加 Team",
    "teamKeys": "Team 密钥",
    "teamKeysTooltip": "在一个页面查看所有 Team 的密钥，按 Team 分组。",
    "default": "默认",
    "help": "帮助与技巧",
    "helpShort": "帮助",
    "powerUps": "扩展",
    "tools": "工具",
    "servertool": "服务端工具",
    "botsRunning": "{{total}} 个 bot 中 {{active}} 个在运行"
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
      "ru": "Русский",
      "current": "当前语言",
      "saveSuccess": "语言设置已更新",
      "saveFailed": "语言设置更新失败"
    },
    "experimentalFeatures": {
      "projects": "项目",
      "enableProjects": "工作一冒出来就丢给项目：协调者为每个任务开一个 Claude Code 线程，各自在独立的 git worktree 里运行。",
      "projectsEnabledInfo": "现在，任何能登录这个 tingly-box 的人都可以在这台机器上、在其项目所指定的仓库里运行 Claude Code 线程。",
      "desk": "工作台",
      "enableDesk": "在浏览器中运行本机的 Claude Code，并选择其工作目录。",
      "deskEnabledInfo": "任何能登录此 tingly-box 的用户都可以在本机启动 Claude Code 会话，并批准其工具调用。",
      "title": "实验性功能",
      "description": "这些实验性功能适用于所有场景。各个场景可以覆盖这些设置。",
      "skills": "Skills",
      "userPrompts": "用户提示词",
      "guardrails": "Guardrails",
      "mcp": "MCP",
      "bench": "Bench",
      "enableUserPrompts": "管理用户请求可复用的指令",
      "enableIdeSkills": "管理来自 IDE 的代码片段和技能",
      "enableGuardrails": "阻止有风险的工具调用并过滤敏感输出",
      "enableMCP": "MCP（Model Context Protocol）工具，如网页搜索和网页获取",
      "enableBench": "探针测试工作台 —— 原始客户端请求、flag 覆盖与 payload 编辑",
      "on": "On",
      "off": "Off",
      "enabled": "已启用",
      "disabled": "已禁用 - 点击启用",
      "guardrailsEnabledInfo": "Guardrails 已启用。侧边栏中提供了「Guardrails」页面用于规则管理。",
      "mcpEnabledInfo": "MCP Tools 已启用。侧边栏 System 下方提供了「MCP Tools」页面进行配置。",
      "requiredMessage": "{{feature}} 尚未开启，请在下方启用后继续。",
      "enableFailed": "无法启用此功能，请检查服务器连接后重试。"
    },
    "about": {
      "title": "关于",
      "version": "版本",
      "license": "许可证",
      "github": "GitHub",
      "devMode": "开发模式",
      "available": "可用",
      "copyVersion": "复制版本号",
      "versionCopied": "版本号已复制到剪贴板",
      "checkUpdate": "点击检查更新",
      "updateAvailable": "新版本 {{version}} 可用，点击查看"
    },
    "serverStatus": {
      "title": "服务器状态",
      "server": "服务器",
      "forceLogout": "强制登出",
      "refreshStatus": "刷新状态"
    },
    "preferences": {
      "title": "外观与语言",
      "themePalette": "配色预览"
    }
  },
  "help": {
    "title": "帮助与技巧",
    "description": "一些容易被忽略但很有用的小功能。",
    "shortcut": {
      "title": "桌面快捷方式",
      "description": "一键重启 Tingly Box 并打开网页界面。仅创建，不会自动更新或自动重启。",
      "create": "创建快捷方式",
      "recreate": "重新创建",
      "creating": "创建中...",
      "alreadyCreated": "已创建",
      "doubleClick": "双击它以启动 Tingly Box 并打开网页界面。",
      "runHeadless": "没有图形界面？在该机器上运行：",
      "createFailed": "创建快捷方式失败：{{error}}"
    },
    "providers": {
      "title": "提供商目录",
      "description": "浏览目录，或粘贴一段配置片段——剩下的交给我们。"
    },
    "routing": {
      "title": "路由与分层指南",
      "description": "回顾直连路由、智能路由和模型分层的工作方式——复用的是其他页面上那份一样的图和步骤。",
      "direct": "直连路由指南",
      "smart": "智能路由指南",
      "tier": "分层（Tier）指南"
    },
    "team": {
      "title": "Team 使用说明",
      "description": "Team 是做什么的、怎么隔离、怎么配置 Team 密钥——和 Team 页面里的那份说明是同一份。",
      "action": "Team 使用说明"
    }
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
        "description": "一条命令：下载新版本并重启服务"
      },
      "npm": {
        "title": "全局安装（npm）",
        "description": "更新已安装的 CLI，然后重启服务以生效",
        "aliasNote": "tb 是 tingly-box 的缩写，`tingly-box restart` 同样可用"
      },
      "docker": {
        "title": "Docker 镜像",
        "description": "从 GitHub Container Registry 拉取镜像"
      }
    }
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
  "onboarding": {
    "title": "欢迎使用 Tingly Box",
    "subtitle": "添加你的第一个 AI 提供商。可以从清单里挑一个，也可以用「粘贴并识别」贴入一段配置文本让系统自动识别。",
    "hint": "识别完全在本地完成，粘贴的内容不会发送到任何第三方。",
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
    "goToHelp": "帮助与文档",
    "success": "服务商添加成功！现在可以创建场景了。",
    "dialog": {
      "title": "服务商已添加",
      "message": "你的 AI 服务商已成功添加。要前往智能应用页面开始使用吗？",
      "goToAgents": "前往智能应用",
      "stay": "留在本页"
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
  "hub": {
    "title": "Tingly Box",
    "status": {
      "healthy": "运行中",
      "unhealthy": "连接已断开",
      "updateAvailable": "有可用更新"
    },
    "quota": {
      "title": "服务商额度",
      "empty": "暂无额度数据",
      "refresh": "刷新额度",
      "sort": {
        "label": "排序",
        "remaining": "额度最紧张优先",
        "name": "名称",
        "type": "类型"
      }
    }
  }
};
