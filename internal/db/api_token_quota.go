package db

import (
	"fmt"
	"time"
)

// TokenQuotaExceeded is separate from authentication failure: a valid key
// becomes usable again at its next known reset without being re-enabled.
type TokenQuotaExceeded struct {
	Limit    int64
	Used     int64
	ResetsAt *time.Time
}

func (e *TokenQuotaExceeded) Error() string { return "weekly token quota exceeded" }

func tokenWeekStart(now time.Time) time.Time {
	now = now.UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
}

// CurrentTokenWeek projects expired counters without writing during reads.
func (r APITokenRecord) CurrentTokenWeek(now time.Time) APITokenRecord {
	if r.QuotaResetMode != "" && r.QuotaResetMode != "legacy" {
		return r
	}
	start := tokenWeekStart(now)
	if !r.TokenWeekStart.Equal(start) {
		r.TokenWeekStart = start
		r.WeeklyTokensUsed = 0
	}
	end := start.AddDate(0, 0, 7)
	r.QuotaResetsAt = &end
	return r
}

// QuotaResetPolicy changes only the clock, never the allocated budget.
// Manual anchors are UTC instants repeated every seven days.
type QuotaResetPolicy struct {
	Mode       string     `json:"mode" binding:"required,oneof=provider manual legacy"`
	ProviderID string     `json:"provider_id,omitempty"`
	Anchor     *time.Time `json:"anchor,omitempty"`
}

// SetQuotaResetSource installs a local-snapshot reader. It must not fetch from
// upstream or re-enter this store: it runs while the token store is locked.
func (s *APITokenStore) SetQuotaResetSource(source func(string) (string, *time.Time)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quotaResetSource = source
}

func nextWeeklyReset(anchor, now time.Time) time.Time {
	anchor = anchor.UTC()
	if !anchor.After(now) {
		// Unix seconds avoid time.Duration's ~290-year overflow for old anchors.
		const weekSeconds int64 = 7 * 24 * 60 * 60
		steps := (now.Unix() - anchor.Unix()) / weekSeconds
		anchor = time.Unix(anchor.Unix()+steps*weekSeconds, int64(anchor.Nanosecond())).UTC()
		if !anchor.After(now) {
			anchor = anchor.Add(7 * 24 * time.Hour)
		}
	}
	return anchor
}

// advanceQuota consumes each observed boundary once. A missing provider
// snapshot never invents another reset; a previously learned deadline is kept.
func advanceQuota(r APITokenRecord, now time.Time, reported *time.Time) APITokenRecord {
	if r.QuotaResetMode == "" || r.QuotaResetMode == "legacy" {
		return r.CurrentTokenWeek(now)
	}
	if r.QuotaResetsAt != nil && !r.QuotaResetsAt.After(now) {
		r.TokenWeekStart = *r.QuotaResetsAt
		r.WeeklyTokensUsed = 0
		r.QuotaResetsAt = nil
	}
	if r.QuotaResetMode == "manual" && r.QuotaResetAnchor != nil {
		next := nextWeeklyReset(*r.QuotaResetAnchor, now)
		r.QuotaResetsAt = &next
	} else if reported != nil && reported.After(now) {
		// An updated future deadline adjusts the clock without minting tokens.
		reset := reported.UTC()
		r.QuotaResetsAt = &reset
	} else if reported != nil && reported.After(r.TokenWeekStart) && !reported.After(now) {
		r.TokenWeekStart = *reported
		r.WeeklyTokensUsed = 0
	}
	return r
}

func sameTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}

func (s *APITokenStore) refreshQuotaLocked(record *APITokenRecord, now time.Time) error {
	current := *record
	var reported *time.Time
	if current.QuotaResetMode == "provider" && s.quotaResetSource != nil {
		current.QuotaProviderID, reported = s.quotaResetSource(current.QuotaProviderID)
	}
	current = advanceQuota(current, now, reported)
	if current.TokenWeekStart.Equal(record.TokenWeekStart) && current.QuotaProviderID == record.QuotaProviderID && sameTime(current.QuotaResetsAt, record.QuotaResetsAt) {
		return nil
	}
	if err := s.db.Model(&APITokenRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
		"weekly_tokens_used": current.WeeklyTokensUsed, "token_week_start": current.TokenWeekStart,
		"quota_provider_id": current.QuotaProviderID, "quota_resets_at": current.QuotaResetsAt,
	}).Error; err != nil {
		return err
	}
	*record = current
	return nil
}

// SetWeeklyTokenLimit preserves consumption when changing a budget. Nil means
// unlimited; zero intentionally blocks further model requests.
func (s *APITokenStore) SetWeeklyTokenLimit(tokenID string, limit *int64, policies ...*QuotaResetPolicy) error {
	if limit != nil && (*limit < 0 || *limit > 9007199254740991) {
		return fmt.Errorf("weekly token limit must be a non-negative safe integer")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.cache[tokenID]
	if !ok {
		return fmt.Errorf("token not found")
	}
	var policy *QuotaResetPolicy
	if len(policies) > 0 {
		policy = policies[0]
	}
	if policy != nil && (policy.Mode != "provider" && policy.Mode != "manual" && policy.Mode != "legacy" || policy.Mode == "manual" && (policy.Anchor == nil || policy.Anchor.IsZero())) {
		return fmt.Errorf("invalid quota reset policy: manual mode requires an anchor")
	}
	now := time.Now()
	if err := s.refreshQuotaLocked(r, now); err != nil {
		return err
	}
	current := *r
	updates := map[string]any{"weekly_token_limit": limit}
	if policy != nil && (policy.Mode != r.QuotaResetMode || policy.ProviderID != r.QuotaProviderID || !sameTime(policy.Anchor, r.QuotaResetAnchor)) {
		current.QuotaResetMode, current.QuotaProviderID = policy.Mode, policy.ProviderID
		current.QuotaResetAnchor = nil
		if policy.Anchor != nil {
			anchor := policy.Anchor.UTC()
			current.QuotaResetAnchor = &anchor
		}
		current.TokenWeekStart = now
		current.QuotaResetsAt = nil
		if policy.Mode == "manual" {
			next := nextWeeklyReset(*policy.Anchor, now)
			current.QuotaResetsAt = &next
		}
		if policy.Mode == "legacy" {
			current.TokenWeekStart = tokenWeekStart(now)
			next := current.TokenWeekStart.AddDate(0, 0, 7)
			current.QuotaResetsAt = &next
		}
		if policy.Mode == "provider" && s.quotaResetSource != nil {
			id, reset := s.quotaResetSource(policy.ProviderID)
			current.QuotaProviderID = id
			if reset != nil && reset.After(now) {
				current.QuotaResetsAt = reset
			}
		}
		updates["quota_reset_mode"], updates["quota_provider_id"] = current.QuotaResetMode, current.QuotaProviderID
		updates["quota_reset_anchor"], updates["quota_resets_at"] = current.QuotaResetAnchor, current.QuotaResetsAt
		updates["token_week_start"] = current.TokenWeekStart
	}
	if err := s.db.Model(&APITokenRecord{}).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
		return err
	}
	*r = current
	r.WeeklyTokenLimit = nil
	if limit != nil {
		v := *limit
		r.WeeklyTokenLimit = &v
	}
	return nil
}

// ChargeTokens runs synchronously before the request finishes. Stable row IDs
// keep in-flight usage attached to the key after a Team move. Regenerated keys
// have new row IDs and do not inherit usage from the old key.
// Counts are attributed to the active period when usage is reported.
func (s *APITokenStore) ChargeTokens(id uint, tokens int64) error {
	if tokens <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var record *APITokenRecord
	for _, r := range s.cache {
		if r.ID == id {
			record = r
			break
		}
	}
	if record == nil {
		return nil
	} // Deleted keys cannot be used again.
	if err := s.refreshQuotaLocked(record, time.Now()); err != nil {
		record.Enabled = false
		return err
	}
	current := *record
	if tokens > 9223372036854775807-current.WeeklyTokensUsed {
		return fmt.Errorf("token counter overflow")
	}
	current.WeeklyTokensUsed += tokens
	err := s.db.Model(&APITokenRecord{}).Where("id = ?", id).Updates(map[string]any{
		"weekly_tokens_used": current.WeeklyTokensUsed,
		"token_week_start":   current.TokenWeekStart,
	}).Error
	// Retain consumption even on a storage error, and fail closed for this key
	// in this process rather than letting unrecorded requests accumulate.
	record.WeeklyTokensUsed = current.WeeklyTokensUsed
	record.TokenWeekStart = current.TokenWeekStart
	if err != nil {
		record.Enabled = false
	}
	return err
}
