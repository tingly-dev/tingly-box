package quota

import (
	"context"
	"errors"
	"fmt"
	"sync"

	typ "github.com/tingly-dev/tingly-box/ai"
)

var (
	// ErrRedeemUnsupported: the provider has no spendable rewards.
	ErrRedeemUnsupported = errors.New("provider has no spendable rewards")
	// ErrRewardNotFound: the reward is not in the provider's list, or is no
	// longer available (already spent, or expired).
	ErrRewardNotFound = errors.New("reward not found or not available")
	// ErrRedeemRejected: the provider answered and said no. The reward was not
	// spent. The wrapped message is the provider's own.
	ErrRedeemRejected = errors.New("provider rejected the reward")
	// ErrRedeemUnconfirmed: the request was sent but its outcome is unknown
	// (timeout, broken connection). The reward may or may not be spent; it is
	// never retried, the caller re-reads the provider's list instead.
	ErrRedeemUnconfirmed = errors.New("reward outcome unconfirmed")
	// ErrRedeemBusy: another redeem for this provider is in flight.
	ErrRedeemBusy = errors.New("another reward is being spent for this provider")
)

// Redeemer is implemented by fetchers whose provider hands out rewards that
// can be spent on the user's behalf. Spending is irreversible, so
// implementations must: re-read the provider's list and refuse an id that is
// not available in it; never retry after the request has been sent; and map
// the provider's answer onto the errors above.
type Redeemer interface {
	// Redeem spends the reward with the given id. It returns the provider's
	// own message on success.
	Redeem(ctx context.Context, provider *typ.Provider, rewardID string) (string, error)
}

// RedeemResult is what a successful spend returns: the provider's message and
// the quota read back afterwards, which is how the caller confirms the effect.
type RedeemResult struct {
	Message string         `json:"message,omitempty"`
	Usage   *ProviderUsage `json:"usage,omitempty"`
}

// redeemGuard keeps two spends for one provider from running at once: a
// double click must not become two cards.
type redeemGuard struct {
	mu      sync.Mutex
	running map[string]struct{}
}

func (g *redeemGuard) acquire(uuid string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running == nil {
		g.running = make(map[string]struct{})
	}
	if _, busy := g.running[uuid]; busy {
		return false
	}
	g.running[uuid] = struct{}{}
	return true
}

func (g *redeemGuard) release(uuid string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.running, uuid)
}

// Redeem spends one reward of a provider, then re-reads the provider's quota
// so the caller sees the effect. An uncertain outcome still refreshes the
// quota (the list shows whether the reward is gone) and is returned as
// ErrRedeemUnconfirmed alongside the refreshed usage.
func (m *Manager) Redeem(ctx context.Context, providerUUID, rewardID string) (*RedeemResult, error) {
	provider, err := m.providerMgr.GetProviderByUUID(providerUUID)
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}
	f, ok := m.registry.Get(inferProviderType(provider))
	if !ok {
		return nil, ErrProviderUnsupported
	}
	redeemer, ok := f.(Redeemer)
	if !ok {
		return nil, ErrRedeemUnsupported
	}
	if !m.redeeming.acquire(provider.UUID) {
		return nil, ErrRedeemBusy
	}
	defer m.redeeming.release(provider.UUID)

	log := m.logger.WithField("provider_uuid", provider.UUID)
	message, redeemErr := redeemer.Redeem(ctx, provider, rewardID)
	switch {
	case redeemErr == nil:
		log.Info("reward spent")
	case errors.Is(redeemErr, ErrRedeemUnconfirmed):
		log.WithError(redeemErr).Warn("reward outcome unknown")
	default:
		log.WithError(redeemErr).Warn("reward not spent")
		return nil, redeemErr
	}

	result := &RedeemResult{Message: message}
	if usage, err := m.fetchProviderQuota(ctx, provider); err == nil {
		result.Usage = usage
	}
	return result, redeemErr
}
