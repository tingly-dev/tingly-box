package quota

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ProviderType provider type enumeration
type ProviderType string

const (
	ProviderTypeAnthropic  ProviderType = "anthropic"
	ProviderTypeOpenAI     ProviderType = "openai"
	ProviderTypeGoogle     ProviderType = "google"
	ProviderTypeGemini     ProviderType = "gemini"
	ProviderTypeCopilot    ProviderType = "copilot"
	ProviderTypeVertexAI   ProviderType = "vertex_ai"
	ProviderTypeZai        ProviderType = "zai"
	ProviderTypeGLM        ProviderType = "glm"
	ProviderTypeKimiK2     ProviderType = "kimi_k2"
	ProviderTypeKimiCode   ProviderType = "kimi_code"
	ProviderTypeOpenRouter ProviderType = "openrouter"
	ProviderTypeMiniMax    ProviderType = "minimax"
	ProviderTypeMiniMaxCN  ProviderType = "minimaxi"
	ProviderTypeCursor     ProviderType = "cursor"
	ProviderTypeCodex      ProviderType = "codex"
)

// WindowType window type enumeration
type WindowType string

const (
	WindowTypeSession    WindowType = "session"     // session quota (hourly)
	WindowTypeDaily      WindowType = "daily"       // daily quota
	WindowTypeWeekly     WindowType = "weekly"      // weekly quota
	WindowTypeMonthly    WindowType = "monthly"     // monthly quota
	WindowTypeCustom     WindowType = "custom"      // custom window
	WindowTypeBalance    WindowType = "balance"     // balance/credits
	WindowTypeModel      WindowType = "model"       // model-specific quota
	WindowTypeCodeReview WindowType = "code_review" // code review quota
)

// UsageUnit usage unit enumeration
type UsageUnit string

const (
	UsageUnitTokens   UsageUnit = "tokens"   // token count
	UsageUnitRequests UsageUnit = "requests" // request count
	UsageUnitCredits  UsageUnit = "credits"  // credits
	UsageUnitCurrency UsageUnit = "currency" // currency
	UsageUnitPercent  UsageUnit = "percent"  // percentage (0-100)
)

// ProviderUsage represents a provider quota snapshot
type ProviderUsage struct {
	// Basic information
	ProviderUUID string       `json:"provider_uuid"` // associated provider UUID
	ProviderName string       `json:"provider_name"` // provider name
	ProviderType ProviderType `json:"provider_type"` // provider type
	FetchedAt    time.Time    `json:"fetched_at"`    // fetch time
	ExpiresAt    time.Time    `json:"expires_at"`    // data expiration time

	// Normalized quota windows for frontend display. Lower tier values are more important and displayed first.
	Windows []*UsageWindow `json:"windows,omitempty"`

	// Cost information (e.g., monthly costs)
	Cost *UsageCost `json:"cost,omitempty"`

	// Account information (optional, for displaying user identity)
	Account *UsageAccount `json:"account,omitempty"`

	// Group breakdowns (e.g., by model, by region)
	Breakdowns []*UsageBreakdown `json:"breakdowns,omitempty"`

	// Error information (if fetch failed)
	LastError   string     `json:"last_error,omitempty"`
	LastErrorAt *time.Time `json:"last_error_at,omitempty"`

	// Raw response data (for debugging and review)
	RawResponse json.RawMessage `json:"raw_response,omitempty"`
}

// UsageBreakdown represents grouped quota breakdowns (e.g., by model, by region)
type UsageBreakdown struct {
	// Group identifiers
	Key   string `json:"key"`   // group key, e.g., model name "gpt-4", region "us-east-1"
	Label string `json:"label"` // display label
	Group string `json:"group"` // group type, e.g., "model", "region", "tier"

	// Quota data
	Windows []*UsageWindow `json:"windows"` // multi-dimensional quota windows for this group

	// Optional metadata
	Description string `json:"description,omitempty"`
}

// UsageWindow represents a single quota window
type UsageWindow struct {
	// Display key and priority tier. Lower tier values are more important and displayed first.
	Key  string `json:"key,omitempty"`
	Tier int    `json:"tier,omitempty"`

	// Window type identifier
	Type WindowType `json:"type"` // session, weekly, monthly, daily, custom, balance

	// Quota data
	Used        float64 `json:"used"`         // used amount
	Limit       float64 `json:"limit"`        // quota limit (0 means unlimited)
	UsedPercent float64 `json:"used_percent"` // usage percentage (0-100)

	// Time window
	ResetsAt      *time.Time `json:"resets_at,omitempty"`      // reset time (if known)
	WindowMinutes int        `json:"window_minutes,omitempty"` // time window size (minutes)

	// Unit information
	Unit UsageUnit `json:"unit"` // tokens, requests, credits, usd

	// Metadata
	Label       string `json:"label"`       // display label, e.g., "Session Quota"
	Description string `json:"description"` // description

	// Limit status (optional)
	Allowed      *bool `json:"allowed,omitempty"`       // whether requests are allowed
	LimitReached *bool `json:"limit_reached,omitempty"` // whether limit is reached
}

// UsageCost represents cost information
type UsageCost struct {
	Used         float64    `json:"used"`          // amount used
	Limit        float64    `json:"limit"`         // budget limit (0 means unlimited)
	CurrencyCode string     `json:"currency_code"` // currency code, e.g., USD
	ResetsAt     *time.Time `json:"resets_at,omitempty"`
	Label        string     `json:"label"` // display label
}

// UsageAccount represents account information
type UsageAccount struct {
	ID             string `json:"id"`              // account ID
	Name           string `json:"name"`            // account name
	Email          string `json:"email,omitempty"` // account email
	Tier           string `json:"tier,omitempty"`  // account tier
	OrganizationID string `json:"organization_id,omitempty"`

	// Spend control status (optional)
	SpendControlReached bool `json:"spend_control_reached,omitempty"` // whether spend control limit is reached
}

// CalculateUsedPercent calculates usage percentage
func (w *UsageWindow) CalculateUsedPercent() float64 {
	if w.Limit <= 0 {
		return 0
	}
	if w.Used >= w.Limit {
		return 100
	}
	return (w.Used / w.Limit) * 100
}

// NormalizeWindows ensures Windows contains all aggregate quota windows in display order.
// Lower tier means more important and is rendered first.
func (p *ProviderUsage) NormalizeWindows() {
	if p == nil {
		return
	}

	for i, window := range p.Windows {
		applyWindowDefaults(window, fmt.Sprintf("window_%d", i))
	}

	sort.SliceStable(p.Windows, func(i, j int) bool {
		left, right := p.Windows[i], p.Windows[j]
		if left == nil && right == nil {
			return false
		}
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return left.Tier < right.Tier
	})
}

func applyWindowDefaults(window *UsageWindow, key string) {
	if window == nil {
		return
	}
	if window.Key == "" {
		window.Key = key
	}
	if window.UsedPercent == 0 {
		window.UsedPercent = window.CalculateUsedPercent()
	}
}

// AddWindow adds a quota window with canonical display metadata.
func (p *ProviderUsage) AddWindow(key string, tier int, window *UsageWindow) *UsageWindow {
	if p == nil || window == nil {
		return nil
	}
	window.Key = key
	window.Tier = tier
	if window.UsedPercent == 0 {
		window.UsedPercent = window.CalculateUsedPercent()
	}
	p.Windows = append(p.Windows, window)
	return window
}

// IsExpired checks if quota data is expired
func (p *ProviderUsage) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

// GetErrorStatus gets error status
func (p *ProviderUsage) GetErrorStatus() ErrorStatus {
	if p.LastError == "" {
		return ErrorStatusOK
	}
	return ErrorStatusError
}

// ErrorStatus error status
type ErrorStatus string

const (
	ErrorStatusOK    ErrorStatus = "ok"
	ErrorStatusError ErrorStatus = "error"
)

// ProviderUsageConfig single provider quota configuration
type ProviderUsageConfig struct {
	Enabled        bool   `json:"enabled"`         // whether this provider is enabled
	Priority       int    `json:"priority"`        // priority (for sorting)
	CustomEndpoint string `json:"custom_endpoint"` // custom API endpoint
	FetcherType    string `json:"fetcher_type"`    // Fetcher type
}

// Config quota fetch configuration
type Config struct {
	// Global settings
	Enabled         bool                           `json:"enabled"`          // whether quota tracking is enabled
	RefreshInterval time.Duration                  `json:"refresh_interval"` // refresh interval (default 5 minutes)
	CacheTTL        time.Duration                  `json:"cache_ttl"`        // cache validity period (default 10 minutes)
	RetryOnFailure  bool                           `json:"retry_on_failure"` // whether to retry on failure
	MaxRetries      int                            `json:"max_retries"`      // maximum retry attempts
	Providers       map[string]ProviderUsageConfig `json:"providers"`        // provider-specific configuration
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:         true,
		RefreshInterval: 15 * time.Minute,
		CacheTTL:        20 * time.Minute,
		RetryOnFailure:  true,
		MaxRetries:      3,
		Providers:       make(map[string]ProviderUsageConfig),
	}
}

// NullableFloat64 for GORM to handle nullable float64
type NullableFloat64 struct {
	Float64 float64
	Valid   bool
}

// Value implements the driver.Valuer interface
func (n NullableFloat64) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Float64, nil
}

// Scan implements the sql.Scanner interface
func (n *NullableFloat64) Scan(value interface{}) error {
	if value == nil {
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case float64:
		n.Float64, n.Valid = v, true
	case float32:
		n.Float64, n.Valid = float64(v), true
	case int64:
		n.Float64, n.Valid = float64(v), true
	case int32:
		n.Float64, n.Valid = float64(v), true
	default:
		return fmt.Errorf("unsupported type: %T", value)
	}
	return nil
}

// MarshalJSON implements json.Marshaler
func (n NullableFloat64) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Float64)
}

// UnmarshalJSON implements json.Unmarshaler
func (n *NullableFloat64) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		n.Valid = false
		return nil
	}
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	n.Float64 = f
	n.Valid = true
	return nil
}

// NullableTime for GORM to handle nullable time.Time
type NullableTime struct {
	Time  time.Time
	Valid bool
}

// Value implements the driver.Valuer interface
func (n NullableTime) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Time, nil
}

// Scan implements the sql.Scanner interface
func (n *NullableTime) Scan(value interface{}) error {
	if value == nil {
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		n.Time, n.Valid = v, true
	case []byte:
		t, err := time.Parse(time.RFC3339, string(v))
		if err != nil {
			return err
		}
		n.Time, n.Valid = t, true
	case string:
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		n.Time, n.Valid = t, true
	default:
		return fmt.Errorf("unsupported type: %T", value)
	}
	return nil
}

// MarshalJSON implements json.Marshaler
func (n NullableTime) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.Time)
}

// UnmarshalJSON implements json.Unmarshaler
func (n *NullableTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		n.Valid = false
		return nil
	}
	var t time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	n.Time = t
	n.Valid = true
	return nil
}
