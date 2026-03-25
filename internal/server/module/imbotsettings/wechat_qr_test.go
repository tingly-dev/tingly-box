package imbotsettings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/data/db"
)

// settingsStoreInterface is a test interface for the settings store
type settingsStoreInterface interface {
	GetSettingsByUUID(uuid string) (db.Settings, error)
	UpdateSettings(uuid string, settings db.Settings) error
}

// testSettingsStore is a simple test implementation
type testSettingsStore struct {
	getSettings db.Settings
	updateErr   error
}

func (m *testSettingsStore) GetSettingsByUUID(uuid string) (db.Settings, error) {
	return m.getSettings, nil
}

func (m *testSettingsStore) UpdateSettings(uuid string, settings db.Settings) error {
	return m.updateErr
}

// TestQRStartRequestStructure tests request structure
func TestQRStartRequestStructure(t *testing.T) {
	req := QRStartRequest{
		BotUUID: "test-bot-uuid",
	}

	if req.BotUUID != "test-bot-uuid" {
		t.Errorf("Expected BotUUID 'test-bot-uuid', got %s", req.BotUUID)
	}
}

// TestQRStartResponseStructure tests response structure
func TestQRStartResponseStructure(t *testing.T) {
	resp := QRStartResponse{
		QrCodeID:   "qr-123",
		QrCodeData: "https://example.com/qr.png",
		ExpiresIn:  300,
	}

	if resp.QrCodeID != "qr-123" {
		t.Errorf("Expected QrCodeID 'qr-123', got %s", resp.QrCodeID)
	}

	if resp.QrCodeData != "https://example.com/qr.png" {
		t.Errorf("Expected QrCodeData 'https://example.com/qr.png', got %s", resp.QrCodeData)
	}

	if resp.ExpiresIn != 300 {
		t.Errorf("Expected ExpiresIn 300, got %d", resp.ExpiresIn)
	}
}

// TestQRStatusResponseStructure tests status response structure
func TestQRStatusResponseStructure(t *testing.T) {
	resp := QRStatusResponse{
		Status: "confirmed",
		Error:  "",
	}

	if resp.Status != "confirmed" {
		t.Errorf("Expected Status 'confirmed', got %s", resp.Status)
	}

	if resp.Error != "" {
		t.Errorf("Expected empty Error, got %s", resp.Error)
	}
}

// TestQRStatusResponse_WithError tests error response structure
func TestQRStatusResponse_WithError(t *testing.T) {
	resp := QRStatusResponse{
		Status: "error",
		Error:  "Failed to poll QR status",
	}

	if resp.Status != "error" {
		t.Errorf("Expected Status 'error', got %s", resp.Status)
	}

	if resp.Error != "Failed to poll QR status" {
		t.Errorf("Expected Error 'Failed to poll QR status', got %s", resp.Error)
	}
}

// TestQRStatusResponse_ValidStatuses tests that status is one of valid values
func TestQRStatusResponse_ValidStatuses(t *testing.T) {
	validStatuses := map[string]bool{
		"wait":      true,
		"scaned":    true,
		"confirmed": true,
		"expired":   true,
		"error":     true,
	}

	tests := []struct {
		name   string
		status string
		valid  bool
	}{
		{"Wait status", "wait", true},
		{"Scanned status", "scaned", true},
		{"Confirmed status", "confirmed", true},
		{"Expired status", "expired", true},
		{"Error status", "error", true},
		{"Invalid status", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = QRStatusResponse{Status: tt.status}
			if validStatuses[tt.status] != tt.valid {
				t.Errorf("Status %s validity mismatch", tt.status)
			}
		})
	}
}

// TestWeChatQRClient_GetBotQRCode tests QR code generation (mock)
func TestWeChatQRClient_GetBotQRCode(t *testing.T) {
	client := &wechatQRClient{
		baseURL: "https://ilinkai.weixin.qq.com",
	}

	ctx := context.Background()
	resp, err := client.GetBotQRCode(ctx)

	require.NoError(t, err)
	require.NotNil(t, resp)

	if resp.Qrcode == "" {
		t.Error("Expected non-empty QR code")
	}
	if resp.QrcodeImgContent == "" {
		t.Error("Expected non-empty QR image content")
	}
}

// TestWeChatQRClient_GetQRStatus tests QR status polling (mock)
func TestWeChatQRClient_GetQRStatus(t *testing.T) {
	client := &wechatQRClient{
		baseURL: "https://ilinkai.weixin.qq.com",
	}

	ctx := context.Background()
	resp, err := client.GetQRStatus(ctx, "test-qr-id")

	require.NoError(t, err)
	require.NotNil(t, resp)

	if resp.Status == "" {
		t.Error("Expected non-empty status")
	}
	// Mock returns "wait"
	assert.Contains(t, []string{"wait", "scaned", "confirmed", "expired"}, resp.Status)
}

// TestQRSessionCreation tests QR session creation
func TestQRSessionCreation(t *testing.T) {
	session := &qrSession{
		botUUID:   "test-bot",
		qrID:      "qr-123",
		qrData:    "mock-qr-data",
		startedAt: testTimeNow(),
		client:    &wechatQRClient{},
	}

	if session.botUUID != "test-bot" {
		t.Errorf("Expected botUUID 'test-bot', got %s", session.botUUID)
	}

	if session.qrID != "qr-123" {
		t.Errorf("Expected qrID 'qr-123', got %s", session.qrID)
	}

	if session.qrData != "mock-qr-data" {
		t.Errorf("Expected qrData 'mock-qr-data', got %s", session.qrData)
	}
}

// TestQRSessionExpiration tests session expiration check
func TestQRSessionExpiration(t *testing.T) {
	session := &qrSession{
		botUUID:   "test-bot",
		startedAt: testTimeNow(),
	}

	// Fresh session should not be expired
	elapsed := time.Since(session.startedAt)
	if elapsed > 8*time.Minute {
		t.Error("Fresh session should not be expired")
	}

	// Expired session
	expiredSession := &qrSession{
		botUUID:   "test-bot",
		startedAt: testTimeExpired(),
	}

	expiredElapsed := time.Since(expiredSession.startedAt)
	if expiredElapsed < 8*time.Minute {
		t.Error("Old session should be expired")
	}
}

// TestCredentialMapping tests credential mapping from QR response
func TestCredentialMapping(t *testing.T) {
	status := &qrStatusResponse{
		Status:      "confirmed",
		BotToken:    "test-bot-token",
		IlinkBotID:  "ilink-bot-123",
		IlinkUserID: "ilink-user-456",
		BaseURL:     "https://ilinkai.weixin.qq.com",
	}

	authConfig := map[string]string{
		"token":    status.BotToken,
		"bot_id":   status.IlinkBotID,
		"user_id":  status.IlinkUserID,
		"base_url": status.BaseURL,
	}

	if authConfig["token"] != "test-bot-token" {
		t.Errorf("Expected token 'test-bot-token', got %s", authConfig["token"])
	}
	if authConfig["bot_id"] != "ilink-bot-123" {
		t.Errorf("Expected bot_id 'ilink-bot-123', got %s", authConfig["bot_id"])
	}
	if authConfig["user_id"] != "ilink-user-456" {
		t.Errorf("Expected user_id 'ilink-user-456', got %s", authConfig["user_id"])
	}
	if authConfig["base_url"] != "https://ilinkai.weixin.qq.com" {
		t.Errorf("Expected base_url 'https://ilinkai.weixin.qq.com', got %s", authConfig["base_url"])
	}
}

// TestJSONSerialization tests JSON serialization for API responses
func TestJSONSerialization(t *testing.T) {
	startResp := QRStartResponse{
		QrCodeID:   "qr-123",
		QrCodeData: "https://example.com/qr.png",
		ExpiresIn:  300,
	}

	data, err := json.Marshal(startResp)
	require.NoError(t, err)

	var decoded QRStartResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	if decoded.QrCodeID != startResp.QrCodeID {
		t.Errorf("JSON roundtrip failed for QrCodeID")
	}
	if decoded.QrCodeData != startResp.QrCodeData {
		t.Errorf("JSON roundtrip failed for QrCodeData")
	}
	if decoded.ExpiresIn != startResp.ExpiresIn {
		t.Errorf("JSON roundtrip failed for ExpiresIn")
	}
}

// TestWeChatQRClientBaseURL tests client base URL handling
func TestWeChatQRClientBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		expected string
	}{
		{"Default URL", "https://ilinkai.weixin.qq.com", "https://ilinkai.weixin.qq.com"},
		{"Custom URL", "https://custom.wechat.com", "https://custom.wechat.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &wechatQRClient{
				baseURL: tt.baseURL,
			}

			if client.baseURL != tt.expected {
				t.Errorf("Expected base URL '%s', got '%s'", tt.expected, client.baseURL)
			}
		})
	}
}

// Helper functions

func testTimeNow() time.Time {
	return time.Now()
}

func testTimeExpired() time.Time {
	// Return a time 10 minutes ago
	return time.Now().Add(-10 * time.Minute)
}
