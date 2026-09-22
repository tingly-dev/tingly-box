package providerquota

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai/quota"
)

// Manager is the quota manager interface.
type Manager interface {
	// GetQuota returns the given provider's quota (reads stored data, does not
	// trigger an upstream fetch; freshness is guaranteed by the background refresher).
	GetQuota(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
	// ListQuota returns the quota list for every provider.
	ListQuota(ctx context.Context) ([]*quota.ProviderUsage, error)
	// QuotaHistory returns immutable stored snapshots.
	QuotaHistory(ctx context.Context, query quota.HistoryQuery) ([]*quota.ProviderUsage, error)
	// Refresh refreshes quota for every enabled provider.
	Refresh(ctx context.Context) ([]*quota.ProviderUsage, error)
	// RefreshProvider refreshes quota for one provider.
	RefreshProvider(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error)
	// Summary returns the aggregate quota summary.
	Summary(ctx context.Context) (*quota.Summary, error)
	// IsProviderSupported reports whether the provider has a registered quota
	// fetcher. Callers should skip quota fetching when this returns false.
	IsProviderSupported(providerUUID string) bool
	// StartAutoRefresh starts automatic refresh.
	StartAutoRefresh(ctx context.Context)
	// StopAutoRefresh stops automatic refresh.
	StopAutoRefresh()
}

// Handler is the quota API handler.
type Handler struct {
	manager Manager
	logger  *logrus.Logger
}

// NewHandler creates the handler. manager may be nil when quota tracking is
// not configured, or during OpenAPI schema generation — every handler method
// checks available() first.
func NewHandler(manager Manager, logger *logrus.Logger) *Handler {
	return &Handler{
		manager: manager,
		logger:  logger,
	}
}

// available reports whether quota tracking is configured, answering 503 when
// it is not. Routes are registered unconditionally (see routes.go) so they
// appear in openapi.json on every build; this is what keeps that safe.
func (h *Handler) available(c *gin.Context) bool {
	if h.manager != nil {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "quota tracking is not enabled on this tingly-box",
	})
	return false
}

// ListQuotaResponse is the list response.
type ListQuotaResponse struct {
	Meta MetaData               `json:"meta"`
	Data []*quota.ProviderUsage `json:"data"`
}

// MetaData is the response metadata.
type MetaData struct {
	Total     int       `json:"total"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HistoryRequest filters historical snapshots. EndTime is exclusive.
type HistoryRequest struct {
	ProviderUUID string `json:"provider" form:"provider" description:"Provider UUID"`
	StartTime    string `json:"start_time" form:"start_time" description:"ISO 8601 start time"`
	EndTime      string `json:"end_time" form:"end_time" description:"ISO 8601 exclusive end time"`
	Limit        int    `json:"limit" form:"limit" description:"Maximum snapshots (1-5000)"`
}

// QuotaHistory returns stored snapshots, newest first.
// GET /api/v1/provider-quota/history
func (h *Handler) QuotaHistory(c *gin.Context) {
	if !h.available(c) {
		return
	}
	var req HistoryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid history query"})
		return
	}
	query := quota.HistoryQuery{ProviderUUID: req.ProviderUUID, Limit: req.Limit}
	parseTime := func(value string) (*time.Time, error) {
		if value == "" {
			return nil, nil
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil, err
		}
		return &parsed, nil
	}
	var err error
	if query.StartTime, err = parseTime(req.StartTime); err == nil {
		query.EndTime, err = parseTime(req.EndTime)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_time and end_time must be ISO 8601 timestamps"})
		return
	}
	usages, err := h.manager.QuotaHistory(c.Request.Context(), query)
	if err != nil {
		h.logger.WithError(err).Error("failed to list quota history")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list quota history"})
		return
	}
	c.JSON(http.StatusOK, ListQuotaResponse{Meta: MetaData{Total: len(usages), UpdatedAt: time.Now()}, Data: usages})
}

// ListQuota returns quota for every provider.
// GET /api/v1/provider-quota
func (h *Handler) ListQuota(c *gin.Context) {
	if !h.available(c) {
		return
	}
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

// GetQuota returns quota for the given provider.
// GET /api/v1/provider-quota/:uuid
func (h *Handler) GetQuota(c *gin.Context) {
	if !h.available(c) {
		return
	}
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
		// Both sentinels mean the provider simply has nothing to show — no
		// data yet, or no fetcher at all. Neither is a server failure.
		if errors.Is(err, quota.ErrUsageNotFound) || errors.Is(err, quota.ErrProviderUnsupported) {
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

// RefreshAll refreshes quota for every provider.
// POST /api/v1/provider-quota/refresh
func (h *Handler) RefreshAll(c *gin.Context) {
	if !h.available(c) {
		return
	}
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

// RefreshProvider refreshes quota for the given provider.
// POST /api/v1/provider-quota/:uuid/refresh
func (h *Handler) RefreshProvider(c *gin.Context) {
	if !h.available(c) {
		return
	}
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
		if errors.Is(err, quota.ErrProviderUnsupported) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "provider does not support quota",
			})
			return
		}
		h.logger.WithError(err).WithField("provider_uuid", uuid).Error("failed to refresh provider quota")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to refresh quota",
		})
		return
	}

	c.JSON(http.StatusOK, usage)
}

// Summary returns the aggregate quota summary.
// GET /api/v1/provider-quota/summary
func (h *Handler) Summary(c *gin.Context) {
	if !h.available(c) {
		return
	}
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

// BatchGetQuotaRequest is the batch quota request.
type BatchGetQuotaRequest struct {
	// ProviderUUIDs is the list of provider UUIDs to fetch quota for.
	ProviderUUIDs []string `json:"provider_uuids" binding:"required"`
}

// BatchGetQuotaResponse is the batch quota response.
type BatchGetQuotaResponse struct {
	Data map[string]*quota.ProviderUsage `json:"data"` // key: provider_uuid, value: quota data
}

// BatchGetQuota returns quota for a specific set of providers.
// POST /api/v1/provider-quota/batch
// Body: { "provider_uuids": ["uuid1", "uuid2", "uuid3"] }
func (h *Handler) BatchGetQuota(c *gin.Context) {
	if !h.available(c) {
		return
	}
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

	// Fetch quota for multiple providers concurrently.
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
				// If a provider has no quota data (or doesn't support quota),
				// skip it silently rather than returning an error.
				if !errors.Is(err, quota.ErrUsageNotFound) && !errors.Is(err, quota.ErrProviderUnsupported) {
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

	// Collect errors, if any.
	var fetchErrors []error
	for err := range errChan {
		fetchErrors = append(fetchErrors, err)
	}

	// If everything failed, return an error.
	if len(result) == 0 && len(fetchErrors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get quota for any provider",
		})
		return
	}

	c.JSON(http.StatusOK, BatchGetQuotaResponse{
		Data: result,
	})
}
