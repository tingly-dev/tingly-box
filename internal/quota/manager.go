package quota

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Manager 配额管理器
type Manager struct {
	config       *Config
	store        Store
	registry     *Registry
	providerMgr  ProviderManager
	logger       *logrus.Logger
	mu           sync.RWMutex
	refresher    *Refresher
}

// ProviderManager 供应商管理器接口（复用现有基础设施）
type ProviderManager interface {
	GetProviderByUUID(uuid string) (*typ.Provider, error)
	ListProviders() []*typ.Provider
}

// NewManager 创建配额管理器
func NewManager(config *Config, store Store, providerMgr ProviderManager, logger *logrus.Logger) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	m := &Manager{
		config:      config,
		store:       store,
		registry:    NewRegistry(),
		providerMgr: providerMgr,
		logger:      logger,
	}

	// 创建后台刷新任务
	m.refresher = NewRefresher(m, logger)

	return m
}

// RegisterFetcher 注册新的配额获取器
func (m *Manager) RegisterFetcher(fetcher Fetcher) error {
	if err := m.registry.Register(fetcher); err != nil {
		return fmt.Errorf("failed to register fetcher: %w", err)
	}
	m.logger.Infof("registered quota fetcher: %s for provider type: %s", fetcher.Name(), fetcher.ProviderType())
	return nil
}

// Refresh 刷新所有启用的供应商配额
func (m *Manager) Refresh(ctx context.Context) ([]*ProviderUsage, error) {
	providers := m.providerMgr.ListProviders()
	if len(providers) == 0 {
		return []*ProviderUsage{}, nil
	}

	// 并发控制：最多同时刷新 5 个
	sem := semaphore.NewWeighted(5)
	var wg sync.WaitGroup
	resultChan := make(chan *ProviderUsage, len(providers))
	errorChan := make(chan error, len(providers))

	for _, provider := range providers {
		// 检查是否启用
		if !m.isProviderEnabled(provider) {
			continue
		}

		wg.Add(1)
		go func(p *typ.Provider) {
			defer wg.Done()

			if err := sem.Acquire(ctx, 1); err != nil {
				m.loggerWithError(p, err).Error("failed to acquire semaphore")
				return
			}
			defer sem.Release(1)

			usage, err := m.fetchProviderQuota(ctx, p)
			if err != nil {
				m.loggerWithError(p, err).Warn("failed to fetch quota")
				errorChan <- err
				return
			}

			resultChan <- usage
		}(provider)
	}

	go func() {
		wg.Wait()
		close(resultChan)
		close(errorChan)
	}()

	// 收集结果
	var results []*ProviderUsage
	var errs []error
	for usage := range resultChan {
		results = append(results, usage)
	}
	for err := range errorChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		m.logger.WithField("errors", len(errs)).Warn("some providers failed to refresh")
	}

	return results, nil
}

// RefreshProvider 刷新指定供应商的配额
func (m *Manager) RefreshProvider(ctx context.Context, providerUUID string) (*ProviderUsage, error) {
	provider, err := m.providerMgr.GetProviderByUUID(providerUUID)
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}

	return m.fetchProviderQuota(ctx, provider)
}

// GetQuota 获取指定供应商的配额（优先使用缓存）
func (m *Manager) GetQuota(ctx context.Context, providerUUID string) (*ProviderUsage, error) {
	usage, err := m.store.Get(ctx, providerUUID)
	if err != nil {
		if err == ErrUsageNotFound {
			return nil, fmt.Errorf("quota not found for provider: %s", providerUUID)
		}
		return nil, err
	}

	// 检查是否过期
	if usage.IsExpired() {
		m.logger.WithField("provider_uuid", providerUUID).Debug("quota expired, fetching fresh data")
		return m.RefreshProvider(ctx, providerUUID)
	}

	return usage, nil
}

// ListQuota 获取所有供应商的配额列表
func (m *Manager) ListQuota(ctx context.Context) ([]*ProviderUsage, error) {
	return m.store.List(ctx)
}

// Summary 获取配额汇总
func (m *Manager) Summary(ctx context.Context) (*Summary, error) {
	usages, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}

	summary := &Summary{
		TotalProviders: len(usages),
		ByStatus:       make(map[string]int),
		ByType:         make(map[ProviderType]int),
	}

	for _, usage := range usages {
		// 按状态统计
		if usage.LastError != "" {
			summary.ErrorProviders++
			summary.ByStatus["error"]++
		} else {
			summary.OKProviders++
			summary.ByStatus["ok"]++
		}

		// 按类型统计
		summary.ByType[usage.ProviderType]++

		// 警告统计
		if usage.Primary != nil && usage.Primary.UsedPercent >= 80 {
			summary.WarningProviders++
		}
	}

	return summary, nil
}

// StartAutoRefresh 启动自动刷新
func (m *Manager) StartAutoRefresh(ctx context.Context) {
	if !m.config.Enabled {
		m.logger.Info("auto-refresh disabled by config")
		return
	}
	m.refresher.Start(ctx, m.config.RefreshInterval)
}

// StopAutoRefresh 停止自动刷新
func (m *Manager) StopAutoRefresh() {
	m.refresher.Stop()
}

// isProviderEnabled 检查供应商是否启用配额获取
func (m *Manager) isProviderEnabled(provider *typ.Provider) bool {
	// 全局开关
	if !m.config.Enabled {
		return false
	}

	// 供应商必须是启用状态
	if !provider.Enabled {
		return false
	}

	// 检查是否有对应的 fetcher
	providerType := inferProviderType(provider)
	_, hasFetcher := m.registry.Get(providerType)
	if !hasFetcher {
		return false
	}

	// 检查供应商特定配置
	if cfg, ok := m.config.Providers[provider.Name]; ok {
		return cfg.Enabled
	}

	// 默认启用
	return true
}

// fetchProviderQuota 获取单个供应商的配额
func (m *Manager) fetchProviderQuota(ctx context.Context, provider *typ.Provider) (*ProviderUsage, error) {
	providerType := inferProviderType(provider)

	fetcher, ok := m.registry.Get(providerType)
	if !ok {
		return nil, ErrFetcherNotFound
	}

	// 验证配置
	if err := fetcher.Validate(provider); err != nil {
		return nil, fmt.Errorf("provider validation failed: %w", err)
	}

	// 获取配额
	usage, err := fetcher.Fetch(ctx, provider)
	if err != nil {
		// 创建错误记录
		usage = &ProviderUsage{
			ProviderUUID: provider.UUID,
			ProviderName: provider.Name,
			ProviderType: providerType,
			FetchedAt:    time.Now(),
			ExpiresAt:    time.Now().Add(m.config.CacheTTL),
			LastError:    err.Error(),
			LastErrorAt:  ptrTime(time.Now()),
		}
	}

	// 保存到存储
	if saveErr := m.store.Save(ctx, usage); saveErr != nil {
		m.logger.WithError(saveErr).Error("failed to save quota")
	}

	return usage, nil
}

// inferProviderType 从 Provider 推断供应商类型
func inferProviderType(provider *typ.Provider) ProviderType {
	// 根据 API 推断类型
	if provider.APIStyle == "anthropic" {
		return ProviderTypeAnthropic
	}

	// 根据 API Base 推断
	apiBase := provider.APIBase
	switch {
	case contains(apiBase, "anthropic.com"):
		return ProviderTypeAnthropic
	case contains(apiBase, "openai.com"), contains(apiBase, "openai.azure.com"):
		return ProviderTypeOpenAI
	case contains(apiBase, "googleapis.com"):
		return ProviderTypeGemini
	case contains(apiBase, "gemini"):
		return ProviderTypeGemini
	case contains(apiBase, "cursor"):
		return ProviderTypeCursor
	case contains(apiBase, "copilot"):
		return ProviderTypeCopilot
	case contains(apiBase, "vertex"):
		return ProviderTypeVertexAI
	case contains(apiBase, "zai.app"):
		return ProviderTypeZai
	case contains(apiBase, "moonshot.cn"):
		return ProviderTypeKimiK2
	case contains(apiBase, "openrouter.ai"):
		return ProviderTypeOpenRouter
	case contains(apiBase, "minimax"):
		return ProviderTypeMiniMax
	case contains(apiBase, "codex"):
		return ProviderTypeCodex
	}

	// 根据名称推断
	name := provider.Name
	switch {
	case contains(name, "anthropic"), contains(name, "claude"):
		return ProviderTypeAnthropic
	case contains(name, "openai"):
		return ProviderTypeOpenAI
	case contains(name, "gemini"), contains(name, "google"):
		return ProviderTypeGemini
	case contains(name, "cursor"):
		return ProviderTypeCursor
	case contains(name, "copilot"), contains(name, "github"):
		return ProviderTypeCopilot
	case contains(name, "vertex"):
		return ProviderTypeVertexAI
	case contains(name, "zai"):
		return ProviderTypeZai
	case contains(name, "kimi"), contains(name, "moonshot"):
		return ProviderTypeKimiK2
	case contains(name, "openrouter"):
		return ProviderTypeOpenRouter
	case contains(name, "minimax"):
		return ProviderTypeMiniMax
	case contains(name, "codex"):
		return ProviderTypeCodex
	}

	// 默认返回空字符串，让调用者处理
	return ""
}

// Summary 配额汇总
type Summary struct {
	TotalProviders   int                   `json:"total_providers"`
	OKProviders      int                   `json:"ok_providers"`
	ErrorProviders   int                   `json:"error_providers"`
	WarningProviders int                   `json:"warning_providers"`
	ByStatus         map[string]int        `json:"by_status"`
	ByType           map[ProviderType]int `json:"by_type"`
}

// loggerWithError 创建带错误日志的 logger
func (m *Manager) loggerWithError(provider *typ.Provider, err error) *logrus.Entry {
	return m.logger.WithFields(logrus.Fields{
		"provider_uuid": provider.UUID,
		"provider_name": provider.Name,
		"error":         err.Error(),
	})
}

// ptrTime 返回 time.Time 指针
func ptrTime(t time.Time) *time.Time {
	return &t
}

// contains 检查字符串包含（忽略大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr))
}
