package config

// ScenarioConfig.Extensions keys. These are the canonical string keys used
// when reading or writing feature toggles stored in the Extensions map
// (as opposed to typed ScenarioFlags fields).
const (
	ExtensionVisionProxyService = "vision_proxy_service"
	ExtensionGuardrails         = "guardrails"
	ExtensionMCP                = "mcp"
	ExtensionSkillUser          = "skill_user"
	ExtensionSkillIDE           = "skill_ide"
)
