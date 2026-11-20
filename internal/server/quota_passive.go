package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/ai/quota/fetcher"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// passiveClaudeQuotaInterval bounds how often one provider's header reading is
// stored: every save appends a history row, and the 5h/7d percentages move
// slowly. A reading that flips the limit status is stored at once.
const passiveClaudeQuotaInterval = 2 * time.Minute

// installPassiveClaudeQuota keeps a Claude OAuth provider's stored quota fresh
// from the anthropic-ratelimit-unified-* headers of the responses it already
// serves (.research/claude-code/2.1.328.md §3.9). The poll stays: it carries
// the spend block, scoped limits and the first snapshot.
func installPassiveClaudeQuota(mgr *quota.Manager) {
	obs := &passiveClaudeQuota{mgr: mgr, last: map[string]passiveClaudeState{}, now: time.Now}
	client.SetClaudeResponseObserver(obs.observe)
}

type passiveClaudeState struct {
	at       time.Time
	rejected bool
}

type passiveClaudeQuota struct {
	mgr  *quota.Manager
	now  func() time.Time
	mu   sync.Mutex
	last map[string]passiveClaudeState
}

func (p *passiveClaudeQuota) observe(provider *typ.Provider, h http.Header) {
	if provider == nil || provider.UUID == "" {
		return
	}
	reading, ok := fetcher.ParseAnthropicUnifiedHeaders(h)
	if !ok {
		return
	}
	rejected := reading.Status == "rejected"
	now := p.now()

	p.mu.Lock()
	prev, seen := p.last[provider.UUID]
	if seen && now.Sub(prev.at) < passiveClaudeQuotaInterval && prev.rejected == rejected {
		p.mu.Unlock()
		return
	}
	p.last[provider.UUID] = passiveClaudeState{at: now, rejected: rejected}
	p.mu.Unlock()

	// Off the request path: the store write must not delay the response.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		saved, err := p.mgr.UpdateStored(ctx, provider.UUID, func(u *quota.ProviderUsage) bool {
			return reading.Apply(u, now)
		})
		if err != nil {
			logrus.WithError(err).WithField("provider_uuid", provider.UUID).Debug("passive claude quota: save failed")
			return
		}
		if saved && rejected {
			logrus.WithField("provider_uuid", provider.UUID).Info("claude subscription reports a rejected window")
		}
	}()
}
