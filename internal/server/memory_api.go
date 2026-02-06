package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// MemoryAPI provides REST endpoints for memory recording
type MemoryAPI struct {
	memoryStore *db.MemoryStore
}

// NewMemoryAPI creates a new memory API
func NewMemoryAPI(memoryStore *db.MemoryStore) *MemoryAPI {
	return &MemoryAPI{
		memoryStore: memoryStore,
	}
}

// RegisterMemoryRoutes registers the memory recording API routes with swagger documentation
func (s *Server) RegisterMemoryRoutes(manager *swagger.RouteManager) {
	// Create API for memory
	memoryAPI := NewMemoryAPI(s.config.GetMemoryStore())

	apiV1 := manager.NewGroup("api", "v1", "")
	apiV1.Router.Use(s.authMW.UserAuthMiddleware())

	// GET /api/v1/memory/rounds - Get memory rounds with filtering and pagination
	apiV1.GET("/memory/rounds", memoryAPI.GetRounds,
		swagger.WithDescription("Get memory rounds with optional filtering by scenario/protocol"),
		swagger.WithTags("memory"),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario (e.g., claude_code, opencode)",
		}),
		swagger.WithQueryConfig("protocol", swagger.QueryParamConfig{
			Name:        "protocol",
			Type:        "string",
			Required:    false,
			Description: "Filter by protocol (anthropic, openai, google)",
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Maximum number of results to return (default 20, max 100)",
			Default:     20,
			Minimum:     intPtr(1),
			Maximum:     intPtr(100),
		}),
		swagger.WithQueryConfig("offset", swagger.QueryParamConfig{
			Name:        "offset",
			Type:        "integer",
			Required:    false,
			Description: "Number of results to skip for pagination",
			Default:     0,
			Minimum:     intPtr(0),
		}),
		swagger.WithResponseModel(MemoryRoundListResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Memory store not available"},
		),
	)

	// GET /api/v1/memory/user-inputs - Get user inputs for memory user page
	apiV1.GET("/memory/user-inputs", memoryAPI.GetUserInputs,
		swagger.WithDescription("Get user inputs (for memory user page display)"),
		swagger.WithTags("memory"),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario",
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Maximum number of results to return (default 20, max 100)",
			Default:     20,
			Minimum:     intPtr(1),
			Maximum:     intPtr(100),
		}),
		swagger.WithResponseModel(MemoryRoundsResponse{}),
	)

	// GET /api/v1/memory/user-inputs/list - Get lightweight list for user page
	apiV1.GET("/memory/user-inputs/list", memoryAPI.GetUserInputsList,
		swagger.WithDescription("Get lightweight list for memory user page with date range filtering"),
		swagger.WithTags("memory"),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario (e.g., claude_code, opencode)",
		}),
		swagger.WithQueryConfig("protocol", swagger.QueryParamConfig{
			Name:        "protocol",
			Type:        "string",
			Required:    false,
			Description: "Filter by protocol (anthropic, openai, google)",
		}),
		swagger.WithQueryConfig("start_date", swagger.QueryParamConfig{
			Name:        "start_date",
			Type:        "string",
			Required:    false,
			Description: "Start date in ISO format (e.g., 2024-01-01)",
		}),
		swagger.WithQueryConfig("end_date", swagger.QueryParamConfig{
			Name:        "end_date",
			Type:        "string",
			Required:    false,
			Description: "End date in ISO format (e.g., 2024-12-31)",
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Maximum number of results to return (default 100, max 500)",
			Default:     100,
			Minimum:     intPtr(1),
			Maximum:     intPtr(500),
		}),
		swagger.WithResponseModel(MemoryRoundListResponse{}),
	)

	// GET /api/v1/memory/rounds/:id - Get full details for a specific round
	apiV1.GET("/memory/rounds/:id", memoryAPI.GetRoundDetail,
		swagger.WithDescription("Get full details for a specific memory round"),
		swagger.WithTags("memory"),
		swagger.WithResponseModel(MemoryRoundDetailResponse{}),
	)

	// GET /api/v1/memory/search - Search memory rounds by user input content
	apiV1.GET("/memory/search", memoryAPI.Search,
		swagger.WithDescription("Search memory rounds by user input content"),
		swagger.WithTags("memory"),
		swagger.WithQueryConfig("q", swagger.QueryParamConfig{
			Name:        "q",
			Type:        "string",
			Required:    true,
			Description: "Search query (required)",
		}),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario",
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Maximum number of results to return (default 20, max 100)",
			Default:     20,
			Minimum:     intPtr(1),
			Maximum:     intPtr(100),
		}),
		swagger.WithResponseModel(MemoryRoundsResponse{}),
	)

	// GET /api/v1/memory/by-project-session - Get rounds by project and/or session ID
	apiV1.GET("/memory/by-project-session", memoryAPI.GetByProjectSession,
		swagger.WithDescription("Get rounds grouped by project or session ID"),
		swagger.WithTags("memory"),
		swagger.WithQueryConfig("project_id", swagger.QueryParamConfig{
			Name:        "project_id",
			Type:        "string",
			Required:    false,
			Description: "Filter by Anthropic project ID",
		}),
		swagger.WithQueryConfig("session_id", swagger.QueryParamConfig{
			Name:        "session_id",
			Type:        "string",
			Required:    false,
			Description: "Filter by session/conversation ID",
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Maximum number of results to return (default 20, max 100)",
			Default:     20,
			Minimum:     intPtr(1),
			Maximum:     intPtr(100),
		}),
		swagger.WithResponseModel(MemoryRoundsResponse{}),
	)

	// GET /api/v1/memory/by-metadata - Get rounds by metadata key-value
	apiV1.GET("/memory/by-metadata", memoryAPI.GetByMetadata,
		swagger.WithDescription("Get rounds by metadata key-value pairs (e.g., anthropic_user_id)"),
		swagger.WithTags("memory"),
		swagger.WithQueryConfig("key", swagger.QueryParamConfig{
			Name:        "key",
			Type:        "string",
			Required:    true,
			Description: "Metadata key to query (required)",
		}),
		swagger.WithQueryConfig("value", swagger.QueryParamConfig{
			Name:        "value",
			Type:        "string",
			Required:    true,
			Description: "Metadata value to match (required)",
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Maximum number of results to return (default 20, max 100)",
			Default:     20,
			Minimum:     intPtr(1),
			Maximum:     intPtr(100),
		}),
		swagger.WithResponseModel(MemoryRoundsResponse{}),
	)

	// DELETE /api/v1/memory/old-records - Delete old memory records
	apiV1.DELETE("/memory/old-records", memoryAPI.DeleteOldRecords,
		swagger.WithDescription("Delete memory records older than specified days"),
		swagger.WithTags("memory"),
		swagger.WithQueryConfig("days", swagger.QueryParamConfig{
			Name:        "days",
			Type:        "integer",
			Required:    true,
			Description: "Number of days (records older than this will be deleted)",
			Minimum:     intPtr(1),
		}),
		swagger.WithResponseModel(MemoryDeleteOldRecordsResponse{}),
	)
}

// MemoryRoundListResponse represents the response for listing memory rounds
type MemoryRoundListResponse struct {
	Success bool                `json:"success"`
	Data    MemoryRoundListData `json:"data"`
	Error   string              `json:"error,omitempty"`
}

// MemoryRoundListData contains the paginated list of memory rounds
type MemoryRoundListData struct {
	Rounds []MemoryRoundItem `json:"rounds"`
	Total  int64             `json:"total"`
}

// MemoryRoundsResponse represents a simple response with a list of memory rounds
// Used by: user-inputs, search, by-project-session, by-metadata endpoints
type MemoryRoundsResponse struct {
	Success bool              `json:"success"`
	Data    []MemoryRoundItem `json:"data"`
}

// MemoryRoundDetailResponse represents the response for a single round detail
type MemoryRoundDetailResponse struct {
	Success bool            `json:"success"`
	Data    MemoryRoundItem `json:"data"`
	Error   string          `json:"error,omitempty"`
}

// MemoryRoundListItem represents a lightweight item for list display
// Contains only minimal fields to reduce data transfer
type MemoryRoundListItem struct {
	ID           uint   `json:"id"`
	Scenario     string `json:"scenario"`
	ProviderName string `json:"provider_name"`
	Model        string `json:"model"`
	Protocol     string `json:"protocol"`
	CreatedAt    string `json:"created_at"`
	IsStreaming  bool   `json:"is_streaming"`
	HasToolUse   bool   `json:"has_tool_use"`
	UserInput    string `json:"user_input"`
}

// MemoryDeleteOldRecordsResponse represents the response for deleting old records
type MemoryDeleteOldRecordsResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		DeletedCount int64 `json:"deleted_count"`
		CutoffDays   int   `json:"cutoff_days"`
	} `json:"data"`
}

// MemoryRoundItem represents a single memory round in the list response
type MemoryRoundItem struct {
	ID           uint              `json:"id"`
	Scenario     string            `json:"scenario"`
	ProviderUUID string            `json:"provider_uuid"`
	ProviderName string            `json:"provider_name"`
	Model        string            `json:"model"`
	Protocol     string            `json:"protocol"`
	RequestID    string            `json:"request_id,omitempty"`
	ProjectID    string            `json:"project_id,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	RoundIndex   int               `json:"round_index"`
	UserInput    string            `json:"user_input"`
	RoundResult  string            `json:"round_result"`
	InputTokens  int               `json:"input_tokens"`
	OutputTokens int               `json:"output_tokens"`
	TotalTokens  int               `json:"total_tokens"`
	CreatedAt    string            `json:"created_at"`
	IsStreaming  bool              `json:"is_streaming"`
	HasToolUse   bool              `json:"has_tool_use"`
}

// MemoryAPI handler methods

// GetRounds retrieves memory rounds with optional filtering and pagination
func (api *MemoryAPI) GetRounds(c *gin.Context) {
	if api.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Memory store not available",
		})
		return
	}

	// Parse query parameters
	scenario := c.Query("scenario")
	protocol := c.Query("protocol")
	limitStr := c.DefaultQuery("limit", "20")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	var records []db.MemoryRoundRecord
	var total int64

	// Query based on filters
	if scenario != "" && protocol != "" {
		// Both filters - need to filter by scenario after getting by protocol
		records, total, err = api.memoryStore.GetRoundsByProtocol(db.ProtocolType(protocol), limit+100, 0) // Get more then filter
		if err == nil {
			// Filter by scenario
			filtered := make([]db.MemoryRoundRecord, 0)
			for _, r := range records {
				if r.Scenario == scenario {
					filtered = append(filtered, r)
					if len(filtered) >= limit {
						break
					}
				}
			}
			// Apply offset and limit
			if offset < len(filtered) {
				end := offset + limit
				if end > len(filtered) {
					end = len(filtered)
				}
				records = filtered[offset:end]
				total = int64(len(filtered))
			} else {
				records = []db.MemoryRoundRecord{}
				total = int64(len(filtered))
			}
		}
	} else if scenario != "" {
		records, total, err = api.memoryStore.GetRoundsByScenario(scenario, limit, offset)
	} else if protocol != "" {
		records, total, err = api.memoryStore.GetRoundsByProtocol(db.ProtocolType(protocol), limit, offset)
	} else {
		// No filter - return empty for now to avoid returning too much data
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "At least one of 'scenario' or 'protocol' filter is required",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve memory rounds",
		})
		return
	}

	// Convert to response format
	items := make([]MemoryRoundItem, len(records))
	for i, r := range records {
		items[i] = convertToMemoryRoundItem(r)
	}

	c.JSON(http.StatusOK, MemoryRoundListResponse{
		Success: true,
		Data: MemoryRoundListData{
			Rounds: items,
			Total:  total,
		},
	})
}

// GetUserInputs retrieves only user inputs for the memory user page
func (api *MemoryAPI) GetUserInputs(c *gin.Context) {
	if api.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Memory store not available",
		})
		return
	}

	scenario := c.Query("scenario")
	limitStr := c.DefaultQuery("limit", "20")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	records, err := api.memoryStore.GetUserInputs(scenario, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve user inputs",
		})
		return
	}

	// Convert to response format
	items := make([]MemoryRoundItem, len(records))
	for i, r := range records {
		items[i] = convertToMemoryRoundItem(r)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// Search searches memory rounds by user input content
func (api *MemoryAPI) Search(c *gin.Context) {
	if api.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Memory store not available",
		})
		return
	}

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Search query 'q' is required",
		})
		return
	}

	scenario := c.Query("scenario")
	limitStr := c.DefaultQuery("limit", "20")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	records, err := api.memoryStore.SearchRounds(scenario, query, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to search memory",
		})
		return
	}

	// Convert to response format
	items := make([]MemoryRoundItem, len(records))
	for i, r := range records {
		items[i] = convertToMemoryRoundItem(r)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// GetByProjectSession retrieves rounds by project and/or session ID
func (api *MemoryAPI) GetByProjectSession(c *gin.Context) {
	if api.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Memory store not available",
		})
		return
	}

	projectID := c.Query("project_id")
	sessionID := c.Query("session_id")
	limitStr := c.DefaultQuery("limit", "20")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if projectID == "" && sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "At least one of 'project_id' or 'session_id' is required",
		})
		return
	}

	records, err := api.memoryStore.GetRoundsByProjectSession(projectID, sessionID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve rounds",
		})
		return
	}

	// Convert to response format
	items := make([]MemoryRoundItem, len(records))
	for i, r := range records {
		items[i] = convertToMemoryRoundItem(r)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// GetByMetadata retrieves rounds by metadata key-value pairs
func (api *MemoryAPI) GetByMetadata(c *gin.Context) {
	if api.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Memory store not available",
		})
		return
	}

	key := c.Query("key")
	value := c.Query("value")
	limitStr := c.DefaultQuery("limit", "20")

	if key == "" || value == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Both 'key' and 'value' parameters are required",
		})
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	records, err := api.memoryStore.GetRoundsByMetadata(key, value, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve rounds",
		})
		return
	}

	// Convert to response format
	items := make([]MemoryRoundItem, len(records))
	for i, r := range records {
		items[i] = convertToMemoryRoundItem(r)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
	})
}

// DeleteOldRecords deletes memory records older than the specified number of days
func (api *MemoryAPI) DeleteOldRecords(c *gin.Context) {
	if api.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Memory store not available",
		})
		return
	}

	daysStr := c.Query("days")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Valid 'days' parameter is required (positive integer)",
		})
		return
	}

	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, 0, -days)

	count, err := api.memoryStore.DeleteOlderThan(cutoffDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete old records",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Deleted %d records older than %d days", count, days),
		"data": gin.H{
			"deleted_count": count,
			"cutoff_days":   days,
		},
	})
}

// GetUserInputsList retrieves lightweight list for memory user page with date range filtering
func (api *MemoryAPI) GetUserInputsList(c *gin.Context) {
	if api.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Memory store not available",
		})
		return
	}

	scenario := c.Query("scenario")
	protocol := c.Query("protocol")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	limitStr := c.DefaultQuery("limit", "100")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	records, err := api.memoryStore.GetUserInputsList(scenario, protocol, startDate, endDate, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve memory list",
		})
		return
	}

	// Convert to lightweight list format
	items := make([]MemoryRoundListItem, len(records))
	for i, r := range records {
		items[i] = MemoryRoundListItem{
			ID:           r.ID,
			Scenario:     r.Scenario,
			ProviderName: r.ProviderName,
			Model:        r.Model,
			Protocol:     string(r.Protocol),
			CreatedAt:    r.CreatedAt.Format("2006-01-02T15:04:05Z"),
			IsStreaming:  r.IsStreaming,
			HasToolUse:   r.HasToolUse,
			UserInput:    r.UserInput,
		}
	}

	c.JSON(http.StatusOK, MemoryRoundListResponse{
		Success: true,
		Data: MemoryRoundListData{
			Rounds: convertToListItems(items),
			Total:  int64(len(items)),
		},
	})
}

// GetRoundDetail retrieves full details for a specific memory round
func (api *MemoryAPI) GetRoundDetail(c *gin.Context) {
	if api.memoryStore == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Memory store not available",
		})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid ID format",
		})
		return
	}

	record, err := api.memoryStore.GetRoundByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Memory round not found",
		})
		return
	}

	c.JSON(http.StatusOK, MemoryRoundDetailResponse{
		Success: true,
		Data:    convertToMemoryRoundItem(*record),
	})
}

// convertToMemoryRoundItem converts a db record to API response format
func convertToMemoryRoundItem(r db.MemoryRoundRecord) MemoryRoundItem {
	// Parse metadata JSON string to map
	var metadata map[string]string
	if r.Metadata != "" {
		json.Unmarshal([]byte(r.Metadata), &metadata)
	}

	return MemoryRoundItem{
		ID:           r.ID,
		Scenario:     r.Scenario,
		ProviderUUID: r.ProviderUUID,
		ProviderName: r.ProviderName,
		Model:        r.Model,
		Protocol:     string(r.Protocol),
		RequestID:    r.RequestID,
		ProjectID:    r.ProjectID,
		SessionID:    r.SessionID,
		Metadata:     metadata,
		RoundIndex:   r.RoundIndex,
		UserInput:    r.UserInput,
		RoundResult:  r.RoundResult,
		InputTokens:  r.InputTokens,
		OutputTokens: r.OutputTokens,
		TotalTokens:  r.TotalTokens,
		CreatedAt:    r.CreatedAt.Format("2006-01-02T15:04:05Z"),
		IsStreaming:  r.IsStreaming,
		HasToolUse:   r.HasToolUse,
	}
}

// convertToListItems converts lightweight list items to full MemoryRoundItem format
// This is needed because the frontend expects the full type structure
func convertToListItems(items []MemoryRoundListItem) []MemoryRoundItem {
	result := make([]MemoryRoundItem, len(items))
	for i, item := range items {
		result[i] = MemoryRoundItem{
			ID:           item.ID,
			Scenario:     item.Scenario,
			ProviderName: item.ProviderName,
			Model:        item.Model,
			Protocol:     item.Protocol,
			CreatedAt:    item.CreatedAt,
			IsStreaming:  item.IsStreaming,
			HasToolUse:   item.HasToolUse,
			UserInput:    item.UserInput,
		}
	}
	return result
}
