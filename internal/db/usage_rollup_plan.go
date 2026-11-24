package db

import "time"

// Usage rollups come in two tiers, both derived from usage_records and both
// keyed by SQLite's UTC bucketing of the stored timestamp:
//
//	usage_hourly  one row per (UTC hour, provider, model, user)   — finest
//	usage_daily   one row per (UTC day,  provider, model, user)   — the sum of the day's hours
//
// plus a per-agent companion of each (usage_hourly_scenario,
// usage_daily_scenario). Hour is the minimum fold: a provider that prices by
// time of day (peak/valley) needs usage per hour, and an hour is the unit a
// query window can be aligned to without touching raw rows. The day tier only
// exists so long windows read a few hundred rows instead of thousands; because
// it is summed from the hour rows, any window gives the same totals whichever
// tiers serve it.
//
// A query window [start, end] is covered by up to five segments:
//
//	raw   [start, firstHour)       partial leading hour (< 1h)
//	hour  [firstHour, firstDay)    whole hours up to the first day boundary
//	day   [firstDay, lastDay)      whole days (only when >= minDailySpan of them)
//	hour  [lastDay, lastHour)      whole hours after the last day boundary
//	raw   [lastHour, end]          the current partial hour plus the grace window
//
// Hours become eligible hourAggGrace after they end; records are stamped when
// they are written (UsageStore.RecordUsage), so only a stalled write can land
// in an hour that was already aggregated.

const (
	// hourAggGrace delays aggregation of a finished hour so a record written
	// just after the hour boundary (a long-running request, a busy writer)
	// is still captured.
	hourAggGrace = 10 * time.Minute
	// minDailySpan is the minimum number of complete days a query must cover
	// before the day tier is worth taking over the hour tier.
	minDailySpan = 2 * 24 * time.Hour
	// dstScanPad widens the raw scan window of an aggregation so rows whose
	// stored wall-clock offset differs from the current one (DST shifts) are
	// still found; the exact strftime guard filters out the excess.
	dstScanPad = 2 * time.Hour
)

// rollupTier describes one rollup granularity: where it lives and how its
// bucket keys are formatted.
type rollupTier struct {
	name          string
	table         string // provider/model/user dimensions
	scenarioTable string // scenario dimension only
	keyCol        string
	layout        string // Go layout of the bucket key
	step          time.Duration
}

var (
	hourTier = &rollupTier{name: "hour", table: "usage_hourly", scenarioTable: "usage_hourly_scenario", keyCol: "hour", layout: hourlyKeyLayout, step: time.Hour}
	dayTier  = &rollupTier{name: "day", table: "usage_daily", scenarioTable: "usage_daily_scenario", keyCol: "date", layout: dailyDateLayout, step: 24 * time.Hour}
)

const (
	dailyDateLayout = "2006-01-02"
	hourlyKeyLayout = "2006-01-02 15"
)

// key renders the bucket key of the bucket starting at t.
func (t *rollupTier) key(at time.Time) string { return at.UTC().Format(t.layout) }

// rollupSegment is one slice of a query window and the source that serves it.
// A tier segment covers the whole buckets [from, to); a raw segment is the
// inclusive time range [from, to] to scan in usage_records.
type rollupSegment struct {
	tier     *rollupTier // nil: raw scan
	from, to time.Time
}

// utcDayStart truncates a time to the start of its UTC day.
func utcDayStart(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// ceilTo rounds t up to a multiple of d since the zero time (UTC-aligned for
// hours and days).
func ceilTo(t time.Time, d time.Duration) time.Time {
	f := t.Truncate(d)
	if f.Equal(t) {
		return f
	}
	return f.Add(d)
}

// planRollupSegments splits [start, end] into segments served from the rollup
// tiers and raw scans as of now. hourOnly keeps the day tier out of the plan
// (queries whose result is itself per-hour). It returns nil when the window
// holds no complete eligible hour, i.e. a plain raw scan is the right plan.
func planRollupSegments(start, end, now time.Time, hourOnly bool) []rollupSegment {
	firstHour := ceilTo(start, time.Hour)
	lastHour := end.Truncate(time.Hour)
	if limit := now.Add(-hourAggGrace).Truncate(time.Hour); limit.Before(lastHour) {
		lastHour = limit
	}
	if !firstHour.Before(lastHour) {
		return nil
	}

	var segs []rollupSegment
	if start.Before(firstHour) {
		segs = append(segs, rollupSegment{from: start, to: firstHour.Add(-time.Nanosecond)})
	}
	firstDay, lastDay := ceilTo(firstHour, 24*time.Hour), lastHour.Truncate(24*time.Hour)
	if !hourOnly && lastDay.Sub(firstDay) >= minDailySpan {
		if firstHour.Before(firstDay) {
			segs = append(segs, rollupSegment{tier: hourTier, from: firstHour, to: firstDay})
		}
		segs = append(segs, rollupSegment{tier: dayTier, from: firstDay, to: lastDay})
		if lastDay.Before(lastHour) {
			segs = append(segs, rollupSegment{tier: hourTier, from: lastDay, to: lastHour})
		}
	} else {
		segs = append(segs, rollupSegment{tier: hourTier, from: firstHour, to: lastHour})
	}
	if end.After(lastHour) {
		segs = append(segs, rollupSegment{from: lastHour, to: end})
	}
	return segs
}
