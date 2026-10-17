// Providers, credentials and access: provider dialogs, API keys, sharing keys, teams, access control
export default {
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
        "useGlobalNotSet": "Use quick proxy (not configured — set in System Settings)",
        "applyGlobal": "Apply quick proxy ({{url}})",
        "applyGlobalApplied": "Quick proxy applied"
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
    "quota": {
      "used": "{{value}} used",
      "balance": "Balance",
      "leftOf": "{{value}} left of {{limit}}",
      "cost": "Cost",
      "readFailed": "Couldn't read quota — Details below has the raw response",
      "noLimits": "No quota limits reported",
      "none": "No quota reading yet",
      "refresh": "Refresh",
      "rawResponse": "Details"
    },
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
  "credentialPage": {
    "empty": "No credentials yet",
    "managing_one": "Managing {{count}} credential",
    "managing_other": "Managing {{count}} credentials",
    "emptyTitle": "Connect your first AI",
    "emptyDescription": "Sign in with a subscription you already have (Claude Code, Codex, Gemini CLI…) or paste an API key (OpenAI, Anthropic, DeepSeek…). Every credential lands here, and routing rules pick models from them.",
    "sectionOAuth": "OAuth",
    "sectionApiKeys": "API Keys",
    "fallbackProviderName": "this provider",
    "unknownError": "Unknown error",
    "loadFailed": "Failed to load providers: {{error}}",
    "deleted": "Provider deleted successfully!",
    "deleteFailed": "Failed to delete provider: {{error}}",
    "toggleFailed": "Failed to toggle provider: {{error}}",
    "tokenRefreshed": "Token refreshed successfully!",
    "reauthorized": "Provider reauthorized successfully!",
    "reauthorizeUnknownIssuer": "Cannot reauthorize: provider issuer is unknown",
    "refreshFailedTitle": "Token refresh failed",
    "refreshFailedBody": "Refreshing the token for <1>{{name}}</1> didn't work. If the credential was revoked or has fully expired, a refresh can't recover it — reauthorize to sign in again. This overwrites the credential in place, keeping the same provider so your routing rules and model keys stay intact.",
    "dismiss": "Dismiss",
    "reauthorize": "Reauthorize"
  },
  "apiKeyModal": {
    "title": "API Key",
    "description": "Your authentication token:",
    "clickToCopy": "Click to copy token",
    "copyButton": "Copy Token"
  },
  "sharingKeys": {
    "title": "Team Keys",
    "titleForTeam": "Team Keys · {{team}}",
    "createToken": "Create Token",
    "createDialogTitle": "Create Team Key",
    "displayName": "Display Name",
    "displayNamePlaceholder": "e.g., Team Alpha Key",
    "displayNameHelper": "A descriptive name for this Team Key",
    "deleteToken": "Delete Token",
    "deleteConfirm": "Are you sure you want to delete the token \"{{name}}\"? This action cannot be undone.",
    "nameRequired": "Display Name is required",
    "createSuccess": "Token created successfully",
    "createFailed": "Failed to create token",
    "deleteSuccess": "Token deleted successfully",
    "deleteFailed": "Failed to delete token",
    "copiedToClipboard": "Token copied to clipboard",
    "copyFailed": "Failed to copy token",
    "disabled": "Token disabled",
    "enabled": "Token enabled",
    "updateFailed": "Failed to update token",
    "moveToken": "Move Key",
    "destinationTeam": "Destination team",
    "noDestinationTeam": "No other active team",
    "moveHelper": "Move {{name}} without rotating its key.",
    "moveSuccess": "Key moved to the new team",
    "moveFailed": "Failed to move key",
    "teamKeysTitle": "Team Keys",
    "allSubtitle": "The keys of every Team, grouped by Team — {{keys}} in total across {{teams}} Teams.",
    "openTeam": "Open Team",
    "emptyTeam": "No Team Keys in this Team yet.",
    "loadFailed": "Failed to load Team Keys",
    "createDisabledTeam": "This Team is disabled. Enable it before creating Team Keys.",
    "overview": {
      "count_one": "{{count}} key",
      "count_other": "{{count}} keys",
      "expand": "Show all keys",
      "collapse": "Hide keys",
      "empty": "No keys in this Team yet. Create one for each person you hand the Team to."
    },
    "table": {
      "name": "Name",
      "user": "User",
      "token": "Token",
      "status": "Status",
      "created": "Created",
      "lastUsed": "Last Used",
      "actions": "Actions",
      "empty": "No Team Keys yet. Create one to share model access with your team.",
      "hide": "Hide",
      "show": "Show",
      "copy": "Copy",
      "active": "Active",
      "inactive": "Disabled",
      "moveTooltip": "Move to another team",
      "deleteTooltip": "Delete token"
    }
  },
  "teams": {
    "accessTitle": "Team Access",
    "keyScopeSummary": "Sharing keys for {{team}} ({{slug}}) work only with /tingly/team and /tingly/team/v1. They cannot access other Teams, scenario endpoints, or management APIs.",
    "allKeysScopeSummary": "Each Team Key works only for the Team it belongs to, via /tingly/team and /tingly/team/v1. Moving a key to another Team takes effect immediately, without rotating it.",
    "keyScopeInfoLabel": "Sharing key access scope",
    "editTeam": "Team settings",
    "name": "Team name",
    "inactive": "Inactive",
    "disabledHint": "This team is disabled. Its Team Keys cannot access model endpoints until the team is enabled.",
    "enableTeam": "Enable team",
    "disableTeam": "Disable team",
    "deleteTeam": "Delete team",
    "deleteConfirm": "Delete {{team}}? Move or delete all of its Team Keys first.",
    "loadFailed": "Failed to load teams",
    "saveFailed": "Failed to save team",
    "createSuccess": "Team created",
    "updateSuccess": "Team updated",
    "deleteSuccess": "Team deleted",
    "deleteFailed": "Team cannot be deleted while it owns Team Keys",
    "enabled": "Team enabled",
    "disabled": "Team disabled",
    "guide": {
      "trigger": "How Team works",
      "title": "How Team works",
      "subtitle": "Step {{current}} of {{total}}",
      "previous": "Previous",
      "next": "Next",
      "gotIt": "Got it!",
      "steps": {
        "usage": {
          "title": "What a Team is for",
          "content": "A Team is an isolated slice of this instance — its own rules, Team Keys, and usage, separate from your other scenarios and Teams.",
          "bullet1": "Give a group — a customer, a sub-team, an external app — model access without sharing your main setup.",
          "bullet2": "Add rules to this Team below, just like on any other scenario page."
        },
        "isolation": {
          "title": "How it's separated",
          "content": "A Team Key belongs to exactly one Team and can only reach /tingly/team[/v1].",
          "bullet1": "Moving or disabling a key or Team takes effect immediately.",
          "bullet2": "Your Global model token isn't Team-scoped — it still has full access."
        },
        "keys": {
          "title": "Configure a Team Key",
          "content": "Click \"Team Keys\" above to create one for this Team.",
          "bullet1": "Name it, then copy the tb-share-… value — shown only once.",
          "bullet2": "Move it to another Team anytime, without rotating it."
        },
        "principles": {
          "title": "How it's used",
          "content": "Point the client at this Team's Base URL, using its Team Key as the API key.",
          "bullet1": "Base URL and API key are shown in the Team Access panel above.",
          "bullet2": "Requests route through the rules below — only models added there are reachable."
        }
      }
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
  }
};
