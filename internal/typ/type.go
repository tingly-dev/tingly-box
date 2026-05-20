package typ

import (
	"encoding/json"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// FlexibleBool is a boolean type that can unmarshal from both bool and int (0/1)
// This handles cases where JSON data may contain numeric values instead of booleans
type FlexibleBool bool

// UnmarshalJSON implements json.Unmarshaler for FlexibleBool
func (fb *FlexibleBool) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as boolean first
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		*fb = FlexibleBool(b)
		return nil
	}

	// Try to unmarshal as number (0 or 1)
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		*fb = FlexibleBool(n != 0)
		return nil
	}

	// Try to unmarshal as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*fb = FlexibleBool(s == "true" || s == "1")
		return nil
	}

	// If all attempts fail, use false as default
	*fb = false
	return nil
}

// MarshalJSON implements json.Marshaler for FlexibleBool
func (fb FlexibleBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(fb))
}

// RuleScenario represents the scenario for a routing rule
type RuleScenario string

const (
	ScenarioOpenAI        RuleScenario = "openai"
	ScenarioAnthropic     RuleScenario = "anthropic"
	ScenarioAgent         RuleScenario = "agent"
	ScenarioCodex         RuleScenario = "codex"
	ScenarioClaudeCode    RuleScenario = "claude_code"
	ScenarioOpenCode      RuleScenario = "opencode"
	ScenarioXcode         RuleScenario = "xcode"
	ScenarioVSCode        RuleScenario = "vscode"
	ScenarioClaudeDesktop RuleScenario = "claude_desktop"
	ScenarioSmartGuide    RuleScenario = "_smart_guide"
	ScenarioGlobal        RuleScenario = "_global"  // Global flags that apply to all scenarios
	ScenarioEmbed         RuleScenario = "embed"    // Embedding application scenario; only serves /embeddings
	ScenarioImageGen      RuleScenario = "imagegen" // Image generation scenario; only serves /images/generations
)

func BuiltinScenarios() []RuleScenario {
	return []RuleScenario{
		ScenarioOpenAI,
		ScenarioAnthropic,
		ScenarioAgent,
		ScenarioCodex,
		ScenarioClaudeCode,
		ScenarioOpenCode,
		ScenarioXcode,
		ScenarioVSCode,
		ScenarioClaudeDesktop,
		ScenarioSmartGuide,
		ScenarioGlobal,
		ScenarioEmbed,
		ScenarioImageGen,
	}
}

// ThinkingEffortLevel represents the thinking effort level for extended thinking
type ThinkingEffortLevel = string

const (
	ThinkingEffortLow     ThinkingEffortLevel = "low"
	ThinkingEffortMedium  ThinkingEffortLevel = "medium"
	ThinkingEffortHigh    ThinkingEffortLevel = "high"
	ThinkingEffortMax     ThinkingEffortLevel = "max"
	ThinkingEffortDefault ThinkingEffortLevel = "" // Use model default
)

// ThinkingBudgetMapping defines budget_tokens for each effort level
// Note: Default max is 31,999 tokens per Claude Code documentation
var ThinkingBudgetMapping = map[ThinkingEffortLevel]int64{
	ThinkingEffortDefault: 31999,
	ThinkingEffortLow:     1024,  // ~1K tokens - minimal reasoning (minimum allowed)
	ThinkingEffortMedium:  5120,  // ~5K tokens - balanced
	ThinkingEffortHigh:    20480, // ~20K tokens - deep reasoning
	ThinkingEffortMax:     31999, // ~32K tokens - maximum (default)
}

// ThinkingMode represents the thinking mode for extended thinking
type ThinkingMode string

const (
	ThinkingModeDefault  ThinkingMode = "default"  // Use model default
	ThinkingModeAdaptive ThinkingMode = "adaptive" // Model decides when to use
	ThinkingModeForce    ThinkingMode = "force"    // Force for all requests
)

// RecordingMode represents the recording mode for scenario recording
type RecordingMode string

const (
	RecordingModeDisabled              RecordingMode = ""                        // Recording disabled (default)
	RecordingModeRequestOnly           RecordingMode = "request"                 // Record transformed request only
	RecordingModeRequestResponse       RecordingMode = "request_response"        // Record transformed request + final response
	RecordingModeStagedRequestResponse RecordingMode = "staged_request_response" // Record original request + transformed request + final response
)

// IsValidRecordingMode checks if the given string is a valid recording mode
func IsValidRecordingMode(mode string) bool {
	switch RecordingMode(mode) {
	case RecordingModeDisabled, RecordingModeRequestOnly, RecordingModeRequestResponse, RecordingModeStagedRequestResponse:
		return true
	default:
		return false
	}
}

// ScenarioFlags represents configuration flags for a scenario
type ScenarioFlags struct {
	Unified  bool `json:"unified" yaml:"unified"`   // Single configuration for all models
	Separate bool `json:"separate" yaml:"separate"` // Separate configuration for each model
	Smart    bool `json:"smart" yaml:"smart"`       // Smart mode with automatic optimization

	// Experimental feature flags (scenario-based opt-in)
	SmartCompact bool          `json:"smart_compact,omitempty" yaml:"smart_compact,omitempty"`   // Enable smart compact (remove thinking blocks)
	RecordingV2  RecordingMode `json:"recording_v2,omitempty" yaml:"recording_v2,omitempty"`     // Enable scenario recording V2 (request/request_response/staged_request_response)
	Beta         bool          `json:"anthropic_beta,omitempty" yaml:"anthropic_beta,omitempty"` // Enable Anthropic beta features (e.g. extended thinking)

	// Stream configuration flags
	DisableStreamUsage bool `json:"disable_stream_usage,omitempty" yaml:"disable_stream_usage,omitempty"` // Don't include usage in streaming chunks (for incompatible clients like xcode)

	// Thinking effort level (empty string = use model default)
	ThinkingEffort ThinkingEffortLevel `json:"thinking_effort,omitempty" yaml:"thinking_effort,omitempty"`

	// Thinking mode for claude_code scenario (default/adaptive/force)
	// Using string directly instead of ThinkingMode type to avoid naming conflicts
	ThinkingMode string `json:"thinking_mode,omitempty" yaml:"thinking_mode,omitempty"`

	CleanHeader bool `json:"clean_header,omitempty" yaml:"clean_header,omitempty"` // Remove billing header from system messages (Claude Code only)
}

// RuleFlags represents per-rule feature flags.
type RuleFlags struct {
	// CursorCompat enables Cursor compatibility handling (rich content normalization, stream usage stripping, tool gating).
	CursorCompat bool `json:"cursor_compat,omitempty" yaml:"cursor_compat,omitempty"`

	// CursorCompatAuto enables Cursor auto-detection based on request headers.
	CursorCompatAuto bool `json:"cursor_compat_auto,omitempty" yaml:"cursor_compat_auto,omitempty"`

	// SkipUsage strips the `usage` field from both streaming and non-streaming responses.
	SkipUsage bool `json:"skip_usage,omitempty" yaml:"skip_usage,omitempty"`

	// CustomUserAgent overrides the User-Agent header sent to upstream providers.
	// Empty value means do not override.
	CustomUserAgent string `json:"custom_user_agent,omitempty" yaml:"custom_user_agent,omitempty"`

	// UseMaxCompletionTokens rewrites the `max_tokens` request field to `max_completion_tokens`
	// (OpenAI's newer field name for o1/o3/gpt-5 family models).
	UseMaxCompletionTokens bool `json:"use_max_completion_tokens,omitempty" yaml:"use_max_completion_tokens,omitempty"`

	// UseMaxTokens rewrites the `max_completion_tokens` request field back to the legacy
	// `max_tokens` field. Use this for providers or models that reject `max_completion_tokens`.
	UseMaxTokens bool `json:"use_max_tokens,omitempty" yaml:"use_max_tokens,omitempty"`

	// OpenAIEndpointOverride forces the OpenAI endpoint selection (chat or
	// responses), overriding the capability-aware adaptive router. Empty or
	// "auto" preserves adaptive behavior. OpenAI providers only; Anthropic
	// and Google providers ignore this. On Codex OAuth providers, "chat"
	// is silently ignored (Codex has no Chat endpoint) and a warning is logged.
	OpenAIEndpointOverride string `json:"openai_endpoint_override,omitempty" yaml:"openai_endpoint_override,omitempty"`
}

// ProfileMeta stores metadata for a scenario profile.
// Profiles allow multiple Rule + ScenarioFlags configurations per base scenario.
// A profile is identified by a short service-generated ID (e.g. "p1", "p2").
type ProfileMeta struct {
	ID      string `json:"id" yaml:"id"`           // Profile ID (e.g. "p1")
	Name    string `json:"name" yaml:"name"`       // Human-readable name (unique within base scenario)
	Unified bool   `json:"unified" yaml:"unified"` // true=unified mode (single model), false=separate mode (individual models, default)
}

// ScenarioConfig represents configuration for a specific scenario
type ScenarioConfig struct {
	Scenario   RuleScenario           `json:"scenario" yaml:"scenario"`
	Flags      ScenarioFlags          `json:"flags" yaml:"flags"`                               // Scenario configuration flags
	Extensions map[string]interface{} `json:"extensions,omitempty" yaml:"extensions,omitempty"` // Reserved for future extensions
}

// GetDefaultFlags returns default flags for a scenario
func (sc *ScenarioConfig) GetDefaultFlags() ScenarioFlags {
	if sc.Flags.Unified || sc.Flags.Separate || sc.Flags.Smart {
		return sc.Flags
	}
	// Default to unified if no flag is set
	return ScenarioFlags{Unified: true}
}

// AuthType represents the authentication type for a provider
// Type alias for backward compatibility with common/provider
type AuthType = ai.AuthType

// AuthType constants - re-exported for backward compatibility
const (
	AuthTypeAPIKey    = ai.AuthTypeAPIKey
	AuthTypeOAuth     = ai.AuthTypeOAuth
	AuthTypeVirtual   = ai.AuthTypeVirtual
	AuthTypeAWSSigV4  = ai.AuthTypeAWSSigV4
	AuthTypeAzureKey  = ai.AuthTypeAzureKey
	AuthTypeGCPVertex = ai.AuthTypeGCPVertex
)

// ProviderSource constants - re-exported for backward compatibility
type ProviderSource = ai.ProviderSource

const (
	ProviderSourceUser    = ai.ProviderSourceUser
	ProviderSourceBuiltin = ai.ProviderSourceBuiltin
)

// OAuthDetail contains OAuth-specific authentication information
// Type alias for backward compatibility with common/provider
type OAuthDetail = ai.OAuthDetail

// VModelDetail contains virtual-model provider configuration
// Type alias for backward compatibility with common/provider
type VModelDetail = ai.VModelDetail

// CredentialBundle holds multi-field credentials for non-bearer auth types
// Type alias for backward compatibility with common/provider
type CredentialBundle = ai.CredentialBundle

// MCPMode defines MCP runtime mode
type MCPMode string

const (
	MCPModeServertool MCPMode = "servertool" // servertool mode: tingly-box connects to external MCP servers and injects tools into AI requests
	MCPModeClienttool MCPMode = "clienttool" // clienttool mode (default): external clients connect to tingly-box
)

// MCPConnectionType defines connection type
type MCPConnectionType string

const (
	MCPConnectionTypeSTDIO MCPConnectionType = "stdio"
	MCPConnectionTypeHTTP  MCPConnectionType = "http"
	MCPConnectionTypeSSE   MCPConnectionType = "sse"
)

// MCPAuthType defines authentication type
type MCPAuthType string

const (
	MCPAuthTypeNone   MCPAuthType = "none"
	MCPAuthTypeHeader MCPAuthType = "headers"
	MCPAuthTypeOAuth  MCPAuthType = "oauth"
)

type ToolVisibility = coretool.ToolVisibility

const (
	ToolVisibilityClient = coretool.ToolVisibilityClient
	ToolVisibilityServer = coretool.ToolVisibilityServer
)

type ToolImplementation string

const (
	ToolImplementationMCP     ToolImplementation = "mcp"
	ToolImplementationVirtual ToolImplementation = "virtual"
)

type ToolProvider string

const (
	ToolProviderBuiltin ToolProvider = "builtin"
	ToolProviderCustom  ToolProvider = "custom"
)

type ToolDescriptor struct {
	Name           string             `json:"name"`
	SourceID       string             `json:"source_id"`
	Visibility     ToolVisibility     `json:"visibility"`
	Implementation ToolImplementation `json:"implementation"`
	Provider       ToolProvider       `json:"provider"`
	Description    string             `json:"description,omitempty"`
}

// MCPClientState defines client connection state
type MCPClientState string

const (
	MCPClientStateConnected    MCPClientState = "connected"
	MCPClientStateConnecting   MCPClientState = "connecting"
	MCPClientStateDisconnected MCPClientState = "disconnected"
	MCPClientStateError        MCPClientState = "error"
)

// MCPRuntimeConfig contains global MCP runtime configuration.
type MCPRuntimeConfig struct {
	Mode                  MCPMode           `json:"mode,omitempty"` // deprecated: kept only for backward compatibility
	Sources               []MCPSourceConfig `json:"sources,omitempty"`
	RequestTimeout        int               `json:"request_timeout,omitempty"`          // seconds, default: 30
	StripDisabledMCPTools bool              `json:"strip_disabled_mcp_tools,omitempty"` // dangerous: strip disabled MCP declarations/tool_calls
}

// MCPSourceConfig defines one MCP source connection.
type MCPSourceConfig struct {
	ID         string            `json:"id,omitempty"`         // unique source id for normalized tool names
	Name       string            `json:"name,omitempty"`       // client name (unique, no spaces/hyphens)
	Enabled    *bool             `json:"enabled,omitempty"`    // nil means enabled (backward-compatible default)
	Transport  string            `json:"transport,omitempty"`  // "http", "stdio", or "sse"
	Endpoint   string            `json:"endpoint,omitempty"`   // endpoint URL for HTTP/SSE transport
	Headers    map[string]string `json:"headers,omitempty"`    // static headers for MCP calls
	Tools      []string          `json:"tools,omitempty"`      // allow list, empty means all
	Command    string            `json:"command,omitempty"`    // command for stdio transport
	Args       []string          `json:"args,omitempty"`       // args for stdio command
	Cwd        string            `json:"cwd,omitempty"`        // working directory for stdio command
	Env        map[string]string `json:"env,omitempty"`        // extra env vars for stdio command
	ProxyURL   string            `json:"proxy_url,omitempty"`  // HTTP proxy URL for outgoing requests
	Visibility ToolVisibility    `json:"visibility,omitempty"` // "client" or "server"

	// Local mode specific fields
	ConnectionType      MCPConnectionType `json:"connection_type,omitempty"`       // stdio/http/sse
	AuthType            MCPAuthType       `json:"auth_type,omitempty"`             // headers/oauth
	AllowedExtraHeaders []string          `json:"allowed_extra_headers,omitempty"` // allowed request headers to forward
	StdioConfig         *MCPStdioConfig   `json:"stdio_config,omitempty"`
	OAuthConfig         *MCPOAuthConfig   `json:"oauth_config,omitempty"`
	ToolsToExecute      []string          `json:"tools_to_execute,omitempty"`      // available tools
	ToolsAutoExec       []string          `json:"tools_to_auto_execute,omitempty"` // auto-execute tools (agent mode)
	IsPingAvailable     *bool             `json:"is_ping_available,omitempty"`     // health check method
	AutoRegistered      bool              `json:"auto_registered,omitempty"`       // true if auto-registered on first connect
	Advisor             *AdvisorConfig    `json:"advisor,omitempty" yaml:"advisor,omitempty"`
}

// AdvisorConfig configures the in-process advisor tool source.
type AdvisorConfig struct {
	// ProviderUUID references a configured provider by UUID.
	ProviderUUID string `json:"provider_uuid,omitempty" yaml:"provider_uuid,omitempty"`
	Model        string `json:"model,omitempty" yaml:"model,omitempty"`

	// ProviderResolver is a function that resolves a provider by UUID at call time.
	// It is not persisted to JSON/YAML and must be set by the server before use.
	ProviderResolver func(string) (*Provider, error) `json:"-" yaml:"-"`

	MaxUsesPerRequest int `json:"max_uses_per_request,omitempty" yaml:"max_uses_per_request,omitempty"`
	// The max token output by adviser. Too much explodes worker's context. 4k is enough for pure suggestions.
	MaxTokens int `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	// TimeoutSeconds overrides the default 60s per-call timeout. Set higher for slow/large models.
	TimeoutSeconds int `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
}

// MCPStdioConfig STDIO connection configuration
type MCPStdioConfig struct {
	Command string   `json:"command"`        // execution command
	Args    []string `json:"args,omitempty"` // command arguments
	Env     []string `json:"env,omitempty"`  // inherited environment variables
	Cwd     string   `json:"cwd,omitempty"`  // working directory
}

// MCPOAuthConfig OAuth 2.0 configuration
type MCPOAuthConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	AuthorizeURL string   `json:"authorize_url"`
	TokenURL     string   `json:"token_url"`
	Scopes       []string `json:"scopes,omitempty"`
}

// MCPTool represents an MCP tool definition
type MCPTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// MCPClient represents a registered MCP client
type MCPClient struct {
	ID     string          `json:"id"`
	Config MCPSourceConfig `json:"config"`
	Tools  []MCPTool       `json:"tools"`
	State  MCPClientState  `json:"state"`
}

// ApplyMCPRuntimeDefaults applies default values to MCP runtime config.
func ApplyMCPRuntimeDefaults(config *MCPRuntimeConfig) {
	if config == nil {
		return
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30
	}
	for i := range config.Sources {
		if config.Sources[i].Enabled == nil {
			config.Sources[i].Enabled = BoolPtr(true)
		}
		if config.Sources[i].Visibility == "" {
			config.Sources[i].Visibility = ToolVisibilityClient
			if config.Sources[i].ID == "advisor" || config.Sources[i].Transport == "advisor" || config.Sources[i].Advisor != nil {
				config.Sources[i].Visibility = ToolVisibilityServer
			}
		}
		// Apply defaults for in-process advisor source.
		if config.Sources[i].Transport == "advisor" || config.Sources[i].Advisor != nil {
			if config.Sources[i].Advisor == nil {
				config.Sources[i].Advisor = &AdvisorConfig{}
			}
			if config.Sources[i].Advisor.MaxUsesPerRequest <= 0 {
				config.Sources[i].Advisor.MaxUsesPerRequest = 3
			}
			if config.Sources[i].Advisor.MaxTokens <= 0 {
				config.Sources[i].Advisor.MaxTokens = 4096
			}
		}
	}
}

// BoolPtr returns a pointer to the given bool.
func BoolPtr(v bool) *bool {
	return &v
}

// IsMCPSourceEnabled returns whether a source is enabled.
// Nil means enabled for backward compatibility with existing configs.
func IsMCPSourceEnabled(source MCPSourceConfig) bool {
	return source.Enabled == nil || *source.Enabled
}

// Provider represents an AI model api key and provider configuration
// Type alias for backward compatibility with common/provider
type Provider = ai.Provider

// Rule represents a request/response configuration with load balancing support
type Rule struct {
	UUID          string                 `json:"uuid"`
	Scenario      RuleScenario           `json:"scenario,required" yaml:"scenario"` // openai, anthropic, claude_code; defaults to openai
	RequestModel  string                 `json:"request_model" yaml:"request_model"`
	ResponseModel string                 `json:"response_model" yaml:"response_model"`
	Description   string                 `json:"description"`
	Services      []*loadbalance.Service `json:"services" yaml:"services"`
	// Per-rule feature flags (e.g. cursor_compat / cursor_compat_auto).
	Flags RuleFlags `json:"flags,omitempty" yaml:"flags,omitempty"`
	// CurrentServiceID is persisted to SQLite, not JSON (provider:model format)
	// This identifies the current service for round-robin load balancing
	CurrentServiceID string `json:"-" yaml:"-"`
	// Unified Tactic Configuration
	LBTactic Tactic `json:"lb_tactic" yaml:"lb_tactic"`
	Active   bool   `json:"active" yaml:"active"`
	// Smart Routing Configuration
	SmartEnabled  bool                        `json:"smart_enabled" yaml:"smart_enabled"`
	SmartAffinity bool                        `json:"smart_affinity,omitempty" yaml:"smart_affinity,omitempty"`
	SmartRouting  []smartrouting.SmartRouting `json:"smart_routing,omitempty" yaml:"smart_routing,omitempty"`
}

// ToJSON implementation
func (r *Rule) ToJSON() interface{} {
	// Ensure Services is populated
	services := r.GetServices()

	// Create the JSON representation (note: current_service_index is persisted to SQLite, not JSON)
	jsonRule := map[string]interface{}{
		"uuid":           r.UUID,
		"scenario":       r.GetScenario(),
		"request_model":  r.RequestModel,
		"response_model": r.ResponseModel,
		"description":    r.Description,
		"services":       services,
		"lb_tactic":      r.LBTactic,
		"active":         r.Active,
		"smart_enabled":  r.SmartEnabled,
		"smart_affinity": r.SmartAffinity,
		"smart_routing":  r.SmartRouting,
	}

	return jsonRule
}

// GetServices returns the services to use for this rule
func (r *Rule) GetServices() []*loadbalance.Service {
	if r.Services == nil {
		r.Services = []*loadbalance.Service{}
	}
	return r.Services
}

// GetScenario returns the scenario, defaulting to openai if empty
func (r *Rule) GetScenario() RuleScenario {
	return r.Scenario
}

// GetDefaultProvider returns the provider from the currently selected service using load balancing tactic
func (r *Rule) GetDefaultProvider() string {
	service := r.GetCurrentService()
	if service != nil {
		return service.Provider
	}
	return ""
}

// GetDefaultModel returns the model from the currently selected service using load balancing tactic
func (r *Rule) GetDefaultModel() string {
	service := r.GetCurrentService()
	if service != nil {
		return service.Model
	}
	return ""
}

// GetActiveServices returns all active services with initialized stats
func (r *Rule) GetActiveServices() []*loadbalance.Service {
	var activeServices []*loadbalance.Service
	for i := range r.Services {
		if r.Services[i].Active {
			r.Services[i].InitializeStats()
			activeServices = append(activeServices, r.Services[i])
		}
	}
	return activeServices
}

// GetTacticType returns the load balancing tactic type
func (r *Rule) GetTacticType() loadbalance.TacticType {
	if r.LBTactic.Type != 0 {
		return r.LBTactic.Type
	}
	// Default to random
	return loadbalance.TacticRandom
}

// GetUUID returns the rule UUID
func (r *Rule) GetUUID() string {
	return r.UUID
}

// SetCurrentServiceID sets the current service ID (used by RuleStateStore hydration)
func (r *Rule) SetCurrentServiceID(serviceID string) {
	r.CurrentServiceID = serviceID
}

// GetCurrentServiceID returns the current service ID
func (r *Rule) GetCurrentServiceID() string {
	return r.CurrentServiceID
}

// GetCurrentService returns the current active service based on CurrentServiceID
func (r *Rule) GetCurrentService() *loadbalance.Service {
	activeServices := r.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	// If CurrentServiceID is set, find and return that service
	if r.CurrentServiceID != "" {
		for _, svc := range activeServices {
			if svc.ServiceID() == r.CurrentServiceID && svc.Active {
				return svc
			}
		}
	}

	// Default to first service if CurrentServiceID not found or not set
	return activeServices[0]
}
