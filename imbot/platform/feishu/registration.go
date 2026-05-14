package feishu

import (
	"context"
	"errors"

	"github.com/larksuite/oapi-sdk-go/v3/scene/registration"
)

// RegistrationOutcome classifies how a one-click app registration ended.
type RegistrationOutcome string

const (
	RegistrationConfirmed RegistrationOutcome = "confirmed" // user authorized; credentials issued
	RegistrationDenied    RegistrationOutcome = "denied"    // user declined authorization
	RegistrationExpired   RegistrationOutcome = "expired"   // QR link expired or polling timed out
	RegistrationError     RegistrationOutcome = "error"     // any other failure
)

// RegistrationQRCode is the verification link to render as a QR code (or open
// directly) so the user can authorize the registration.
type RegistrationQRCode struct {
	URL      string
	ExpireIn int // link lifetime in seconds
}

// RegistrationResult holds the credentials minted by a successful one-click
// registration.
type RegistrationResult struct {
	ClientID     string // App ID
	ClientSecret string // App Secret
	TenantBrand  string // "feishu" or "lark", as reported by the platform
}

// RegistrationOptions configures a one-click app registration run.
type RegistrationOptions struct {
	// Source is the SDK source tag, surfaced in the verification URL.
	// Defaults to "tingly-box" when empty.
	Source string
	// OnQRCode is invoked once the verification link is ready. Required.
	OnQRCode func(RegistrationQRCode)
	// OnDomainSwitch is invoked if the SDK detects a Lark tenant and switches
	// from the Feishu domain to the Lark domain. Optional.
	OnDomainSwitch func()
}

// RegisterApp drives the Feishu/Lark one-click app registration flow (OAuth 2.0
// Device Authorization Grant, RFC 8628). It blocks until the user authorizes,
// declines, the link expires, or ctx is cancelled.
//
// The returned outcome classifies the ending; result is non-nil only when the
// outcome is RegistrationConfirmed. The raw SDK error is returned alongside for
// logging — it is non-nil for every outcome except RegistrationConfirmed.
//
// This is the platform-level building block; callers (HTTP handlers, CLI) own
// session management and credential persistence.
func RegisterApp(ctx context.Context, opts RegistrationOptions) (RegistrationOutcome, *RegistrationResult, error) {
	source := opts.Source
	if source == "" {
		source = "tingly-box"
	}

	sdkOpts := &registration.Options{
		Source: source,
		OnQRCode: func(info *registration.QRCodeInfo) {
			if opts.OnQRCode != nil {
				opts.OnQRCode(RegistrationQRCode{URL: info.URL, ExpireIn: info.ExpireIn})
			}
		},
		OnStatusChange: func(info *registration.StatusChangeInfo) {
			if info.Status == registration.StatusDomainSwitched && opts.OnDomainSwitch != nil {
				opts.OnDomainSwitch()
			}
		},
	}

	result, err := registration.RegisterApp(ctx, sdkOpts)
	if err != nil {
		var accessDenied *registration.AccessDeniedError
		var expired *registration.ExpiredError
		switch {
		case errors.As(err, &accessDenied):
			return RegistrationDenied, nil, err
		case errors.As(err, &expired):
			return RegistrationExpired, nil, err
		default:
			return RegistrationError, nil, err
		}
	}

	out := &RegistrationResult{
		ClientID:     result.ClientID,
		ClientSecret: result.ClientSecret,
	}
	if result.UserInfo != nil {
		out.TenantBrand = result.UserInfo.TenantBrand
	}
	return RegistrationConfirmed, out, nil
}
