package cron

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// CalculateNextRun calculates the next run time for a schedule
func CalculateNextRun(schedule Schedule) (int64, error) {
	switch schedule.Kind {
	case ScheduleKindAt:
		return calculateAtSchedule(schedule)
	case ScheduleKindEvery:
		return calculateEverySchedule(schedule)
	case ScheduleKindCron:
		return calculateCronSchedule(schedule)
	default:
		return 0, fmt.Errorf("unknown schedule kind: %s", schedule.Kind)
	}
}

// calculateAtSchedule calculates next run for "at" schedule
func calculateAtSchedule(schedule Schedule) (int64, error) {
	if schedule.At == "" {
		return 0, fmt.Errorf("'at' schedule requires 'at' field")
	}

	// Parse ISO 8601 timestamp
	t, err := time.Parse(time.RFC3339, schedule.At)
	if err != nil {
		return 0, fmt.Errorf("invalid timestamp: %w", err)
	}

	return t.UnixMilli(), nil
}

// calculateEverySchedule calculates next run for "every" schedule
func calculateEverySchedule(schedule Schedule) (int64, error) {
	if schedule.EveryMs <= 0 {
		return 0, fmt.Errorf("'every' schedule requires positive 'everyMs' value")
	}

	now := time.Now().UnixMilli()

	// Without anchor: next run is now + interval
	if schedule.AnchorMs == nil {
		return now + schedule.EveryMs, nil
	}

	// With anchor: calculate next aligned time
	anchor := *schedule.AnchorMs
	elapsed := now - anchor

	// If anchor is in the future, use it
	if elapsed < 0 {
		return anchor, nil
	}

	// Calculate how many periods have passed
	periods := elapsed / schedule.EveryMs

	// Next run is anchor + (periods + 1) * interval
	nextRun := anchor + (periods+1)*schedule.EveryMs

	return nextRun, nil
}

// calculateCronSchedule calculates next run for "cron" schedule
func calculateCronSchedule(schedule Schedule) (int64, error) {
	if schedule.Expr == "" {
		return 0, fmt.Errorf("'cron' schedule requires 'expr' field")
	}

	// Parse cron expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(schedule.Expr)
	if err != nil {
		return 0, fmt.Errorf("invalid cron expression: %w", err)
	}

	// Get current time in appropriate timezone
	now := time.Now()
	if schedule.TZ != "" {
		loc, err := time.LoadLocation(schedule.TZ)
		if err != nil {
			return 0, fmt.Errorf("invalid timezone: %w", err)
		}
		now = now.In(loc)
	}

	// Calculate next run time
	next := sched.Next(now)

	return next.UnixMilli(), nil
}
