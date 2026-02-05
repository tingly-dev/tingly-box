package main

import (
	"fmt"
	"os"
	"os/user"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// Config holds the configuration
type Config struct {
	JWTSecret      string
	SessionTimeout time.Duration
}

// loadConfig loads configuration from environment
func loadConfig() (*Config, error) {
	secret := os.Getenv("RCC_JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("RCC_JWT_SECRET not set")
	}

	timeoutStr := os.Getenv("RCC_SESSION_TIMEOUT")
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

	// Verify this is an SSH session
	sshClientIP := os.Getenv("SSH_CLIENT")
	if sshClientIP == "" {
		sshClientIP = os.Getenv("SSH_CONNECTION")
	}
	if sshClientIP == "" {
		// Not an SSH session - warn but allow anyway (for testing)
		fmt.Fprintf(os.Stderr, "Warning: Not running in SSH session (SSH_CLIENT not set)\n")
	}

	// Get user info
	username := os.Getenv("USER")
	if username == "" {
		u, err := user.Current()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Cannot determine user: %v\n", err)
			os.Exit(1)
		}
		username = u.Username
	}

	// Generate token
	jwtManager := auth.NewJWTManager(cfg.JWTSecret)

	// Client ID includes SSH context for audit
	clientID := fmt.Sprintf("ssh:%s@%s", username, sshClientIP)

	token, err := jwtManager.GenerateAPIKey(clientID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating token: %v\n", err)
		os.Exit(1)
	}

	// Output the token
	fmt.Println(token)
}
