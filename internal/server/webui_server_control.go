package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

func (s *Server) GetStatus(c *gin.Context) {
	providers := s.config.ListProviders()
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
	response.Data.Port = s.config.GetServerPort()
	response.Data.ProvidersTotal = len(providers)
	response.Data.ProvidersEnabled = enabledCount
	response.Data.RequestCount = 0

	c.JSON(http.StatusOK, response)
}

func (s *Server) GetHistory(c *gin.Context) {
	response := HistoryResponse{
		Success: true,
	}

	if s.multiLogger != nil {
		actionLogger := s.multiLogger.WithSource(pkgobs.LogSourceAction)
		history := actionLogger.GetMemoryLatest(50)
		response.Data = history
	} else {
		response.Data = []interface{}{}
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) StartServer(c *gin.Context) {
	response := ServerActionResponse{
		Success: false,
		Message: "Start server via web UI not supported. Please use CLI: tingly start",
	}
	c.JSON(http.StatusNotImplemented, response)
}

func (s *Server) StopServer(c *gin.Context) {
	// Get the global server instance
	server := GetGlobalServer()
	if server == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "No server instance available to stop",
		})
		return
	}

	// Stop the server gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to stop server: %v", err),
		})
		return
	}

	// Log the action
	logrus.WithFields(logrus.Fields{
		"action": obs.ActionStopServer,
		"source": "web_ui",
	}).Info("Server stopped via web interface")

	// Send shutdown signal to main process
	select {
	case shutdownChan <- struct{}{}:
	default:
		// Channel already has a signal
	}

	response := ServerActionResponse{
		Success: true,
		Message: "Server stopped successfully. The application will now exit.",
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) RestartServer(c *gin.Context) {
	response := ServerActionResponse{
		Success: false,
		Message: "Restart server via web UI not supported. Please use CLI: tingly restart",
	}
	c.JSON(http.StatusNotImplemented, response)
}
