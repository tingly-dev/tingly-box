// Routing rules and templates
export default {
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
      "providerDisabled": "该提供商已禁用——路由会跳过此服务，直到重新启用。",
      "selectProvider": "选择提供商",
      "selectModel": "选择模型",
      "testService": "测试服务",
      "editProvider": "编辑服务商凭证",
      "deleteService": "删除服务",
      "quota": {
        "left": "剩余 {{value}}",
        "resetsIn": "{{duration}} 后重置",
        "updated": "{{duration}} 前更新",
        "clickToRefresh": "点击刷新",
        "refreshing": "刷新中…",
        "refreshFailed": "刷新失败，显示的是上次读数"
      }
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
        "hoverHint": "悬停示意图中的节点可查看操作按钮",
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
        "hoverHint": "悬停示意图中的节点可查看操作按钮",
        "toolbarLabel": "页面工具栏",
        "clickHere": "点这里",
        "steps": {
          "connectAI": {
            "title": "连接 AI 提供商",
            "content": "路由至少需要一个可转发的 AI 服务。点击页面工具栏的「连接 AI」添加提供商——可粘贴 API 密钥、通过 OAuth 登录，或指向自托管服务器。在此之前，规则没有可路由的目标。",
            "annotation": {
              "toolbar": "「连接 AI」在页面工具栏中",
              "empty": "空规则还没有模型"
            }
          },
          "addModel": {
            "title": "添加第一个模型",
            "content": "每条规则将一个请求模型映射到一个或多个模型。在空规则中点击「＋ 添加模型」，选择已连接的提供商和模型。想给这个 Agent 添加更多模型？使用工具栏的「新建模型」；打开其中的「多选」可一次添加多个，每个模型各建一条规则。",
            "annotation": {
              "addModel": "「＋ 添加模型」——选择提供商和模型",
              "newRule": "「新建模型」添加模型，每个模型一条规则"
            }
          },
          "editModel": {
            "title": "更换或移除模型",
            "content": "点击任意模型卡片即可编辑——换成其他模型、切换提供商，或移动到其他层级。悬停卡片会显示操作按钮；点击垃圾桶图标可将其从规则中移除。",
            "annotation": {
              "click": "点击卡片编辑或更换模型",
              "remove": "悬停后点垃圾桶图标移除"
            }
          },
          "loadBalance": {
            "title": "层级内负载均衡",
            "content": "当多个模型处于同一层级（如 T0）时，传入流量会在它们之间均匀分配，从而平衡负载、避免单个模型过载。",
            "annotation": {
              "sameTier": "同一层级 = 负载均衡",
              "services": "多个模型分摊流量"
            }
          },
          "tierFallback": {
            "title": "层级故障转移链",
            "content": "层级编号越小越先尝试：T0 是首选；当 T0 的所有模型都失败时，流量会依次级联到 T1、T2……使用卡片上的 ↑/↓ 按钮在层级间移动模型，即可搭建故障转移链。",
            "annotation": {
              "primary": "T0 — 主模型（首先尝试）",
              "fallback": "T1 — 备选模型（T0 失败时启用）"
            }
          },
          "smartIntro": {
            "title": "什么是智能路由？",
            "content": "智能路由让您通过自定义条件控制每个请求由哪个模型处理。可按模型名称、令牌数量、用户组或任意请求参数路由——无需管理复杂的层级配置即可实现精细控制。",
            "annotation": {
              "smartButton": "通过入口的模式开关切换到智能路由",
              "conditional": "基于规则的条件路由"
            }
          },
          "smartConditions": {
            "title": "智能路由条件",
            "content": "每条智能规则都有一个决定其何时生效的条件——例如模型名称「包含 claude」，或令牌数量「大于 4000」（用于大上下文）。规则自上而下依次匹配，命中的第一条生效。",
            "annotation": {
              "modelBased": "按模型名称路由",
              "tokenBased": "按令牌数量路由"
            }
          },
          "smartAdvanced": {
            "title": "高级智能路由",
            "content": "将多条智能规则组合成更丰富的策略：Claude 请求走一条路径、大上下文走另一条、高级用户走第三条。未命中任何规则的请求会落入默认服务。",
            "annotation": {
              "defaultRoute": "未命中请求的默认路由",
              "claudeRoute": "Claude 模型的路由",
              "largeContext": "大上下文窗口的路由"
            }
          }
        }
      }
    },
    "nodes": {
      "addModel": "添加模型"
    },
    "guideDiagrams": {
      "empty": { "description": "示例规则" },
      "single-provider": { "description": "单提供商规则" },
      "two-providers-same-tier": { "description": "负载均衡示例" },
      "two-providers-different-tiers": { "description": "主备路由示例" },
      "three-tiers": { "description": "多层级故障转移示例" },
      "runtime-failover": { "description": "故障转移场景" },
      "direct-single": { "description": "单提供商直接路由" },
      "direct-multiple-tiers": { "description": "多层级直接路由" },
      "smart-basic": {
        "description": "基础条件智能路由",
        "smart": { "0": "将 Claude 请求路由到 Anthropic" }
      },
      "smart-conditions": {
        "description": "多条件智能路由",
        "smart": { "0": "将 Claude 请求路由到 Anthropic", "1": "将大令牌请求路由到 Azure" }
      },
      "smart-complex": {
        "description": "复杂条件智能路由",
        "smart": { "0": "将 Claude 请求路由到 Anthropic", "1": "将大令牌请求路由到 Azure", "2": "将 @@@ds 命令路由到 DeepSeek" }
      }
    },
    "menu": {
      "refreshModels": "刷新模型",
      "deleteProvider": "删除提供商",
      "deleteService": "删除服务",
      "deleteSmartRule": "删除智能规则"
    },
    "ruleConditions": {
      "defaultTitle": "默认",
      "addCondition": "添加智能条件",
      "conditionsHint": "条件从上到下依次检查，先匹配到的生效。",
      "closeConditions": "关闭智能条件",
      "closeConditionsTooltip": "条件会保留；关闭期间所有请求走 Default，再次添加即可恢复。"
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
  "ruleAutoSave": {
    "saved": "配置已保存",
    "failed": "保存失败：{{error}}",
    "unknownError": "未知错误",
    "error": "保存配置出错"
  },
  "ruleHeader": {
    "plugins": "插件",
    "pluginsTooltip": "此规则已启用的插件",
    "endpointNotApplicable": "只对 OpenAI 兼容上游生效。此规则目前没有这类上游，所以选择暂时不起作用。",
    "managePlugins": "管理插件",
    "test": "测试",
    "smartConditionOn": "关闭智能条件",
    "smartConditionOff": "开启智能条件",
    "smartConditionAdd": "添加智能条件",
    "ruleActions": "规则操作",
    "compact": {
      "chip": "压缩：{{value}}",
      "auto": "跟随默认",
      "aria": "此模型的自动压缩：{{value}}",
      "tooltip": "设置 Codex 在上下文用到多少时压缩这个模型的对话",
      "title": "自动压缩触发点",
      "description": "上下文用到窗口的这个比例时，Codex 会先压缩历史对话。只写入 Codex 配置，不影响请求转发。",
      "following": "当前跟随 Auto Config（{{percent}}%）。拖动即只为这个模型单独设置。",
      "window": "窗口",
      "line": "压缩线",
      "remain": "压缩时剩余",
      "low": "压缩时仅剩约 {{remain}} tokens（建议不低于 {{min}}）。长工具输出可能撞上窗口上限，导致压缩失败。",
      "limited": "受「可用窗口 {{effective}}%」限制，实际按 {{applied}}% 生效。要更晚压缩，请先在 Auto Config 调高可用窗口。",
      "zoneLow": "{{from}}–{{to}}%：压缩时剩余太少",
      "zoneCapped": "超过 {{effective}}% 按 {{effective}}% 生效（可用窗口）",
      "needsApply": "改动需点 Auto Config 应用后生效。",
      "reset": "恢复为 Auto Config 的值（{{percent}}%）"
    }
  },
  "templateActions": {
    "troubleshoot": "错误检查",
    "more": "更多",
    "sortMenuToName": "按名称排序（A→Z）",
    "sortMenuToOriginal": "按原始顺序排序",
    "collapseAllRules": "收起所有规则",
    "expandAllRules": "展开所有规则",
    "connectAI": "连接 AI",
    "newRule": "新建模型",
    "createNewRule": "为这个 Agent 添加模型，每个模型各建一条路由规则",
    "selectedModels_other": "已选 {{count}} 个模型",
    "multiSelectTitle": "为每个模型创建规则",
    "selectMultiple": "多选",
    "clearSelection": "清除",
    "addModels_other": "添加 {{count}} 个模型",
    "howRoutingWorks": "路由原理",
    "sortOriginal": "原始顺序",
    "sortByName": "按名称（A→Z）",
    "sortTooltipToName": "当前按原始顺序显示 — 点击按名称排序",
    "sortTooltipToOriginal": "当前按名称排序 — 点击恢复原始顺序"
  },
  "templatePage": {
    "noProviders": {
      "title": "尚未配置提供商",
      "description": "添加一个 API Key 提供商以开始路由请求",
      "action": "开始使用"
    },
    "noRules": "还没有规则。点击「新建模型」添加。",
    "noRulesFixed": "还没有模型规则。",
    "copiedToClipboard": "{{label}} 已复制到剪贴板！"
  },
  "decisionsGuide": {
    "title": "Decisions API 快速开始",
    "closeAriaLabel": "关闭快速开始",
    "description": "从你定义的选项中选出一个答案。请求体原样透传给服务商，网关只路由 `model`。openai 和 team 端点的用法相同。"
  },
  "context1M": {
    "enabledTitle": "已启用 1M 上下文窗口",
    "disabledTitle": "1M 已关闭",
    "enabledBody": "模型名称已添加 [1m] 后缀，以支持扩展上下文。",
    "disabledBody": "已移除强制添加的 [1m] 模型后缀；客户端主动请求 1M 时仍会生效。",
    "requiresApplyHint": "请应用下方的配置并重启 {{client}} 以使更改生效。",
    "appliedOn": "已开启 1M 上下文，{{client}} 配置已自动更新，重启 {{client}} 后生效。",
    "appliedOff": "已关闭 1M 上下文，{{client}} 配置已自动更新，重启 {{client}} 后生效。",
    "restartOnlyHint": "请重启 {{client}} 并重新选择模型以使更改生效。"
  }
};
