// Package quotawindow sends one tiny model request per hour to every enabled
// OAuth provider. Subscription quota windows (Claude Code 5h, Codex primary)
// only start counting on the first real request, so this keeps the window
// moving while the account is idle.
package quotawindow

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Send is the shape of probe.E2EProber.Probe.
type Send func(context.Context, *probe.E2ERequest) (*probe.E2EData, error)

// Run ticks hourly until ctx is done. Blocks; run it in a goroutine.
func Run(ctx context.Context, prober *probe.E2EProber, cfg *config.Config) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			Tick(ctx, prober.Probe, cfg.ListProviders())
		}
	}
}

// Tick sends one request to each enabled OAuth provider, trying its models
// in order until one goes through.
func Tick(ctx context.Context, send Send, providers []*typ.Provider) {
	for _, p := range providers {
		if p == nil || !p.Enabled || !p.IsOAuth() {
			continue
		}
		for _, model := range p.Models {
			if ctx.Err() != nil {
				return
			}
			stream := false
			reqCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
			res, err := send(reqCtx, &probe.E2ERequest{
				TargetType:   probe.E2ETargetProvider,
				ProviderUUID: p.UUID,
				Model:        model,
				Direct:       true,
				Stream:       &stream,
				Message:      "Reply with the single word: ok",
			})
			cancel()
			if err == nil && res != nil && res.Success {
				break
			}
			logrus.WithFields(logrus.Fields{"provider": p.Name, "model": model}).
				WithError(err).Warn("quota window request failed")
		}
	}
}
