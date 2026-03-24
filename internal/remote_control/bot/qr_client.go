package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// QRCodeResponse represents the QR code response from Weixin API
type QRCodeResponse struct {
	Qrcode           string `json:"qrcode,omitempty"`
	QrcodeImgContent string `json:"qrcode_img_content,omitempty"`
}

// QRStatusResponse represents the QR status response from Weixin API
type QRStatusResponse struct {
	Status      string `json:"status,omitempty"` // wait, scaned, confirmed, expired
	BotToken    string `json:"bot_token,omitempty"`
	IlinkBotID  string `json:"ilink_bot_id,omitempty"`
	BaseURL     string `json:"baseurl,omitempty"`
	IlinkUserID string `json:"ilink_user_id,omitempty"`
}

// WeChatQRClient is a CLI-friendly wrapper for Weixin QR authentication
type WeChatQRClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewWeChatQRClient creates a new Weixin QR client
func NewWeChatQRClient(baseURL string) *WeChatQRClient {
	if baseURL == "" {
		baseURL = "https://ilinkai.weixin.qq.com"
	}
	return &WeChatQRClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetBotQRCode fetches a QR code for Weixin bot login
func (c *WeChatQRClient) GetBotQRCode(ctx context.Context, botType string) (*QRCodeResponse, error) {
	if botType == "" {
		botType = "3" // Default bot type
	}

	// Build URL with query params (GET request)
	u, err := url.Parse(c.baseURL + "/ilink/bot/get_bot_qrcode")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	query := u.Query()
	query.Set("bot_type", botType)
	u.RawQuery = query.Encode()

	logrus.Debugf("GetBotQRCode URL: %s", u.String())

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers (no Authorization for QR code request)
	headers := c.buildHeaders()
	for k, v := range headers {
		// Skip Authorization header for QR code login
		if k == "Authorization" {
			continue
		}
		req.Header.Set(k, v)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Log response for debugging
	logrus.Debugf("GetBotQRCode response: %s", string(body))

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %d %s: %s", resp.StatusCode, resp.Status, string(body))
	}

	// Unmarshal response
	var result QRCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// GetQRStatus polls the QR code status
func (c *WeChatQRClient) GetQRStatus(ctx context.Context, qrcode string) (*QRStatusResponse, error) {
	// Build URL with query params
	u, err := url.Parse(c.baseURL + "/ilink/bot/get_qrcode_status")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	query := u.Query()
	query.Set("qrcode", qrcode)
	u.RawQuery = query.Encode()

	// Create request with longer timeout for long-poll
	client := &http.Client{Timeout: 35 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers
	headers := c.buildHeaders()
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Add required header for QR status polling
	req.Header.Set("iLink-App-ClientVersion", "1")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		// Check if this is a timeout error (either context deadline or client timeout)
		// Network timeouts should be distinguished from other errors
		if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
			// Timeout is expected for long-poll QR status checks
			// Return wait status so caller can retry
			return &QRStatusResponse{Status: "wait"}, nil
		}
		if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			return &QRStatusResponse{Status: "wait"}, nil
		}
		// Other network errors (connection refused, DNS failure, etc.) should be propagated
		return nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %d %s: %s", resp.StatusCode, resp.Status, string(body))
	}

	// Unmarshal response
	var result QRStatusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// buildHeaders creates the required headers for Weixin API
func (c *WeChatQRClient) buildHeaders() map[string]string {
	// Simplified headers for QR flow (no auth token needed initially)
	return map[string]string{
		"Content-Type": "application/json",
	}
}

// PollQRStatus polls the QR status until confirmed or expired
// Returns the confirmed credentials or error
func PollQRStatus(ctx context.Context, client *WeChatQRClient, qrID string, pollInterval time.Duration) (*QRStatusResponse, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Retry counter for transient failures
	const maxRetries = 3
	retryCount := 0

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-ticker.C:
			status, err := client.GetQRStatus(ctx, qrID)
			if err != nil {
				// Check if this is a transient network error
				if retryCount < maxRetries && isTransientError(err) {
					retryCount++
					logrus.WithError(err).WithField("retry", retryCount).Warn("Transient error polling QR status, retrying...")
					continue
				}
				return nil, fmt.Errorf("failed to get QR status after %d retries: %w", retryCount, err)
			}
			// Reset retry count on success
			retryCount = 0

			switch status.Status {
			case "wait", "scaned":
				// Continue polling
				logrus.Debugf("QR status: %s", status.Status)

			case "confirmed":
				logrus.Info("QR code confirmed by user")
				return status, nil

			case "expired":
				return nil, fmt.Errorf("QR code expired")

			default:
				return nil, fmt.Errorf("unknown QR status: %s", status.Status)
			}
		}
	}
}

// isTransientError checks if an error is transient (retryable)
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	// Check for network-related errors that are typically transient
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "temporary failure") ||
		strings.Contains(errStr, "network")
}
