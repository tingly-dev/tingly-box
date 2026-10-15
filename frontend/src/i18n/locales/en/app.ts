// App shell and shared copy: navigation, system settings, help, login, onboarding
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
    "copyModelName": "Copy model name",
    "modelNameCopied": "Copied!",
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
    "workingFolders": "Working folders",
    "noWorkingFolders": "No working folders yet",
    "workingFolderWaiting": "{{count}} waiting on you",
    "desk": "Desk",
    "projects": "Projects",
    "projectsWaiting": "A thread is waiting on you",
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
      "useDecisions": "Decisions",
      "useImageGen": "Image API",
      "useTeam": "Team",
      "useCustom": "Custom",
      "apiKeys": "API Keys",
      "oauth": "OAuth",
      "credential": "Credential",
      "prompt": "Prompt"
    },
    "sidebar": {
      "pinToTop": "Pin to top",
      "unpin": "Unpin",
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
      "doneEditing": "Done",
      "expand": "Expand sidebar"
    },
    // One-time callouts pointing at a control (components/CoachMark).
    "coachMarks": {
      "gotIt": "Got it",
      "agentVisibility": {
        "title": "Choose which agents show",
        "text": "Click the eye to show or hide agents in this list. Hidden ones stay here, one click away."
      },
      "powerUps": {
        "title": "Power-ups moved here",
        "text": "Team, Image, Remote, Bench and the other optional items are now switched on and off from this menu, under Power-ups."
      }
    },
    "activityBar": {
      "disconnected": "Disconnected",
      "disconnectedDebug": "Disconnected (Debug)",
      "offline": "Offline",
      "devMode": "Dev",
      "newVersionAvailable": "Update",
      "error": "Error",
      "theme": "Theme",
      "light": "Light",
      "dark": "Dark",
      "system": "System",
      "sunlit": "Sunlit",
      "claude": "Claude",
      "ds": "DeepSeek",
      "click": "Click",
      "feedback": "Feedback",
      "feedbackTooltip": "Send Feedback (opens GitHub Issues)",
      "preferences": "Preferences",
      "allSettings": "All settings"
    },
    "themeMenu": {
      "switchTo": "Switch to:",
      "theme": "Theme:"
    },
    "easterEgg": "Hi, I'm Tingly-Box, Your Smart AI Orchestrator",
    "githubStar": {
      "text": "Enjoying Tingly-Box? A star on GitHub helps others find it.",
      "cta": "Star on GitHub",
      "short": "Star"
    },
    "dashboard": "Dashboard",
    // Rail-level label only — the sidebar rows and the Team page's own
    // title keep using nav.useTeam ("Team", untranslated everywhere).
    "team": "Team",
    "usage": "Usage",
    "bench": "Bench",
    "userUsage": "Team usage",
    "quotaHistory": "Quota history",
    "heatmap": "Heatmap",
    "today": "Today",
    "yesterday": "Yesterday",
    "days": "Days",
    "remote": "Remote",
    "image": "Image",
    "imagePlayground": "Playground",
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
    "protectedCredentials": "Secrets",
    "mcp": "MCP",
    "sources": "Sources",
    "localMode": "Local Mode",
    "credentials": "Credentials",
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
    "addProfile": "New profile",
    "addTeam": "Add Team",
    "teamKeys": "Team Keys",
    "teamKeysTooltip": "The keys of every Team on one page, grouped by Team.",
    "default": "default",
    "help": "Tips & Help",
    "helpShort": "Help",
    "powerUps": "Power-ups",
    "tools": "Tools",
    "servertool": "Servertool",
    "botsRunning": "{{active}} of {{total}} bots running"
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
      "projects": "Projects",
      "enableProjects": "Hand a project its work as it comes up: a coordinator runs each task as a Claude Code thread in its own git worktree.",
      "projectsEnabledInfo": "Anyone who can sign in to this tingly-box can now run Claude Code threads on this machine, in the repositories their projects name.",
      "desk": "Desk",
      "enableDesk": "Run Claude Code on this machine from a browser tab, in a folder you choose.",
      "deskEnabledInfo": "Anyone who can sign in to this tingly-box can now start Claude Code sessions on this machine and approve the tool calls they make.",
      "title": "Experimental Features",
      "description": "These experimental features apply globally to all scenarios. Individual scenarios can override these settings.",
      "skills": "Skills",
      "userPrompts": "User prompts",
      "guardrails": "Guardrails",
      "mcp": "MCP",
      "bench": "Bench",
      "enableUserPrompts": "Manage reusable instructions for user requests",
      "enableIdeSkills": "Manage code snippets and skills from your IDE",
      "enableGuardrails": "Block risky tool calls and filter sensitive outputs",
      "enableMCP": "MCP (Model Context Protocol) tools such as web search and web fetch",
      "enableBench": "The probe test workbench — raw client requests, flag overlays and payload editing",
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
      "title": "Appearance & Language",
      "themePalette": "Palette preview"
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
      "title": "Provider Catalog",
      "description": "Browse the catalog or paste a config snippet — we'll figure out the rest."
    },
    "routing": {
      "title": "Routing & Tier Guides",
      "description": "Revisit how direct routing, smart routing, and model tiers work — the same diagrams and steps used elsewhere in the product.",
      "direct": "Direct Routing Guide",
      "smart": "Smart Routing Guide",
      "tier": "Tier Guide"
    },
    "team": {
      "title": "Team Guide",
      "description": "What a Team is for, how it's isolated, and how to configure a Team Key — the same guide available on any Team page.",
      "action": "How Team works"
    }
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
        "description": "Update the installed CLI, then restart the server to apply",
        "aliasNote": "tb is short for tingly-box; `tingly-box restart` works too"
      },
      "docker": {
        "title": "Docker Image",
        "description": "Pull from GitHub Container Registry"
      }
    }
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
  "hub": {
    "title": "Tingly Box",
    "status": {
      "healthy": "Running",
      "unhealthy": "Disconnected",
      "updateAvailable": "Update available"
    },
    "quota": {
      "title": "Provider Quota",
      "empty": "No quota data yet",
      "refresh": "Refresh quota",
      "sort": {
        "label": "Sort providers",
        "remaining": "Closest to limit",
        "name": "Name",
        "type": "Type"
      }
    }
  }
};
