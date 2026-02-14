package testing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// MockServer is a mock HTTP server for testing platform APIs
type MockServer struct {
	Server    *httptest.Server
	handler   http.HandlerFunc
	mu        sync.RWMutex
	requests  []*http.Request
	responses map[string][]byte
	delay     time.Duration
}

// NewMockServer creates a new mock server
func NewMockServer(t *testing.T) *MockServer {
	ms := &MockServer{
		responses: make(map[string][]byte),
		delay:     0,
	}

	ms.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ms.mu.Lock()
		defer ms.mu.Unlock()

		// Record request
		ms.requests = append(ms.requests, r)

		// Check for predefined response
		if resp, ok := ms.responses[r.URL.Path]; ok {
			// Add delay if configured
			if ms.delay > 0 {
				time.Sleep(ms.delay)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(resp)
			return
		}

		// Use custom handler if set
		if ms.handler != nil {
			ms.handler(w, r)
			return
		}

		// Default 404
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))

	t.Cleanup(func() {
		ms.Close()
	})

	return ms
}

// Close closes the mock server
func (ms *MockServer) Close() {
	if ms.Server != nil {
		ms.Server.Close()
	}
}

// URL returns the server's URL
func (ms *MockServer) URL() string {
	return ms.Server.URL
}

// SetResponse sets a predefined response for a path
func (ms *MockServer) SetResponse(path string, response interface{}) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	data, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	ms.responses[path] = data
}

// SetHandler sets a custom handler
func (ms *MockServer) SetHandler(handler http.HandlerFunc) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.handler = handler
}

// SetDelay sets the response delay
func (ms *MockServer) SetDelay(delay time.Duration) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.delay = delay
}

// GetRequests returns all recorded requests
func (ms *MockServer) GetRequests() []*http.Request {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	requests := make([]*http.Request, len(ms.requests))
	copy(requests, ms.requests)
	return requests
}

// ClearRequests clears all recorded requests
func (ms *MockServer) ClearRequests() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.requests = nil
}

// GetRequestCount returns the number of requests to a path
func (ms *MockServer) GetRequestCount(path string) int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	count := 0
	for _, req := range ms.requests {
		if req.URL.Path == path {
			count++
		}
	}
	return count
}

// PlatformMockServer is a platform-specific mock server
type PlatformMockServer struct {
	*MockServer
	Platform core.Platform
}

// NewTelegramMockServer creates a mock Telegram server
func NewTelegramMockServer(t *testing.T) *PlatformMockServer {
	ms := &PlatformMockServer{
		MockServer: NewMockServer(t),
		Platform:   core.PlatformTelegram,
	}

	ms.SetHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bot" + ms.getBotToken() + "/getMe":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"id":                          123456789,
					"is_bot":                      true,
					"first_name":                  "TestBot",
					"username":                    "test_bot",
					"can_join_groups":             true,
					"can_read_all_group_messages": true,
				},
			})
		case "/bot" + ms.getBotToken() + "/sendMessage":
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok": true,
					"result": map[string]interface{}{
						"message_id": 12345,
						"date":       time.Now().Unix(),
					},
				})
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return ms
}

// NewDiscordMockServer creates a mock Discord server
func NewDiscordMockServer(t *testing.T) *PlatformMockServer {
	ms := &PlatformMockServer{
		MockServer: NewMockServer(t),
		Platform:   core.PlatformDiscord,
	}

	ms.SetHandler(func(w http.ResponseWriter, r *http.Request) {
		// Discord gateway mock
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"url": ms.Server.URL + "/gateway",
		})
	})

	return ms
}

// NewSlackMockServer creates a mock Slack server
func NewSlackMockServer(t *testing.T) *PlatformMockServer {
	ms := &PlatformMockServer{
		MockServer: NewMockServer(t),
		Platform:   core.PlatformSlack,
	}

	ms.SetHandler(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth.test":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":      true,
				"url":     ms.Server.URL,
				"team":    "T123456",
				"user":    "U123456",
				"team_id": "T123456",
			})
		case "/api/chat.postMessage":
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":      true,
					"ts":      fmt.Sprintf("%d.000000", time.Now().Unix()),
					"channel": req["channel"],
				})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return ms
}

// NewFeishuMockServer creates a mock Feishu/Lark server
func NewFeishuMockServer(t *testing.T) *PlatformMockServer {
	ms := &PlatformMockServer{
		MockServer: NewMockServer(t),
		Platform:   core.PlatformFeishu,
	}

	ms.SetHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/auth/v3/tenant_access_token/internal":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":                0,
				"tenant_access_token": "mock-access-token",
				"expire":              7200,
			})
		case r.URL.Path == "/v1/im/v1/messages":
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"code":   0,
					"msg":    "success",
					"msg_id": "mock_msg_" + fmt.Sprint(time.Now().UnixNano()),
				})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 9999,
				"msg":  "not found",
			})
		}
	})

	return ms
}

// NewWhatsAppMockServer creates a mock WhatsApp server
func NewWhatsAppMockServer(t *testing.T) *PlatformMockServer {
	ms := &PlatformMockServer{
		MockServer: NewMockServer(t),
		Platform:   core.PlatformWhatsApp,
	}

	ms.SetHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == fmt.Sprintf("/%s/phone_numbers", ms.getPhoneID()):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"phone_numbers": []map[string]interface{}{
					{
						"id":                   ms.getPhoneID(),
						"verified_name":        "Test Business",
						"display_phone_number": "+1234567890",
					},
				},
			})
		case r.URL.Path == fmt.Sprintf("/%s/messages", ms.getPhoneID()):
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"messaging_product": "whatsapp",
					"contacts": []map[string]string{
						{
							"input": req["recipient"].(map[string]string)["phone_number"],
							"wa_id": req["recipient"].(map[string]string)["phone_number"],
						},
					},
					"messages": []map[string]string{
						{
							"id": fmt.Sprintf("wamid.%s", fmt.Sprint(time.Now().UnixNano())),
						},
					},
				})
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return ms
}

// Helper methods

func (ms *PlatformMockServer) getBotToken() string {
	return "mock-bot-token-12345"
}

func (ms *PlatformMockServer) getPhoneID() string {
	return "123456789012"
}

// BotTestHelper provides utilities for testing bots
type BotTestHelper struct {
	t          *testing.T
	config     *core.Config
	mockServer *PlatformMockServer
	botFactory func(*core.Config) (core.Bot, error)
}

// NewBotTestHelper creates a new bot test helper
func NewBotTestHelper(t *testing.T, platform core.Platform) *BotTestHelper {
	helper := &BotTestHelper{t: t}

	// Create mock server
	switch platform {
	case core.PlatformTelegram:
		helper.mockServer = NewTelegramMockServer(t)
	case core.PlatformDiscord:
		helper.mockServer = NewDiscordMockServer(t)
	case core.PlatformSlack:
		helper.mockServer = NewSlackMockServer(t)
	case core.PlatformFeishu:
		helper.mockServer = NewFeishuMockServer(t)
	case core.PlatformWhatsApp:
		helper.mockServer = NewWhatsAppMockServer(t)
	default:
		// Create a generic platform mock server
		helper.mockServer = &PlatformMockServer{
			MockServer: NewMockServer(t),
			Platform:   platform,
		}
	}

	// Create config with mock server URL
	helper.config = &core.Config{
		Platform: platform,
		Enabled:  true,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: helper.mockServer.getBotToken(),
		},
		Options: map[string]interface{}{
			"apiURL": helper.mockServer.URL(),
		},
		Logging: &core.LoggingConfig{
			Level:      "debug",
			Timestamps: false,
		},
	}

	return helper
}

// SetBotFactory sets the bot factory function
func (h *BotTestHelper) SetBotFactory(factory func(*core.Config) (core.Bot, error)) {
	h.botFactory = factory
}

// GetConfig returns the test configuration
func (h *BotTestHelper) GetConfig() *core.Config {
	return h.config
}

// GetMockServer returns the mock server
func (h *BotTestHelper) GetMockServer() *PlatformMockServer {
	return h.mockServer
}

// GetMockServerForTests returns the mock server for direct access in tests
// Use this for accessing request counts and other mock server details
func (h *BotTestHelper) GetMockServerForTests() *PlatformMockServer {
	return h.mockServer
}

// CreateBot creates a bot instance with the test config
func (h *BotTestHelper) CreateBot() (core.Bot, error) {
	if h.botFactory != nil {
		return h.botFactory(h.config)
	}
	// Fallback - tests must set a factory function
	return nil, fmt.Errorf("bot factory not set, call SetBotFactory first")
}

// AssertConnected asserts the bot is connected
func (h *BotTestHelper) AssertConnected(bot core.Bot) {
	if !bot.IsConnected() {
		h.t.Errorf("Bot should be connected")
	}
}

// AssertNotConnected asserts the bot is not connected
func (h *BotTestHelper) AssertNotConnected(bot core.Bot) {
	if bot.IsConnected() {
		h.t.Errorf("Bot should not be connected")
	}
}

// AssertReady asserts the bot is ready
func (h *BotTestHelper) AssertReady(bot core.Bot) {
	status := bot.Status()
	if !status.Ready {
		h.t.Errorf("Bot should be ready")
	}
}

// AssertMessageSent asserts a message was sent to the mock server
func (h *BotTestHelper) AssertMessageSent(expectedPath string) {
	count := h.mockServer.GetRequestCount(expectedPath)
	if count == 0 {
		h.t.Errorf("Expected at least one request to %s", expectedPath)
	}
}

// WaitForRequests waits for the specified number of requests
func (h *BotTestHelper) WaitForRequests(count int, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if h.mockServer.GetRequestCount("") >= count {
				return nil
			}
		case <-deadline:
			return fmt.Errorf("timeout waiting for %d requests", count)
		case <-time.After(100 * time.Millisecond):
			// Check if we should exit early
		}
	}
}

// CreateTestMessage creates a test message
func CreateTestMessage(platform core.Platform, text string) core.Message {
	return core.Message{
		ID:        "test-msg-123",
		Platform:  platform,
		Timestamp: time.Now().Unix(),
		Sender: core.Sender{
			ID:          "test-user-123",
			DisplayName: "Test User",
		},
		Recipient: core.Recipient{
			ID:   "test-chat-123",
			Type: "direct",
		},
		Content:  core.NewTextContent(text),
		ChatType: core.ChatTypeDirect,
		Metadata: make(map[string]interface{}),
	}
}

// CreateTestMediaMessage creates a test media message
func CreateTestMediaMessage(platform core.Platform, mediaType, url string) core.Message {
	return core.Message{
		ID:        "test-media-123",
		Platform:  platform,
		Timestamp: time.Now().Unix(),
		Sender: core.Sender{
			ID:          "test-user-123",
			DisplayName: "Test User",
		},
		Recipient: core.Recipient{
			ID:   "test-chat-123",
			Type: "direct",
		},
		Content: core.NewMediaContent([]core.MediaAttachment{
			{
				Type: mediaType,
				URL:  url,
			},
		}, ""),
		ChatType: core.ChatTypeDirect,
		Metadata: make(map[string]interface{}),
	}
}

// AssertMessageContent asserts a message has specific content
func AssertMessageContent(t *testing.T, msg core.Message, expectedText string) {
	if !msg.IsTextContent() {
		t.Errorf("Expected text content, got %s", msg.Content.ContentType())
		return
	}

	if msg.GetText() != expectedText {
		t.Errorf("Expected text '%s', got '%s'", expectedText, msg.GetText())
	}
}

// AssertMediaContent asserts a message has media content
func AssertMediaContent(t *testing.T, msg core.Message, expectedType string) {
	if !msg.IsMediaContent() {
		t.Errorf("Expected media content, got %s", msg.Content.ContentType())
		return
	}

	media := msg.GetMedia()
	if len(media) == 0 {
		t.Error("Expected non-empty media array")
		return
	}

	if media[0].Type != expectedType {
		t.Errorf("Expected media type '%s', got '%s'", expectedType, media[0].Type)
	}
}
