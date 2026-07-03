package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

// StatusResponse represents the server status API response
type StatusResponse struct {
	Success bool `json:"success" example:"true"`
	Data    struct {
		ServerRunning    bool `json:"server_running" example:"true"`
		Port             int  `json:"port" example:"12580"`
		ProvidersTotal   int  `json:"providers_total" example:"3"`
		ProvidersEnabled int  `json:"providers_enabled" example:"2"`
		RequestCount     int  `json:"request_count" example:"100"`
	} `json:"data"`
}

// HistoryResponse represents the response for request history
type HistoryResponse struct {
	Success bool        `json:"success" example:"true"`
	Data    interface{} `json:"data"`
}

// ServerActionResponse represents the response for server actions (start/stop/restart)
type ServerActionResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Server stopped successfully"`
}

// GetStatus returns server status and statistics.
func (h *WebHandler) GetStatus(c *gin.Context) {
	providers := h.deps.Config.ListProviders()
	enabledCount := 0
	for _, p := range providers {
		if p.Enabled {
			enabledCount++
		}
	}

	response := StatusResponse{
		Success: true,
	}
	response.Data.ServerRunning = true
	response.Data.Port = h.deps.Config.GetServerPort()
	response.Data.ProvidersTotal = len(providers)
	response.Data.ProvidersEnabled = enabledCount
	response.Data.RequestCount = 0

	c.JSON(http.StatusOK, response)
}

// GetHistory returns request history from the action log.
func (h *WebHandler) GetHistory(c *gin.Context) {
	response := HistoryResponse{
		Success: true,
	}

	if h.deps.MultiLogger != nil {
		actionLogger := h.deps.MultiLogger.WithSource(pkgobs.LogSourceAction)
		history := actionLogger.GetMemoryLatest(50)
		response.Data = history
	} else {
		response.Data = []interface{}{}
	}

	c.JSON(http.StatusOK, response)
}

// StartServer is a placeholder: starting the server via the web UI is not
// supported — the server itself must already be running to serve this
// request, so start would be a no-op even if implemented.
func (h *WebHandler) StartServer(c *gin.Context) {
	response := ServerActionResponse{
		Success: false,
		Message: "Start server via web UI not supported. Please use CLI: tingly start",
	}
	c.JSON(http.StatusNotImplemented, response)
}

// RestartServer is a placeholder: restarting via the web UI is not supported.
func (h *WebHandler) RestartServer(c *gin.Context) {
	response := ServerActionResponse{
		Success: false,
		Message: "Restart server via web UI not supported. Please use CLI: tingly restart",
	}
	c.JSON(http.StatusNotImplemented, response)
}
