package command

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	remote_coder "github.com/tingly-dev/tingly-box/internal/remote_coder"
	remote_coderconfig "github.com/tingly-dev/tingly-box/internal/remote_coder/config"
)

// RemoteCoderCommand creates the `rc` subcommand for running remote-coder.
func RemoteCoderCommand(appManager *AppManager) *cobra.Command {
	var (
		port                 int
		dbPath               string
		sessionTimeout       string
		messageRetentionDays int
		rateLimitMax         int
		rateLimitWindow      string
		rateLimitBlock       string
		jwtSecret            string
		enableDebug          bool
	)

	cmd := &cobra.Command{
		Use:   "rc",
		Short: "Run the remote-coder service",
		RunE: func(cmd *cobra.Command, args []string) error {
			if appManager == nil || appManager.AppConfig() == nil {
				return fmt.Errorf("app configuration is not initialized")
			}

			if enableDebug || isEnvTrue("RCC_DEBUG") {
				logrus.SetLevel(logrus.DebugLevel)
				logrus.Info("Remote-coder debug mode enabled")
			}

			opts := remote_coderconfig.Options{}
			if cmd.Flags().Changed("port") {
				opts.Port = &port
			}
			if cmd.Flags().Changed("db-path") {
				opts.DBPath = &dbPath
			}
			if cmd.Flags().Changed("session-timeout") {
				parsed, err := time.ParseDuration(sessionTimeout)
				if err != nil {
					return fmt.Errorf("invalid session-timeout: %w", err)
				}
				opts.SessionTimeout = &parsed
			}
			if cmd.Flags().Changed("message-retention-days") {
				opts.MessageRetentionDays = &messageRetentionDays
			}
			if cmd.Flags().Changed("rate-limit-max") {
				opts.RateLimitMax = &rateLimitMax
			}
			if cmd.Flags().Changed("rate-limit-window") {
				parsed, err := time.ParseDuration(rateLimitWindow)
				if err != nil {
					return fmt.Errorf("invalid rate-limit-window: %w", err)
				}
				opts.RateLimitWindow = &parsed
			}
			if cmd.Flags().Changed("rate-limit-block") {
				parsed, err := time.ParseDuration(rateLimitBlock)
				if err != nil {
					return fmt.Errorf("invalid rate-limit-block: %w", err)
				}
				opts.RateLimitBlock = &parsed
			}
			if cmd.Flags().Changed("jwt-secret") {
				opts.JWTSecret = &jwtSecret
			}

			cfg, err := remote_coderconfig.LoadFromAppConfig(appManager.AppConfig().GetGlobalConfig(), opts)
			if err != nil {
				return err
			}

			return remote_coder.Run(context.Background(), cfg)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "remote-coder port (overrides config)")
	cmd.Flags().StringVar(&dbPath, "db-path", "", "remote-coder SQLite db path (overrides config)")
	cmd.Flags().StringVar(&sessionTimeout, "session-timeout", "", "session timeout duration (e.g., 30m)")
	cmd.Flags().IntVar(&messageRetentionDays, "message-retention-days", 0, "message retention in days")
	cmd.Flags().IntVar(&rateLimitMax, "rate-limit-max", 0, "max rate limit attempts")
	cmd.Flags().StringVar(&rateLimitWindow, "rate-limit-window", "", "rate limit window duration (e.g., 5m)")
	cmd.Flags().StringVar(&rateLimitBlock, "rate-limit-block", "", "rate limit block duration (e.g., 5m)")
	cmd.Flags().StringVar(&jwtSecret, "jwt-secret", "", "override JWT secret used for auth")
	cmd.Flags().BoolVar(&enableDebug, "debug", false, "enable debug logging for remote-coder")

	return cmd
}

func isEnvTrue(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}
