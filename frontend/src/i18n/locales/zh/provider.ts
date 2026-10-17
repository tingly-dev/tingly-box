// Providers, credentials and access: provider dialogs, API keys, sharing keys, teams, access control
export default {
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
  "cloudDialog": {
    "name": "名称",
    "connect": "连接",
    "connected": "提供商连接成功！",
    "connectFailed": "连接提供商失败",
    "showAdvanced": "高级选项（可选）",
    "hideAdvanced": "收起高级选项",
    "awsAuthHint": "使用 Access Key ID + Secret 密钥对认证，或仅使用 Bedrock API Key。",
    "validation": {
      "awsEitherOr": "请提供 Access Key ID + Secret Access Key，或提供 Bedrock API Key",
      "badRegion": "Region 格式应形如 \"us-east-1\"（小写字母、数字、连字符）",
      "badLocation": "Location 格式应形如 \"us-east5\" 或 \"global\"（小写字母、数字、连字符）",
      "badSaJson": "Service Account JSON 不是有效的 JSON——请粘贴完整的密钥文件内容",
      "badEndpoint": "Endpoint 应为完整 URL，例如 https://my-resource.openai.azure.com"
    },
    "fields": {
      "region": {"label": "Region"},
      "access_key_id": {"label": "Access Key ID"},
      "secret_access_key": {"label": "Secret Access Key"},
      "bearer_token": {"label": "Bedrock API Key", "helper": "与密钥对二选一——提供其中一种即可"},
      "session_token": {"label": "Session Token", "helper": "可选——用于临时（STS）凭证"},
      "project_id": {"label": "Project ID"},
      "location": {"label": "Location"},
      "service_account_json": {"label": "Service Account JSON"},
      "endpoint": {"label": "Endpoint"},
      "api_version": {"label": "API Version"},
      "api_key": {"label": "API Key"}
    }
  },
  "providerDialog": {
    "addTitle": "连接 AI",
    "addDescription": "选择提供商并输入您的 API 密钥以连接 AI 服务。支持多种协议的提供商可以启用多个协议。",
    "editTitle": "编辑 AI 配置",
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
      "editAction": "编辑名称",
      "fallback": "自定义服务商"
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
    "export": {
      "action": "复制导出",
      "copying": "复制中...",
      "base64": "复制 Base64",
      "jsonl": "复制 JSONL"
    },
    "advanced": {
      "title": "高级",
      "proxyUrl": {
        "label": "HTTP/SOCKS 代理 URL（可选）",
        "placeholder": "http://127.0.0.1:7890 或 socks5://127.0.0.1:7890",
        "helper": "可选：使用代理绕过区域限制。将保存以供将来使用。",
        "useGlobal": "使用常用代理（{{url}}）",
        "useGlobalNotSet": "使用常用代理（未配置 — 请在系统设置中配置）",
        "applyGlobal": "应用常用代理（{{url}}）",
        "applyGlobalApplied": "已应用常用代理"
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
    },
    "provider": {
      "placeholder": "Base URL"
    },
    "v1Hint": {
      "message": "大多数 OpenAI 兼容 API 需要 /v1 后缀。",
      "apply": "追加 /v1"
    }
  },
  "providerTable": {
    "quota": {
      "used": "已用 {{value}}",
      "balance": "余额",
      "leftOf": "剩余 {{value}} / {{limit}}",
      "cost": "费用",
      "readFailed": "无法读取额度——可点下方「详情」查看原始响应",
      "noLimits": "上游未报告额度限制",
      "none": "暂无额度数据",
      "refresh": "刷新",
      "rawResponse": "详情"
    },
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
  "credentialPage": {
    "empty": "还没有凭证",
    "managing_other": "共 {{count}} 个凭证",
    "emptyTitle": "连接你的第一个 AI",
    "emptyDescription": "用你已有的订阅登录（Claude Code、Codex、Gemini CLI…），或粘贴 API Key（OpenAI、Anthropic、DeepSeek…）。所有凭证都会出现在这里，路由规则从中选择模型。",
    "sectionOAuth": "OAuth",
    "sectionApiKeys": "API Key",
    "fallbackProviderName": "该服务商",
    "unknownError": "未知错误",
    "loadFailed": "加载服务商失败：{{error}}",
    "deleted": "服务商已删除",
    "deleteFailed": "删除服务商失败：{{error}}",
    "toggleFailed": "切换服务商状态失败：{{error}}",
    "tokenRefreshed": "令牌已刷新",
    "reauthorized": "服务商已重新授权",
    "reauthorizeUnknownIssuer": "无法重新授权：未知该服务商的签发方",
    "refreshFailedTitle": "令牌刷新失败",
    "refreshFailedBody": "刷新 <1>{{name}}</1> 的令牌没有成功。如果凭证已被撤销或完全过期，刷新无法恢复，需要重新授权并再次登录。重新授权会原地覆盖该凭证，沿用同一个服务商，路由规则和模型 Key 都不受影响。",
    "dismiss": "忽略",
    "reauthorize": "重新授权"
  },
  "apiKeyModal": {
    "title": "API 密钥",
    "description": "您的身份验证令牌：",
    "clickToCopy": "点击复制令牌",
    "copyButton": "复制令牌"
  },
  "sharingKeys": {
    "title": "Team 密钥",
    "titleForTeam": "Team 密钥 · {{team}}",
    "createToken": "创建令牌",
    "createDialogTitle": "创建 Team 密钥",
    "displayName": "显示名称",
    "displayNamePlaceholder": "例如：Team Alpha Key",
    "displayNameHelper": "为此 Team 密钥起一个便于识别的名称",
    "deleteToken": "删除令牌",
    "deleteConfirm": "确定要删除令牌 \"{{name}}\" 吗？此操作无法撤销。",
    "nameRequired": "显示名称为必填项",
    "createSuccess": "令牌创建成功",
    "createFailed": "创建令牌失败",
    "deleteSuccess": "令牌删除成功",
    "deleteFailed": "删除令牌失败",
    "copiedToClipboard": "令牌已复制到剪贴板",
    "copyFailed": "复制密钥失败",
    "disabled": "令牌已停用",
    "enabled": "令牌已启用",
    "updateFailed": "更新令牌失败",
    "moveToken": "移动密钥",
    "destinationTeam": "目标 Team",
    "noDestinationTeam": "没有其他已启用的 Team",
    "moveHelper": "移动 {{name}}，原密钥不会轮换。",
    "moveSuccess": "密钥已移动到新的 Team",
    "moveFailed": "移动密钥失败",
    "teamKeysTitle": "Team 密钥",
    "allSubtitle": "所有 Team 的密钥，按 Team 分组 — 共 {{teams}} 个 Team，{{keys}} 个密钥。",
    "openTeam": "打开 Team",
    "emptyTeam": "该 Team 还没有 Team 密钥。",
    "loadFailed": "加载 Team 密钥失败",
    "createDisabledTeam": "该 Team 已停用，请先启用再创建 Team 密钥。",
    "overview": {
      "count_other": "{{count}} 个 Key",
      "expand": "展开全部 Key",
      "collapse": "收起 Key",
      "empty": "该 Team 还没有 Key，为每位要分发的人创建一个。"
    },
    "table": {
      "name": "名称",
      "user": "用户",
      "token": "密钥",
      "status": "状态",
      "created": "创建时间",
      "lastUsed": "最近使用",
      "actions": "操作",
      "empty": "还没有 Team 密钥。创建一个即可向团队分享模型访问。",
      "hide": "隐藏",
      "show": "显示",
      "copy": "复制",
      "active": "已启用",
      "inactive": "已停用",
      "moveTooltip": "移动到其他 Team",
      "deleteTooltip": "删除密钥"
    }
  },
  "teams": {
    "accessTitle": "Team 访问",
    "keyScopeSummary": "{{team}}（{{slug}}）的 Team 密钥仅可访问 /tingly/team 与 /tingly/team/v1，不能访问其他 Team、场景端点或管理 API。",
    "allKeysScopeSummary": "每个 Team 密钥只对其所属 Team 生效，通过 /tingly/team 与 /tingly/team/v1 访问。移动到其他 Team 会立即生效，密钥本身不变。",
    "keyScopeInfoLabel": "Team 密钥访问范围",
    "editTeam": "Team 设置",
    "name": "Team 名称",
    "inactive": "已停用",
    "disabledHint": "此 Team 已停用；重新启用前，其 Team 密钥无法访问模型接口。",
    "enableTeam": "启用 Team",
    "disableTeam": "停用 Team",
    "deleteTeam": "删除 Team",
    "deleteConfirm": "删除 {{team}}？请先移动或删除它的全部 Team 密钥。",
    "loadFailed": "加载 Team 失败",
    "saveFailed": "保存 Team 失败",
    "createSuccess": "Team 已创建",
    "updateSuccess": "Team 已更新",
    "deleteSuccess": "Team 已删除",
    "deleteFailed": "Team 仍有 Team 密钥，无法删除",
    "enabled": "Team 已启用",
    "disabled": "Team 已停用",
    "guide": {
      "trigger": "Team 使用说明",
      "title": "Team 使用说明",
      "subtitle": "第 {{current}} 步，共 {{total}} 步",
      "previous": "上一步",
      "next": "下一步",
      "gotIt": "知道了！",
      "steps": {
        "usage": {
          "title": "Team 是做什么用的",
          "content": "Team 是这个实例里一块独立的空间——有自己的规则、Team 密钥和用量，与其他场景、其他 Team 都是分开的。",
          "bullet1": "把模型访问权交给某个群体（客户、子团队、外部应用），而不必分享你的主配置。",
          "bullet2": "在下方为这个 Team 添加规则，方式和其他场景页一样。"
        },
        "isolation": {
          "title": "它是怎么被隔离的",
          "content": "一个 Team 密钥只能绑定一个 Team，且只能访问 /tingly/team[/v1]。",
          "bullet1": "移动密钥或停用 Team，都会立即生效。",
          "bullet2": "全局 model token 不受 Team 范围限制，仍拥有完整访问权限。"
        },
        "keys": {
          "title": "配置 Team 密钥",
          "content": "点击上方的\"Team 密钥\"按钮，为这个 Team 创建一个。",
          "bullet1": "起个名称，复制 tb-share-… 开头的密钥（仅显示一次）。",
          "bullet2": "之后可以随时移动到另一个 Team，无需重新生成。"
        },
        "principles": {
          "title": "怎么用",
          "content": "让客户端指向这个 Team 的 Base URL，用它的 Team 密钥作为 API Key。",
          "bullet1": "Base URL 和 API Key 都在上方的 Team Access 面板里。",
          "bullet2": "请求会按下方的规则路由——只有添加到规则里的模型才能被访问。"
        }
      }
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
  }
};
