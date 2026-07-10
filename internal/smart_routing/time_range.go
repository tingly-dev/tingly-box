package smartrouting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// utcNow is the router's sole clock source. Tests may replace it temporarily.
var utcNow = func() time.Time { return time.Now().UTC() }

type timeRange struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
	Outside  bool   `json:"outside"`

	startMinute int
	endMinute   int
	location    *time.Location
}

func parseTimeRange(value string) (timeRange, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(value))
	decoder.DisallowUnknownFields()

	var tr timeRange
	if err := decoder.Decode(&tr); err != nil {
		return timeRange{}, fmt.Errorf("invalid time range JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return timeRange{}, fmt.Errorf("invalid time range JSON: multiple values")
		}
		return timeRange{}, fmt.Errorf("invalid time range JSON: %w", err)
	}
	if tr.Start == "" || tr.End == "" || tr.Timezone == "" {
		return timeRange{}, fmt.Errorf("start, end, and timezone are required")
	}

	var err error
	if tr.startMinute, err = parseTimeOfDay(tr.Start); err != nil {
		return timeRange{}, fmt.Errorf("invalid start: %w", err)
	}
	if tr.endMinute, err = parseTimeOfDay(tr.End); err != nil {
		return timeRange{}, fmt.Errorf("invalid end: %w", err)
	}
	if tr.startMinute == tr.endMinute {
		return timeRange{}, fmt.Errorf("start and end must differ")
	}
	tr.location, err = time.LoadLocation(tr.Timezone)
	if err != nil {
		return timeRange{}, fmt.Errorf("invalid timezone %q: %w", tr.Timezone, err)
	}
	return tr, nil
}

func parseTimeOfDay(value string) (int, error) {
	if len(value) != len("00:00") || value[2] != ':' || value[0] < '0' || value[0] > '2' || value[1] < '0' || value[1] > '9' || value[3] < '0' || value[3] > '5' || value[4] < '0' || value[4] > '9' {
		return 0, fmt.Errorf("must be strict HH:mm")
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 23 {
		return 0, fmt.Errorf("must be between 00:00 and 23:59")
	}
	return hour*60 + minute, nil
}

func (tr timeRange) includes(now time.Time) bool {
	local := now.In(tr.location)
	minute := local.Hour()*60 + local.Minute()
	if tr.startMinute < tr.endMinute {
		return minute >= tr.startMinute && minute < tr.endMinute
	}
	return minute >= tr.startMinute || minute < tr.endMinute
}
