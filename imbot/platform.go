package imbot

import "github.com/tingly-dev/tingly-box/imbot/core"

// PlatformAuthConfig defines the authentication requirements for each platform
type PlatformAuthConfig struct {
	Platform    string      `json:"platform"`     // Platform identifier
	AuthType    string      `json:"auth_type"`    // "token", "oauth", "qr", "basic"
	DisplayName string      `json:"display_name"` // Human-readable platform name
	Category    string      `json:"category"`     // "im", "enterprise", "business"
	Fields      []FieldSpec `json:"fields"`       // Required/optional auth fields

	// Auth describes how the stored auth map becomes a core.AuthConfig.
	//
	// Deliberately separate from Fields: Fields is the settings-UI form, this
	// is the wire mapping. They usually overlap, but not always — Weixin's
	// credentials arrive from the QR flow and have no form fields at all, so
	// deriving the wire mapping from Fields would leave Weixin bots unable to
	// authenticate.
	Auth AuthMapping `json:"-"`
}

// AuthMapping says which auth-map keys feed which core.AuthConfig fields, and
// which of them a bot cannot start without.
//
// This exists so that adding a platform is a table edit rather than a hunt for
// every switch statement over platform names. Lark is the cautionary tale: it
// was present in this table but missing from two hand-written switches in the
// bot manager, so Lark bots were rejected as having no valid credentials and,
// had they got past that, would have been handed a token-type auth config that
// the Feishu client rejects.
type AuthMapping struct {
	// Type is the core.AuthConfig type: token, oauth, qr, none.
	Type string
	// TokenKey and friends name the auth-map key feeding each AuthConfig field.
	// An empty name means the field is not used by this platform.
	TokenKey        string
	ClientIDKey     string
	ClientSecretKey string
	AccountIDKey    string
	AuthDirKey      string
	// OptionKeys are auth-map entries forwarded verbatim into Config.Options
	// rather than into AuthConfig.
	OptionKeys []string
	// RequiredKeys must all be present and non-empty before a bot may start.
	RequiredKeys []string
}

// FieldSpec defines a single auth field
type FieldSpec struct {
	Key         string `json:"key"`         // Field key in auth map
	Label       string `json:"label"`       // Display label for the field
	Placeholder string `json:"placeholder"` // Placeholder text
	Required    bool   `json:"required"`    // Whether this field is required
	Secret      bool   `json:"secret"`      // Whether this field should be masked (password/token)
	HelperText  string `json:"helperText"`  // Additional guidance for users
}

// PlatformConfigs maps platform identifiers to their auth configurations
var PlatformConfigs = map[string]PlatformAuthConfig{
	"telegram": {
		Platform: "telegram",
		Auth:     AuthMapping{Type: "token", TokenKey: "token", RequiredKeys: []string{"token"}},
		AuthType: "token",
		Category: "im",
		Fields: []FieldSpec{
			{
				Key:         "token",
				Label:       "Bot Token",
				Placeholder: "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
				Required:    true,
				Secret:      true,
				HelperText:  "Get from @BotFather on Telegram",
			},
		},
	},
	"slack": {
		Platform: "slack",
		Auth:     AuthMapping{Type: "token", TokenKey: "token", RequiredKeys: []string{"token"}},
		AuthType: "token",
		Category: "im",
		Fields: []FieldSpec{
			{
				Key:         "token",
				Label:       "Bot Token",
				Placeholder: "xoxb-your-token-here",
				Required:    true,
				Secret:      true,
				HelperText:  "Must start with 'xoxb-'. Get from Slack API",
			},
		},
	},
	"discord": {
		Platform: "discord",
		Auth:     AuthMapping{Type: "token", TokenKey: "token", RequiredKeys: []string{"token"}},
		AuthType: "token",
		Category: "im",
		Fields: []FieldSpec{
			{
				Key:         "token",
				Label:       "Bot Token",
				Placeholder: "MTIzNDU2Nzg5OABCDEF123456789",
				Required:    true,
				Secret:      true,
				HelperText:  "Must start with 'Bot ' prefix. Get from Discord Developer Portal",
			},
		},
	},
	"dingtalk": {
		Platform: "dingtalk",
		Auth:     AuthMapping{Type: "oauth", ClientIDKey: "clientId", ClientSecretKey: "clientSecret", RequiredKeys: []string{"clientId", "clientSecret"}},
		AuthType: "oauth",
		Category: "enterprise",
		Fields: []FieldSpec{
			{
				Key:         "clientId",
				Label:       "App Key",
				Placeholder: "ding-your-app-key",
				Required:    true,
				Secret:      true,
				HelperText:  "Also known as AppKey or ClientId",
			},
			{
				Key:         "clientSecret",
				Label:       "App Secret",
				Placeholder: "Your app secret",
				Required:    true,
				Secret:      true,
				HelperText:  "Also known as AppSecret or ClientSecret",
			},
		},
	},
	"feishu": {
		Platform: "feishu",
		Auth:     AuthMapping{Type: "oauth", ClientIDKey: "clientId", ClientSecretKey: "clientSecret", RequiredKeys: []string{"clientId", "clientSecret"}},
		AuthType: "oauth",
		Category: "enterprise",
		Fields: []FieldSpec{
			{
				Key:         "clientId",
				Label:       "App ID",
				Placeholder: "cli-your-app-id",
				Required:    true,
				Secret:      true,
				HelperText:  "Also known as AppID or ClientId",
			},
			{
				Key:         "clientSecret",
				Label:       "App Secret",
				Placeholder: "Your app secret",
				Required:    true,
				Secret:      true,
				HelperText:  "Also known as AppSecret or ClientSecret",
			},
		},
	},
	"lark": {
		Platform: "lark",
		Auth:     AuthMapping{Type: "oauth", ClientIDKey: "clientId", ClientSecretKey: "clientSecret", RequiredKeys: []string{"clientId", "clientSecret"}},
		AuthType: "oauth",
		Category: "enterprise",
		Fields: []FieldSpec{
			{
				Key:         "clientId",
				Label:       "App ID",
				Placeholder: "cli-your-app-id",
				Required:    true,
				Secret:      true,
				HelperText:  "Also known as AppID or ClientId",
			},
			{
				Key:         "clientSecret",
				Label:       "App Secret",
				Placeholder: "Your app secret",
				Required:    true,
				Secret:      true,
				HelperText:  "Also known as AppSecret or ClientSecret",
			},
		},
	},
	"whatsapp": {
		Platform: "whatsapp",
		Auth:     AuthMapping{Type: "token", TokenKey: "token", AccountIDKey: "phoneNumberId", RequiredKeys: []string{"token"}},
		AuthType: "token",
		Category: "business",
		Fields: []FieldSpec{
			{
				Key:         "token",
				Label:       "Access Token",
				Placeholder: "Your WhatsApp access token",
				Required:    true,
				Secret:      true,
				HelperText:  "Get from Meta for Developers",
			},
			{
				Key:         "phoneNumberId",
				Label:       "Phone Number ID",
				Placeholder: "Your phone number ID",
				Required:    false,
				Secret:      false,
				HelperText:  "Optional: The phone number ID for sending messages",
			},
		},
	},
	"weixin": {
		Platform: "weixin",
		Auth:     AuthMapping{Type: "qr", TokenKey: "token", AccountIDKey: "bot_id", AuthDirKey: "user_id", OptionKeys: []string{"user_id", "base_url"}, RequiredKeys: []string{"token", "bot_id"}},
		AuthType: "qr",
		Category: "enterprise",
		Fields:   []FieldSpec{}, // No fields - credentials set via QR flow
	},
	"wecom": {
		Platform: "wecom",
		Auth:     AuthMapping{Type: "oauth", ClientIDKey: "clientId", ClientSecretKey: "clientSecret", RequiredKeys: []string{"clientId", "clientSecret"}},
		AuthType: "oauth",
		Category: "enterprise",
		Fields: []FieldSpec{
			{
				Key:         "clientId",
				Label:       "Bot ID",
				Placeholder: "Your WeCom AI Bot ID",
				Required:    true,
				Secret:      false,
				HelperText:  "The AI Bot ID from WeCom developer console",
			},
			{
				Key:         "clientSecret",
				Label:       "Bot Secret",
				Placeholder: "Your WeCom AI Bot secret",
				Required:    true,
				Secret:      true,
				HelperText:  "The AI Bot secret from WeCom developer console",
			},
		},
	},
	"tingly": {
		Platform: "tingly",
		Auth:     AuthMapping{Type: "none", TokenKey: "token"},
		AuthType: "none",
		Category: "im",
		Fields:   []FieldSpec{}, // No required credentials
	},
}

// init derives each platform's DisplayName from the single source of truth in
// core (PlatformNames), so the settings-UI metadata cannot drift from the
// runtime metadata.
func init() {
	for id, cfg := range PlatformConfigs {
		cfg.DisplayName = core.GetPlatformName(core.Platform(id))
		PlatformConfigs[id] = cfg
	}
}

// GetPlatformConfig returns the auth config for a given platform
func GetPlatformConfig(platform string) (PlatformAuthConfig, bool) {
	config, exists := PlatformConfigs[platform]
	return config, exists
}

// GetPlatformsByCategory returns platforms grouped by category
func GetPlatformsByCategory() map[string][]PlatformAuthConfig {
	result := make(map[string][]PlatformAuthConfig)
	for _, config := range PlatformConfigs {
		result[config.Category] = append(result[config.Category], config)
	}
	return result
}

// GetAllPlatforms returns all platform configurations as a slice
func GetAllPlatforms() []PlatformAuthConfig {
	platforms := make([]PlatformAuthConfig, 0, len(PlatformConfigs))
	for _, config := range PlatformConfigs {
		platforms = append(platforms, config)
	}
	return platforms
}

// IsValidPlatform reports whether the platform has a settings/auth-config
// entry (i.e. it can be configured from the UI). This is deliberately narrower
// than core.IsValidPlatform, which reports whether the identifier is a known
// platform at all — the latter also covers reserved platforms (signal,
// googlechat, bluebubbles) that have no configuration form yet.
func IsValidPlatform(platform string) bool {
	_, exists := PlatformConfigs[platform]
	return exists
}

// CategoryLabels provides display labels for categories
var CategoryLabels = map[string]string{
	"im":         "IM Platforms",
	"enterprise": "Enterprise",
	"business":   "Business",
}

// AuthTypeLabels provides display labels for auth types
var AuthTypeLabels = map[string]string{
	"token": "Token",
	"oauth": "OAuth",
	"qr":    "QR Code",
	"basic": "Basic Auth",
}

// defaultAuthMapping is used for platforms with no entry in PlatformConfigs.
// Historically that meant "assume a bot token", which is the least surprising
// guess for a new IM platform.
var defaultAuthMapping = AuthMapping{Type: "token", TokenKey: "token", RequiredKeys: []string{"token"}}

// AuthMappingFor returns a platform's auth mapping, falling back to the
// token-based default for unknown platforms.
func AuthMappingFor(platform string) AuthMapping {
	if cfg, ok := PlatformConfigs[platform]; ok && cfg.Auth.Type != "" {
		return cfg.Auth
	}
	return defaultAuthMapping
}

// BuildAuthConfig converts a bot's stored auth map into a core.AuthConfig.
func BuildAuthConfig(platform string, auth map[string]string) AuthConfig {
	m := AuthMappingFor(platform)
	pick := func(key string) string {
		if key == "" {
			return ""
		}
		return auth[key]
	}
	return AuthConfig{
		Type:         m.Type,
		Token:        pick(m.TokenKey),
		ClientID:     pick(m.ClientIDKey),
		ClientSecret: pick(m.ClientSecretKey),
		AccountID:    pick(m.AccountIDKey),
		AuthDir:      pick(m.AuthDirKey),
	}
}

// MissingAuthKeys lists the required auth keys a bot has not been given. An
// empty result means the bot has everything it needs to attempt a connection.
func MissingAuthKeys(platform string, auth map[string]string) []string {
	var missing []string
	for _, key := range AuthMappingFor(platform).RequiredKeys {
		if auth[key] == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

// AuthOptions returns the auth-map entries a platform expects in Config.Options
// rather than in AuthConfig (Weixin's user_id / base_url).
func AuthOptions(platform string, auth map[string]string) map[string]interface{} {
	keys := AuthMappingFor(platform).OptionKeys
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		if v, ok := auth[key]; ok && v != "" {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
