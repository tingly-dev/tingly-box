package weixin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// defaultQRBaseURL is the Weixin iLink endpoint used to provision bot
// credentials via the QR-code onboarding flow.
const defaultQRBaseURL = "https://ilinkai.weixin.qq.com"

// QRClient talks to the Weixin iLink API to provision bot credentials through
// the QR-code onboarding flow: fetch a QR code, then poll until the user scans
// and confirms it. This is distinct from InteractionHandler's QR login, which
// authenticates an already-provisioned bot's runtime session.
type QRClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewQRClient creates a QRClient. An empty baseURL falls back to the default
// Weixin iLink endpoint. The HTTP client timeout is long enough to cover the
// status long-poll.
func NewQRClient(baseURL string) *QRClient {
	if baseURL == "" {
		baseURL = defaultQRBaseURL
	}
	return &QRClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 35 * time.Second},
	}
}

// BotQRCode is the QR code payload returned by the Weixin iLink API.
type BotQRCode struct {
	Qrcode           string `json:"qrcode,omitempty"`
	QrcodeImgContent string `json:"qrcode_img_content,omitempty"`
}

// QRStatus is the QR scan status returned by the Weixin iLink API. When Status
// is "confirmed" the credential fields are populated.
type QRStatus struct {
	Status      string `json:"status,omitempty"` // wait, scaned, confirmed, expired
	BotToken    string `json:"bot_token,omitempty"`
	IlinkBotID  string `json:"ilink_bot_id,omitempty"`
	IlinkUserID string `json:"ilink_user_id,omitempty"`
	BaseURL     string `json:"baseurl,omitempty"`
}

// GetBotQRCode fetches a QR code for Weixin bot provisioning. botType defaults
// to "3" (官方小程序机器人) when empty.
func (c *QRClient) GetBotQRCode(ctx context.Context, botType string) (*BotQRCode, error) {
	if botType == "" {
		botType = "3"
	}

	u, err := url.Parse(c.baseURL + "/ilink/bot/get_bot_qrcode")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	query := u.Query()
	query.Set("bot_type", botType)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %d %s: %s", resp.StatusCode, resp.Status, string(body))
	}

	var result BotQRCode
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &result, nil
}

// GetQRStatus polls the scan status for a QR code. A request timeout is treated
// as a normal "wait" outcome so the caller can keep polling.
func (c *QRClient) GetQRStatus(ctx context.Context, qrcode string) (*QRStatus, error) {
	u, err := url.Parse(c.baseURL + "/ilink/bot/get_qrcode_status")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	query := u.Query()
	query.Set("qrcode", qrcode)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("iLink-App-ClientVersion", "1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// A timed-out long-poll is expected; report it as "wait".
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &QRStatus{Status: "wait"}, nil
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %d %s: %s", resp.StatusCode, resp.Status, string(body))
	}

	var result QRStatus
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &result, nil
}
