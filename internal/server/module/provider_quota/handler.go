package provider_quota

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
)

// Manager 配额管理器接口
type Manager interface {
	// GetQuota 获取指定供应商的配额（优先使用缓存）
	GetQuota(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
	// GetQuotaNoCache 获取指定供应商的配额（绕过缓存，直接从数据库读取最新数据）
	GetQuotaNoCache(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
	// ListQuota 获取所有供应商的配额列表
	ListQuota(ctx context.Context) ([]*quota.ProviderUsage, error)
	// Refresh 刷新所有启用的供应商配额
	Refresh(ctx context.Context) ([]*quota.ProviderUsage, error)
	// RefreshProvider 刷新指定供应商的配额
	RefreshProvider(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
	// Summary 获取配额汇总
	Summary(ctx context.Context) (*quota.Summary, error)
	// StartAutoRefresh 启动自动刷新
	StartAutoRefresh(ctx context.Context)
	// StopAutoRefresh 停止自动刷新
	StopAutoRefresh()
}

// Handler 配额 API 处理器
type Handler struct {
	manager Manager
	logger  *logrus.Logger
}

// NewHandler 创建处理器
func NewHandler(manager Manager, logger *logrus.Logger) *Handler {
	return &Handler{
		manager: manager,
		logger:  logger,
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	quota := r.Group("/provider-quota")
	{
		quota.GET("", h.ListQuota)
		quota.POST("/batch", h.BatchGetQuota) // 批量获取指定 providers 的 quota
		quota.GET("/:uuid", h.GetQuota)
		quota.POST("/refresh", h.RefreshAll)
		quota.POST("/:uuid/refresh", h.RefreshProvider)
		quota.GET("/summary", h.Summary)
	}
}

// ListQuotaResponse 列表响应
type ListQuotaResponse struct {
	Meta MetaData               `json:"meta"`
	Data []*quota.ProviderUsage `json:"data"`
}

// MetaData 元数据
type MetaData struct {
	Total     int       `json:"total"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListQuota 获取所有供应商配额
// GET /api/v1/provider-quota
func (h *Handler) ListQuota(c *gin.Context) {
	ctx := c.Request.Context()

	usages, err := h.manager.ListQuota(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to list quota")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list quota",
		})
		return
	}

	c.JSON(http.StatusOK, ListQuotaResponse{
		Meta: MetaData{
			Total:     len(usages),
			UpdatedAt: time.Now(),
		},
		Data: usages,
	})
}

// GetQuota 获取指定供应商配额
// GET /api/v1/provider-quota/:uuid
func (h *Handler) GetQuota(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider_uuid is required",
		})
		return
	}

	ctx := c.Request.Context()

	usage, err := h.manager.GetQuota(ctx, uuid)
	if err != nil {
		if err == quota.ErrUsageNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "quota not found for provider",
			})
			return
		}
		h.logger.WithError(err).WithField("provider_uuid", uuid).Error("failed to get quota")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get quota",
		})
		return
	}

	c.JSON(http.StatusOK, usage)
}

// RefreshAll 刷新所有配额
// POST /api/v1/provider-quota/refresh
func (h *Handler) RefreshAll(c *gin.Context) {
	ctx := c.Request.Context()

	usages, err := h.manager.Refresh(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to refresh all quota")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to refresh quota",
		})
		return
	}

	c.JSON(http.StatusOK, ListQuotaResponse{
		Meta: MetaData{
			Total:     len(usages),
			UpdatedAt: time.Now(),
		},
		Data: usages,
	})
}

// RefreshProvider 刷新指定供应商配额
// POST /api/v1/provider-quota/:uuid/refresh
func (h *Handler) RefreshProvider(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider_uuid is required",
		})
		return
	}

	ctx := c.Request.Context()

	usage, err := h.manager.RefreshProvider(ctx, uuid)
	if err != nil {
		h.logger.WithError(err).WithField("provider_uuid", uuid).Error("failed to refresh provider quota")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to refresh quota",
		})
		return
	}

	c.JSON(http.StatusOK, usage)
}

// Summary 获取配额汇总
// GET /api/v1/provider-quota/summary
func (h *Handler) Summary(c *gin.Context) {
	ctx := c.Request.Context()

	summary, err := h.manager.Summary(ctx)
	if err != nil {
		h.logger.WithError(err).Error("failed to get summary")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get summary",
		})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// BatchGetQuotaRequest 批量获取配额请求
type BatchGetQuotaRequest struct {
	// ProviderUUIDs 需要获取配额的供应商 UUID 列表
	ProviderUUIDs []string `json:"provider_uuids" binding:"required"`
}

// BatchGetQuotaResponse 批量获取配额响应
type BatchGetQuotaResponse struct {
	Data map[string]*quota.ProviderUsage `json:"data"` // key: provider_uuid, value: quota data
}

// BatchGetQuota 批量获取指定供应商的配额
// POST /api/v1/provider-quota/batch
// Body: { "provider_uuids": ["uuid1", "uuid2", "uuid3"] }
func (h *Handler) BatchGetQuota(c *gin.Context) {
	var req BatchGetQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request body",
			"details": err.Error(),
		})
		return
	}

	if len(req.ProviderUUIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "provider_uuids cannot be empty",
		})
		return
	}

	ctx := c.Request.Context()

	// 并发获取多个 provider 的 quota
	result := make(map[string]*quota.ProviderUsage)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(req.ProviderUUIDs))

	for _, uuid := range req.ProviderUUIDs {
		wg.Add(1)
		go func(providerUUID string) {
			defer wg.Done()
			usage, err := h.manager.GetQuota(ctx, providerUUID)
			if err != nil {
				// 如果某个 provider 没有 quota 数据，不返回错误，只是跳过
				if err != quota.ErrUsageNotFound {
					h.logger.WithError(err).WithField("provider_uuid", providerUUID).Warn("failed to get quota for provider")
					errChan <- err
				}
				return
			}
			mu.Lock()
			result[providerUUID] = usage
			mu.Unlock()
		}(uuid)
	}

	wg.Wait()
	close(errChan)

	// 收集错误（如果有）
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	// 如果全部失败，返回错误
	if len(result) == 0 && len(errors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get quota for any provider",
		})
		return
	}

	c.JSON(http.StatusOK, BatchGetQuotaResponse{
		Data: result,
	})
}
