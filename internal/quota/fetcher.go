package quota

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Fetcher 配额获取器接口
type Fetcher interface {
	// Name 返回 fetcher 名称
	Name() string

	// ProviderType 返回支持的供应商类型
	ProviderType() ProviderType

	// Fetch 获取当前配额
	Fetch(ctx context.Context, provider *typ.Provider) (*ProviderUsage, error)

	// Validate 验证配置是否有效
	Validate(provider *typ.Provider) error

	// RequiresAuth 返回需要的认证类型
	RequiresAuth() typ.AuthType
}

// Registry Fetcher 注册表
type Registry struct {
	fetchers map[ProviderType]Fetcher
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	return &Registry{
		fetchers: make(map[ProviderType]Fetcher),
	}
}

// Register 注册 fetcher
func (r *Registry) Register(fetcher Fetcher) error {
	pt := fetcher.ProviderType()
	if _, exists := r.fetchers[pt]; exists {
		return ErrFetcherAlreadyRegistered
	}
	r.fetchers[pt] = fetcher
	return nil
}

// Unregister 注销 fetcher
func (r *Registry) Unregister(pt ProviderType) {
	delete(r.fetchers, pt)
}

// Get 获取指定类型的 fetcher
func (r *Registry) Get(pt ProviderType) (Fetcher, bool) {
	f, ok := r.fetchers[pt]
	return f, ok
}

// List 列出所有已注册的 fetcher
func (r *Registry) List() map[ProviderType]Fetcher {
	// 返回副本
	result := make(map[ProviderType]Fetcher, len(r.fetchers))
	for k, v := range r.fetchers {
		result[k] = v
	}
	return result
}

// ProviderTypes 返回所有支持的供应商类型
func (r *Registry) ProviderTypes() []ProviderType {
	types := make([]ProviderType, 0, len(r.fetchers))
	for pt := range r.fetchers {
		types = append(types, pt)
	}
	return types
}

var (
	ErrFetcherAlreadyRegistered = &quotaError{"fetcher already registered for this provider type"}
	ErrFetcherNotFound          = &quotaError{"fetcher not found for provider type"}
)

type quotaError struct {
	msg string
}

func (e *quotaError) Error() string {
	return e.msg
}
