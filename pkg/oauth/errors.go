package oauth

import (
	"errors"
)

var (
	// ErrTokenNotFound is returned when a token is not found in storage
	ErrTokenNotFound = errors.New("oauth: token not found")

	// ErrInvalidProvider is returned when an invalid provider is specified
	ErrInvalidProvider = errors.New("oauth: invalid provider")

	// ErrInvalidState is returned when the OAuth state parameter is invalid
	ErrInvalidState = errors.New("oauth: invalid state")

	// ErrStateExpired is returned when the OAuth state has expired
	ErrStateExpired = errors.New("oauth: state expired")

	// ErrInvalidCode is returned when the authorization code is invalid
	ErrInvalidCode = errors.New("oauth: invalid authorization code")

	// ErrTokenExchangeFailed is returned when token exchange fails
	ErrTokenExchangeFailed = errors.New("oauth: token exchange failed")

	// ErrNoRefreshToken is returned when a refresh is attempted but no refresh token is available
	ErrNoRefreshToken = errors.New("oauth: no refresh token available")

	// ErrProviderNotConfigured is returned when a provider is not configured
	ErrProviderNotConfigured = errors.New("oauth: provider not configured")

	// ErrInvalidCallback is returned when the callback parameters are invalid
	ErrInvalidCallback = errors.New("oauth: invalid callback")
)
