package quota

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ProviderType 供应商类型枚举
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
	ProviderTypeOpenRouter ProviderType = "openrouter"
	ProviderTypeMiniMax    ProviderType = "minimax"
	ProviderTypeMiniMaxCN  ProviderType = "minimaxi"
	ProviderTypeCursor     ProviderType = "cursor"
	ProviderTypeCodex      ProviderType = "codex"
)

// WindowType 窗口类型枚举
type WindowType string

const (
	WindowTypeSession WindowType = "session" // 会话配额（小时级）
	WindowTypeDaily   WindowType = "daily"   // 日配额
	WindowTypeWeekly  WindowType = "weekly"  // 周配额
	WindowTypeMonthly WindowType = "monthly" // 月配额
	WindowTypeCustom  WindowType = "custom"  // 自定义窗口
	WindowTypeBalance WindowType = "balance" // 余额/积分
)

// UsageUnit 用量单位枚举
type UsageUnit string

const (
	UsageUnitTokens   UsageUnit = "tokens"   // Token 数量
	UsageUnitRequests UsageUnit = "requests" // 请求次数
	UsageUnitCredits  UsageUnit = "credits"  // 积分
	UsageUnitCurrency UsageUnit = "currency" // 货币
)

// ProviderUsage 表示供应商配额快照
type ProviderUsage struct {
	// 基础信息
	ProviderUUID string       `json:"provider_uuid"` // 关联的 Provider UUID
	ProviderName string       `json:"provider_name"` // 供应商名称
	ProviderType ProviderType `json:"provider_type"` // 供应商类型
	FetchedAt    time.Time    `json:"fetched_at"`    // 获取时间
	ExpiresAt    time.Time    `json:"expires_at"`    // 数据过期时间

	// 配额窗口（最多支持 3 个级别：primary, secondary, tertiary）
	Primary   *UsageWindow `json:"primary,omitempty"`
	Secondary *UsageWindow `json:"secondary,omitempty"`
	Tertiary  *UsageWindow `json:"tertiary,omitempty"`

	// 费用信息（如月度费用）
	Cost *UsageCost `json:"cost,omitempty"`

	// 账户信息（可选，用于显示用户身份）
	Account *UsageAccount `json:"account,omitempty"`

	// 分组明细（如按模型、按区域等）
	Breakdowns []*UsageBreakdown `json:"breakdowns,omitempty"`

	// 错误信息（如果获取失败）
	LastError   string     `json:"last_error,omitempty"`
	LastErrorAt *time.Time `json:"last_error_at,omitempty"`
}

// UsageBreakdown 表示配额的分组明细（如按模型、按区域）
type UsageBreakdown struct {
	// 分组标识
	Key   string `json:"key"`   // 分组键，如模型名称 "gpt-4"、区域 "us-east-1"
	Label string `json:"label"` // 显示标签
	Group string `json:"group"` // 分组类型，如 "model", "region", "tier"

	// 配额数据
	Windows []*UsageWindow `json:"windows"` // 该分组的多维度配额窗口

	// 可选的元数据
	Description string `json:"description,omitempty"`
}

// UsageWindow 表示一个配额窗口
type UsageWindow struct {
	// 窗口类型标识
	Type WindowType `json:"type"` // session, weekly, monthly, daily, custom, balance

	// 配额数据
	Used        float64 `json:"used"`         // 已使用量
	Limit       float64 `json:"limit"`        // 配额限制（0 表示无限制）
	UsedPercent float64 `json:"used_percent"` // 使用百分比 (0-100)

	// 时间窗口
	ResetsAt      *time.Time `json:"resets_at,omitempty"`      // 重置时间（如果已知）
	WindowMinutes int        `json:"window_minutes,omitempty"` // 时间窗口大小（分钟）

	// 单位信息
	Unit UsageUnit `json:"unit"` // tokens, requests, credits, usd

	// 元数据
	Label       string `json:"label"`       // 显示标签，如 "Session Quota"
	Description string `json:"description"` // 描述信息
}

// UsageCost 表示费用信息
type UsageCost struct {
	Used         float64    `json:"used"`          // 已使用金额
	Limit        float64    `json:"limit"`         // 预算限制（0 表示无限制）
	CurrencyCode string     `json:"currency_code"` // 货币代码，如 USD
	ResetsAt     *time.Time `json:"resets_at,omitempty"`
	Label        string     `json:"label"` // 显示标签
}

// UsageAccount 表示账户信息
type UsageAccount struct {
	ID             string `json:"id"`              // 账户 ID
	Name           string `json:"name"`            // 账户名称
	Email          string `json:"email,omitempty"` // 账户邮箱
	Tier           string `json:"tier,omitempty"`  // 账户层级
	OrganizationID string `json:"organization_id,omitempty"`
}

// CalculateUsedPercent 计算使用百分比
func (w *UsageWindow) CalculateUsedPercent() float64 {
	if w.Limit <= 0 {
		return 0
	}
	if w.Used >= w.Limit {
		return 100
	}
	return (w.Used / w.Limit) * 100
}

// IsExpired 检查配额数据是否过期
func (p *ProviderUsage) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

// GetErrorStatus 获取错误状态
func (p *ProviderUsage) GetErrorStatus() ErrorStatus {
	if p.LastError == "" {
		return ErrorStatusOK
	}
	return ErrorStatusError
}

// ErrorStatus 错误状态
type ErrorStatus string

const (
	ErrorStatusOK    ErrorStatus = "ok"
	ErrorStatusError ErrorStatus = "error"
)

// ProviderUsageConfig 单个供应商的配额配置
type ProviderUsageConfig struct {
	Enabled        bool   `json:"enabled"`         // 是否启用此供应商
	Priority       int    `json:"priority"`        // 优先级（用于排序）
	CustomEndpoint string `json:"custom_endpoint"` // 自定义 API 端点
	FetcherType    string `json:"fetcher_type"`    // Fetcher 类型
}

// Config 配额获取配置
type Config struct {
	// 全局设置
	Enabled         bool                           `json:"enabled"`          // 是否启用配额追踪
	RefreshInterval time.Duration                  `json:"refresh_interval"` // 刷新间隔（默认 5 分钟）
	CacheTTL        time.Duration                  `json:"cache_ttl"`        // 缓存有效期（默认 10 分钟）
	RetryOnFailure  bool                           `json:"retry_on_failure"` // 失败时是否重试
	MaxRetries      int                            `json:"max_retries"`      // 最大重试次数
	Providers       map[string]ProviderUsageConfig `json:"providers"`        // 供应商特定配置
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Enabled:         true,
		RefreshInterval: 5 * time.Minute,
		CacheTTL:        10 * time.Minute,
		RetryOnFailure:  true,
		MaxRetries:      3,
		Providers:       make(map[string]ProviderUsageConfig),
	}
}

// NullableFloat64 用于 GORM 处理可为空的 float64
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

// NullableTime 用于 GORM 处理可为空的 time.Time
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
