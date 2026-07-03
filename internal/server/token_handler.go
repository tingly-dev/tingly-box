package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TokenResponse represents the token response
type TokenResponse struct {
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	Type  string `json:"type" example:"Bearer"`
}

// tokenErrorResponse mirrors server.ErrorResponse's wire format. Kept local
// to webui (rather than importing the root server package for it) to avoid
// an import cycle — the root server package already imports webui.
type tokenErrorResponse struct {
	Error tokenErrorDetail `json:"error"`
}

type tokenErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// GenerateToken handles token generation requests
func (h *WebHandler) GenerateToken(c *gin.Context) {
	var req struct {
		ClientID string `json:"client_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, tokenErrorResponse{
			Error: tokenErrorDetail{
				Message: "Invalid request body: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	token, err := h.deps.JWTManager.GenerateToken(req.ClientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, tokenErrorResponse{
			Error: tokenErrorDetail{
				Message: "Failed to generate token: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	token = "tingly-box-" + token
	err = h.deps.Config.SetModelToken(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, tokenErrorResponse{
			Error: tokenErrorDetail{
				Message: "Failed to save token: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	response := struct {
		Success bool          `json:"success"`
		Data    TokenResponse `json:"data"`
	}{
		Success: true,
		Data:    TokenResponse{Token: token, Type: "Bearer"},
	}

	c.JSON(http.StatusOK, response)
}

// GetToken handles token retrieval requests - generates a token if it doesn't exist
func (h *WebHandler) GetToken(c *gin.Context) {
	globalConfig := h.deps.Config

	// Check if token already exists
	if globalConfig != nil && globalConfig.HasModelToken() {
		token := globalConfig.GetModelToken()
		c.JSON(http.StatusOK, gin.H{
			"token": token,
			"type":  "Bearer",
		})
		return
	}

	// Generate a new token if it doesn't exist
	// Use a default client ID for automatic token generation
	clientID := "auto-generated"
	token, err := h.deps.JWTManager.GenerateToken(clientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, tokenErrorResponse{
			Error: tokenErrorDetail{
				Message: "Failed to generate token: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Save the token to config
	token = "tingly-box-" + token
	err = globalConfig.SetModelToken(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, tokenErrorResponse{
			Error: tokenErrorDetail{
				Message: "Failed to save token: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	response := struct {
		Success bool          `json:"success"`
		Data    TokenResponse `json:"data"`
	}{
		Success: true,
		Data:    TokenResponse{Token: token, Type: "Bearer"},
	}

	c.JSON(http.StatusOK, response)
}
