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
    "info": "Info"
  },
  "layout": {
    "appTitle": "Tingly Box",
    "slogan": "Ready for AI Agent Dev",
    "version": "version {{version}}",
    "nav": {
      "home": "Scenario",
      "settings": "Settings",
      "useOpenAI": "OpenAI SDK",
      "useAnthropic": "Anthropic SDK",
      "useClaudeCode": "Claude Code",
      "apiKeys": "API Keys",
      "oauth": "OAuth",
      "credential": "Credential"
    }
  },
  "login": {
    "title": "Tingly Box",
    "subtitle": "Authentication Required",
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
      "button": "Add Your First API Key"
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
    "addButton": "Add API Key",
    "emptyCardTitle": "No Model API Key Configured",
    "emptyCardSubtitle": "Get started by adding your first API token or key",
    "emptyCardButton": "Add Your First Provider",
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
    "addTitle": "Add New API Key",
    "editTitle": "Edit API Key",
    "addButton": "Add API Key",
    "apiStyle": {
      "label": "API Style",
      "placeholder": "Select API style...",
      "helperOpenAI": "Supports models from OpenAI, Google and many other OpenAI-compatible providers",
      "helperAnthropic": "For Anthropic-compatible AI providers, commonly used with Claude Code.",
      "openAI": "OpenAI Compatible",
      "anthropic": "Anthropic Compatible"
    },
    "keyName": {
      "label": "API Key Name",
      "placeholder": "e.g., OpenAI",
      "autoFill": "Key For {{title}}"
    },
    "providerOrUrl": {
      "label": "Provider or Custom Base URL",
      "placeholder": "Select a provider or enter custom URL"
    },
    "apiKey": {
      "label": "API Key",
      "placeholderAdd": "Your API token",
      "placeholderEdit": "Leave empty to keep current token",
      "helperEdit": "Leave empty to keep current token"
    },
    "enabled": "Enabled",
    "advanced": {
      "title": "Advanced",
      "proxyUrl": {
        "label": "Proxy URL",
        "placeholder": "e.g., http://127.0.0.1:7890 or socks5://127.0.0.1:1080",
        "helper": "Optional HTTP or SOCKS proxy for API requests"
      }
    },
    "verification": {
      "verifying": "Verifying...",
      "verifyButton": "Verify",
      "missingFields": "Please fill in all required fields (API Style, Name, API Base URL, API Key)",
      "failed": "Verification failed",
      "networkError": "Network error or unable to connect to verification service",
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
      "noKey": "No API key",
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
      "selectProvider": "Select provider",
      "selectModel": "Select model"
    },
    "menu": {
      "refreshModels": "Refresh Models",
      "deleteProvider": "Delete Provider"
    },
    "tooltips": {
      "addProviderFirst": "Add a provider to enable request forwarding",
      "addProviderSecond": "Add another provider (with 2+ providers, load balancing will be enabled based on strategy)",
      "addProviderMore": "Add another provider (requests will be load balanced across all providers)",
      "addFirstProvider": "Add your first provider"
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
    }
  },
  "system": {
    "pageTitle": "Server Status",
    "status": {
      "running": "Running",
      "stopped": "Stopped",
      "server": "Server: {{url}}",
      "keys": "Keys: {{enabled}}/{{total}}",
      "uptime": "Uptime: {{uptime}}",
      "lastUpdated": "Last Updated: {{time}}",
      "loading": "Loading..."
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
    "unifiedConfig": "Unified Configuration",
    "separateConfig": "Separate Configuration",
    "switchToSeparate": "Switch to Separate",
    "switchToUnified": "Switch to Unified",
    "modal": {
      "title": "Claude Code Configuration Guide",
      "subtitle": "Follow these steps to configure Claude Code to use Tingly Box as your AI proxy",
      "dontRemindAgain": "Do not remind again",
      "showGuide": "Config Claude Code"
    }
  }
};
