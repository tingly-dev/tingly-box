package db

import (
	"time"

	"github.com/tingly-dev/tingly-box/ai/quota"
)

// ProviderWeeklyReset uses only account-level periodic weekly windows, never
// credit balances, model-specific breakdowns or changes in usage percentages.
// Conflicting weekly windows are ambiguous and require a manual schedule.
func ProviderWeeklyReset(usage *quota.ProviderUsage) *time.Time {
	if usage == nil {
		return nil
	}
	var reset *time.Time
	for _, window := range usage.Windows {
		if window == nil || window.Type != quota.WindowTypeWeekly || window.Kind != quota.WindowKindLimit || window.Unknown || window.Unlimited || window.ResetsAt == nil {
			continue
		}
		if reset != nil && !reset.Equal(*window.ResetsAt) {
			return nil
		}
		value := window.ResetsAt.UTC()
		reset = &value
	}
	return reset
}
