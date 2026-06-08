export default {
  "common": {
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
    "copy": "Copy",
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
    "openClaw": "OpenClaw",
    "prompt": "Prompt"
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
      "useXcode": "Xcode",
      "useVSCode": "VS Code",
      "useEmbed": "Embedding",
      "useImageGen": "Image Gen",
      "playground": "Playground",
      "apiKeys": "API Keys",
      "oauth": "OAuth",
      "credential": "Credential",
      "prompt": "Prompt"
    },
    "sidebar": {
      "newProfile": "New Profile",
      "profileName": "Profile name",
      "mode": "Mode",
      "modeUnified": "Unified: Single model for all",
      "modeSeparate": "Separate: Individual models",
      "separate": "Separate",
      "unified": "Unified",
      "createProfileTooltip": "Create a new Claude Code profile with custom settings",
      "sloganTooltip": "For all Solo Builders, Dev Teams and Agents."
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
    "heatmap": "Heatmap",
    "today": "Today",
    "yesterday": "Yesterday",
    "days": "Days",
    "remote": "Remote",
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
    "virtualModelsTooltip": "Built-in synthetic model providers for onboarding, demos, and dry-runs. They respond locally without contacting any upstream.",
    "accessControl": "Access Control",
    "status": "Status",
    "system": "System",
    "general": "General",
    "experimental": "Experimental",
    "logs": "Troubleshoot",
    "userRequest": "User Request",
    "skills": "Skills",
    "addProfile": "Add Profile",
    "default": "default",
    "onboarding": "Quick Add Provider",
    "onboardingHint": "Browse or paste config",
    "onboardingShort": "Onboard"
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
        "description": "Run once with latest version"
      },
      "bundle": {
        "title": "Offline Bundle (npx)",
        "description": "Download with built-in binary for network issues"
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
  "providerDialog": {
    "addTitle": "Connect AI",
    "addDescription": "Select a provider and enter your API key to connect AI services. Multiple protocols can be enabled for providers that support them.",
    "editTitle": "Edit Connection",
    "addButton": "Connect",
    "apiStyle": {
      "label": "API Style",
      "placeholder": "Select API style...",
      "helperOpenAI": "Supports models from OpenAI, Google and many other OpenAI-compatible providers",
      "helperAnthropic": "For Anthropic-compatible AI providers, commonly used with Claude Code.",
      "openAI": "OpenAI Compatible",
      "anthropic": "Anthropic Compatible",
      "switchWarning": "API style changed. Base URL has been reset. Please select a compatible provider."
    },
    "provider": {
      "label": "Base URL",
      "placeholder": "Pick a known service or paste a base URL"
    },
    "region": {
      "cn": "China",
      "global": "Global"
    },
    "protocol": {
      "label": "Protocol"
    },
    "fusion": {
      "modeLabel": "Fusion mode",
      "tooltipTitle": "How both protocols are saved",
      "normalModeDesc": "Normal mode (unchecked): creates two separate base URLs, one for OpenAI and one for Anthropic.",
      "fusionModeDesc": "Fusion mode (checked): merges into one provider, switching between OpenAI and Anthropic on demand.",
      "outcomeSplit": "Will create 2 separate base URLs (sharing this key).",
      "outcomeMerged": "Will keep 1 base URL serving both OpenAI and Anthropic."
    },
    "keyName": {
      "label": "Name",
      "placeholder": "e.g., OpenAI",
      "default": "Default Provider",
      "helper": "Leave blank to use the auto-generated name. You can rename later.",
      "editAction": "Edit name"
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
    "createNewRule": "Create new routing rule"
  },
  "probe": {
    "testRule": "Test Rule",
    "testProvider": "Test Service",
    "shape": "Shape",
    "nonstream": "Nonstream",
    "stream": "Stream",
    "scope": "Scope",
    "throughTB": "Through TB",
    "direct": "Direct",
    "scopeHint": "Direct skips Tingly-Box's routing & middleware, to tell whether a failure is upstream or inside TB.",
    "run": "Run Test",
    "running": "Testing…",
    "runHint": "Pick a shape, then click Run Test",
    "rerun": "Re-run",
    "copyResponse": "Copy response",
    "copied": "Copied!",
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
      "selectProvider": "Select Provider",
      "selectModel": "Select Model",
      "testService": "Test Service",
      "deleteService": "Delete Service"
    },
    "tier": {
      "invalidInput": "Please enter a valid number.",
      "tooltipSet": "Tier {{tier}} (lower = tried first). Click to change.",
      "tooltipUnset": "No tier set (load balanced with other T0 services). Click to assign.",
      "ariaLabel": "Tier {{tier}}",
      "ariaUnset": "No tier",
      "editTitle": "Set Tier",
      "adjustTier": "Adjust tier",
      "helpHigher": "Lower number = higher priority (T0 is tried first). Services in the same tier are load balanced.",
      "helpZero": "Set to 0 for T0 — the first tier.",
      "tierLabel": "T{{index}}",
      "tierBalanced": "Balanced",
      "dividerHelp": "Lower-numbered tiers are always tried first. Only when all services in a tier fail (circuit open) does traffic fall through to the next tier. Services within the same tier are load-balanced.",
      "tooltip": "T0 is tried first, T1 is the fallback, and so on. Services within the same tier are load-balanced.",
      "addTierTooltip": "Add a new fallback tier",
      "nodeTooltipPrimaryTitle": "T0 — Highest priority",
      "nodeTooltipPrimaryBody": "Tried first on every request. Services here are load-balanced.",
      "nodeTooltipFallbackTitle": "T{{tier}} — Fallback tier",
      "nodeTooltipFallbackBody": "Tried only when all higher-priority tiers are unavailable (lower number = higher priority). Services here are load-balanced.",
      "nodeMoveHint": "↑ / ↓  on a service card to move it to a different tier",
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
        "hoverHint": "Action buttons shown - try hovering over nodes!",
        "steps": {
          "1": {
            "title": "What is a Tier?",
            "content": "Tiers organize your services by priority. T0 (tier zero) is the highest priority tier — services here are tried first on every request. Lower tier numbers mean higher priority.",
            "annotation": {
              "tier": "T0 — Highest priority tier",
              "service": "Your service card with model and provider info"
            }
          },
          "2": {
            "title": "Multiple Services in One Tier",
            "content": "When you have multiple services in the same tier (like T0), they share the incoming traffic. This is called load balancing — requests are distributed across all services in the tier.",
            "annotation": {
              "loadBalance": "Same tier = load balanced",
              "multiple": "Multiple services share traffic"
            }
          },
          "3": {
            "title": "Setting Up Primary and Fallback",
            "content": "Use the ↑/↓ buttons on service cards to move them between tiers. Services in T0 are your primary choice. Services in T1, T2, etc. act as fallbacks — they only run when all higher-priority tiers fail.",
            "annotation": {
              "primary": "T0 — Primary services (tried first)",
              "fallback": "T1 — Fallback services (used when T0 fails)",
              "actionButtons": "↑/↓ buttons move services between tiers"
            }
          },
          "4": {
            "title": "Automatic Failover",
            "content": "When all services in a tier fail (circuit breaker opens), traffic automatically falls back to the next tier. Once the tier recovers (circuit breaker closes), traffic returns to it automatically. You don't need to do anything — it just works.",
            "annotation": {
              "circuitBreaker": "Circuit breaker monitors service health",
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
        "hoverHint": "Action buttons shown - try hovering over nodes!",
        "steps": {
          "1": {
            "title": "What is Direct Routing?",
            "content": "Direct routing is the simplest way to forward requests. Traffic flows through your tiers in order — T0 first, then T1, T2, and so on. Within each tier, services are load-balanced evenly. This works great when all your services are equivalent and you just need primary/fallback layers.",
            "annotation": {
              "entryNode": "Entry node - routing mode selector",
              "directButton": "Direct mode selected"
            }
          },
          "2": {
            "title": "Load Balancing Within Tiers",
            "content": "When multiple services are in the same tier, they share the incoming traffic evenly. This load balancing distributes requests across all services in the tier, preventing any single service from becoming overwhelmed.",
            "annotation": {
              "loadBalance": "Same tier = load balanced",
              "services": "Multiple services share traffic"
            }
          },
          "3": {
            "title": "Tier-Based Fallback Chain",
            "content": "Services in T0 are your primary choice. If all T0 services fail, traffic automatically falls back to T1, then T2, and so on. This creates a cascading failover chain that ensures high availability for your applications.",
            "annotation": {
              "primary": "T0 — Primary services (tried first)",
              "fallback": "T1 — Fallback services (used when T0 fails)",
              "tierBased": "Tier-based automatic failover"
            }
          },
          "4": {
            "title": "What is Smart Routing?",
            "content": "Smart routing lets you define custom conditions to control which service handles each request. Route based on model name, token count, user groups, or any request parameter. This gives you fine-grained control without managing complex tier configurations.",
            "annotation": {
              "smartMode": "Smart mode selected",
              "smartButton": "Smart routing button",
              "conditional": "Conditional routing based on rules"
            }
          },
          "5": {
            "title": "Smart Routing Conditions",
            "content": "Each smart rule has a condition that determines when it applies. Common conditions include: model name matching (e.g., 'contains claude'), token count (e.g., 'gt 4000' for large contexts), or custom fields. Rules are evaluated in order — the first matching rule wins.",
            "annotation": {
              "conditions": "Multiple smart rules with conditions",
              "modelBased": "Route by model name",
              "tokenBased": "Route by token count"
            }
          },
          "6": {
            "title": "Advanced Smart Routing",
            "content": "Combine multiple smart rules to create sophisticated routing strategies. For example: route Claude requests to one service, large contexts to another, and premium users to a third. The default service handles everything that doesn't match any rule.",
            "annotation": {
              "complex": "Complex routing with multiple conditions",
              "defaultRoute": "Default route for unmatched requests",
              "claudeRoute": "Route for Claude models",
              "largeContext": "Route for large context windows"
            }
          }
        }
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
      "addServiceFirst": "Add a service to enable request forwarding",
      "addServiceSecond": "Add another service (load balancing will be enabled)"
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
      "current": "Current",
      "saveSuccess": "Language settings updated",
      "saveFailed": "Failed to update language settings"
    },
    "experimentalFeatures": {
      "title": "Experimental Features",
      "description": "These experimental features apply globally to all scenarios. Individual scenarios can override these settings.",
      "skills": "Skills",
      "guardrails": "Guardrails",
      "mcp": "MCP",
      "enableIdeSkills": "Enable IDE Skills feature for managing code snippets and skills from IDEs",
      "enableGuardrails": "Enable Guardrails - block risky tool calls and filter sensitive outputs",
      "enableMCP": "Enable MCP Tools - Configure MCP (Model Context Protocol) tools like web search and web fetch",
      "on": "On",
      "off": "Off",
      "enabled": "enabled",
      "disabled": "disabled - Click to enable",
      "guardrailsEnabledInfo": "Guardrails is enabled. A \"Guardrails\" page is available in the sidebar for rule management.",
      "mcpEnabledInfo": "MCP Tools is enabled. An \"MCP Tools\" page is available under System in the sidebar for configuration."
    },
    "about": {
      "title": "About",
      "version": "Version",
      "license": "License",
      "github": "GitHub",
      "devMode": "Dev Mode",
      "available": "available"
    },
    "serverStatus": {
      "title": "Server Status",
      "server": "Server",
      "forceLogout": "Force logout",
      "refreshStatus": "Refresh status"
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
  "claudeCode": {
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
      "quickStart": "Quick Start",
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
      "separate": "Separate",
      "unifiedDescription": "All models use the same routing rule",
      "separateDescription": "Each model uses its own routing rule",
      "modeUpdated": "Profile mode updated to {{mode}}",
      "modeUpdateFailed": "Failed to update profile mode"
    }
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
    "agentNav": {
      "title": "Quick Start",
      "description": "Select agent to start"
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
    "subtitle": "Add your first AI provider to get started. Browse the catalog or paste a config snippet — we’ll figure out the rest.",
    "hint": "Detection runs locally in the box; pasted text is not sent to any third party.",
    "tab": {
      "browse": "Browse providers",
      "paste": "Paste & detect"
    },
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
    "goToHelp": "Help & Docs"
  },
  "scenarioOverview": {
    "title": "Agents",
    "subtitle": "Pick a scenario to configure. Hide the ones you don't use to keep the sidebar tidy.",
    "showInSidebar": "Show in sidebar",
    "hidden": "Hidden",
    "editTooltip": "Manage visible agents",
    "descriptions": {
      "claude_code": "Route Claude Code with custom profiles and per-task models.",
      "claude_desktop": "Connect Claude Desktop as an MCP client through Tingly Box.",
      "codex": "Configure Codex CLI through your provider keys.",
      "opencode": "Open-source coding agent powered by your provider.",
      "xcode": "Bring your model into Xcode's coding intelligence.",
      "vscode": "Power VS Code Copilot Chat through Tingly Box.",
      "openai": "Drop-in OpenAI-compatible SDK endpoint.",
      "anthropic": "Drop-in Anthropic-compatible SDK endpoint.",
      "embed": "Route embedding requests to your provider.",
      "imagegen": "Route image generation through Tingly Box.",
      "agent": "OpenClaw — universal agent runner."
    }
  }
};
