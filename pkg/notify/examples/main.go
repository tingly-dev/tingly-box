// Example demonstrating the notification system usage
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/notify"
	"github.com/tingly-dev/tingly-box/pkg/notify/provider/discord"
	"github.com/tingly-dev/tingly-box/pkg/notify/provider/email"
	"github.com/tingly-dev/tingly-box/pkg/notify/provider/slack"
	"github.com/tingly-dev/tingly-box/pkg/notify/provider/system"
	"github.com/tingly-dev/tingly-box/pkg/notify/provider/webhook"
)

func main() {
	ctx := context.Background()

	// Create a multiplexer to send to multiple providers
	notifier := notify.NewMultiplexer(
		notify.WithMinLevel(notify.LevelInfo),
		notify.WithDefaultRetry(2), // Retry failed sends up to 2 times
	)

	// Add webhook provider
	notifier.AddProvider(webhook.New(
		"https://hooks.example.com/notify",
		webhook.WithName("main-webhook"),
		webhook.WithHeaders(map[string]string{
			"X-Custom-Header": "my-app",
		}),
	))

	// Add Slack provider with retry config
	notifier.AddProviderWithConfig(
		slack.New(slack.Config{
			Token:     "xoxb-your-bot-token",
			Channel:   "#alerts",
			Username:  "Alert Bot",
			IconEmoji: ":robot_face:",
		}),
		&notify.ProviderConfig{
			RetryCount: 3,       // Retry up to 3 times
			RetryDelay: 2 * time.Second, // Wait 2 seconds between retries
			Timeout:    30 * time.Second,
		},
	)

	// Add Discord provider
	notifier.AddProvider(discord.New(discord.Config{
		WebhookURL: "https://discord.com/api/webhooks/xxx/yyy",
		Username:   "Alert Bot",
	}))

	// Add email provider
	notifier.AddProvider(email.New(email.Config{
		Host:     "smtp.gmail.com:587",
		Username: "alerts@example.com",
		Password: "app-password",
		From:     "alerts@example.com",
		To:       []string{"team@example.com"},
	}))

	// Add system notifications (if supported)
	if system.IsSupported() {
		notifier.AddProvider(system.New(system.Config{
			AppName: "MyApp",
		}))
	}

	// Example 1: Simple info notification
	fmt.Println("Sending info notification...")
	results, err := notifier.Send(ctx, &notify.Notification{
		Title:   "Deployment Complete",
		Message: "Application v1.2.3 has been deployed successfully",
		Level:   notify.LevelInfo,
		Tags:    []string{"deployment", "production"},
	})
	printResults(results, err)

	// Example 2: Warning notification
	fmt.Println("\nSending warning notification...")
	results, err = notifier.Send(ctx, &notify.Notification{
		Title:   "High Memory Usage",
		Message: "Server memory usage has exceeded 80% threshold",
		Level:   notify.LevelWarning,
		Category: "infrastructure",
		Tags:    []string{"memory", "alert"},
		Metadata: map[string]interface{}{
			"server":   "prod-web-01",
			"usage":    "85%",
			"threshold": "80%",
		},
	})
	printResults(results, err)

	// Example 3: Critical error with links
	fmt.Println("\nSending critical notification...")
	results, err = notifier.Send(ctx, &notify.Notification{
		Title:   "Database Connection Failed",
		Message: "Unable to connect to primary database. Failover initiated.",
		Level:   notify.LevelCritical,
		Category: "database",
		Tags:    []string{"critical", "outage"},
		Links: []notify.Link{
			{Text: "View Dashboard", URL: "https://dashboard.example.com"},
			{Text: "Runbook", URL: "https://wiki.example.com/db-failover"},
		},
		Metadata: map[string]interface{}{
			"database": "postgres-primary",
			"error":    "connection timeout",
		},
	})
	printResults(results, err)

	// Example 4: Send to specific provider only
	fmt.Println("\nSending to webhook only...")
	result, err := notifier.SendTo(ctx, "main-webhook", &notify.Notification{
		Title:   "Test",
		Message: "This goes only to the webhook provider",
		Level:   notify.LevelInfo,
	})
	if err != nil {
		log.Printf("SendTo error: %v", err)
	}
	fmt.Printf("Result: %+v\n", result)

	// Example 5: Filter by level (debug messages won't be sent)
	fmt.Println("\nSending debug notification (filtered)...")
	results, err = notifier.Send(ctx, &notify.Notification{
		Message: "This debug message won't be sent due to minLevel filter",
		Level:   notify.LevelDebug,
	})
	if len(results) == 0 {
		fmt.Println("Debug notification was filtered out (expected)")
	}
}

func printResults(results []*notify.Result, err error) {
	if err != nil {
		log.Printf("Send error: %v", err)
	}

	for _, r := range results {
		status := "SUCCESS"
		if !r.Success {
			status = "FAILED"
		}
		fmt.Printf("  [%s] %s: %v (latency: %v)\n",
			status, r.Provider, r.Error, r.Latency)
	}
}

// Helper function to create a notification builder pattern
func NewNotification() *NotificationBuilder {
	return &NotificationBuilder{
		n: &notify.Notification{
			Metadata: make(map[string]interface{}),
		},
	}
}

// NotificationBuilder provides a fluent interface for building notifications
type NotificationBuilder struct {
	n *notify.Notification
}

func (b *NotificationBuilder) Title(title string) *NotificationBuilder {
	b.n.Title = title
	return b
}

func (b *NotificationBuilder) Message(message string) *NotificationBuilder {
	b.n.Message = message
	return b
}

func (b *NotificationBuilder) Level(level notify.Level) *NotificationBuilder {
	b.n.Level = level
	return b
}

func (b *NotificationBuilder) Category(category string) *NotificationBuilder {
	b.n.Category = category
	return b
}

func (b *NotificationBuilder) Tags(tags ...string) *NotificationBuilder {
	b.n.Tags = append(b.n.Tags, tags...)
	return b
}

func (b *NotificationBuilder) Metadata(key string, value interface{}) *NotificationBuilder {
	b.n.Metadata[key] = value
	return b
}

func (b *NotificationBuilder) Link(text, url string) *NotificationBuilder {
	b.n.Links = append(b.n.Links, notify.Link{Text: text, URL: url})
	return b
}

func (b *NotificationBuilder) Image(url string) *NotificationBuilder {
	b.n.ImageURL = url
	return b
}

func (b *NotificationBuilder) Build() *notify.Notification {
	return b.n
}

// Example usage of builder:
// notifier.Send(ctx, NewNotification().
//     Title("Alert").
//     Message("Something happened").
//     Level(notify.LevelError).
//     Category("system").
//     Tags("alert", "error").
//     Metadata("server", "prod-1").
//     Link("Dashboard", "https://dash.example.com").
//     Build())
