package quota

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const quotaHistoryRetention = 30 * 24 * time.Hour

type quotaDailyExtreme struct {
	minID, maxID uint
	min, max     float64
}

// CompactHistory keeps today's five-minute samples and the lowest and highest
// observation of each quota window on completed local days. It keeps original
// snapshots, so no new record format or synthetic quota values are needed.
func (s *GormStore) CompactHistory(ctx context.Context, now time.Time) error {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	retention := now.Add(-quotaHistoryRetention)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("unixepoch(fetched_at) < ?", retention.Unix()).Delete(&ProviderUsageHistoryRecord{}).Error; err != nil {
			return fmt.Errorf("delete old quota history: %w", err)
		}
		var providers []string
		if err := tx.Model(&ProviderUsageHistoryRecord{}).
			Where("unixepoch(fetched_at) >= ? AND unixepoch(fetched_at) < ?", retention.Unix(), today.Unix()).
			Distinct("provider_uuid").Pluck("provider_uuid", &providers).Error; err != nil {
			return fmt.Errorf("list quota history providers: %w", err)
		}
		for _, provider := range providers {
			var records []ProviderUsageHistoryRecord
			if err := tx.Where("provider_uuid = ? AND unixepoch(fetched_at) >= ? AND unixepoch(fetched_at) < ?", provider, retention.Unix(), today.Unix()).
				Order("unixepoch(fetched_at) ASC, id ASC").Find(&records).Error; err != nil {
				return fmt.Errorf("load quota history for %s: %w", provider, err)
			}
			keep := quotaDailyExtremes(records, today.Location())
			if len(keep) == len(records) {
				continue
			}
			ids := make([]uint, 0, len(keep))
			for id := range keep {
				ids = append(ids, id)
			}
			if err := tx.Where("provider_uuid = ? AND unixepoch(fetched_at) >= ? AND unixepoch(fetched_at) < ?", provider, retention.Unix(), today.Unix()).
				Where("id NOT IN ?", ids).Delete(&ProviderUsageHistoryRecord{}).Error; err != nil {
				return fmt.Errorf("remove redundant quota history for %s: %w", provider, err)
			}
		}
		return nil
	})
}

func quotaDailyExtremes(records []ProviderUsageHistoryRecord, location *time.Location) map[uint]bool {
	extremes := make(map[string]quotaDailyExtreme)
	lastByDay := make(map[string]uint)
	numericDay := make(map[string]bool)
	for i := range records {
		record := &records[i]
		day := record.FetchedAt.In(location).Format("2006-01-02")
		lastByDay[day] = record.ID
		for _, window := range record.toProviderUsage().Windows {
			if window == nil {
				continue
			}
			var value float64
			mode := "used"
			switch {
			case window.Countable():
				value = window.UsedPercent
			case window.Available != nil:
				value = *window.Available
				mode = "available"
			default:
				continue
			}
			numericDay[day] = true
			key := fmt.Sprintf("%s:%s:%d:%s:%s:%s", day, window.Key, window.WindowMinutes, window.Unit, window.CurrencyCode, mode)
			extreme, exists := extremes[key]
			if !exists {
				extremes[key] = quotaDailyExtreme{minID: record.ID, maxID: record.ID, min: value, max: value}
				continue
			}
			if value < extreme.min {
				extreme.min, extreme.minID = value, record.ID
			}
			if value > extreme.max {
				extreme.max, extreme.maxID = value, record.ID
			}
			extremes[key] = extreme
		}
	}
	keep := make(map[uint]bool)
	for _, extreme := range extremes {
		keep[extreme.minID] = true
		keep[extreme.maxID] = true
	}
	for day, id := range lastByDay {
		if !numericDay[day] {
			keep[id] = true
		}
	}
	return keep
}
