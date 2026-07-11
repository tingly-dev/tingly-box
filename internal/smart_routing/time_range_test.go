package smartrouting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func TestParseTimeRange(t *testing.T) {
	valid := `{"start":"09:00","end":"17:00","timezone":"Asia/Shanghai","outside":false}`
	tr, err := parseTimeRange(valid)
	require.NoError(t, err)
	require.Equal(t, 9*60, tr.startMinute)
	require.Equal(t, 17*60, tr.endMinute)
	require.Equal(t, "Asia/Shanghai", tr.location.String())

	for _, value := range []string{
		``, `[]`, `{"start":"9:00","end":"17:00","timezone":"UTC"}`,
		`{"start":"24:00","end":"17:00","timezone":"UTC"}`,
		`{"start":"09:00","end":"09:00","timezone":"UTC"}`,
		`{"start":"09:00","end":"17:00","timezone":"Not/AZone"}`,
		`{"start":"09:00","end":"17:00","timezone":"UTC","extra":true}`,
		`{"start":"09:00","end":"17:00","timezone":"UTC"} {}`,
	} {
		_, err := parseTimeRange(value)
		require.Error(t, err, value)
	}

	// outside is deliberately optional for manually-authored forward-compatible rules.
	_, err = parseTimeRange(`{"start":"09:00","end":"17:00","timezone":"UTC"}`)
	require.NoError(t, err)
}

func TestEvaluateTimeRangeOp(t *testing.T) {
	original := utcNow
	t.Cleanup(func() { utcNow = original })
	utcNow = func() time.Time { return time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC) }

	r := &Router{}
	standard := SmartOp{Position: PositionTime, Operation: OpTimeRange, Value: `{"start":"09:00","end":"17:00","timezone":"UTC","outside":false}`}
	res := r.evaluateTimeRangeOp(&standard)
	require.True(t, res.Matched, res.Reason)
	require.Contains(t, res.Actual, "utc=2026-07-10T09:00:00Z")
	require.Contains(t, res.Actual, "timezone=UTC")

	utcNow = func() time.Time { return time.Date(2026, 7, 10, 17, 0, 0, 0, time.UTC) }
	require.False(t, r.evaluateTimeRangeOp(&standard).Matched, "end is exclusive")

	outside := standard
	outside.Value = `{"start":"09:00","end":"17:00","timezone":"UTC","outside":true}`
	require.True(t, r.evaluateTimeRangeOp(&outside).Matched)

	utcNow = func() time.Time { return time.Date(2026, 7, 10, 23, 0, 0, 0, time.UTC) }
	overnight := SmartOp{Position: PositionTime, Operation: OpTimeRange, Value: `{"start":"22:00","end":"06:00","timezone":"UTC","outside":false}`}
	require.True(t, r.evaluateTimeRangeOp(&overnight).Matched)

	utcNow = func() time.Time { return time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC) }
	shanghai := SmartOp{Position: PositionTime, Operation: OpTimeRange, Value: `{"start":"09:00","end":"10:00","timezone":"Asia/Shanghai","outside":false}`}
	require.True(t, r.evaluateTimeRangeOp(&shanghai).Matched)

	// DST fall-back (US 2026-11-01): 02:00 EDT -> 01:00 EST at 06:00 UTC, so the
	// wall-clock hour 01:00-02:00 occurs twice. 05:30 UTC is the first 01:30 (EDT),
	// 06:30 UTC the second 01:30 (EST); wall-clock-minute comparison must match both.
	newYork := SmartOp{Position: PositionTime, Operation: OpTimeRange, Value: `{"start":"01:00","end":"02:00","timezone":"America/New_York","outside":false}`}
	utcNow = func() time.Time { return time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC) }
	require.True(t, r.evaluateTimeRangeOp(&newYork).Matched, "first 01:30 (EDT)")
	utcNow = func() time.Time { return time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC) }
	require.True(t, r.evaluateTimeRangeOp(&newYork).Matched, "second 01:30 (EST)")

	invalid := SmartOp{Position: PositionTime, Operation: OpTimeRange, Value: `{}`}
	res = r.evaluateTimeRangeOp(&invalid)
	require.False(t, res.Matched)
	require.Contains(t, res.Reason, "invalid time range")
}

func TestTimeRangeValidationAndRouting(t *testing.T) {
	op := SmartOp{Position: PositionTime, Operation: OpTimeRange, Value: `{"start":"22:00","end":"06:00","timezone":"America/Los_Angeles","outside":false}`, Meta: SmartOpMeta{Type: ValueTypeString}}
	require.NoError(t, ValidateSmartOp(&op), "metadata must not override catalog value type")

	invalid := op
	invalid.Value = `{"start":"22:00","end":"22:00","timezone":"UTC"}`
	require.Error(t, ValidateSmartOp(&invalid))

	original := utcNow
	t.Cleanup(func() { utcNow = original })
	utcNow = func() time.Time { return time.Date(2026, 7, 10, 6, 30, 0, 0, time.UTC) } // 23:30 PDT
	router, err := NewRouter([]SmartRouting{{
		Description: "night service",
		Ops:         []SmartOp{op},
		Services:    testServices(),
	}})
	require.NoError(t, err)
	services, matched := router.EvaluateRequest(&RequestContext{})
	require.True(t, matched)
	require.Len(t, services, 1)
}

func testServices() []*loadbalance.Service {
	return []*loadbalance.Service{{Provider: "night", Model: "model", Active: true}}
}
