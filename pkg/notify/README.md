# Notify Package

A unified notification system for Go applications that supports multiple notification providers with a clean, consistent API.

## Features

- **Unified Interface**: Single API for sending notifications to multiple channels
- **Multiple Providers**: Webhook, Slack, Discord, Email, and System notifications
- **Concurrent Delivery**: Send to multiple providers simultaneously
- **Provider Filtering**: Send to specific providers or all configured providers
- **Notification Levels**: Debug, Info, Warning, Error, Critical
- **Rich Content**: Support for titles, messages, links, images, metadata, and tags
- **Extensible**: Easy to add custom providers

## Installation

```bash
go get github.com/tingly-dev/tingly-box/pkg/notify
```

## Quick Start

```go
package main

import (
    "context"
    "github.com/tingly-dev/tingly-box/pkg/notify"
    "github.com/tingly-dev/tingly-box/pkg/notify/provider/webhook"
    "github.com/tingly-dev/tingly-box/pkg/notify/provider/slack"
)

func main() {
    ctx := context.Background()

    // Create multiplexer
    notifier := notify.NewMultiplexer()

    // Add providers
    notifier.AddProvider(webhook.New("https://hooks.example.com/notify"))
    notifier.AddProvider(slack.New(slack.Config{
        Token:   "xoxb-your-token",
        Channel: "#alerts",
    }))

    // Send notification
    results, err := notifier.Send(ctx, &notify.Notification{
        Title:   "Deployment Complete",
        Message: "App v1.2.3 deployed successfully",
        Level:   notify.LevelInfo,
        Tags:    []string{"deployment", "production"},
    })

    // Check results
    for _, r := range results {
        if r.Success {
            println(r.Provider, "sent successfully")
        } else {
            println(r.Provider, "failed:", r.Error.Error())
        }
    }
}
```

## Notification Structure

```go
type Notification struct {
    Title        string                 // Notification title/subject
    Message      string                 // Main content (required)
    Level        Level                  // debug, info, warning, error, critical
    Category     string                 // For grouping related notifications
    Tags         []string               // For filtering and routing
    Timestamp    time.Time              // Defaults to now
    Metadata     map[string]interface{} // Additional context
    Links        []Link                 // Clickable links
    ImageURL     string                 // Image to include
    ProviderData map[string]interface{} // Provider-specific overrides
}

type Link struct {
    Text string
    URL  string
}
```

## Providers

### Webhook

Send HTTP POST requests with JSON payload.

```go
import "github.com/tingly-dev/tingly-box/pkg/notify/provider/webhook"

provider := webhook.New(
    "https://hooks.example.com/notify",
    webhook.WithName("my-webhook"),
    webhook.WithHeaders(map[string]string{
        "X-Custom-Header": "value",
    }),
    webhook.WithAuth("Bearer token"),
    webhook.WithTimeout(30 * time.Second),
)
```

### Slack

Send messages to Slack channels via the Chat API.

```go
import "github.com/tingly-dev/tingly-box/pkg/notify/provider/slack"

provider := slack.New(slack.Config{
    Token:     "xoxb-your-bot-token",
    Channel:   "#alerts",
    Username:  "Alert Bot",
    IconEmoji: ":robot_face:",
})
```

**Metadata overrides:**
- `channel`: Override the default channel

### Discord

Send messages via Discord webhooks.

```go
import "github.com/tingly-dev/tingly-box/pkg/notify/provider/discord"

provider := discord.New(discord.Config{
    WebhookURL: "https://discord.com/api/webhooks/xxx/yyy",
    Username:   "Alert Bot",
    AvatarURL:  "https://example.com/avatar.png",
})
```

### Email

Send emails via SMTP.

```go
import "github.com/tingly-dev/tingly-box/pkg/notify/provider/email"

provider := email.New(email.Config{
    Host:     "smtp.gmail.com:587",
    Username: "alerts@example.com",
    Password: "app-password",
    From:     "alerts@example.com",
    To:       []string{"team@example.com"},
    UseTLS:   true,
})
```

**Metadata overrides:**
- `to`: Override recipient list

### System

Send desktop notifications (macOS, Linux, Windows).

```go
import "github.com/tingly-dev/tingly-box/pkg/notify/provider/system"

if system.IsSupported() {
    provider := system.New(system.Config{
        AppName: "MyApp",
        Sound:   "default",
    })
}
```

## Multiplexer

The multiplexer sends notifications to multiple providers concurrently.

```go
// Create with options
notifier := notify.NewMultiplexer(
    notify.WithMinLevel(notify.LevelInfo), // Filter out debug messages
)

// Add providers
notifier.AddProvider(webhookProvider)
notifier.AddProvider(slackProvider)

// Send to all providers
results, err := notifier.Send(ctx, notification)

// Send to specific provider
result, err := notifier.SendTo(ctx, "slack", notification)

// Manage providers
notifier.RemoveProvider("webhook")
provider := notifier.GetProvider("slack")
names := notifier.ListProviders()

// Cleanup
notifier.Close()
```

## Notification Levels

```go
notify.LevelDebug    // Debug information, filtered by default
notify.LevelInfo     // General information
notify.LevelWarning  // Warning conditions
notify.LevelError    // Error conditions
notify.LevelCritical // Critical failures
```

Filter by minimum level:

```go
notifier := notify.NewMultiplexer(
    notify.WithMinLevel(notify.LevelWarning),
)
```

## Error Handling

```go
results, err := notifier.Send(ctx, notification)

// err indicates if ANY provider failed
if err != nil {
    log.Printf("Some providers failed: %v", err)
}

// Check individual results
for _, result := range results {
    if result.Success {
        log.Printf("%s sent successfully (latency: %v)",
            result.Provider, result.Latency)
    } else {
        log.Printf("%s failed: %v", result.Provider, result.Error)
    }
}
```

## Custom Providers

Implement the `Provider` interface:

```go
type Provider interface {
    Name() string
    Send(ctx context.Context, notification *Notification) (*Result, error)
    Close() error
}
```

Example custom provider:

```go
type CustomProvider struct {
    name string
    // your fields
}

func (p *CustomProvider) Name() string {
    return p.name
}

func (p *CustomProvider) Send(ctx context.Context, n *notify.Notification) (*notify.Result, error) {
    // your implementation
    return &notify.Result{Provider: p.name, Success: true}, nil
}

func (p *CustomProvider) Close() error {
    return nil
}
```

## Examples

See `examples/main.go` for complete usage examples including:
- Basic notifications
- Warning and error notifications
- Notifications with links and metadata
- Provider-specific sending
- Level filtering
- Builder pattern for fluent notification creation

## License

MIT
