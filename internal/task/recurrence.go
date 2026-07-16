package task

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// RecurrenceSpec describes a standard five-field cron schedule evaluated in
// an IANA timezone. The scheduler persists the next concrete instant on the
// Task, so it does not need to evaluate cron expressions during polling.
type RecurrenceSpec struct {
	Cron     string `json:"cron"`
	Timezone string `json:"timezone"`
}

var recurrenceParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

// NextOccurrence validates recurrence and returns its first occurrence after
// the supplied instant. A missing timezone defaults to UTC so persisted tasks
// do not depend on the host machine's local timezone.
func NextOccurrence(recurrence json.RawMessage, after time.Time) (time.Time, error) {
	var spec RecurrenceSpec
	if err := json.Unmarshal(recurrence, &spec); err != nil {
		return time.Time{}, fmt.Errorf("%w: decode: %v", ErrInvalidRecurrence, err)
	}
	spec.Cron = strings.TrimSpace(spec.Cron)
	if spec.Cron == "" {
		return time.Time{}, fmt.Errorf("%w: cron is required", ErrInvalidRecurrence)
	}

	timezone := strings.TrimSpace(spec.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: timezone %q: %v", ErrInvalidRecurrence, timezone, err)
	}

	schedule, err := recurrenceParser.Parse(spec.Cron)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: cron %q: %v", ErrInvalidRecurrence, spec.Cron, err)
	}
	return schedule.Next(after.In(location)).UTC(), nil
}
