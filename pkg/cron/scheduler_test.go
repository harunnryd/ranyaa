package cron

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculateAtSchedule(t *testing.T) {
	t.Run("valid ISO 8601 timestamp", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindAt,
			At:   "2024-12-25T14:00:00Z",
		}

		nextRun, err := CalculateNextRun(schedule)
		require.NoError(t, err)

		expected := time.Date(2024, 12, 25, 14, 0, 0, 0, time.UTC).UnixMilli()
		assert.Equal(t, expected, nextRun)
	})

	t.Run("invalid timestamp", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindAt,
			At:   "invalid",
		}

		_, err := CalculateNextRun(schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid timestamp")
	})

	t.Run("missing at field", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindAt,
		}

		_, err := CalculateNextRun(schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires 'at' field")
	})
}

func TestCalculateEverySchedule(t *testing.T) {
	t.Run("without anchor", func(t *testing.T) {
		schedule := Schedule{
			Kind:    ScheduleKindEvery,
			EveryMs: 60000, // 1 minute
		}

		before := time.Now().UnixMilli()
		nextRun, err := CalculateNextRun(schedule)
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		// Next run should be approximately now + 60000ms
		assert.GreaterOrEqual(t, nextRun, before+60000)
		assert.LessOrEqual(t, nextRun, after+60000)
	})

	t.Run("with anchor in past", func(t *testing.T) {
		now := time.Now().UnixMilli()
		anchor := now - 150000 // 2.5 minutes ago

		schedule := Schedule{
			Kind:     ScheduleKindEvery,
			EveryMs:  60000, // 1 minute
			AnchorMs: &anchor,
		}

		nextRun, err := CalculateNextRun(schedule)
		require.NoError(t, err)

		// Should align to next interval after now
		// anchor + 3 * 60000 = anchor + 180000
		expected := anchor + 180000
		assert.Equal(t, expected, nextRun)
	})

	t.Run("with anchor in future", func(t *testing.T) {
		now := time.Now().UnixMilli()
		anchor := now + 60000 // 1 minute in future

		schedule := Schedule{
			Kind:     ScheduleKindEvery,
			EveryMs:  60000,
			AnchorMs: &anchor,
		}

		nextRun, err := CalculateNextRun(schedule)
		require.NoError(t, err)

		// Should use anchor as next run
		assert.Equal(t, anchor, nextRun)
	})

	t.Run("negative interval", func(t *testing.T) {
		schedule := Schedule{
			Kind:    ScheduleKindEvery,
			EveryMs: -1000,
		}

		_, err := CalculateNextRun(schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "positive 'everyMs'")
	})

	t.Run("zero interval", func(t *testing.T) {
		schedule := Schedule{
			Kind:    ScheduleKindEvery,
			EveryMs: 0,
		}

		_, err := CalculateNextRun(schedule)
		assert.Error(t, err)
	})
}

func TestCalculateCronSchedule(t *testing.T) {
	t.Run("valid cron expression - every hour", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindCron,
			Expr: "0 * * * *", // Every hour at minute 0
		}

		nextRun, err := CalculateNextRun(schedule)
		require.NoError(t, err)

		// Next run should be in the future
		now := time.Now().UnixMilli()
		assert.Greater(t, nextRun, now)

		// Should be at minute 0
		nextTime := time.UnixMilli(nextRun)
		assert.Equal(t, 0, nextTime.Minute())
	})

	t.Run("valid cron expression - daily at 9am", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindCron,
			Expr: "0 9 * * *", // Daily at 9:00 AM
		}

		nextRun, err := CalculateNextRun(schedule)
		require.NoError(t, err)

		nextTime := time.UnixMilli(nextRun)
		assert.Equal(t, 9, nextTime.Hour())
		assert.Equal(t, 0, nextTime.Minute())
	})

	t.Run("with timezone", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindCron,
			Expr: "0 9 * * *",
			TZ:   "America/New_York",
		}

		nextRun, err := CalculateNextRun(schedule)
		require.NoError(t, err)

		// Should be in the future
		now := time.Now().UnixMilli()
		assert.Greater(t, nextRun, now)

		// Verify timezone is applied
		loc, _ := time.LoadLocation("America/New_York")
		nextTime := time.UnixMilli(nextRun).In(loc)
		assert.Equal(t, 9, nextTime.Hour())
	})

	t.Run("invalid cron expression", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindCron,
			Expr: "invalid",
		}

		_, err := CalculateNextRun(schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid cron expression")
	})

	t.Run("invalid timezone", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindCron,
			Expr: "0 9 * * *",
			TZ:   "Invalid/Timezone",
		}

		_, err := CalculateNextRun(schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid timezone")
	})

	t.Run("missing expr field", func(t *testing.T) {
		schedule := Schedule{
			Kind: ScheduleKindCron,
		}

		_, err := CalculateNextRun(schedule)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "requires 'expr' field")
	})
}

func TestCalculateNextRunUnknownKind(t *testing.T) {
	schedule := Schedule{
		Kind: "unknown",
	}

	_, err := CalculateNextRun(schedule)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown schedule kind")
}
