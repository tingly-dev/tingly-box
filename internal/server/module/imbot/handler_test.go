package imbot

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

func TestListSettings_NilStore(t *testing.T) {
	handler := &Handler{
		config: nil,
		store:  nil,
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/settings", handler.ListSettings)

	req, _ := http.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, "ImBot settings store not available")
}

func TestGetSettings_NilStore(t *testing.T) {
	handler := &Handler{
		config: nil,
		store:  nil,
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/settings/:uuid", handler.GetSettings)

	req, _ := http.NewRequest("GET", "/settings/test-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	body := w.Body.String()
	assert.Contains(t, body, "ImBot settings store not available")
}

func TestListResponseStructure(t *testing.T) {
	settings := []db.Settings{
		{
			UUID:     "uuid1",
			Name:     "Bot 1",
			Platform: "telegram",
			Enabled:  true,
		},
		{
			UUID:     "uuid2",
			Name:     "Bot 2",
			Platform: "discord",
			Enabled:  false,
		},
	}

	response := ListResponse{
		Success:  true,
		Settings: settings,
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if len(response.Settings) != 2 {
		t.Errorf("expected 2 settings, got %d", len(response.Settings))
	}

	if response.Settings[0].Name != "Bot 1" {
		t.Errorf("expected first settings name 'Bot 1', got %q", response.Settings[0].Name)
	}
}

func TestSettingsResponseStructure(t *testing.T) {
	settings := db.Settings{
		UUID:     "test-uuid",
		Name:     "Test Bot",
		Platform: "telegram",
		Enabled:  true,
	}

	response := SettingsResponse{
		Success:  true,
		Settings: settings,
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.Settings.UUID != "test-uuid" {
		t.Errorf("expected UUID 'test-uuid', got %q", response.Settings.UUID)
	}

	if response.Settings.Name != "Test Bot" {
		t.Errorf("expected Name 'Test Bot', got %q", response.Settings.Name)
	}
}

func TestCreateRequestStructure(t *testing.T) {
	auth := map[string]string{
		"token": "test-token",
	}

	request := CreateRequest{
		UUID:     "test-uuid",
		Name:     "Test Bot",
		Platform: "telegram",
		AuthType: "token",
		Auth:     auth,
		Enabled:  true,
	}

	if request.UUID != "test-uuid" {
		t.Errorf("expected UUID 'test-uuid', got %q", request.UUID)
	}

	if request.Platform != "telegram" {
		t.Errorf("expected Platform 'telegram', got %q", request.Platform)
	}

	if request.Auth["token"] != "test-token" {
		t.Errorf("expected Auth token 'test-token', got %q", request.Auth["token"])
	}

	if !request.Enabled {
		t.Error("expected Enabled to be true")
	}
}

func TestUpdateRequestStructure(t *testing.T) {
	enabled := true
	smartGuideProvider := "provider-uuid"

	request := UpdateRequest{
		Name:               "Updated Bot",
		Platform:           "discord",
		Enabled:            &enabled,
		SmartGuideProvider: &smartGuideProvider,
	}

	if request.Name != "Updated Bot" {
		t.Errorf("expected Name 'Updated Bot', got %q", request.Name)
	}

	if request.Platform != "discord" {
		t.Errorf("expected Platform 'discord', got %q", request.Platform)
	}

	if *request.Enabled != true {
		t.Error("expected Enabled to be true")
	}

	if *request.SmartGuideProvider != "provider-uuid" {
		t.Errorf("expected Provider 'provider-uuid', got %q", *request.SmartGuideProvider)
	}
}

func TestToggleResponseStructure(t *testing.T) {
	response := ToggleResponse{
		Success: true,
		Enabled: true,
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if !response.Enabled {
		t.Error("expected Enabled to be true")
	}
}

func TestPlatformsResponseStructure(t *testing.T) {
	platforms := []PlatformConfig{
		{
			Platform:    "telegram",
			DisplayName: "Telegram",
			AuthType:    "token",
			Category:    "messaging",
		},
	}

	response := PlatformsResponse{
		Success:   true,
		Platforms: platforms,
		Categories: gin.H{
			"messaging": []string{"telegram", "discord"},
		},
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if len(response.Platforms) != 1 {
		t.Errorf("expected 1 platform, got %d", len(response.Platforms))
	}

	if response.Platforms[0].Platform != "telegram" {
		t.Errorf("expected Platform 'telegram', got %q", response.Platforms[0].Platform)
	}
}

func TestPlatformConfigStructure(t *testing.T) {
	config := PlatformConfig{
		Platform:    "telegram",
		DisplayName: "Telegram Bot",
		AuthType:    "token",
		Category:    "messaging",
		Fields: []imbot.FieldSpec{
			{
				Key:      "token",
				Label:    "Bot Token",
				Required: true,
				Secret:   true,
			},
		},
	}

	if config.Platform != "telegram" {
		t.Errorf("expected Platform 'telegram', got %q", config.Platform)
	}

	if config.DisplayName != "Telegram Bot" {
		t.Errorf("expected DisplayName 'Telegram Bot', got %q", config.DisplayName)
	}

	if len(config.Fields) != 1 {
		t.Errorf("expected 1 field, got %d", len(config.Fields))
	}
}

func TestDeleteResponseStructure(t *testing.T) {
	response := DeleteResponse{
		Success: true,
		Message: "Settings deleted successfully",
	}

	if !response.Success {
		t.Error("expected Success to be true")
	}

	if response.Message != "Settings deleted successfully" {
		t.Errorf("expected Message 'Settings deleted successfully', got %q", response.Message)
	}
}
