package server

import (
	"net/http"
	"strconv"

	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
)

// LoadBalancerAPI provides REST endpoints for load balancer management
type LoadBalancerAPI struct {
	loadBalancer *LoadBalancer
	config       *config.Config
}

// NewLoadBalancerAPI creates a new load balancer API
func NewLoadBalancerAPI(loadBalancer *LoadBalancer, cfg *config.Config) *LoadBalancerAPI {
	return &LoadBalancerAPI{
		loadBalancer: loadBalancer,
		config:       cfg,
	}
}

// RegisterRoutes registers the load balancer API routes
func (api *LoadBalancerAPI) RegisterRoutes(router *gin.RouterGroup) {
	loadBalancer := router.Group("/load-balancer")
	{
		// Rule management
		loadBalancer.GET("/rules/:ruleId", api.GetRule)
		loadBalancer.GET("/rules/:ruleId/summary", api.GetRuleSummary)
		loadBalancer.PUT("/rules/:ruleId/tactic", api.UpdateRuleTactic)

		// Statistics
		loadBalancer.GET("/rules/:ruleId/stats", api.GetRuleStats)
		loadBalancer.POST("/rules/:ruleId/stats/clear", api.ClearRuleStats)
		loadBalancer.GET("/rules/:ruleId/services/:serviceId/stats", api.GetServiceStats)
		loadBalancer.POST("/rules/:ruleId/services/:serviceId/stats/clear", api.ClearServiceStats)

		// Global statistics
		loadBalancer.GET("/stats", api.GetAllStats)
		loadBalancer.POST("/stats/clear", api.ClearAllStats)

		// Current service information
		loadBalancer.GET("/rules/:ruleId/current-service", api.GetCurrentService)
	}
}

// GetRule returns a specific rule configuration
func (api *LoadBalancerAPI) GetRule(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// GetRuleSummary returns a comprehensive summary of a rule including statistics
func (api *LoadBalancerAPI) GetRuleSummary(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	summary := api.loadBalancer.GetRuleSummary(rule)
	c.JSON(http.StatusOK, gin.H{"summary": summary})
}

// UpdateRuleTactic updates the load balancing tactic for a rule
func (api *LoadBalancerAPI) UpdateRuleTactic(c *gin.Context) {
	ruleId := c.Param("ruleId")

	var req struct {
		Tactic string                 `json:"tactic" binding:"required"`
		Params map[string]interface{} `json:"params,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	// Validate tactic
	tacticType := config.ParseTacticType(req.Tactic)
	if !config.IsValidTactic(req.Tactic) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported tactic: " + req.Tactic})
		return
	}

	// Create tactic with params using the helper function
	rule.LBTactic = config.ParseTacticFromMap(tacticType, req.Params)
	if err := api.config.UpdateRequestConfigByUUID(ruleId, *rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update rule: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tactic updated successfully", "tactic": req.Tactic})
}

// GetRuleStats returns statistics for all services in a rule
func (api *LoadBalancerAPI) GetRuleStats(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	services := rule.GetServices()
	stats := make(map[string]interface{})

	for _, service := range services {
		serviceStats := api.loadBalancer.GetServiceStats(service.Provider, service.Model)
		if serviceStats != nil {
			stats[service.ServiceID()] = serviceStats
		}
	}

	c.JSON(http.StatusOK, gin.H{"rule_id": ruleId, "rule_name": rule.RequestModel, "stats": stats})
}

// ClearRuleStats clears statistics for all services in a rule
func (api *LoadBalancerAPI) ClearRuleStats(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	services := rule.GetServices()
	for _, service := range services {
		api.loadBalancer.ClearServiceStats(service.Provider, service.Model)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Statistics cleared for rule: " + rule.RequestModel})
}

// GetServiceStats returns statistics for a specific service
func (api *LoadBalancerAPI) GetServiceStats(c *gin.Context) {
	ruleId := c.Param("ruleId")
	serviceId := c.Param("serviceId")

	// Validate that the service belongs to the rule
	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	services := rule.GetServices()
	var foundService *config.Service
	for _, service := range services {
		if service.ServiceID() == serviceId {
			foundService = &service
			break
		}
	}

	if foundService == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found in rule"})
		return
	}

	stats := api.loadBalancer.GetServiceStats(foundService.Provider, foundService.Model)
	if stats == nil {
		c.JSON(http.StatusOK, gin.H{"rule_id": ruleId, "service_id": serviceId, "stats": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"rule_id": ruleId, "service_id": serviceId, "stats": stats})
}

// ClearServiceStats clears statistics for a specific service
func (api *LoadBalancerAPI) ClearServiceStats(c *gin.Context) {
	ruleId := c.Param("ruleId")
	serviceId := c.Param("serviceId")

	// Validate that the service belongs to the rule
	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	services := rule.GetServices()
	var foundService *config.Service
	for _, service := range services {
		if service.ServiceID() == serviceId {
			foundService = &service
			break
		}
	}

	if foundService == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found in rule"})
		return
	}

	api.loadBalancer.ClearServiceStats(foundService.Provider, foundService.Model)
	c.JSON(http.StatusOK, gin.H{"message": "Statistics cleared for service: " + serviceId})
}

// GetAllStats returns statistics for all services
func (api *LoadBalancerAPI) GetAllStats(c *gin.Context) {
	stats := api.loadBalancer.GetAllServiceStats()
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// ClearAllStats clears all statistics
func (api *LoadBalancerAPI) ClearAllStats(c *gin.Context) {
	api.loadBalancer.ClearAllStats()
	c.JSON(http.StatusOK, gin.H{"message": "All statistics cleared"})
}

// GetCurrentService returns the currently active service for a rule
func (api *LoadBalancerAPI) GetCurrentService(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	selectedService, err := api.loadBalancer.SelectService(rule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to select service: " + err.Error()})
		return
	}

	if selectedService == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No available service"})
		return
	}

	stats := api.loadBalancer.GetServiceStats(selectedService.Provider, selectedService.Model)

	response := gin.H{
		"rule_id":    ruleId,
		"rule_name":  rule.RequestModel,
		"service":    selectedService,
		"service_id": selectedService.ServiceID(),
		"tactic":     rule.GetTacticType().String(),
	}

	if stats != nil {
		response["stats"] = stats
	}

	c.JSON(http.StatusOK, response)
}

// GetServiceHealth checks the health of all services in a rule
func (api *LoadBalancerAPI) GetServiceHealth(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRequestConfigByRequestModel(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	services := rule.GetServices()
	health := make(map[string]interface{})

	for _, service := range services {
		serviceHealth := gin.H{
			"active":     service.Active,
			"service_id": service.ServiceID(),
		}

		stats := api.loadBalancer.GetServiceStats(service.Provider, service.Model)
		if stats != nil {
			serviceHealth["last_used"] = stats.LastUsed
			serviceHealth["window_expired"] = stats.IsWindowExpired()
			serviceHealth["request_count"] = stats.WindowRequestCount
		}

		health[service.ServiceID()] = serviceHealth
	}

	c.JSON(http.StatusOK, gin.H{"rule_id": ruleId, "rule_name": rule.RequestModel, "health": health})
}

// GetMetrics returns load balancing metrics
func (api *LoadBalancerAPI) GetMetrics(c *gin.Context) {
	// Query parameters
	limitStr := c.DefaultQuery("limit", "100")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}

	allStats := api.loadBalancer.GetAllServiceStats()

	// Sort by total tokens consumed (top services first)
	type serviceMetric struct {
		ServiceID            string `json:"service_id"`
		RequestCount         int64  `json:"request_count"`
		WindowRequestCount   int64  `json:"window_request_count"`
		WindowTokensConsumed int64  `json:"window_tokens_consumed"`
		WindowInputTokens    int64  `json:"window_input_tokens"`
		WindowOutputTokens   int64  `json:"window_output_tokens"`
		LastUsed             string `json:"last_used"`
	}

	var metrics []serviceMetric
	for serviceID, stats := range allStats {
		metrics = append(metrics, serviceMetric{
			ServiceID:            serviceID,
			RequestCount:         stats.RequestCount,
			WindowRequestCount:   stats.WindowRequestCount,
			WindowTokensConsumed: stats.WindowTokensConsumed,
			WindowInputTokens:    stats.WindowInputTokens,
			WindowOutputTokens:   stats.WindowOutputTokens,
			LastUsed:             stats.LastUsed.Format("2006-01-02T15:04:05Z"),
		})
	}

	// Return top services by limit
	if len(metrics) > limit {
		metrics = metrics[:limit]
	}

	c.JSON(http.StatusOK, gin.H{
		"metrics":        metrics,
		"total_services": len(allStats),
	})
}
