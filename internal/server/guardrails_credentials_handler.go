package server

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
)

type protectedCredentialResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	AliasToken  string   `json:"alias_token"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Enabled     bool     `json:"enabled"`
	SecretMask  string   `json:"secret_mask"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

type protectedCredentialDetailResponse struct {
	protectedCredentialResponse
	Secret string `json:"secret,omitempty"`
}

type protectedCredentialsListResponse struct {
	Data []protectedCredentialResponse `json:"data"`
}

type protectedCredentialCreateRequest struct {
	Name        string   `json:"name" binding:"required"`
	Type        string   `json:"type" binding:"required"`
	Secret      string   `json:"secret" binding:"required"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

type protectedCredentialUpdateRequest struct {
	Name        *string  `json:"name,omitempty"`
	Type        *string  `json:"type,omitempty"`
	Secret      *string  `json:"secret,omitempty"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

type protectedCredentialMutationResponse struct {
	Success    bool                        `json:"success"`
	Credential protectedCredentialResponse `json:"credential"`
}

func (s *Server) guardrailsCredentialStore() (*guardrails.ProtectedCredentialStore, error) {
	if s.config == nil || s.config.ConfigDir == "" {
		return nil, errors.New("config directory not set")
	}
	return guardrails.NewProtectedCredentialStore(getGuardrailsCredentialsPath(s.config.ConfigDir)), nil
}

func toProtectedCredentialResponse(credential guardrails.ProtectedCredential) protectedCredentialResponse {
	return protectedCredentialResponse{
		ID:          credential.ID,
		Name:        credential.Name,
		Type:        credential.Type,
		AliasToken:  credential.AliasToken,
		Description: credential.Description,
		Tags:        credential.Tags,
		Enabled:     credential.Enabled,
		SecretMask:  guardrails.MaskedSecret(credential.Secret),
		CreatedAt:   credential.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   credential.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// GetGuardrailsCredentials returns protected credentials without exposing raw secrets.
func (s *Server) GetGuardrailsCredentials(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	credentials, err := store.List()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	response := make([]protectedCredentialResponse, 0, len(credentials))
	for _, credential := range credentials {
		response = append(response, toProtectedCredentialResponse(credential))
	}
	c.JSON(200, protectedCredentialsListResponse{Data: response})
}

// GetGuardrailsCredential returns a single protected credential, including the
// current secret, for the local editor dialog. Secrets stay hidden by default in
// the UI and are only fetched when the user opens the edit flow.
func (s *Server) GetGuardrailsCredential(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	credentialID := strings.TrimSpace(c.Param("id"))
	if credentialID == "" {
		c.JSON(400, gin.H{"success": false, "error": "credential id is required"})
		return
	}

	resolved, err := store.Resolve([]string{credentialID})
	if err != nil {
		status := 400
		if errors.Is(err, guardrails.ErrProtectedCredentialNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	if len(resolved) == 0 {
		c.JSON(404, gin.H{"success": false, "error": "protected credential not found"})
		return
	}

	response := toProtectedCredentialResponse(resolved[0])
	c.JSON(200, gin.H{
		"success": true,
		"data": protectedCredentialDetailResponse{
			protectedCredentialResponse: response,
			Secret:                      resolved[0].Secret,
		},
	})
}

func (s *Server) CreateGuardrailsCredential(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	var req protectedCredentialCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	credential, err := guardrails.NewProtectedCredential(req.Name, req.Type, req.Secret, req.Description, req.Tags, enabled)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	credential, err = store.Create(credential)
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	s.refreshGuardrailsCredentialCacheOrWarn("guardrails credential create")

	c.JSON(200, protectedCredentialMutationResponse{
		Success:    true,
		Credential: toProtectedCredentialResponse(credential),
	})
}

func (s *Server) UpdateGuardrailsCredential(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	credentialID := strings.TrimSpace(c.Param("id"))
	if credentialID == "" {
		c.JSON(400, gin.H{"success": false, "error": "credential id is required"})
		return
	}

	var req protectedCredentialUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}

	updated, err := store.Update(credentialID, func(existing *guardrails.ProtectedCredential) error {
		name := existing.Name
		if req.Name != nil {
			name = *req.Name
		}
		credentialType := existing.Type
		if req.Type != nil {
			credentialType = *req.Type
		}
		secret := ""
		if req.Secret != nil {
			secret = *req.Secret
		}
		description := existing.Description
		if req.Description != nil {
			description = *req.Description
		}
		tags := existing.Tags
		if req.Tags != nil {
			tags = req.Tags
		}
		enabled := existing.Enabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		return guardrails.UpdateProtectedCredential(existing, name, credentialType, secret, description, tags, enabled)
	})
	if err != nil {
		status := 400
		if errors.Is(err, guardrails.ErrProtectedCredentialNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	s.refreshGuardrailsCredentialCacheOrWarn("guardrails credential update")

	c.JSON(200, protectedCredentialMutationResponse{
		Success:    true,
		Credential: toProtectedCredentialResponse(updated),
	})
}

func (s *Server) DeleteGuardrailsCredential(c *gin.Context) {
	store, err := s.guardrailsCredentialStore()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	credentialID := strings.TrimSpace(c.Param("id"))
	if credentialID == "" {
		c.JSON(400, gin.H{"success": false, "error": "credential id is required"})
		return
	}
	if err := store.Delete(credentialID); err != nil {
		status := 400
		if errors.Is(err, guardrails.ErrProtectedCredentialNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	s.refreshGuardrailsCredentialCacheOrWarn("guardrails credential delete")
	c.JSON(200, gin.H{"success": true, "credential_id": credentialID})
}
