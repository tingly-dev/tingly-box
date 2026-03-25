package bot

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Agent routing constants
const (
	agentTinglyBox  agentboot.AgentType = "tingly-box" // @tb - Smart Guide (default)
	agentClaudeCode agentboot.AgentType = agentboot.AgentTypeClaude
	agentMock       agentboot.AgentType = agentboot.AgentTypeMockAgent
)

var defaultBashAllowlist = map[string]struct{}{
	"cd":  {},
	"ls":  {},
	"pwd": {},
}

// Platforms that do NOT support verbose mode (can only receive final messages)
var nonVerbosePlatforms = map[string]struct{}{
	"weixin": {},
}

// Platforms with low-frequency sending limitations (need rate limiting)
var lowFrequencyPlatforms = map[string]struct{}{
	"weixin": {},
}

// ResponseMeta contains metadata for response formatting
type ResponseMeta struct {
	ProjectPath string
	ChatID      string
	UserID      string
	SessionID   string
	AgentType   string // Current agent identifier (e.g., "tingly-box", "claude")
}

// SettingsStore defines the interface for bot settings storage
// This allows both the legacy bot.Store and the new db.ImBotSettingsStore to be used
type SettingsStore interface {
	// GetSettingsByUUIDInterface returns settings by UUID as interface{}
	GetSettingsByUUIDInterface(uuid string) (interface{}, error)
	// ListEnabledSettingsInterface returns all enabled settings as interface{}
	ListEnabledSettingsInterface() (interface{}, error)
}

// Lifecycle defines the interface for controlling bot lifecycle
// This allows the API layer to control bot startup/shutdown without direct dependency on the Manager type
type Lifecycle interface {
	// Start starts a bot by UUID
	Start(ctx context.Context, uuid string) error
	// Stop stops a bot by UUID
	Stop(uuid string)
	// IsRunning checks if a bot is running
	IsRunning(uuid string) bool
	// Sync ensures running bots match the enabled settings
	Sync(ctx context.Context) error
}

// runningBot tracks a running bot instance
type runningBot struct {
	cancel   context.CancelFunc
	stopped  bool          // marker to indicate if bot is being stopped
	doneChan chan struct{} // closed when goroutine finishes
}

// SupportsVerboseMode checks if the platform supports verbose mode
// Some platforms (e.g., Weixin) can only receive final messages, not intermediate ones
func SupportsVerboseMode(platform string) bool {
	_, nonVerbose := nonVerbosePlatforms[platform]
	return !nonVerbose
}

// IsLowFrequencyPlatform checks if the platform has low-frequency sending limitations
// Some platforms (e.g., Weixin) need rate limiting for message sending
func IsLowFrequencyPlatform(platform string) bool {
	_, isLowFreq := lowFrequencyPlatforms[platform]
	return isLowFreq
}
