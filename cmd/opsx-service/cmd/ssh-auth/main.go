package main

import (
	"fmt"
	"os"
	"os/user"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// Config holds SSH auth configuration
type Config struct {
	JWTSecret      string
	SessionTimeout time.Duration
}

// loadConfig loads configuration from environment
func loadConfig() (*Config, error) {
	secret := os.Getenv("OPSX_JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("OPSX_JWT_SECRET not set")
	}

	timeoutStr := os.Getenv("OPSX_SESSION_TIMEOUT")
	timeout := 30 * time.Minute
	if timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err == nil {
			timeout = d
		}
	}

	return &Config{
		JWTSecret:      secret,
		SessionTimeout: timeout,
	}, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Get current user info from environment (set by SSH)
	username := os.Getenv("USER")
	if username == "" {
		// Fallback to os user lookup
		u, err := user.Current()
		if err == nil {
			username = u.Username
		} else {
			username = "unknown"
		}
	}

	// Get SSH client IP if available
	sshClientIP := os.Getenv("SSH_CLIENT")
	if sshClientIP == "" {
		sshClientIP = os.Getenv("SSH_CONNECTION")
	}
	if sshClientIP == "" {
		sshClientIP = "localhost"
	}

	// Generate JWT manager
	jwtManager := auth.NewJWTManager(cfg.JWTSecret)

	// Generate token with SSH context
	clientID := fmt.Sprintf("ssh:%s@%s", username, sshClientIP)

	token, err := jwtManager.GenerateAPIKey(clientID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating token: %v\n", err)
		os.Exit(1)
	}

	// Output the token (only the token, easy to parse)
	fmt.Println(token)
}
