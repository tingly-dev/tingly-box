package imbot

// PlatformAuthConfig defines the authentication requirements for each platform
type PlatformAuthConfig struct {
	Platform    string      `json:"platform"`     // Platform identifier
	AuthType    string      `json:"auth_type"`    // "token", "oauth", "qr", "basic"
	DisplayName string      `json:"display_name"` // Human-readable platform name
	Category    string      `json:"category"`     // "im", "enterprise", "business"
	Fields      []FieldSpec `json:"fields"`       // Required/optional auth fields
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
		Platform:    "telegram",
		AuthType:    "token",
		DisplayName: "Telegram",
		Category:    "im",
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
		Platform:    "slack",
		AuthType:    "token",
		DisplayName: "Slack",
		Category:    "im",
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
		Platform:    "discord",
		AuthType:    "token",
		DisplayName: "Discord",
		Category:    "im",
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
		Platform:    "dingtalk",
		AuthType:    "oauth",
		DisplayName: "DingTalk",
		Category:    "enterprise",
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
		Platform:    "feishu",
		AuthType:    "oauth",
		DisplayName: "Feishu / Lark",
		Category:    "enterprise",
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
		Platform:    "whatsapp",
		AuthType:    "token",
		DisplayName: "WhatsApp",
		Category:    "business",
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

// IsValidPlatform checks if a platform identifier is valid
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
