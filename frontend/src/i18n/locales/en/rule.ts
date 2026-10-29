// Routing rules and templates
export default {
  "rule": {
    "pageTitle": "Advanced Proxy Configuration",
    "subtitle": "Configure local models to forward requests to remote providers",
    "addButton": "Add Forwarding Rule",
    "emptyState": {
      "title": "No rules configured",
      "description": "Click \"Add Rule\" to create your first rule"
    },
    "card": {
      "unspecifiedModel": "Please specify model name",
      "useKey": "Use {{count}} {{key}}",
      "key": "Key",
      "keys": "Keys",
      "responseAs": "Response as {{model}}"
    },
    "graph": {
      "title": "Request Proxy Visualization",
      "requestLocalModel": "Request Model Name",
      "responseModel": "Response Model",
      "requestLocalTooltip": "The model name that clients use to make requests. This will be matched against incoming API calls.",
      "responseTooltip": "The model name returned to clients. Responses from upstream providers will be transformed to show this model name instead.",
      "forwardingToProviders": "Forwarding to Providers",
      "addProvider": "Add Provider",
      "noProviders": "No providers configured",
      "legend": "• Click provider node to select provider and model",
      "selectProvider": "Select Provider",
      "selectModel": "Select Model"
    },
    "service": {
      "providerNotFound": "Provider not found. Please refresh or re-import.",
      "providerDisabled": "Provider is disabled — routing skips this service until it is re-enabled.",
      "selectProvider": "Select Provider",
      "selectModel": "Select Model",
      "testService": "Test Service",
      "editProvider": "Edit Provider",
      "deleteService": "Delete Service",
      "quota": {
        "left": "{{value}} left",
        "resetsIn": "resets in {{duration}}",
        "updated": "Updated {{duration}} ago",
        "clickToRefresh": "click to refresh",
        "refreshing": "Refreshing…",
        "refreshFailed": "Refresh failed — showing the last reading"
      }
    },
    "tier": {
      "invalidInput": "Please enter a valid number.",
      "tooltipSet": "Tier {{tier}} (lower = tried first). Click to change.",
      "tooltipUnset": "No tier set (load balanced with other T0 models). Click to assign.",
      "ariaLabel": "Tier {{tier}}",
      "ariaUnset": "No tier",
      "editTitle": "Set Tier",
      "adjustTier": "Adjust tier",
      "helpHigher": "Lower number = higher priority (T0 is tried first). Models in the same tier are load balanced.",
      "helpZero": "Set to 0 for T0 — the first tier.",
      "tierLabel": "T{{index}}",
      "tierBalanced": "Balanced",
      "dividerHelp": "Lower-numbered tiers are always tried first. Only when all models in a tier fail (circuit open) does traffic fall through to the next tier. Models within the same tier are load-balanced.",
      "tooltip": "T0 is tried first, T1 is the fallback, and so on. Models within the same tier are load-balanced.",
      "addTierTooltip": "Add a new fallback tier",
      "nodeTooltipPrimaryTitle": "T0 — Highest priority",
      "nodeTooltipPrimaryBody": "Tried first on every request. Models here are load-balanced.",
      "nodeTooltipFallbackTitle": "T{{tier}} — Fallback tier",
      "nodeTooltipFallbackBody": "Tried only when all higher-priority tiers are unavailable (lower number = higher priority). Models here are load-balanced.",
      "nodeMoveHint": "↑ / ↓  on a model card to move it to a different tier",
      "nodeTooltipLearnMore": "View tier guide →",
      "guideButtonAriaLabel": "View tier guide",
      "guide": {
        "title": "Understanding Tiers",
        "subtitle": "Step {{current}} of {{total}}",
        "previous": "Previous",
        "next": "Next",
        "gotIt": "Got it!",
        "close": "Close",
        "firstRunHint": "💡 You just added your second provider. Configure tiers to set up primary and fallback routing!",
        "dontShowAgain": "Don't show this again",
        "hoverHint": "Hover over a node in the diagram to see its actions",
        "steps": {
          "1": {
            "title": "What is a Tier?",
            "content": "Tiers organize your models by priority. T0 (tier zero) is the highest priority tier — models here are tried first on every request. Lower tier numbers mean higher priority.",
            "annotation": {
              "tier": "T0 — Highest priority tier",
              "service": "Your model card with model and provider info"
            }
          },
          "2": {
            "title": "Multiple Models in One Tier",
            "content": "When you have multiple models in the same tier (like T0), they share the incoming traffic. This is called load balancing — requests are distributed across all models in the tier.",
            "annotation": {
              "loadBalance": "Same tier = load balanced",
              "multiple": "Multiple models share traffic"
            }
          },
          "3": {
            "title": "Setting Up Primary and Fallback",
            "content": "Use the ↑/↓ buttons on model cards to move them between tiers. Models in T0 are your primary choice. Models in T1, T2, etc. act as fallbacks — they only run when all higher-priority tiers fail.",
            "annotation": {
              "primary": "T0 — Primary models (tried first)",
              "fallback": "T1 — Fallback models (used when T0 fails)",
              "actionButtons": "↑/↓ buttons move models between tiers"
            }
          },
          "4": {
            "title": "Automatic Failover",
            "content": "When all models in a tier fail (circuit breaker opens), traffic automatically falls back to the next tier. Once the tier recovers (circuit breaker closes), traffic returns to it automatically. You don't need to do anything — it just works.",
            "annotation": {
              "circuitBreaker": "Circuit breaker monitors model health",
              "automaticFailover": "Automatic failover to next tier"
            }
          },
          "5": {
            "title": "Multi-Tier Fallback Chain",
            "content": "You can create as many tiers as you need. T0 → T1 → T2 → ... Traffic cascades down until it finds a working tier. Use this for cost optimization (cheap first, expensive as backup) or regional failover (local first, remote as backup).",
            "annotation": {
              "priority": "Lower number = higher priority",
              "cascade": "Traffic cascades down through tiers"
            }
          }
        }
      }
    },
    "routing": {
      "directTooltipTitle": "Direct Routing",
      "directTooltipBody": "Load balance across all services in tier order. Simple and predictable.",
      "smartTooltipTitle": "Smart Routing",
      "smartTooltipBody": "Route based on custom conditions like model name, token count, or user groups.",
      "tooltipHint": "Click a button to switch modes",
      "viewDirectGuide": "View direct routing guide →",
      "viewSmartGuide": "View smart routing guide →",
      "guide": {
        "directTitle": "Direct Routing Guide",
        "smartTitle": "Smart Routing Guide",
        "subtitle": "Step {{current}} of {{total}}",
        "previous": "Previous",
        "next": "Next",
        "gotIt": "Got it!",
        "close": "Close",
        "hoverHint": "Hover over a node in the diagram to see its actions",
        "toolbarLabel": "Page toolbar",
        "clickHere": "Click here",
        "steps": {
          "connectAI": {
            "title": "Connect an AI provider",
            "content": "Routing needs at least one AI service to forward to. Use Connect AI in the page toolbar to add a provider — paste an API key, sign in with OAuth, or point at a self-hosted server. Until you do, a rule has nothing to route to.",
            "annotation": {
              "toolbar": "Connect AI lives in the page toolbar",
              "empty": "An empty rule has no model yet"
            }
          },
          "addModel": {
            "title": "Add your first model",
            "content": "Each rule maps a request model to one or more models. In an empty rule, click ＋ Add model to pick a connected provider and a model. Want more models on this agent? Use New Model in the toolbar; turn on Select multiple there to add several at once, one rule each.",
            "annotation": {
              "addModel": "＋ Add model — pick provider + model",
              "newRule": "New Model adds models, one rule each"
            }
          },
          "editModel": {
            "title": "Change or remove a model",
            "content": "Click any model card to edit it — swap to a different model, switch the provider, or move it to another tier. Hover the card to reveal its actions; the trash icon removes it from the rule.",
            "annotation": {
              "click": "Click a card to edit / swap the model",
              "remove": "Hover → trash icon to remove"
            }
          },
          "loadBalance": {
            "title": "Load balancing within a tier",
            "content": "When several models share the same tier (T0), incoming traffic is spread evenly across them. This balances load and prevents any single model from being overwhelmed.",
            "annotation": {
              "sameTier": "Same tier = load balanced",
              "services": "Multiple models share traffic"
            }
          },
          "tierFallback": {
            "title": "Tier-based fallback chain",
            "content": "Lower tiers are tried first: T0 is primary, and if every T0 model fails, traffic cascades to T1, then T2, and so on. Use the up/down actions on a card to move it between tiers and build a failover chain.",
            "annotation": {
              "primary": "T0 — primary (tried first)",
              "fallback": "T1 — fallback (used when T0 fails)"
            }
          },
          "smartIntro": {
            "title": "What is Smart Routing?",
            "content": "Smart routing lets you define custom conditions to control which model handles each request. Route by model name, token count, user group, or any request parameter — fine-grained control without juggling tier configurations.",
            "annotation": {
              "smartButton": "Switch to Smart with the entry toggle",
              "conditional": "Conditional routing based on rules"
            }
          },
          "smartConditions": {
            "title": "Smart routing conditions",
            "content": "Each smart rule has a condition that decides when it applies — e.g. model name 'contains claude', or token count 'gt 4000' for large contexts. Rules are evaluated top to bottom; the first match wins.",
            "annotation": {
              "modelBased": "Route by model name",
              "tokenBased": "Route by token count"
            }
          },
          "smartAdvanced": {
            "title": "Advanced smart routing",
            "content": "Stack multiple smart rules into a richer strategy: send Claude requests one way, large contexts another, premium users to a third. Anything that matches no rule falls through to the default services.",
            "annotation": {
              "defaultRoute": "Default route for unmatched requests",
              "claudeRoute": "Route for Claude models",
              "largeContext": "Route for large context windows"
            }
          }
        }
      }
    },
    "nodes": {
      "addModel": "Add model"
    },
    "guideDiagrams": {
      "empty": { "description": "Example rule" },
      "single-provider": { "description": "Single provider rule" },
      "two-providers-same-tier": { "description": "Load balancing example" },
      "two-providers-different-tiers": { "description": "Primary and fallback example" },
      "three-tiers": { "description": "Multi-tier fallback example" },
      "runtime-failover": { "description": "Failover scenario" },
      "direct-single": { "description": "Direct routing with single provider" },
      "direct-multiple-tiers": { "description": "Direct routing with multiple tiers" },
      "smart-basic": {
        "description": "Smart routing with basic conditions",
        "smart": { "0": "Route Claude requests to Anthropic" }
      },
      "smart-conditions": {
        "description": "Smart routing with multiple conditions",
        "smart": { "0": "Route Claude requests to Anthropic", "1": "Route large token requests to Azure" }
      },
      "smart-complex": {
        "description": "Smart routing with complex conditions",
        "smart": { "0": "Route Claude requests to Anthropic", "1": "Route large token requests to Azure", "2": "Route @@@ds commands to DeepSeek" }
      }
    },
    "menu": {
      "refreshModels": "Refresh Models",
      "deleteProvider": "Delete Provider",
      "deleteService": "Delete Service",
      "deleteSmartRule": "Delete Smart Rule"
    },
    "ruleConditions": {
      "defaultTitle": "Default",
      "addCondition": "Add condition",
      "conditionsHint": "Conditions are checked from top to bottom; the first match wins.",
      "conditionsPaused": "Conditions are saved but paused",
      "conditionsResume": "Resume"
    },
    "tooltips": {
      "addProviderFirst": "Add a provider to enable request forwarding",
      "addProviderSecond": "Add another provider (with 2+ providers, load balancing will be enabled based on strategy)",
      "addProviderMore": "Add another provider (requests will be load balanced across all providers)",
      "addFirstProvider": "Add your first provider",
      "addServiceFirst": "Add a model to enable request forwarding",
      "addServiceSecond": "Add another model (load balancing will be enabled)"
    },
    "notifications": {
      "loadFailed": "Failed to load data",
      "requestModelRequired": "Request model name is required",
      "modelRequired": "Please select a model for provider {{name}}",
      "saved": "Rule \"{{model}}\" saved successfully",
      "saveFailed": "Failed to save rule: {{error}}",
      "saveError": "Error saving rule: {{error}}",
      "reset": "Rule reset to latest saved state",
      "modelsRefreshed": "Successfully refreshed models for {{name}}",
      "modelsRefreshFailed": "Failed to refresh models: {{error}}",
      "modelsRefreshError": "Failed to refresh models: {{error}}"
    },
    "deleteDialog": {
      "title": "Delete Rule",
      "description": "Are you sure you want to delete this rule? This action cannot be undone.",
      "cancelButton": "Cancel",
      "confirmButton": "Delete"
    },
    "status": {
      "clickToActivate": "Click to activate",
      "clickToDeactivate": "Click to deactivate",
      "cannotToggle": "Cannot toggle"
    },
    "smart": {
      "untitledRule": "Untitled Smart Rule",
      "noOperation": "No Operation",
      "noValue": "No value",
      "deleteTooltip": "Delete smart rule",
      "unconditional": "Unconditional, ignore"
    }
  },
  "ruleAutoSave": {
    "saved": "Configuration saved successfully",
    "failed": "Failed to save: {{error}}",
    "unknownError": "Unknown error",
    "error": "Error saving configuration"
  },
  "ruleHeader": {
    "plugins": "Plugins",
    "pluginsTooltip": "Plugins enabled on this rule",
    "endpointNotApplicable": "Only applies to OpenAI-compatible upstreams. This rule has none, so the choice has no effect for now.",
    "managePlugins": "Manage plugins",
    "test": "Test",
    "ruleActions": "Rule actions",
    "compact": {
      "chip": "Compact: {{value}}",
      "auto": "Auto",
      "aria": "Auto-compact for this model: {{value}}",
      "tooltip": "Choose when Codex compacts this model's context",
      "title": "Auto-compact trigger",
      "description": "Codex compacts the conversation once the context reaches this share of the window. It only goes into the Codex config; requests are not affected.",
      "following": "Following Auto Config ({{percent}}%). Drag to set a value for this model only.",
      "window": "Window",
      "line": "Compacts at",
      "remain": "Free when compacting",
      "low": "Only about {{remain}} tokens will be free when compacting (at least {{min}} recommended). A long tool output could overflow the window and make compaction fail.",
      "limited": "Capped by the usable window ({{effective}}%), so {{applied}}% is what takes effect. To compact later, raise Usable window in Auto Config.",
      "zoneLow": "{{from}}–{{to}}%: too few tokens left when compacting",
      "zoneCapped": "Above {{effective}}% counts as {{effective}}% (usable window)",
      "needsApply": "Takes effect after Auto Config is applied.",
      "reset": "Use Auto Config value ({{percent}}%)"
    }
  },
  "templateActions": {
    "troubleshoot": "Troubleshoot",
    "more": "More",
    "sortMenuToName": "Sort by name (A→Z)",
    "sortMenuToOriginal": "Sort by original order",
    "collapseAllRules": "Collapse all rules",
    "expandAllRules": "Expand all rules",
    "connectAI": "Connect AI",
    "newRule": "New Model",
    "createNewRule": "Add models to this agent; each one gets its own routing rule",
    "selectedModels_one": "{{count}} model selected",
    "selectedModels_other": "{{count}} models selected",
    "multiSelectTitle": "Create a rule for each model",
    "selectMultiple": "Select multiple",
    "clearSelection": "Clear",
    "addModels_one": "Add {{count}} model",
    "addModels_other": "Add {{count}} models",
    "howRoutingWorks": "How routing works",
    "sortOriginal": "Original order",
    "sortByName": "Name (A→Z)",
    "sortTooltipToName": "Showing original order — click to sort by name",
    "sortTooltipToOriginal": "Showing by name — click to restore original order"
  },
  "templatePage": {
    "noProviders": {
      "title": "No Providers Configured",
      "description": "Add an API key provider to start routing requests",
      "action": "Get started"
    },
    "noRules": "No rules yet. Click \"New Model\" to add one.",
    "noRulesFixed": "No model rules yet.",
    "copiedToClipboard": "{{label}} copied to clipboard!"
  },
  "decisionsGuide": {
    "title": "Decisions API Quick Start",
    "closeAriaLabel": "Close quick start",
    "description": "Pick one answer from options you define. The body is passed through to the provider as-is; only `model` is routed. Works the same on the openai and team endpoints."
  },
  "context1M": {
    "enabledTitle": "1M Context Window Enabled",
    "disabledTitle": "1M Off",
    "enabledBody": "Model names have been updated with [1m] suffix for extended context support.",
    "disabledBody": "The forced [1m] model suffix has been removed. Client requests for 1M are still honored.",
    "requiresApplyHint": "Please apply the configuration below and restart {{client}} for changes to take effect.",
    "appliedOn": "1M context on. {{client}}'s config is updated; restart {{client}} to take effect.",
    "appliedOff": "1M context off. {{client}}'s config is updated; restart {{client}} to take effect.",
    "restartOnlyHint": "Please restart {{client}} and re-select the model for changes to take effect."
  }
};
