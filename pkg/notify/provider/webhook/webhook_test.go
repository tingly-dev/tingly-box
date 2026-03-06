package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/notify"
)

// TestWebhookNew tests webhook provider creation
func TestWebhookNew(t *testing.T) {
	p := New("https://example.com/webhook")

	if p.Name() != "webhook" {
		t.Errorf("expected name 'webhook', got %v", p.Name())
	}
	if p.url != "https://example.com/webhook" {
		t.Errorf("expected url 'https://example.com/webhook', got %v", p.url)
	}
	if p.method != "POST" {
		t.Errorf("expected method 'POST', got %v", p.method)
	}
}

// TestWebhookWithOptions tests webhook creation with options
func TestWebhookWithOptions(t *testing.T) {
	customClient := &http.Client{Timeout: 60 * time.Second}
	headers := map[string]string{"X-Custom": "value"}

	p := New("https://example.com/webhook",
		WithName("custom-webhook"),
		WithMethod("PUT"),
		WithClient(customClient),
		WithHeaders(headers),
		WithAuth("Bearer token"),
	)

	if p.Name() != "custom-webhook" {
		t.Errorf("expected name 'custom-webhook', got %v", p.Name())
	}
	if p.method != "PUT" {
		t.Errorf("expected method 'PUT', got %v", p.method)
	}
	if p.client.Timeout != 60*time.Second {
		t.Errorf("expected client timeout 60s, got %v", p.client.Timeout)
	}
}

// TestWebhookSendSuccess tests successful webhook send
func TestWebhookSendSuccess(t *testing.T) {
	var receivedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %v", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %v", r.Header.Get("Content-Type"))
		}

		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	p := New(server.URL)

	ctx := context.Background()
	result, err := p.Send(ctx, &notify.Notification{
		Message: "test message",
		Level:   notify.LevelInfo,
		Title:   "Test Title",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected successful result")
	}
	if receivedPayload["message"] != "test message" {
		t.Errorf("expected message 'test message', got %v", receivedPayload["message"])
	}
}

// TestWebhookSendErrorStatus tests webhook with error status code
func TestWebhookSendErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	}))
	defer server.Close()

	p := New(server.URL)

	ctx := context.Background()
	result, err := p.Send(ctx, &notify.Notification{Message: "test"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result.Success {
		t.Error("expected failed result")
	}
}

// TestWebhookSendInvalidURL tests webhook with invalid URL
func TestWebhookSendInvalidURL(t *testing.T) {
	p := New("http://invalid-url-that-does-not-exist:12345/webhook")
	p.client.Timeout = 100 * time.Millisecond

	ctx := context.Background()
	result, err := p.Send(ctx, &notify.Notification{Message: "test"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil && result.Success {
		t.Error("expected failed result")
	}
}

// TestWebhookSendWithContextCancellation tests context cancellation
func TestWebhookSendWithContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := New(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result, err := p.Send(ctx, &notify.Notification{Message: "test"})

	if err == nil {
		t.Fatal("expected error from context cancellation, got nil")
	}
	if result != nil && result.Success {
		t.Error("expected failed result")
	}
}

// TestWebhookSendWithHeaders tests custom headers
func TestWebhookSendWithHeaders(t *testing.T) {
	var headers http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := New(server.URL,
		WithHeaders(map[string]string{"X-Custom-Header": "custom-value"}),
		WithAuth("Bearer secret-token"),
	)

	ctx := context.Background()
	_, err := p.Send(ctx, &notify.Notification{Message: "test"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headers.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected X-Custom-Header 'custom-value', got %v", headers.Get("X-Custom-Header"))
	}
	if headers.Get("Authorization") != "Bearer secret-token" {
		t.Errorf("expected Authorization 'Bearer secret-token', got %v", headers.Get("Authorization"))
	}
}

// TestWebhookSendWithPUT tests PUT method
func TestWebhookSendWithPUT(t *testing.T) {
	var method string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := New(server.URL, WithMethod("PUT"))

	ctx := context.Background()
	_, err := p.Send(ctx, &notify.Notification{Message: "test"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if method != "PUT" {
		t.Errorf("expected method 'PUT', got %v", method)
	}
}

// TestWebhookPayloadStructure tests the payload structure
func TestWebhookPayloadStructure(t *testing.T) {
	var receivedPayload map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	links := []notify.Link{
		{Text: "Link1", URL: "https://example.com"},
	}

	p := New(server.URL)
	ctx := context.Background()
	_, err := p.Send(ctx, &notify.Notification{
		Title:    "Test Title",
		Message:  "Test Message",
		Level:    notify.LevelWarning,
		Category: "test-category",
		Tags:     []string{"tag1", "tag2"},
		Links:    links,
		Metadata: map[string]interface{}{"key": "value"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedPayload["title"] != "Test Title" {
		t.Errorf("expected title 'Test Title', got %v", receivedPayload["title"])
	}
	if receivedPayload["message"] != "Test Message" {
		t.Errorf("expected message 'Test Message', got %v", receivedPayload["message"])
	}
	if receivedPayload["level"] != "warning" {
		t.Errorf("expected level 'warning', got %v", receivedPayload["level"])
	}
	if receivedPayload["category"] != "test-category" {
		t.Errorf("expected category 'test-category', got %v", receivedPayload["category"])
	}
}

// TestWebhookClose tests Close method
func TestWebhookClose(t *testing.T) {
	p := New("https://example.com/webhook")

	err := p.Close()
	if err != nil {
		t.Errorf("Close() should not return error, got %v", err)
	}
}

// TestWebhookValidateNotification tests that invalid notifications are rejected
func TestWebhookValidateNotification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := New(server.URL)
	ctx := context.Background()

	// Empty message should fail
	_, err := p.Send(ctx, &notify.Notification{Message: ""})
	if err == nil {
		t.Error("expected error for empty message")
	}
}
