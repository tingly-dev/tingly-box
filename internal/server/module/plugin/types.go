package plugin

// RegisterPluginRequest registers external plugin code as a tingly-box upstream.
// It is idempotent by name: calling it again (e.g. every time the plugin
// process starts) updates the existing provider instead of duplicating it.
type RegisterPluginRequest struct {
	Name     string `json:"name" binding:"required" description:"Plugin / provider name" example:"my-rag"`
	Endpoint string `json:"endpoint" binding:"required" description:"Plugin base URL" example:"http://127.0.0.1:8765/v1"`
	ModelID  string `json:"model_id,omitempty" description:"Model id the plugin advertises" example:"plugin/my-rag"`
	Token    string `json:"token,omitempty" description:"Token tingly-box should send to the plugin (empty = no key)"`
	Scenario string `json:"scenario,omitempty" description:"Scenario to bind a rule under; omit to create only the provider" example:"experiment"`
	Tier     int    `json:"tier,omitempty" description:"Tier for the bound service (0 = highest priority)"`
	APIStyle string `json:"api_style,omitempty" description:"Wire protocol the plugin's endpoint speaks: \"openai\" or \"anthropic\"; empty defaults to \"openai\"" example:"anthropic"`
}

// RegisterPluginResponse reports what was created or updated.
type RegisterPluginResponse struct {
	ProviderUUID string `json:"provider_uuid"`
	ModelID      string `json:"model_id"`
	Scenario     string `json:"scenario,omitempty"`
	RuleUUID     string `json:"rule_uuid,omitempty"`
	// Ready is true when a rule is bound, so clients can select the model now.
	Ready bool   `json:"ready"`
	Note  string `json:"note,omitempty"`
}

// PluginInfo is a list view of a plugin provider.
type PluginInfo struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	ModelID  string `json:"model_id,omitempty"`
}

// PluginsResponse wraps the plugin list.
type PluginsResponse struct {
	Success bool         `json:"success"`
	Data    []PluginInfo `json:"data"`
}
