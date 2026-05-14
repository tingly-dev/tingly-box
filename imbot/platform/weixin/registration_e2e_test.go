//go:build e2e
// +build e2e

package weixin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQRClient_GetBotQRCode exercises QR code generation against the live
// Weixin iLink API.
func TestQRClient_GetBotQRCode(t *testing.T) {
	client := NewQRClient("")

	ctx := context.Background()
	resp, err := client.GetBotQRCode(ctx, "3")

	require.NoError(t, err)
	require.NotNil(t, resp)

	if resp.Qrcode == "" {
		t.Error("Expected non-empty QR code")
	}
	if resp.QrcodeImgContent == "" {
		t.Error("Expected non-empty QR image content")
	}
}

// TestQRClient_GetQRStatus exercises QR status polling against the live Weixin
// iLink API.
func TestQRClient_GetQRStatus(t *testing.T) {
	client := NewQRClient("")

	ctx := context.Background()
	resp, err := client.GetQRStatus(ctx, "test-qr-id")

	require.NoError(t, err)
	require.NotNil(t, resp)

	if resp.Status == "" {
		t.Error("Expected non-empty status")
	}
	assert.Contains(t, []string{"wait", "scaned", "confirmed", "expired"}, resp.Status)
}
