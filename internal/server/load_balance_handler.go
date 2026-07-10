package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ServiceHealthResponse represents the health check response for services
type ServiceHealthResponse struct {
	Rule   string                 `json:"rule" example:"gpt-4"`
	Health map[string]interface{} `json:"health"`
}

// LoadBalancerEngine is the narrow slice of the AI Model API's load-balancer
// engine (internal/server(aimodel).LoadBalancer) that the admin REST surface
// needs. Declared as an interface here — rather than importing the concrete
// type — to avoid an import cycle, since the root server package already
// imports this webui package for static-asset wiring.
type LoadBalancerEngine interface {
	SelectService(rule *typ.Rule) (*loadbalance.Service, error)
	GetServiceStats(provider, model string) *loadbalance.ServiceStats
	GetAllServiceStats() map[string]*loadbalance.ServiceStats
	ClearServiceStats(provider, model string)
	ClearAllStats()
	GetRuleSummary(rule *typ.Rule) map[string]interface{}
	HealthFilter() *typ.HealthFilter
}

// LoadBalancerAPI provides REST endpoints for load balancer management
type LoadBalancerAPI struct {
	loadBalancer LoadBalancerEngine
	config       *config.Config
}

// NewLoadBalancerAPI creates a new load balancer API
func NewLoadBalancerAPI(loadBalancer LoadBalancerEngine, cfg *config.Config) *LoadBalancerAPI {
	return &LoadBalancerAPI{
		loadBalancer: loadBalancer,
		config:       cfg,
	}
}

// RegisterRoutes registers the load balancer API routes
func (api *LoadBalancerAPI) RegisterRoutes(loadBalancer *gin.RouterGroup) {
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

		// Health monitoring
		loadBalancer.GET("/rules/:ruleId/health", api.GetServicesHealth)
		loadBalancer.POST("/services/:serviceId/health/reset", api.ResetServiceHealth)
	}
}

// GetRule returns a specific rule configuration
func (api *LoadBalancerAPI) GetRule(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRuleByUUID(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"rule": rule})
}

// GetRuleSummary returns a comprehensive summary of a rule including statistics
func (api *LoadBalancerAPI) GetRuleSummary(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRuleByUUID(ruleId)
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

	rule := api.config.GetRuleByUUID(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	// Validate tactic
	tacticType := loadbalance.ParseTacticType(req.Tactic)
	if !typ.IsValidTactic(req.Tactic) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported tactic: " + req.Tactic})
		return
	}

	// Create tactic with params using the helper function
	rule.LBTactic = typ.ParseTacticFromMap(tacticType, req.Params)
	if err := api.config.UpdateRequestConfigByUUID(ruleId, *rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update rule: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tactic updated successfully", "tactic": req.Tactic})
}

// GetRuleStats returns statistics for all services in a rule
func (api *LoadBalancerAPI) GetRuleStats(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRuleByUUID(ruleId)
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

	rule := api.config.GetRuleByUUID(ruleId)
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

// findRuleService resolves a (ruleId, serviceId) pair to the matching service,
// writing the appropriate 404 response and returning nil when either is missing.
func (api *LoadBalancerAPI) findRuleService(c *gin.Context, ruleId, serviceId string) *loadbalance.Service {
	rule := api.config.GetRuleByUUID(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return nil
	}

	for _, service := range rule.GetServices() {
		if service.ServiceID() == serviceId {
			return service
		}
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "Service not found in rule"})
	return nil
}

// GetServiceStats returns statistics for a specific service
func (api *LoadBalancerAPI) GetServiceStats(c *gin.Context) {
	ruleId := c.Param("ruleId")
	serviceId := c.Param("serviceId")

	service := api.findRuleService(c, ruleId, serviceId)
	if service == nil {
		return
	}

	stats := api.loadBalancer.GetServiceStats(service.Provider, service.Model)
	c.JSON(http.StatusOK, gin.H{"rule_id": ruleId, "service_id": serviceId, "stats": stats})
}

// ClearServiceStats clears statistics for a specific service
func (api *LoadBalancerAPI) ClearServiceStats(c *gin.Context) {
	ruleId := c.Param("ruleId")
	serviceId := c.Param("serviceId")

	service := api.findRuleService(c, ruleId, serviceId)
	if service == nil {
		return
	}

	api.loadBalancer.ClearServiceStats(service.Provider, service.Model)
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

	rule := api.config.GetRuleByUUID(ruleId)
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

// GetServicesHealth returns health status for all services in a rule
func (api *LoadBalancerAPI) GetServicesHealth(c *gin.Context) {
	ruleId := c.Param("ruleId")

	rule := api.config.GetRuleByUUID(ruleId)
	if rule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	services := rule.GetServices()
	healthData := make(map[string]interface{})
	breakers := loadbalance.DefaultBreakerStore()

	for _, service := range services {
		serviceID := service.ServiceID()
		serviceHealth := gin.H{
			"service_id":   serviceID,
			"provider":     service.Provider,
			"model":        service.Model,
			"active":       service.Active,
			"tier":         service.Tier,
			"health_known": false,
		}

		// Circuit-breaker state (rule-scoped): the second health channel, fed
		// by 5xx/transport failures. This is what drives tier failover and
		// breaker-aware selection, so surface it — "why is my traffic on the
		// backup" is unanswerable without it. retry_in_seconds is the time
		// until an open breaker admits its next recovery probe (0 otherwise).
		breaker := breakers.Get(rule.UUID, serviceID)
		serviceHealth["breaker_state"] = breaker.State().String()
		serviceHealth["breaker_retry_in_seconds"] = int(breaker.RetryIn().Seconds())

		// Get health from load balancer's health filter
		healthFilter := api.loadBalancer.HealthFilter()
		if healthFilter != nil {
			isHealthy := healthFilter.IsHealthy(serviceID)
			serviceHealth["healthy"] = isHealthy
			serviceHealth["health_known"] = true

			// Get detailed health info if available
			if monitor := healthFilter.GetHealthMonitor(); monitor != nil {
				health := monitor.GetHealth(serviceID)
				if health != nil {
					serviceHealth["status"] = health.Status.String()
					serviceHealth["consecutive_errors"] = health.ConsecutiveErrors
					serviceHealth["rate_limited"] = health.RateLimited
					serviceHealth["auth_error"] = health.AuthError
					if !health.LastErrorTime.IsZero() {
						serviceHealth["last_error_time"] = health.LastErrorTime
					}
					if health.LastError != nil {
						serviceHealth["last_error"] = health.LastError.Error()
					}
				}
			}
		} else {
			// No health filter, assume healthy
			serviceHealth["healthy"] = true
		}

		healthData[serviceID] = serviceHealth
	}

	c.JSON(http.StatusOK, ServiceHealthResponse{
		Rule:   ruleId,
		Health: healthData,
	})
}

// ResetServiceHealth manually resets a service's health to healthy
func (api *LoadBalancerAPI) ResetServiceHealth(c *gin.Context) {
	serviceId := c.Param("serviceId")

	healthFilter := api.loadBalancer.HealthFilter()
	if healthFilter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Health monitoring not available"})
		return
	}

	monitor := healthFilter.GetHealthMonitor()
	if monitor == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Health monitoring not available"})
		return
	}

	monitor.ResetHealth(serviceId)

	c.JSON(http.StatusOK, gin.H{
		"message":    "Service health reset successfully",
		"service_id": serviceId,
	})
}
