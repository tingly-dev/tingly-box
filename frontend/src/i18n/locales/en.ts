export default {
  "common": {
    "back": "Back",
    "add": "Add",
    "cancel": "Cancel",
    "save": "Save",
    "delete": "Delete",
    "edit": "Edit",
    "confirm": "Confirm",
    "loading": "Loading...",
    "enabled": "Enabled",
    "disabled": "Disabled",
    "active": "Active",
    "inactive": "Inactive",
    "close": "Close",
    "done": "Done",
    "applying": "Applying...",
    "copy": "Copy",
    "copied": "Copied",
    "refresh": "Refresh",
    "verify": "Verify",
    "saveChanges": "Save Changes",
    "success": "Success",
    "error": "Error",
    "warning": "Warning",
    "info": "Info",
    "on": "On",
    "off": "Off",
    "direct": "Direct",
    "theme": "Theme",
    "prompt": "Prompt",
    "and": "and",
    "clear": "Clear selection",
    "dismiss": "Dismiss",
    "moveDown": "Move down",
    "moveUp": "Move up",
    "saved": "Saved"
  },
  "layout": {
    "appTitle": "Tingly Box",
    "slogan": "Your Intelligence, Orchestrated.",
    "version": "version<br/>{{version}}",
    "settings": "Settings",
    "nav": {
      "home": "Agent",
      "settings": "Settings",
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
      "useImageGen": "Image",
      "useTeam": "Team",
      "useCustom": "Custom",
      "apiKeys": "API Keys",
      "oauth": "OAuth",
      "credential": "Credential",
      "prompt": "Prompt"
    },
    "sidebar": {
      "newProfile": "New Profile",
      "newTeam": "New Team",
      "profileName": "Profile name",
      "mode": "Mode",
      "modeUnified": "Unified: Single model for all",
      "modeSeparate": "Separate: Individual models",
      "separate": "Separate",
      "unified": "Unified",
      "createProfileTooltip": "Create a new Claude Code profile with custom settings",
      "createTeamTooltip": "Create an isolated Team workspace",
      "sloganTooltip": "For all Solo Builders, Dev Teams and Agents.",
      "collapse": "Collapse sidebar",
      "expand": "Expand sidebar"
    },
    "activityBar": {
      "disconnected": "Disconnected",
      "disconnectedDebug": "Disconnected (Debug)",
      "devMode": "Dev",
      "newVersionAvailable": "Update",
      "error": "Error",
      "theme": "Theme",
      "light": "Light",
      "dark": "Dark",
      "system": "System",
      "sunlit": "Sunlit",
      "claude": "Claude",
      "click": "Click",
      "feedback": "Feedback",
      "feedbackTooltip": "Send Feedback (opens GitHub Issues)"
    },
    "themeMenu": {
      "switchTo": "Switch to:",
      "theme": "Theme:"
    },
    "easterEgg": "Hi, I'm Tingly-Box, Your Smart AI Orchestrator",
    "dashboard": "Dashboard",
    "usage": "Usage",
    "userUsage": "Team usage",
    "heatmap": "Heatmap",
    "today": "Today",
    "yesterday": "Yesterday",
    "days": "Days",
    "remote": "Remote",
    "remoteControl": "Remote Control",
    "notify": "IM Notify",
    "bots": "Bots",
    "overview": "Overview",
    "platforms": {
      "weixin": "Weixin",
      "wecom": "WeCom",
      "telegram": "Telegram",
      "feishu": "Feishu",
      "lark": "Lark",
      "dingtalk": "DingTalk"
    },
    "guardrails": "Guardrails",
    "policyGroups": "Policy Groups",
    "policies": "Policies",
    "guardrailsHistory": "History",
    "mcp": "MCP",
    "sources": "Sources",
    "localMode": "Local Mode",
    "modelKey": "Model Key",
    "tinglyBox": "Sharing",
    "tinglyBoxTooltip": "Distribute model access without sharing your provider credentials. Each share token tracks usage independently and can be revoked at any time without affecting the others.",
    "virtualModels": "Virtual Models",
    "virtualModelsNavLabel": "VModel",
    "virtualModelsTooltip": "Built-in synthetic model providers for onboarding, demos, and dry-runs. They respond locally without contacting any upstream.",
    "accessControl": "Access Control",
    "status": "Status",
    "system": "System",
    "general": "General",
    "experimental": "Experimental",
    "develop": "Develop",
    "logs": "Logs",
    "userRequest": "User Request",
    "skills": "Skills",
    "addProfile": "Add Profile",
    "addTeam": "Add Team",
    "default": "default",
    "help": "Tips & Help",
    "helpShort": "Help",
    "tools": "Tools",
    "servertool": "Servertool"
  },
  "health": {
    "connected": "Connected",
    "disconnected": "Disconnected",
    "checking": "Checking...",
    "lastChecked": "Last checked: {{time}}",
    "never": "Never",
    "retry": "Retry",
    "disconnectMessage": "Connection to server lost. Please check if the server is running.",
    "disconnectTitle": "Connection Lost"
  },
  "update": {
    "newVersionAvailable": "New Version Available",
    "versionAvailable": "New: {{latest}} (you have {{current}})",
    "download": "Download",
    "close": "Close",
    "checking": "Checking for updates...",
    "message": "A new version is available on GitHub. Would you like to download it now?",
    "later": "Later",
    "check": "Check for Updates",
    "checkUpdates": "Manual Update Check",
    "upToDate": "You're on the latest version",
    "updateAvailable": "New version available",
    "currentVersion": "Current version: {{version}}",
    "versionComparison": "{{latest}} is available (you have {{current}})",
    "releaseNotes": "View Release Notes",
    "updateMethods": "Update Methods",
    "copy": "Copy",
    "copied": "Copied!",
    "error": "Failed to check for updates",
    "methods": {
      "npx": {
        "title": "Quick Update (npx)",
        "description": "One command: downloads the new version and restarts the server"
      },
      "npm": {
        "title": "Global Install (npm)",
        "description": "Update the installed CLI, then restart the server to apply"
      },
      "docker": {
        "title": "Docker Image",
        "description": "Pull from GitHub Container Registry"
      }
    }
  },
  "login": {
    "title": "Tingly Box",
    "subtitle": "Your Intelligence, Orchestrated",
    "tokenLabel": "Authentication Token",
    "tokenHelper": "Enter your user authentication token for UI and management access",
    "loginButton": "Login",
    "validating": "Validating...",
    "generateTokenButton": "Generate New Token",
    "errors": {
      "invalidToken": "Invalid token. Please check your token and try again.",
      "validationFailed": "Failed to validate token. Please check your connection and try again.",
      "enterValidToken": "Please enter a valid token"
    },
    "success": {
      "loginSuccess": "Login successful! Redirecting..."
    }
  },
  "home": {
    "tabs": {
      "useOpenAI": "Use OpenAI",
      "useAnthropic": "Use Anthropic",
      "useClaudeCode": "Use Claude Code"
    },
    "emptyState": {
      "title": "No API Keys Available",
      "description": "Get started by adding your first AI API Key to use the service.",
      "button": "Connect Your First AI"
    },
    "token": {
      "generated": "{{label}} copied to clipboard!",
      "copyFailed": "Failed to copy to clipboard",
      "generationFailed": "Failed to generate token: {{error}}",
      "refresh": {
        "title": "Confirm Token Refresh",
        "alert": "Important Reminder",
        "description": "Modifying the token will cause configured tools to become unavailable. Are you sure you want to continue generating a new token?",
        "button": "Confirm Refresh"
      }
    },
    "notifications": {
      "providerAdded": "Provider added successfully!",
      "providerAddFailed": "Failed to add provider: {{error}}"
    }
  },
  "provider": {
    "pageTitle": "Credentials",
    "subtitleWithCount": "Managing {{count}} providers and API keys",
    "subtitleEmpty": "No API keys configured yet",
    "addButton": "Connect AI",
    "emptyCardTitle": "No Model API Key Configured",
    "emptyCardSubtitle": "Get started by connecting your first AI provider",
    "emptyCardButton": "Connect Your First Provider",
    "emptyCardContent": "Configure your API tokens and keys to access AI services",
    "notifications": {
      "loadFailed": "Failed to load providers: {{error}}",
      "added": "Provider added successfully!",
      "updated": "Provider updated successfully!",
      "deleted": "Provider deleted successfully!",
      "addFailed": "Failed to add provider: {{error}}",
      "updateFailed": "Failed to update provider: {{error}}",
      "deleteFailed": "Failed to delete provider: {{error}}",
      "toggleFailed": "Failed to toggle provider: {{error}}",
      "loadDetailFailed": "Failed to load provider details: {{error}}"
    }
  },
  "cloudDialog": {
    "name": "Name",
    "connect": "Connect",
    "connected": "Provider connected successfully!",
    "connectFailed": "Failed to connect provider",
    "showAdvanced": "Advanced (optional)",
    "hideAdvanced": "Hide advanced",
    "awsAuthHint": "Authenticate with an Access Key ID + Secret pair, or a Bedrock API Key alone.",
    "validation": {
      "awsEitherOr": "Provide either Access Key ID + Secret Access Key, or a Bedrock API Key",
      "badRegion": "Region must look like \"us-east-1\" (lowercase letters, digits, hyphens)",
      "badLocation": "Location must look like \"us-east5\" or \"global\" (lowercase letters, digits, hyphens)",
      "badSaJson": "Service Account JSON is not valid JSON — paste the full key file content",
      "badEndpoint": "Endpoint must be a full URL like https://my-resource.openai.azure.com"
    },
    "fields": {
      "region": {"label": "Region"},
      "access_key_id": {"label": "Access Key ID"},
      "secret_access_key": {"label": "Secret Access Key"},
      "bearer_token": {"label": "Bedrock API Key", "helper": "Alternative to the access key pair — provide one or the other"},
      "session_token": {"label": "Session Token", "helper": "Optional — for temporary (STS) credentials"},
      "project_id": {"label": "Project ID"},
      "location": {"label": "Location"},
      "service_account_json": {"label": "Service Account JSON"},
      "endpoint": {"label": "Endpoint"},
      "api_version": {"label": "API Version"},
      "api_key": {"label": "API Key"}
    }
  },
  "providerDialog": {
    "addTitle": "Connect AI",
    "addDescription": "Select a provider and enter your API key to connect AI services. Multiple protocols can be enabled for providers that support them.",
    "editTitle": "Edit AI Config",
    "addButton": "Connect",
    "protocol": {
      "label": "Protocols",
      "openAILabel": "OpenAI Compatible",
      "anthropicLabel": "Anthropic Compatible",
      "helperOpenAI": "Supports models from OpenAI, Google and many other OpenAI-compatible providers",
      "helperAnthropic": "For Anthropic-compatible AI providers, commonly used with Claude Code",
      "fromTemplate": "from template",
      "recommendedBadge": "Recommended"
    },
    "candidates": {
      "title": "Matching providers — click to fill URLs"
    },
    "keyName": {
      "label": "Name",
      "placeholder": "e.g., OpenAI",
      "default": "Default Provider",
      "helper": "Leave blank to use the auto-generated name. You can rename later.",
      "editAction": "Edit name",
      "fallback": "Custom Provider"
    },
    "providerOrUrl": {
      "label": "Provider or Custom Base URL",
      "placeholder": "Select a provider or enter custom URL"
    },
    "apiKey": {
      "label": "API Key",
      "placeholderAdd": "Your API key",
      "placeholderEdit": "Leave empty to keep current key",
      "helperEdit": "Leave empty to keep current key"
    },
    "enabled": "Enabled",
    "export": {
      "action": "Copy Export",
      "copying": "Copying...",
      "base64": "Copy Base64",
      "jsonl": "Copy JSONL"
    },
    "advanced": {
      "title": "Advanced",
      "proxyUrl": {
        "label": "HTTP/SOCKS Proxy URL (Optional)",
        "placeholder": "http://127.0.0.1:7890 or socks5://127.0.0.1:7890",
        "helper": "Optional: Use a proxy to bypass region restrictions. Saved for future use.",
        "useGlobal": "Use quick proxy ({{url}})",
        "useGlobalNotSet": "Use quick proxy (not configured — set in System Settings)"
      }
    },
    "verification": {
      "verifying": "Verifying...",
      "verifyButton": "Verify",
      "missingFields": "Please fill in all required fields (API Style, Name, API Base URL, API Key)",
      "failed": "Connection check failed",
      "networkError": "Unable to connect. Please check your network and proxy settings.",
      "failureHint": "You can still add this provider using the 'Add Anyway' button if you're sure the configuration is correct.",
      "responseTime": "Response time: {{time}}ms",
      "modelsAvailable": "{{count}} models available",
      "testResult": "Test result: {{result}}"
    },
    "forceAdd": {
      "title": "Add Provider Anyway?",
      "providerInfo": "Please confirm your provider configuration:",
      "message": "The connection check failed. This could be due to network issues, incorrect API key, or the provider not supporting standard verification methods.",
      "explanation": "Some providers may not pass standard checks but still work correctly.",
      "whyFailed": "Connection check failed:",
      "errorDetails": "Error details",
      "noKey": "Not provided",
      "confirmNoteTitle": "Are you sure you want to continue?",
      "confirmNote": "Please verify that your Base URL and API Key are correct before adding. You can still add this provider, but it may not work properly if the configuration is incorrect.",
      "cancel": "Go Back",
      "confirm": "Confirm to Add"
    },
    "provider": {
      "placeholder": "Base URL"
    },
    "v1Hint": {
      "apply": "Append /v1",
      "message": "Most OpenAI-compatible APIs need a /v1 suffix."
    }
  },
  "providerTable": {
    "columns": {
      "name": "Name",
      "apiKey": "API Key",
      "apiBase": "API Base",
      "apiStyle": "API Style",
      "actions": "Actions",
      "status": "Status"
    },
    "status": {
      "enabled": "Enabled",
      "disabled": "Disabled"
    },
    "token": {
      "notSet": "Not set",
      "view": "View Token",
      "viewTooltip": "View Token"
    },
    "deleteModal": {
      "title": "Delete Provider",
      "description": "Are you sure you want to delete provider \"{{name}}\"? This action cannot be undone.",
      "cancelButton": "Cancel",
      "confirmButton": "Delete"
    },
    "tokenModal": {
      "title": "API Key - {{providerName}}",
      "loading": "Loading API key...",
      "failedToLoad": "Failed to load token",
      "copyButton": "Copy Token",
      "loadingTooltip": "Loading...",
      "closeTooltip": "Close"
    }
  },
  "templateActions": {
    "troubleshoot": "Troubleshoot",
    "collapseAllRules": "Collapse all rules",
    "expandAllRules": "Expand all rules",
    "connectAI": "Connect AI",
    "newRule": "New Rule",
    "createNewRule": "Create new routing rule",
    "howRoutingWorks": "How routing works",
    "sortOriginal": "Original order",
    "sortByName": "Name (A→Z)",
    "sortTooltipToName": "Showing original order — click to sort by name",
    "sortTooltipToOriginal": "Showing by name — click to restore original order"
  },
  "probe": {
    "quickTest": "Quick test (stream)",
    "testAll": "Test All",
    "testAllHint": "Run a quick streaming test on every active rule",
    "viewDetails": "View details",
    "dismiss": "Dismiss",
    "testRule": "Test Rule",
    "testProvider": "Test Service",
    "shape": "Request",
    "shapeHint": "Streaming vs non-streaming — closest to production traffic by default.",
    "nonstream": "Nonstream",
    "stream": "Stream",
    "tool": "Tool",
    "toolOff": "Off",
    "toolOn": "On",
    "toolHint": "Attach a tool definition so the probe exercises tool calling. Composes with both stream and nonstream: nonstream lifts structured tool calls, stream keeps the raw chunks.",
    "scope": "Scope",
    "throughTB": "Through TB",
    "direct": "Direct",
    "scopeHint": "Direct skips Tingly-Box's routing & middleware, to tell whether a failure is upstream or inside TB.",
    "scopeRuleLocked": "Rule probes must traverse TB's middleware — that is what they test.",
    "protocol": "Protocol",
    "protocolHint": "Client-side wire protocol. Through TB, the loopback speaks it and TB transforms to the upstream exactly as production traffic does.",
    "protocolLockedRule": "Fixed by the rule's scenario.",
    "protocolLockedProvider": "This provider speaks a single protocol.",
    "protocolGoogle": "Google providers use their own SDK — no protocol selection.",
    "vision": "Vision",
    "visionNone": "Off",
    "visionUser": "User",
    "visionTool": "Tool",
    "visionHint": "Attach a red test image — in the user message, or returned from a synthetic tool round (the shape agent tools use for screenshots). A vision-capable route answers \"red\"; anything else means the image was dropped or corrupted along the path.",
    "visionGoogle": "Google providers use their own SDK — no vision probe.",
    "message": "Message",
    "messageHint": "Custom message override; empty uses the default per tool setting.",
    "thinking": "Thinking",
    "thinkingNone": "None",
    "thinkingLow": "Low",
    "thinkingMedium": "Medium",
    "thinkingHigh": "High",
    "thinkingMax": "Max",
    "thinkingHint": "Extended-thinking effort. Orthogonal to request shape — composes with both stream and nonstream. Maps to the provider's native thinking knob (Anthropic budget_tokens, OpenAI reasoning_effort, Gemini thinking_budget).",
    "run": "Run Test",
    "running": "Testing…",
    "runHint": "Pick a request type, then click Run Test",
    "rerun": "Re-run",
    "copyResponse": "Copy response",
    "copied": "Copied!",
    "requestConfig": "Request Config",
    "advanced": "Advanced",
    "copy": "Copy",
    "emptyTitle": "Not run yet",
    "emptyBody": "Set the request config on the left, then Run Test — the verdict, request journey, and cURL appear here.",
    "curl": "cURL",
    "curlCopy": "Copy cURL",
    "curlKeyHint": "Replace {{key}} with your own key before running.",
    "curlFailed": "Failed to build the cURL command.",
    "success": "Success",
    "failed": "Failed",
    "journey": "Request Journey",
    "response": "Response",
    "rawJson": "Raw JSON",
    "rawJsonHide": "Hide Raw JSON",
    "noText": "(No text extracted — see raw JSON)",
    "pending": "— pending",
    "flagsNone": "(none)",
    "directValue": "Direct (bypassed TB)",
    "row": {
      "rule": "Rule",
      "flags": "Flags",
      "routing": "Routing",
      "provider": "Provider",
      "endpoint": "Endpoint",
      "upstreamUrl": "Upstream URL",
      "requestUrl": "Request URL"
    }
  },
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
      "deleteService": "Delete Service"
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
            "content": "Each rule maps a request model to one or more models. In an empty rule, click ＋ Add model to pick a connected provider and a model. Need a separate rule for a different request model? Use New Rule in the toolbar.",
            "annotation": {
              "addModel": "＋ Add model — pick provider + model",
              "newRule": "New Rule adds another request-model mapping"
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
  "system": {
    "pageTitle": "Server Status",
    "status": {
      "running": "Running",
      "stopped": "Stopped",
      "server": "Server",
      "keys": "Keys",
      "connected": "Connected",
      "uptime": "Uptime",
      "lastUpdated": "Last Updated: {{time}}",
      "loading": "Loading...",
      "unavailable": "Unavailable"
    },
    "prompts": {
      "enterPort": "Enter port for server:",
      "enterClientId": "Enter client ID (web):"
    },
    "confirmations": {
      "stopServer": "Are you sure you want to stop server?"
    },
    "notifications": {
      "startSuccess": "{{message}}",
      "stopSuccess": "{{message}}",
      "restartSuccess": "{{message}}",
      "startFailed": "{{error}}",
      "stopFailed": "{{error}}",
      "restartFailed": "{{error}}",
      "tokenGenerated": "Token generated successfully",
      "tokenGenerateFailed": "{{error}}"
    },
    "proxy": {
      "title": "Proxy Settings",
      "label": "Proxy",
      "respectEnvProxy": {
        "label": "Respect Environment Proxy",
        "helper": "When enabled, providers without explicit proxy configuration will use system proxy settings (HTTP_PROXY, HTTPS_PROXY, macOS system proxy, Clash, etc.)"
      },
      "globalProxyUrl": {
        "label": "Quick Proxy",
        "description": "Save a proxy you reuse often so providers and OAuth can pick it up with one click — per-provider proxy still wins if set.",
        "helper": "Reusable across providers and OAuth. Per-provider proxy takes priority.",
        "saveSuccess": "Quick proxy saved",
        "saveFailed": "Failed to save quick proxy"
      },
      "notifications": {
        "updateSuccess": "Proxy settings updated successfully",
        "updateFailed": "Failed to update proxy settings: {{error}}"
      }
    },
    "accessControl": {
      "userToken": "User Token (Control Panel)",
      "modelToken": "Model Token (API Proxy)",
      "userTokenDesc": "This token protects access to the web control panel. Share the Model Token with API users instead.",
      "modelTokenDesc": "Share this token with users who need API access.",
      "copy": "Copy",
      "copied": "Copied!",
      "resetToken": "Reset Token",
      "resetting": "Resetting...",
      "viewFullToken": "View Full Token",
      "fullTokenWarning": "Keep your token secure. Anyone with this token can access your control panel.",
      "secure": "Token is secure (randomly generated)",
      "warning": {
        "default": "You are using the default user token. This is a security risk! Please reset to a secure random token.",
        "resetNow": "Reset Now"
      },
      "reset": {
        "title": "Reset User Token",
        "confirm": "Are you sure you want to reset your user token?",
        "points": {
          "new": "A new random token will be generated",
          "session": "Your current session will be updated automatically",
          "other": "Any other browsers/devices will need to log in again",
          "stop": "The old token will immediately stop working"
        },
        "warning": "Make sure you have access to this device before resetting.",
        "cancel": "Cancel"
      },
      "success": {
        "title": "Token Reset Successfully",
        "message": "Your new user token has been generated and saved to your session.",
        "saved": "I've Saved My Token"
      }
    },
    "language": {
      "title": "Language",
      "description": "Select interface display language",
      "en": "English",
      "zh": "中文",
      "ru": "Русский",
      "current": "Current",
      "saveSuccess": "Language settings updated",
      "saveFailed": "Failed to update language settings"
    },
    "experimentalFeatures": {
      "title": "Experimental Features",
      "description": "These experimental features apply globally to all scenarios. Individual scenarios can override these settings.",
      "skills": "Skills",
      "userPrompts": "User prompts",
      "guardrails": "Guardrails",
      "mcp": "MCP",
      "enableUserPrompts": "Manage reusable instructions for user requests",
      "enableIdeSkills": "Manage code snippets and skills from your IDE",
      "enableGuardrails": "Block risky tool calls and filter sensitive outputs",
      "enableMCP": "MCP (Model Context Protocol) tools such as web search and web fetch",
      "on": "On",
      "off": "Off",
      "enabled": "enabled",
      "disabled": "disabled - Click to enable",
      "guardrailsEnabledInfo": "Guardrails is enabled. A \"Guardrails\" page is available in the sidebar for rule management.",
      "mcpEnabledInfo": "MCP Tools is enabled. An \"MCP Tools\" page is available under System in the sidebar for configuration.",
      "requiredMessage": "{{feature}} is off. Turn it on below to continue.",
      "enableFailed": "Could not enable this feature. Check the server connection and try again."
    },
    "about": {
      "title": "About",
      "version": "Version",
      "license": "License",
      "github": "GitHub",
      "devMode": "Dev Mode",
      "available": "available",
      "copyVersion": "Copy version",
      "versionCopied": "Version copied to clipboard",
      "checkUpdate": "Click to check for updates",
      "updateAvailable": "Version {{version}} available — click to view"
    },
    "serverStatus": {
      "title": "Server Status",
      "server": "Server",
      "forceLogout": "Force logout",
      "refreshStatus": "Refresh status"
    },
    "preferences": {
      "title": "Appearance & Language"
    }
  },
  "help": {
    "title": "Tips & Help",
    "description": "A few easy-to-miss but useful things.",
    "shortcut": {
      "title": "Desktop Shortcut",
      "description": "One click to restart Tingly Box and open the web UI. Never auto-updates or auto-restarts on its own — creating it is the only thing this does.",
      "create": "Create Shortcut",
      "recreate": "Recreate",
      "creating": "Creating...",
      "alreadyCreated": "Already created",
      "doubleClick": "Double-click it to start Tingly Box and open the web UI.",
      "runHeadless": "No graphical session on this machine? Run this instead:",
      "createFailed": "Failed to create shortcut: {{error}}"
    },
    "providers": {
      "title": "Providers",
      "description": "Browse the catalog or paste a config snippet — we'll figure out the rest."
    },
    "routing": {
      "title": "Routing & Tier Guides",
      "description": "Revisit how direct routing, smart routing, and model tiers work — the same diagrams and steps used elsewhere in the product.",
      "direct": "Direct Routing Guide",
      "smart": "Smart Routing Guide",
      "tier": "Tier Guide"
    }
  },
  "serverInfo": {
    "title": "API Endpoints",
    "openAI": {
      "label": "OpenAI Base URL",
      "copyTooltip": "Copy OpenAI Base URL",
      "copyCurlTooltip": "Copy OpenAI cURL Example"
    },
    "anthropic": {
      "label": "Anthropic Base URL",
      "copyTooltip": "Copy Anthropic Base URL",
      "copyCurlTooltip": "Copy Anthropic cURL Example"
    },
    "docker": {
      "tooltip": "Docker mode. To access from container, configure network: --network=host on Linux, or use host.docker.internal on Docker Desktop (Mac/Windows)"
    },
    "authentication": {
      "title": "Authentication",
      "apiKeyLabel": "API Key",
      "showTokenTooltip": "Show token",
      "hideTokenTooltip": "Hide token",
      "copyTokenTooltip": "Copy Token",
      "generateTooltip": "Generate New Token"
    },
    "notifications": {
      "copied": "{{label}} copied to clipboard!",
      "copyFailed": "Failed to copy to clipboard",
      "generateFailed": "Failed to generate token: {{error}}"
    }
  },
  "apiKeyModal": {
    "title": "API Key",
    "description": "Your authentication token:",
    "clickToCopy": "Click to copy token",
    "copyButton": "Copy Token"
  },
  "history": {
    "pageTitle": "Activity Log & History",
    "subtitle": "{{count}} recent activity entries"
  },
  "openCodeConfig": {
    "title": "Configure OpenCode",
    "subtitle": "Set up OpenCode to use Tingly Box as your AI model proxy",
    "configLocation": "Config Location:",
    "configurationTitle": "Configuration"
  },
  "codexConfig": {
    "title": "Configure Codex",
    "subtitle": "Configure Codex to use Tingly Box through `~/.codex/config.toml` and `~/.codex/auth.json`",
    "resetTooltip": "Reset model & reasoning to defaults",
    "tabQuick": "Quick Config",
    "tabManual": "Manual",
    "applySuccess": "Codex configuration applied to ~/.codex",
    "applyFailed": "Failed to apply configuration",
    "selectOAuthProvider": "Select a Codex OAuth provider to export.",
    "authMode": {
      "title": "Authentication",
      "apikey": {
        "label": "Tingly Box gateway",
        "captionPrefix": "codex routes through Tingly Box. Gateway key written to"
      },
      "hybrid": {
        "label": "Tingly Box gateway + keep official ChatGPT login",
        "caption": "Routes through Tingly Box, but the gateway key moves into config.toml so your ChatGPT login in auth.json stays intact — Codex App still recognizes your account (remote control, plugins, account display)."
      },
      "chatgpt": {
        "label": "Direct to OpenAI",
        "caption": "codex talks to OpenAI directly using your official ChatGPT login. No gateway."
      }
    },
    "oauthAccount": {
      "title": "ChatGPT account",
      "tooltipHybrid": "The gateway token is written into config.toml's provider block (experimental_bearer_token). Pick a stored Codex login to (re)write it into auth.json, or leave it as \u201cKeep existing\u201d to not touch the file.",
      "tooltipChatgpt": "Exports the OAuth tokens to ~/.codex/auth.json and removes the tingly gateway keys from config.toml so codex CLI talks directly to OpenAI. Tingly Box will not manage token refresh after this — codex CLI owns the token lifecycle. If id_token is missing in the exported file, re-authenticate via the OAuth page and apply again.",
      "noProvider": "No Codex OAuth provider — log in first",
      "selectProvider": "Select a Codex OAuth provider",
      "keepExisting": "Keep existing auth.json (don\u2019t modify)"
    }
  },
  "dshConfig": {
    "title": "Configure DeepSeek Harness",
    "subtitle": "Configure dsh to use Tingly Box through `$DSH_HOME/settings.yaml` and `$DSH_HOME/.credentials.yaml`",
    "resetTooltip": "Reset model capabilities to defaults",
    "tabQuick": "Quick Config",
    "tabManual": "Manual",
    "applySuccess": "DeepSeek Harness configuration applied to $DSH_HOME",
    "applyFailed": "Failed to apply configuration",
    "modelsPreviewNote": "Model ids written to settings.yaml: {{models}}"
  },
  "claudeCode": {
    "configModes": {
      "unified": {
        "label": "Unified Model",
        "description": "Config unified model for all claude code requests"
      },
      "separate": {
        "label": "Separate Model",
        "description": "Config different models for claude code scenario, like subagent, summary, default, ..."
      },
      "smart": {
        "label": "Smart",
        "description": "(WIP) Smart routing according to request field / content / model feature / user intent / ..."
      }
    },
    "modeChange": {
      "title": "Change Configuration Mode?",
      "body": "You are about to switch from {{from}} to {{to}} mode.",
      "hint": "After changing the mode, you will need to reapply the configuration to Claude Code for the changes to take effect.",
      "cancel": "Cancel",
      "confirm": "Confirm",
      "success": "Configuration mode changed to {{mode}}. Please reapply the configuration to Claude Code.",
      "failed": "Failed to save configuration mode",
      "applyFailed": "Failed to apply configurations"
    },
    "configPath": "Add env config to Claude Code config file",
    "copyConfig": "Config",
    "oneClickScript": "One-Click Script",
    "jsonConfig": "JSON Config",
    "step1": "1. Configure Model",
    "step2": "2. Skip Onboarding - Make Claude Code directly usable",
    "step3": "3. Status Line Integration (Optional)",
    "unifiedConfig": "Unified Configuration",
    "separateConfig": "Separate Configuration",
    "switchToSeparate": "Switch to Separate",
    "switchToUnified": "Switch to Unified",
    "configButton": "Auto Config",
    "quickApply": "Auto Config",
    "quickApplyWithStatusLine": "Auto Config & Status Line",
    "statusLine": {
      "description": "Install status line integration to show real-time request information in your terminal.",
      "jsonDescription": "Configure the status line integration to display real-time request information in your terminal prompt.",
      "addToSettingsJson": "Add the statusLine section to your ~/.claude/settings.json (alongside the env section):",
      "manualSetup": "Or manually download and install the status line script:",
      "downloadLink": "Download Status Line Script"
    },
    "modal": {
      "title": "Claude Code Configuration Guide",
      "subtitle": "Follow these steps to configure Claude Code to use Tingly Box as your AI proxy",
      "dontRemindAgain": "Do not remind again"
    },
    "profile": {
      "renameProfile": "Rename profile",
      "deleteProfile": "Delete profile",
      "quickStart": "Start",
      "settingsFile": "Settings",
      "settingsFileWarning": "Generated runtime settings. Manual edits are overwritten by Profile Overrides and Model Rules.",
      "resolvingSettingsFile": "Resolving generated settings path…",
      "settingsGenerated": "Generated",
      "settingsNotGenerated": "Not generated yet",
      "copySettingsFile": "Copy settings file path",
      "switchToGlobal": "Switch to global command",
      "switchToNpm": "Switch to npm command",
      "copyCommand": "Copy command",
      "clickToCopy": "Click to copy command",
      "renameTitle": "Rename Profile",
      "profileName": "Profile Name",
      "save": "Save",
      "deleteTitle": "Delete Profile",
      "deleteConfirm": "Are you sure you want to delete profile {{name}}?",
      "deleteWarning": "This will remove the profile and all its associated rules and flags. This action cannot be undone.",
      "profileRenamed": "Profile renamed",
      "profileDeleted": "Profile deleted",
      "renameFailed": "Failed to rename profile",
      "deleteFailed": "Failed to delete profile",
      "mode": "Mode",
      "unified": "Unified",
      "separate": "Separate"
    },
    "defaultModeLabel": "Default Mode"
  },
  "prompt": {
    "menu": "Prompt",
    "user": {
      "title": "User Recordings",
      "subtitle": "Browse and manage your IDE recordings",
      "filters": "Filters",
      "searchPlaceholder": "Search recordings...",
      "userFilter": "User",
      "allUsers": "All Users",
      "projectFilter": "Project",
      "allProjects": "All Projects",
      "typeFilter": "Type",
      "allTypes": "All Types",
      "recordingsFound": "{{count}} recording(s) found",
      "recordingsFor": "Recordings for {{date}}",
      "noRecordings": "No recordings found for this date",
      "actions": {
        "play": "Play",
        "viewDetails": "View Details",
        "delete": "Delete"
      },
      "types": {
        "code-review": "Code Review",
        "debug": "Debug",
        "refactor": "Refactor",
        "test": "Test",
        "custom": "Custom"
      }
    },
    "skill": {
      "title": "Skills",
      "subtitle": "Manage skills from your IDE directories",
      "addPath": "Add Path",
      "autoDiscover": "Auto-Discover",
      "refreshAll": "Refresh All",
      "adapterConfig": "Adapter Configuration",
      "locations": "Locations",
      "selectLocation": "Select a location to view skills",
      "noLocations": "No skill locations added",
      "noSkills": "No skills found in this location",
      "skillsCount": "{{count}} skills",
      "searchPlaceholder": "Search skills...",
      "ideFilter": "IDE Source",
      "allIdes": "All IDEs",
      "openAll": "Open All",
      "openFolder": "Open Folder",
      "actions": {
        "refresh": "Refresh",
        "remove": "Remove",
        "open": "Open"
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
        "custom": "Custom"
      },
      "dialog": {
        "title": "Add Skill Path",
        "nameLabel": "Display Name",
        "namePlaceholder": "e.g., My Claude Code Skills",
        "pathLabel": "Path",
        "pathPlaceholder": "/path/to/skills",
        "ideSourceLabel": "IDE Source",
        "cancel": "Cancel",
        "add": "Add"
      },
      "discoveryDialog": {
        "title": "Discover IDE Skills",
        "description": "Scan your home directory for supported IDEs and import their skills.",
        "scanning": "Scanning for installed IDEs...",
        "foundIdes": "Found {{count}} IDE(s)",
        "foundWithSkills": "Found {{ides}} IDE(s) with {{skills}} skill(s)",
        "noIdesFound": "No supported IDEs found. Add skill paths manually.",
        "selectToImport": "Select IDEs to import skills from",
        "selectAll": "Select All",
        "deselectAll": "Deselect All",
        "importSelected": "Import Selected ({{count}})",
        "importButton": "Import Selected"
      },
      "detailDialog": {
        "title": "Skill Details",
        "path": "Path",
        "fileType": "File Type",
        "size": "Size",
        "modified": "Last Modified",
        "contentHash": "Content Hash",
        "description": "Description",
        "preview": "Preview",
        "openInEditor": "Open in Editor",
        "unknownSize": "Unknown",
        "unknownDate": "Unknown",
        "loadError": "Failed to load skill content"
      }
    },
    "command": {
      "title": "Commands",
      "comingSoon": "Command management feature coming soon..."
    }
  },
  "accessControl": {
    "pageTitle": "Access Control",
    "pageDescription": "Manage your authentication tokens for control panel and API access.",
    "userToken": {
      "title": "User Token (Control Panel)",
      "description": "This token protects access to the web control panel. Keep it secure and don't share it with API users.",
      "resetToken": "Reset User Token",
      "resetTitle": "Reset User Token",
      "resetConfirm": "Are you sure you want to reset your user token?",
      "resetPoints": {
        "new": "A new random token will be generated",
        "session": "Your current session will be updated automatically",
        "other": "Any other browsers/devices will need to log in again",
        "stop": "The old token will immediately stop working"
      },
      "resetWarning": "Make sure you have access to this device before resetting.",
      "resetCancel": "Cancel",
      "resetConfirmButton": "Reset",
      "resetSuccess": "User Token Reset Successfully",
      "resetSuccessMessage": "Your new user token has been generated and saved to your session.",
      "saved": "I've Saved My Token",
      "pullToken": "Pull latest token from server"
    },
    "modelToken": {
      "title": "Model Token (API Proxy)",
      "description": "Share this token with users who need API access to LLM endpoints.",
      "sharing": "Share the Model Token (above) with users who need to access the LLM API. Keep the User Token private.",
      "resetToken": "Reset Model Token",
      "resetTitle": "Reset Model Token",
      "resetConfirm": "Are you sure you want to reset the model token?",
      "resetPoints": {
        "new": "A new random token will be generated",
        "stop": "The old token will immediately stop working - all API clients will need to update"
      },
      "resetWarning": "Make sure all API clients have been notified before resetting.",
      "resetCancel": "Cancel",
      "resetConfirmButton": "Reset",
      "resetSuccess": "Model Token Reset Successfully",
      "resetSuccessMessage": "Your new model token has been generated. Make sure to update your API clients.",
      "saved": "I've Updated My Clients",
      "pullToken": "Pull latest token from server"
    },
    "securityInfo": {
      "title": "Token Security",
      "description": "Understanding the difference between User Token and Model Token:",
      "point1": "User Token: Protects the web control panel and administrative functions",
      "point2": "Model Token: Used by API clients to access LLM endpoints (/openai/*, /anthropic/*, /tingly/*)",
      "point3": "Share Model Token with API users, but never share User Token"
    },
    "copy": "Copy",
    "copied": "Copied!",
    "resetting": "Resetting...",
    "viewFullToken": "View Full Token",
    "fullTokenWarning": "Keep your token secure. Anyone with this token can access your control panel.",
    "secure": "Token is secure (randomly generated)",
    "warning": {
      "default": "You are using the default user token. This is a security risk!",
      "description": "The default token is publicly known and should be replaced with a secure random token.",
      "resetNow": "Reset Now"
    },
    "success": {
      "title": "Token Reset Successfully",
      "message": "Your new user token has been generated and saved to your session. Make sure to save it securely.",
      "saved": "I've Saved My Token"
    }
  },
  "dashboard": {
    "userUsage": {
      "title": "Team usage",
      "subtitle": "See how every registered user is consuming shared AI access.",
      "timeRange": "Time range",
      "viewMode": "View",
      "byAccount": "By account",
      "byModel": "By model",
      "byProvider": "By provider",
      "modelsUsed": "Models used",
      "acrossProviders_one": "Across {{count}} provider",
      "acrossProviders_other": "Across {{count}} providers",
      "providersUsed": "Providers used",
      "acrossModels_one": "Across {{count}} model",
      "acrossModels_other": "Across {{count}} models",
      "searchModels": "Search models",
      "searchProviders": "Search providers",
      "noModels": "No models match your search.",
      "noProviders": "No providers match your search.",
      "accountsUsingModel_one": "{{count}} account",
      "accountsUsingModel_other": "{{count}} accounts",
      "accountsUsingModelTitle": "Accounts using this model",
      "accountsUsingProviderTitle": "Accounts using this provider",
      "selectModel": "Select a model to see details.",
      "selectProvider": "Select a provider to see details.",
      "registeredUsers": "Registered users",
      "primaryAccount": "Primary account",
      "primary": "Primary",
      "globalToken": "Global model token",
      "globalTokenUsage": "Usage through the global model token",
      "activeUsers_one": "{{count}} active in this period",
      "activeUsers_other": "{{count}} active in this period",
      "totalTokens": "Total tokens",
      "acrossUsers_one": "Across {{count}} active user",
      "acrossUsers_other": "Across {{count}} active users",
      "tokenBreakdown": "Cache: {{cache}} · Input: {{input}} · Output: {{output}}",
      "cacheHitRate": "Cache hit rate",
      "cacheWriteIncluded": "incl. {{value}} written",
      "requests": "Requests",
      "averagePerUser": "{{value}} per active user",
      "errors": "Errors",
      "rowHint": "Select a user to inspect their usage mix.",
      "search": "Search users",
      "sortBy": "Sort users",
      "sortTokens": "Most tokens",
      "sortRequests": "Most requests",
      "sortErrors": "Highest errors",
      "sortName": "Name",
      "user": "User",
      "tokens": "Tokens",
      "total": "Total",
      "provider": "Provider",
      "model": "Model",
      "cacheWrite": "Cache Write",
      "cacheHit": "Cache Hit",
      "errorRate": "Error rate",
      "disabled": "Disabled",
      "enabled": "Enabled",
      "lastUsed": "Last used {{value}}",
      "neverUsed": "Never used",
      "joined": "Added {{value}}",
      "input": "Input",
      "output": "Output",
      "reasoning": "Reasoning",
      "cacheRead": "Cache Read",
      "allModels": "All models",
      "modelMix": "Where their tokens went",
      "noUsers": "No users match your search.",
      "noUsage": "No usage in this period",
      "noUsageHint": "The user remains listed because their access is registered.",
      "selectUser": "Select a user to see details.",
      "topAccounts": "Top accounts",
      "topModels": "Top models",
      "topProviders": "Top providers",
      "others": "Others",
      "noUsageHintShort": "Try a longer time range.",
      "noUsersHint": "Try a different search term or time range."
    },
    "agentNav": {
      "title": "Quick Start",
      "description": "Select agent to start"
    },
    "overview": {
      "title": "Usage Dashboard",
      "provider": "Provider",
      "model": "Model",
      "identity": "Identity",
      "allProviders": "All providers",
      "allModels": "All models",
      "allIdentities": "All identities",
      "sharingKeys": "Sharing Keys",
      "disabledSuffix": "(disabled)",
      "clearFilters": "Clear all filters",
      "auto": "Auto",
      "refreshData": "Refresh data",
      "mainAccount": "Main account",
      "unnamedSharingKey": "Unnamed sharing key",
      "range": {
        "today": "Today",
        "yesterday": "Yesterday",
        "3d": "3 days",
        "7d": "7 days",
        "30d": "30 days",
        "90d": "90 days"
      },
      "authType": {
        "apiKey": "API Key",
        "bearerToken": "Bearer Token",
        "basicAuth": "Basic Auth",
        "vmodel": "Virtual Model",
        "other": "Other"
      },
      "statCards": {
        "totalRequests": "Total Requests",
        "totalTokens": "Total Tokens",
        "cacheHitRate": "Cache Hit Rate",
        "errorRate": "Error Rate",
        "streamedRate": "Streamed Rate",
        "tokenBreakdown": "Input: {{input}} + Cache: {{cache}}\nOutput: {{output}}",
        "errors": "{{count}} errors",
        "streamed": "{{count}} streamed",
        "cacheRead": "read",
        "cacheWrite": "written"
      },
      "viewModes": {
        "summary": "Summary",
        "byRequest": "By Request",
        "activity": "Activity"
      }
    },
    "requestsView": {
      "title": "Requests",
      "total": "{{total}} total",
      "all": "All",
      "success": "Success",
      "error": "Error",
      "colTime": "Time",
      "colModel": "Model",
      "colScenario": "Scenario",
      "colCacheRead": "Cache Read",
      "colCacheWrite": "Cache Write",
      "colInput": "Input",
      "colOutput": "Output",
      "colReasoning": "Reasoning",
      "colLatency": "Latency",
      "colTTFT": "TTFT",
      "colTPS": "TPS",
      "colStatus": "Status",
      "colStream": "Stream",
      "empty": "No requests found",
      "emptyHint": "Try changing the status filter",
      "ok": "OK",
      "err": "ERR",
      "streamed": "Streamed",
      "tpsFormula": "{{count}} decode intervals / {{ms}}ms after TTFT"
    },
    "performance": {
      "title": "Response Performance",
      "latency": "Latency",
      "sampleCount": "n={{n}}"
    },
    "heatmap": {
      "title": "Token Activity",
      "lastMonths": "Last 12 months",
      "fixedWindow": "Fixed {{days}}-day window — not affected by the range selector (the Provider / Model / Identity filters still apply).",
      "empty": "No activity in the last {{days}} days.",
      "totalTokens": "{{tokens}} total tokens",
      "input": "Input",
      "cache": "Cache",
      "output": "Output",
      "tokens": "tokens",
      "activeDays": "active days",
      "longestStreak": "longest streak",
      "maxDay": "max/day",
      "less": "Less",
      "more": "More",
      "weekdayMon": "Mon",
      "weekdaySun": "Sun"
    },
    "usageByModel": {
      "title": "Usage by Model",
      "provider": "Provider",
      "model": "Model",
      "empty": "No usage data available",
      "emptyHint": "Select a different time range or check back later"
    },
    "metricLabels": {
      "requests": "Requests",
      "total": "Total",
      "cacheRead": "Cache Read",
      "cacheWrite": "Cache Write",
      "cacheHit": "Cache Hit",
      "input": "Input Tokens",
      "output": "Output Tokens",
      "reasoning": "Reasoning Tokens",
      "errorRate": "Error Rate"
    },
    "chart": {
      "dailyTitle": "Token Usage Over Time (Daily)",
      "hourlyTitle": "Token Usage Over Time (5-Min)",
      "cacheRead": "Cache Read",
      "input": "Input",
      "output": "Output",
      "inputTokens": "Input Tokens",
      "outputTokens": "Output Tokens",
      "cacheRatio": "Cache Ratio:",
      "inclWritten": "(incl. {{n}} written)",
      "noData": "No data available",
      "noDataHint": "Select a different time range or check back later"
    }
  },
  "mcp": {
    "pageTitle": "MCP Tools",
    "info": "Configure MCP (Model Context Protocol) tools to enable web search and web fetch capabilities. The MCP server runs as a local stdio subprocess or connects to a remote HTTP endpoint.",
    "connection": {
      "title": "Connection Settings",
      "endpoint": "MCP Server Endpoint",
      "endpointPlaceholder": "http://localhost:3000",
      "endpointHelp": "HTTP endpoint for the MCP server (e.g., npx @modelcontextprotocol/server-filesystem)",
      "command": "Command",
      "commandPlaceholder": "python3",
      "scriptPath": "Script Path",
      "scriptPathPlaceholder": "builtin",
      "scriptPathHelp": "Path to the MCP server script (or 'builtin' for Go tools)",
      "workingDir": "Working directory",
      "timeout": "Request Timeout (seconds)",
      "timeoutHelp": "Timeout for MCP tool calls",
      "transportHttp": "Use HTTP Transport (uncheck for stdio)",
      "transportStdio": "Use Stdio Transport"
    },
    "tools": {
      "title": "Tool Configuration",
      "description": "Select which MCP tools to enable:",
      "webSearch": "Web Search",
      "webSearchDesc": "Search web pages with Serper API. Requires SERPER_API_KEY environment variable.",
      "webFetch": "Web Fetch",
      "webFetchDesc": "Fetch and convert URLs to markdown via Jina Reader. Optional JINA_API_KEY."
    },
    "proxy": {
      "title": "Proxy Settings",
      "useGlobal": "Use Global Proxy Configuration",
      "useGlobalHelp": "When enabled, the MCP server will inherit HTTP_PROXY, HTTPS_PROXY, and NO_PROXY environment variables from the system."
    },
    "actions": {
      "save": "Save Configuration",
      "reset": "Reset to Default",
      "reload": "Reload",
      "docs": "MCP Protocol Docs",
      "saving": "Saving...",
      "savedSuccess": "MCP configuration saved successfully",
      "savedError": "Failed to save MCP configuration"
    },
    "currentConfig": "Current Configuration"
  },
  "onboarding": {
    "title": "Welcome to Tingly Box",
    "subtitle": "Add your first AI provider to get started. Browse the catalog, or use Paste & detect with a config snippet — we’ll figure out the rest.",
    "hint": "Detection runs locally in the box; pasted text is not sent to any third party.",
    "browse": {
      "searchPlaceholder": "Search providers",
      "empty": "No providers match your search.",
      "selectProvider": "Select this provider",
      "customProvider": "Custom Provider",
      "customProviderHint": "Bring your own endpoint",
      "section": {
        "global": "Global",
        "china": "China (Mainland)",
        "custom": "Custom"
      }
    },
    "paste": {
      "detectButton": "Detect",
      "manualFill": "Fill in manually",
      "noMatch": "No URL or API key detected. You can fill in the form manually.",
      "pickHint": "Pick the URL and the token you want to use, then click \"Use selected\".",
      "urlsTitle": "Detected URLs",
      "tokensTitle": "Detected tokens",
      "noURL": "No URLs detected.",
      "noToken": "No tokens detected.",
      "useSelected": "Use selected"
    },
    "quickLinks": "Quick Links",
    "goToDashboard": "Dashboard",
    "goToHelp": "Help & Docs",
    "dialog": {
      "goToAgents": "Go to Agents",
      "message": "Your AI provider has been added successfully. Would you like to go to the agents page to start using it?",
      "stay": "Stay Here",
      "title": "Provider Added"
    },
    "success": "Provider added successfully! You can now create scenarios."
  },
  "imageGenQuickStart": {
    "title": "Image API Quick Start",
    "closeAriaLabel": "Close quick start",
    "description": "Call the image generation or edit endpoint, then decode the base64 response into an image file. The model token is available from GET /api/v1/token."
  },
  "templatePage": {
    "noProviders": {
      "title": "No Providers Configured",
      "description": "Add an API key provider to start routing requests",
      "action": "Get started"
    },
    "noRules": "No rules yet. Click \"New Rule\" to add one.",
    "copiedToClipboard": "{{label}} copied to clipboard!"
  },
  "sharingKeys": {
    "title": "Sharing Keys",
    "titleForTeam": "Sharing Keys · {{team}}",
    "createToken": "Create Token",
    "createDialogTitle": "Create Sharing Key",
    "displayName": "Display Name",
    "displayNamePlaceholder": "e.g., Team Alpha Key",
    "displayNameHelper": "A descriptive name for this sharing key",
    "deleteToken": "Delete Token",
    "deleteConfirm": "Are you sure you want to delete the token \"{{name}}\"? This action cannot be undone.",
    "nameRequired": "Display Name is required",
    "createSuccess": "Token created successfully",
    "createFailed": "Failed to create token",
    "deleteSuccess": "Token deleted successfully",
    "deleteFailed": "Failed to delete token",
    "copiedToClipboard": "Token copied to clipboard",
    "disabled": "Token disabled",
    "enabled": "Token enabled",
    "updateFailed": "Failed to update token",
    "moveToken": "Move Key",
    "destinationTeam": "Destination team",
    "noDestinationTeam": "No other active team",
    "moveHelper": "Move {{name}} without rotating its key.",
    "moveSuccess": "Key moved to the new team",
    "moveFailed": "Failed to move key"
  },
  "teams": {
    "accessTitle": "Team Access",
    "keyScopeSummary": "Sharing keys for {{team}} ({{slug}}) work only with /tingly/team and /tingly/team/v1. They cannot access other Teams, scenario endpoints, or management APIs.",
    "keyScopeInfoLabel": "Sharing key access scope",
    "editTeam": "Team settings",
    "name": "Team name",
    "inactive": "Inactive",
    "disabledHint": "This team is disabled. Its sharing keys cannot access model endpoints until the team is enabled.",
    "enableTeam": "Enable team",
    "disableTeam": "Disable team",
    "deleteTeam": "Delete team",
    "deleteConfirm": "Delete {{team}}? Move or delete all of its sharing keys first.",
    "loadFailed": "Failed to load teams",
    "saveFailed": "Failed to save team",
    "createSuccess": "Team created",
    "updateSuccess": "Team updated",
    "deleteSuccess": "Team deleted",
    "deleteFailed": "Team cannot be deleted while it owns sharing keys",
    "enabled": "Team enabled",
    "disabled": "Team disabled"
  },
  "context1M": {
    "enabledTitle": "1M Context Window Enabled",
    "disabledTitle": "1M Context Window Disabled",
    "enabledBody": "Model names have been updated with [1m] suffix for extended context support.",
    "disabledBody": "Model names have been updated to remove [1m] suffix.",
    "requiresApplyHint": "Please apply the configuration below and restart {{client}} for changes to take effect.",
    "restartOnlyHint": "Please restart {{client}} and re-select the model for changes to take effect."
  },
  "playground": {
    "imageTitle": "Image Playground",
    "noImageModels": "Add an image generation model rule below to start generating images.",
    "model": "Model",
    "prompt": "Prompt",
    "promptPlaceholder": "Describe the image you want to generate…",
    "editPromptPlaceholder": "Describe the change you want to make…",
    "size": "Size",
    "quality": "Quality",
    "count": "N",
    "generate": "Generate",
    "generateAnother": "Generate another · {{count}} running",
    "generatingNew": "Generating new images…",
    "modeGenerate": "Generate",
    "modeEdit": "Edit",
    "editBadge": "Edited",
    "originalBadge": "Original",
    "edit": "Edit Image",
    "editAnother": "Edit another · {{count}} running",
    "editingNew": "Editing images…",
    "referenceImages": "Reference images",
    "referenceHint": "Up to {{max}} images · PNG, JPEG, or WebP",
    "addReferenceImage": "Add image",
    "dropReferenceImage": "Drop images here, click to browse, or paste",
    "removeReferenceImage": "Remove reference image {{number}}",
    "noReferenceImages": "Add at least one image to edit",
    "useAsReference": "Edit this image",
    "referenceLoadFailed": "Could not use this image as a reference",
    "referenceThumbAlt": "Reference image {{number}}",
    "viewSourceImage": "View original image {{number}}",
    "previewEmpty": "Your generated images will appear here",
    "previewHint": "Each generation will be kept for this session.",
    "sessionOutputs": "Session outputs",
    "runCountOne": "1 generation",
    "runCount": "{{count}} generations",
    "openResult": "Open generated image {{number}}",
    "resultAlt": "Generated image {{number}}",
    "emptyResult": "No image returned",
    "requestFailed": "Request failed",
    "closePreview": "Close image preview",
    "copyPrompt": "Copy prompt",
    "promptCopied": "Copied",
    "download": "Download",
    "downloadFailed": "Could not download this image",
    "slice": {
      "action": "Split into tiles",
      "title": "Split into tiles",
      "close": "Close slicer",
      "hint": "Cuts an evenly divided grid — a sticker sheet, a contact sheet, a spritesheet. Adjust the margin and gap until the outlines sit on the artwork, then click a tile to leave it out.",
      "rows": "Rows",
      "cols": "Columns",
      "margin": "Outer margin",
      "gutter": "Gap between tiles",
      "exportSize": "Output size",
      "exportOriginal": "Original",
      "exportOriginalWithSize": "Original · {{width}}×{{height}} px",
      "selectedCount": "{{selected}} of {{total}} tiles",
      "downloadOne": "Download this tile",
      "downloadZip": "Download {{count}} PNGs (ZIP)",
      "cancel": "Cancel",
      "tile": "Tile {{number}}",
      "sheetAlt": "Image being sliced",
      "failed": "Could not slice this image",
      "loadFailed": "This image could not be read for slicing. Providers that return a remote URL may block browser access to their pixels."
    }
  },
  "agentSetup": {
    "quickStart": "Quick Start",
    "done": "Done",
    "expand": "Expand",
    "collapse": "Collapse",
    "resetProgress": "Reset progress",
    "hint": {
      "connectProvider": "Connect an AI provider to get started",
      "selectModel": "Choose a model to continue",
      "install": "Install {{agent}} on your machine",
      "apply": "One-click {{action}} to finish"
    },
    "provider": {
      "label": "Connect AI Provider",
      "countOne": "1 provider",
      "count": "{{count}} providers",
      "connect": "Connect AI",
      "addMore": "+ Connect",
      "tooltip": "Connect an AI provider (e.g. OpenAI, Anthropic, DeepSeek) to start using {{agent}}."
    },
    "model": {
      "label": "Select a Model",
      "configured": "Configured",
      "skipped": "Skipped",
      "skip": "Skip",
      "choose": "Choose Model",
      "change": "Change",
      "tooltip": "Choose which model {{agent}} will use in the Model Rules section below."
    },
    "install": {
      "label": "Install {{agent}}",
      "installed": "Installed",
      "confirm": "I've installed it",
      "confirmTooltip": "Run the install command below, then confirm here once {{agent}} is installed.",
      "description": "Install {{agent}} on your local machine — copy and run the command below.",
      "npmOfficial": "npm official",
      "npmMirror": "npm mirror",
      "copy": "Copy",
      "copied": "Copied!"
    },
    "apply": {
      "label": "Auto Config",
      "applied": "Applied",
      "button": "Auto Config",
      "success": "Config applied!",
      "viewConfig": "Config",
      "viewConfigAdvanced": "{{label}} (Advanced)",
      "skip": "Skip",
      "tooltip": "One click to write the proxy configuration to {{agent}}'s settings file.",
      "failed": "Apply failed"
    }
  },
  "scenarioPage": {
    "config": "Config",
    "autoConfig": "Auto Config",
    "quickStart": "Quick Start",
    "unknownError": "Unknown error",
    "codex": {
      "configTitle": "Codex Configuration",
      "applyFailed": "Failed to apply Codex config"
    },
    "opencode": {
      "configTitle": "OpenCode Configuration",
      "applyFailed": "Failed to apply OpenCode config",
      "previewFailed": "Failed to load config preview: {{reason}}",
      "previewFailedGeneric": "Failed to load config preview"
    },
    "sharingKeys": "Sharing Keys",
    "modelRules": "Model Rules",
    "embedModelRules": "Embedding Model Rules",
    "imageGenModelRules": "Image Model Rules",
    "tooltip": {
      "claude_code": "AI-powered CLI development agent for implementation, testing, and git operations",
      "claude_desktop": "Route Claude Desktop's third-party inference through your configured providers",
      "codex": "OpenAI Codex AI coding assistant with Tingly Box proxy",
      "opencode": "OpenCode AI development environment with BYOK support",
      "xcode": "Xcode AI coding assistant through Tingly Box proxy for iOS/macOS development",
      "vscode": "Bring Your Own Key: Use your own API keys with VS Code Copilot through Tingly Box proxy",
      "cursor": "Cursor AI code editor through Tingly Box proxy, with Cursor compatibility handling enabled by default. Cursor calls this URL from its own cloud, so it must be a publicly reachable HTTPS address — not localhost.",
      "pi": "Pi coding agent through Tingly Box proxy",
      "dsh": "DeepSeek Harness (dsh) agent harness through Tingly Box proxy",
      "imagegen": "AI-powered image generation and editing through Tingly Box proxy with multiple model support"
    },
    "vscode": {
      "installDescription": "Install the Tingly Box extension from VS Code or the Marketplace.",
      "installInVSCode": "Install in VS Code",
      "viewMarketplace": "View Marketplace",
      "applyStepLabel": "Follow VS Code Guide",
      "applyStepDescription": "Open the Tingly Box extension in VS Code and follow its built-in setup guide.",
      "openGuide": "Open Guide",
      "configTitle": "Configure VS Code",
      "modalDescription": "Install the Tingly Box extension, then follow the setup guide inside VS Code. The extension handles the required endpoint and API key configuration for you.",
    },
    "pi": {
      "installDescription": "Install pi on your local machine — see the repo for setup instructions.",
      "viewRepo": "View on GitHub",
      "applyStepLabel": "Configure Pi",
      "applyStepDescription": "Point pi's provider base URL and API key at the values above — see pi's docs for the exact config option names.",
      "openGuide": "View Config",
      "configTitle": "Configure Pi",
      "modalDescription": "Point pi's provider base URL and API key at the values shown on this page, then check the repo below for pi's exact configuration options."
    },
    "dsh": {
      "openWebUi": "Open Web UI",
      "installDescription": "Run DeepSeek Harness locally with `npx @deepseek-ai/dsh web` (requires Node.js) — the Web UI serves at http://127.0.0.1:3080. Tingly Box does not launch dsh for you; use the button below to jump to it once it's running. Note: dsh is in developer preview and its config may change.",
      "viewRepo": "View on GitHub",
      "applyStepLabel": "Configure DSH",
      "applyStepDescription": "Point dsh's model provider plugin base URL and API key at the values above — see the dsh docs for the exact plugin config.",
      "openGuide": "View Config",
      "configTitle": "Configure DSH",
      "modalDescription": "Point dsh's model provider plugin at the base URL and API key shown on this page, then check the repo below for dsh's plugin configuration options.",
      "applyFailed": "Failed to apply configuration"
    }
  },
  "scenarioOverview": {
    "title": "Agents",
    "subtitle": "Pick a scenario to configure. Hide the ones you don't use to keep the sidebar tidy.",
    "showInSidebar": "Show in sidebar",
    "hideFromSidebar": "Hide from sidebar",
    "hidden": "Hidden",
    "notConfigured": "Not configured",
    "ruleCountOne": "1 rule",
    "ruleCount": "{{count}} rules",
    "editTooltip": "Manage visible agents",
    // Full product names for overview cards where the short nav label is ambiguous.
    "titles": {
      "dsh": "DeepSeek Harness",
    },
    "descriptions": {
      "claude_code": "Route Claude Code with custom profiles and per-task models.",
      "claude_desktop": "Use your own providers as Claude Desktop's third-party inference gateway.",
      "codex": "Configure Codex CLI through your provider keys.",
      "opencode": "Open-source coding agent powered by your provider.",
      "xcode": "Bring your model into Xcode's coding intelligence.",
      "vscode": "Power VS Code Copilot Chat through Tingly Box.",
      "cursor": "Bring your model into Cursor with compatibility handling built in.",
      "pi": "Route the pi coding agent through your provider.",
      "dsh": "Route the dsh coding agent through your provider.",
      "openai": "Drop-in OpenAI-compatible SDK endpoint.",
      "anthropic": "Drop-in Anthropic-compatible SDK endpoint.",
      "embed": "Route embedding requests to your provider.",
      "imagegen": "Route image generation and editing through Tingly Box.",
      "custom": "Bring your own request model name — a generic catch-all scenario. Hidden by default.",
      "team": "Shared central model deployment for your whole team. Hidden by default."
    }
  },
  "bots": {
    "activeCount": "active {{active}} / {{total}}",
    "card": {
      "notifyChip": "Notify",
      "remoteAgentChip": "Remote"
    },
    "overview": {
      "allConnections": "All connections",
      "allPlatforms": "All",
      "connectBot": "Connect a bot",
      "emptyDescription": "Connect a bot to drive Claude Code from chat (Remote) or deliver notifications (Notify).",
      "emptyTitle": "No bots connected yet",
      "pageSubtitle": "Connect and maintain the messaging accounts used by Remote Control and IM Notify.",
      "platformTitle": "{{platform}} Bots",
      "subtitle_one": "{{count}} bot connected",
      "subtitle_other": "{{count}} bots connected",
      "title": "Bots"
    },
    "table": {
      "access": "Access",
      "actions": "Actions",
      "botId": "Bot UUID",
      "capabilities": "Capabilities",
      "chatIdCopied": "Chat ID copied",
      "chatIdCopyFailed": "Copy failed — check clipboard permissions",
      "chatsTitle": "Reachable chats — copy the Chat ID for notify/interact",
      "copyChatId": "Copy Chat ID",
      "copyUuid": "Copy Bot UUID",
      "itsPlatform": "its platform",
      "name": "Name",
      "noChats": "No chats yet. Send any message to this bot on {{platform}} and its Chat ID will appear here.",
      "noChatsPairFirst": "No chats yet. Pair this bot (see Pairing), then send it a message on {{platform}} — its Chat ID appears here.",
      "notRunning": "This bot isn’t running. Start it, then send it a message on {{platform}} — its Chat ID appears here.",
      "paired": "paired",
      "platform": "Platform",
      "showChats": "Show reachable chats (copy Chat ID)",
      "status": "Status",
      "uuidCopied": "Bot UUID copied",
      "uuidCopyFailed": "Copy failed — check clipboard permissions"
    },
    "toggle": {
      "disabled": "Bot disabled",
      "enabled": "Bot enabled",
      "failed": "Failed to toggle bot: {{error}}",
      "failedGeneric": "Failed to toggle bot"
    }
  },
  "nodes": {
    "imBotUUID": "Bot UUID",
    "platformBotUUID": "Bot UUID",
    "platformHint": "the IM platform this chain runs on"
  },
  "notify": {
    "chat": {
      "copied": "Target UUID copied",
      "copyFailed": "Copy failed — check clipboard permissions",
      "deleted": "Chat deleted"
    },
    "emptyDescription": "Connect a bot on the Bots page first, then come back here to send it notifications.",
    "emptyPlatformDescription": "Pick another platform above, or add one on the Bots page.",
    "emptyPlatformTitle": "No {{platform}} bots",
    "emptyTitle": "No bots connected yet",
    "group": {
      "allowAndTest": "Allow Notify & Test",
      "copyChatId": "Copy internal target UUID",
      "custom": "Custom",
      "customHint": "Compose a custom message (free-form)",
      "deleteChat": "Delete this Direct Chat record",
      "deleteChatBody": "Its pairing, whitelist, and project binding are removed. If it messages the bot again it re-registers as a brand-new chat (re-pairing required when pairing is enforced). Session history is untouched. To block it instead, use Disable.",
      "deleteChatTitle": "Delete this chat?",
      "disableChat": "Disable — silently drop its messages",
      "disableHint": "Disable Notify for this bot",
      "disabledBody": "Bot is off — enable it to see and send to its reachable chats.",
      "disabledChat": "disabled",
      "empty": "No chats yet. Send any message to this bot on {{platform}} and its Chat ID appears here.",
      "emptyPairFirst": "No chats yet. Pair this bot, then send it a message on {{platform}} — its Chat ID appears here.",
      "enableChat": "Enable — accept its messages again",
      "enableHint": "Enable Notify. The bot starts automatically if needed.",
      "hideDisabled": "Hide disabled",
      "noTargets": "No observed targets",
      "paired": "paired",
      "refresh": "Refresh reachable chats",
      "showDisabled": "Show disabled ({{count}})",
      "targetCount": "{{direct}} direct · {{groups}} groups"
    },
    "guide": {
      "action": "API guide",
      "auth": {
        "body": "Any integration can drive a bot’s chat. Auth reuses your existing operator user token (the same one this web UI uses) as a Bearer header — no new credential to mint. Interactive prompts (/interact) and one-way notifications (/notify) are separate URLs, so the request shape is the mode.",
        "title": "1. Authenticate with your user token"
      },
      "chatid": {
        "body": "Each Chat node on this page shows the real platform Chat ID for recognition; its tooltip also shows the stable internal Target UUID required by the API. Use the copy action beside the node, or send a test directly from the probe bench.",
        "title": "3. Copy the target UUID from Delivery targets"
      },
      "description": "Authentication, request examples, and target IDs",
      "send": {
        "body": "POST to /api/v1/bots/{bot}/notify with the bot UUID in the path. A 200 means delivered.",
        "json": "Request body:",
        "title": "2. Send a one-way notification"
      },
      "title": "IM Notify API Guide"
    },
    "loadFailed": "Failed to load Notify targets",
    "probe": {
      "showRaw": "Show raw payload"
    },
    "subtitle": "Authorize a target, send through the production path, and see whether delivery worked.",
    "target": {
      "blocked": "Target blocked",
      "direct": "Direct",
      "group": "Group",
      "unblocked": "Target unblocked"
    },
    "targetsSubtitle": "Direct Chats and Groups observed by your connected bots.",
    "targetsTitle": "Delivery targets",
    "test": {
      "bodyField": "Body (markdown)",
      "level": "Level",
      "paired": "paired",
      "pickKnownChat": "Select a discovered chat first",
      "resetMarkdown": "Reset to markdown sample",
      "send": "Send",
      "sendFailed": "Send failed: {{error}}",
      "sent": "Notification sent",
      "target": "Target",
      "targetPlaceholder": "No authorized targets yet",
      "title": "Send a test notification",
      "titleField": "Title (optional)"
    },
    "title": "IM Notify",
    "toggleFailed": "Failed to update Notify"
  },
  "remoteAgent": {
    "ccProfile": {
      "chip": "Profile",
      "default": "Default",
      "defaultSecondary": "Main claude_code scenario",
      "defaultTooltip": "Uses the main claude_code scenario. Click to route @cc through a Claude Code profile.",
      "dialogSubtitle": "Remote @cc sessions route through the selected profile — its rules, model mapping, and settings overrides.",
      "dialogTitle": "Claude Code Profile for @cc",
      "empty": "No Claude Code profiles yet. Create one on the Claude Code scenario page first.",
      "missingTooltip": "Profile \"{{id}}\" no longer exists — @cc falls back to the default claude_code scenario. Click to pick another.",
      "profileTooltip": "Claude Code profile",
      "scenario": "Scenario",
      "separate": "separate",
      "unified": "unified"
    },
    "emptyDescription": "Remote Control runs on top of a bot. Create a {{platform}} bot connection first, then mount it here.",
    "emptyTitle": "No {{platform}} Bots Yet",
    "notify": {
      "ccProfileUpdateFailed": "Failed to update Claude Code profile",
      "ccProfileUpdated": "Claude Code profile updated"
    },
    "pageSubtitle": "Choose who can control each bot and where chat commands route.",
    "pageTitle": "Remote Control",
    "routesSubtitle": "Access → Bot → Agent. Click a node to change that part of the route.",
    "routesTitle": "{{platform}} routes"
  },
  "remoteControl": {
    "authForm": {
      "botId": "Bot ID:",
      "manualOption": "Enter manually",
      "noFieldsDefined": "No auth fields defined for this platform.",
      "oauthIntro": "Enter your App credentials from the developer console.",
      "rebindAccount": "Re-bind Account",
      "scanQrOption": "One-click (scan QR)",
      "storedSecurely": "This will be stored securely",
      "userId": "User ID:",
      "weixinAccountBound": "Weixin account bound",
      "weixinBindingTitle": "Weixin QR Code Binding"
    },
    "bots": {
      "addBot": "Connect a bot",
      "addPlatformBot": "Add {{platform}} Bot",
      "configuredCount_one": "{{count}} bot configured",
      "configuredCount_other": "{{count}} bots configured",
      "emptyDescription": "Configure {{platform}} bots to enable remote-control chat integration.",
      "emptyTitle": "No {{platform}} Bots Configured"
    },
    "card": {
      "delete": "Delete",
      "deleteConfirm": "Are you sure you want to delete \"{{name}}\"? This action cannot be undone.",
      "deleteTitle": "Delete Bot Configuration",
      "disableBot": "Disable Bot",
      "edit": "Edit",
      "enableBot": "Enable Bot",
      "enableToRestart": "Enable bot to restart",
      "noModelConfigured": "No model configured - click to select a model",
      "remoteAgentOff": "Turn off Remote Control. The bot remains available to other capabilities.",
      "remoteAgentOn": "Turn on Remote Control. The bot starts automatically if needed.",
      "restartBot": "Restart Bot"
    },
    "dialog": {
      "addSubtitle": "Choose a messaging platform and provide the credentials needed to connect it.",
      "addTitle": "Connect a bot",
      "advancedAgentPolicy": "Advanced agent policy",
      "advancedAgentPolicyHelper": "Limits what an authorized controller may execute; it does not grant access.",
      "alias": "Alias",
      "aliasHelper": "Optional: a friendly name for this bot configuration.",
      "bashAllowlist": "Bash Allowlist",
      "bashAllowlistHelper": "Allowlisted /bash subcommands. Default: cd, ls, pwd.",
      "cancel": "Cancel",
      "connect": "Connect bot",
      "editSubtitle": "Update this connection. Capabilities and people are managed from Access.",
      "editTitle": "Edit bot",
      "platform": "Platform",
      "proxyUrl": "Proxy URL",
      "proxyUrlHelper": "Optional HTTP/HTTPS proxy for bot API requests.",
      "save": "Save changes",
      "saving": "Saving..."
    },
    "feishuQr": {
      "createdBody": "Credentials were saved automatically. Your bot is ready.",
      "createdTitle": "{{label}} app created!",
      "deniedWarning": "Authorization was declined in {{label}}.",
      "errorFallback": "An error occurred during {{label}} registration",
      "expiredWarning": "The QR code expired. Please get a new one.",
      "getNewQr": "Get New QR Code",
      "headerLabel": "{{label}} One-Click App Creation",
      "preparing": "Preparing one-click {{label}} registration...",
      "refreshQr": "Refresh QR Code",
      "registrationFailed": "Registration failed",
      "retry": "Retry",
      "scanTitle": "Scan to create your {{label}} app",
      "startFailed": "Failed to start one-click registration",
      "statusFailed": "Failed to check registration status",
      "step1": "1. Open {{label}} on your phone and scan the QR code",
      "step2": "2. Confirm authorization — the app, permissions and events are created for you",
      "tryAgain": "Try Again",
      "uuidRequired": "Bot UUID is required"
    },
    "guide": {
      "action": "Setup guide",
      "collapsedHint": "Connection steps, credentials, and examples",
      "drawerHint": "Connection steps, credentials, and examples",
      "showLess": "Show Less",
      "showMore": "Show More",
      "title": "{{platform}} Setup Guide"
    },
    "guides": {
      "comingSoon": "{{platform}} bot integration is currently under development. Stay tuned for updates!",
      "dingtalk": {
        "description": "Enterprise communication and collaboration",
        "step1Config": "Configuration:",
        "step1CreateApp": "Create a new app - Add Robot capability",
        "step1GetKeys": "Get AppKey (Client ID) and AppSecret (Client Secret) from \"Credentials\"",
        "step1LinkLabel": "DingTalk Open Platform",
        "step1Permissions": "Permissions: Add necessary permissions for sending messages",
        "step1Publish": "Publish the app",
        "step1StreamMode": "Toggle",
        "step1Title": "1. Create a DingTalk bot",
        "step1Visit": "Visit",
        "step2Text": "Select \"Connect a bot\" and enter the App Key and App Secret to create your bot.",
        "tip": "Tip: DingTalk uses Stream Mode - no public IP required. Configure traffic proxy as needed."
      },
      "discord": {
        "description": "Voice, video, and text communication"
      },
      "feishu": {
        "description": "Enterprise collaboration platform",
        "step1TextAfter": ", and scan the QR code with the Feishu mobile app. The app, permissions and events are created automatically and the credentials are saved for you.",
        "step2LinkLabel": "Feishu one-click app creation",
        "step2Open": "Open",
        "tip": "Tip: Feishu uses WebSocket - no public IP needed. Configure traffic proxy as needed."
      },
      "feishuFamily": {
        "oneClickOption": "One-click (scan QR)",
        "step1TextBefore": "Select \"Connect a bot\", then choose",
        "step1Title": "1. Scan to create (recommended)",
        "step2Configure": "Enter an app name and confirm — bot capability, permissions, events and Long Connection mode are pre-configured for you",
        "step2CopyAfterBefore": ", then enter them via \"Add Bot\" →",
        "step2CopyBefore": "Copy the generated",
        "step2LogIn": "and log in",
        "step2Title": "2. Or create manually"
      },
      "lark": {
        "description": "Global version of Feishu",
        "step1TextAfter": ", and scan the QR code with the Lark mobile app. The app, permissions and events are created automatically and the credentials are saved for you.",
        "step2LinkLabel": "Lark one-click app creation",
        "tip": "Tip: Lark uses WebSocket - no public IP needed. Configure traffic proxy as needed."
      },
      "qq": {
        "description": "Tencent instant messaging platform"
      },
      "slack": {
        "description": "Business communication platform"
      },
      "telegram": {
        "description": "Popular cloud-based instant messaging service",
        "step1Open": "Open Telegram, search",
        "step1Send": "Send",
        "step1SendTail": ", follow the prompts, and copy the token",
        "step1Title": "1. Create a bot",
        "step2Text": "Select \"Connect a bot\" and paste the token to create your bot.",
        "step2Title": "2. Add bot",
        "tip": "Tip: Configure traffic proxy as needed for network access."
      },
      "wecom": {
        "createBot": "Create Bot",
        "createManually": "Create Manually",
        "description": "Enterprise Weixin communication platform",
        "step1AndClick": "and click",
        "step1GoTo": "Go to",
        "step1LinkLabel": "WeCom Admin → AI Assistant",
        "step1Title": "1. Open WeCom Admin Console",
        "step2LinkLabel": "Create via API Mode",
        "step2TextBefore": "Scroll to the bottom of the page and click",
        "step2Title": "2. Create via API mode",
        "step3ApiConfig": "API Config:",
        "step3ApiConfigTextBefore": "Under Connection Method, select",
        "step3ClickToRetrieve": "Click to Retrieve",
        "step3LongConnection": "Long Connection",
        "step3Permissions": "Permissions:",
        "step3PermissionsTextBefore": "Configure as needed, then click",
        "step3Save": "Save",
        "step3SecretAfter": "— save the",
        "step3SecretBefore": "In the Secret section, click",
        "step3Title": "3. Configure the bot",
        "step3VisibleScope": "Visible Scope:",
        "step3VisibleScopeText": "Set who can use the bot",
        "step4Text": "Select \"Connect a bot\" and enter the Bot ID and Secret to connect.",
        "step4Title": "4. Add bot",
        "tip": "Tip: WeCom AI Bot uses WebSocket long connection — no public IP required."
      },
      "weixin": {
        "betaLabel": "Beta:",
        "betaText": "Weixin integration is in beta. Please provide feedback for any issues.",
        "description": "China\\",
        "step1TextAfter": "installed on your device.",
        "step1TextBefore": "Make sure you have the latest version of",
        "step1Title": "1. Install latest Weixin",
        "step2Text": "Select \"Connect a bot\" and scan the QR code with Weixin to bind your account."
      }
    },
    "modelDialog": {
      "title": "Configure SmartGuide Model"
    },
    "notify": {
      "botCreated": "Bot created successfully.",
      "botDeleted": "Bot deleted successfully",
      "botRestarted": "Bot restarted",
      "botUpdated": "Bot updated successfully.",
      "deleteFailed": "Failed to delete bot: {{error}}",
      "deleteFailedGeneric": "Failed to delete bot",
      "loadFailed": "Failed to load bot settings",
      "missingFields": "Missing required fields: {{fields}}",
      "modelUpdateFailed": "Failed to update bot configuration",
      "modelUpdated": "Bot model configuration updated",
      "qrBindRequired": "Please complete WeChat QR binding before saving",
      "remoteAgentOff": "Remote Control disabled",
      "remoteAgentOn": "Remote Control enabled",
      "restartFailed": "Failed to restart bot: {{error}}",
      "restartFailedGeneric": "Failed to restart bot",
      "saveFailed": "Failed to save bot settings",
      "toggleFailedGeneric": "Failed to toggle bot",
      "unboundReuse": "Found an unbound bot, reusing it for QR binding",
      "unknownPlatform": "Unknown platform: {{platform}}"
    },
    "pairing": {
      "copied": "Pairing command copied",
      "copy": "Copy",
      "copyFailed": "Copy failed — check clipboard permissions",
      "expired": "expired",
      "expiresIn": "expires in {{time}}",
      "fetchFailed": "Failed to fetch pairing code",
      "hide": "Hide",
      "noActiveCode": "No active code — bot may be stopped, or the code was already consumed. Click Rotate to mint a new one.",
      "reveal": "Reveal",
      "rotateFailed": "Rotate failed",
      "rotateTooltip": "Rotate (invalidates current code)",
      "rotated": "Pairing code rotated"
    },
    "platformSelector": {
      "empty": "No platforms available. Make sure the remote-control service is running.",
      "loading": "Loading platforms..."
    },
    "weixinQr": {
      "errorFallback": "An error occurred during Weixin binding",
      "expiredWarning": "QR code expired. Please refresh to get a new one.",
      "getNewQr": "Get New QR Code",
      "headerLabel": "Weixin QR Code Binding",
      "initializing": "Initializing Weixin QR binding...",
      "refreshQr": "Refresh QR Code",
      "retry": "Retry",
      "scanTitle": "Scan QR Code to Bind",
      "scannedWaiting": "QR code scanned! Please confirm on your Weixin...",
      "startFailed": "Failed to start QR login",
      "statusFailed": "Failed to check QR status",
      "step1": "1. Open Weixin on your phone and scan the QR code",
      "step2": "2. Confirm to complete binding",
      "successBody": "Your bot is now connected to Weixin.",
      "successTitle": "Weixin Binding Successful!",
      "uuidRequired": "Bot UUID is required"
    }
  }
};
