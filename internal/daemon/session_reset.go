package daemon

import (
	"context"
	"strings"
	"time"

	"github.com/harun/ranya/internal/tracing"
)

func (d *Daemon) maybeResetSession(ctx context.Context, sessionKey string) error {
	if d == nil || d.sessionMgr == nil || d.config == nil {
		return nil
	}
	resetCfg := d.config.Session.Reset
	mode := strings.ToLower(strings.TrimSpace(resetCfg.Mode))
	if mode == "" || mode == "off" {
		return nil
	}

	info, err := d.sessionMgr.GetSessionInfo(sessionKey)
	if err != nil {
		return nil
	}
	lastModified, ok := info["lastModified"].(time.Time)
	if !ok || lastModified.IsZero() {
		return nil
	}

	now := time.Now()
	shouldReset := false
	switch mode {
	case "idle":
		if resetCfg.IdleMinutes <= 0 {
			return nil
		}
		idleFor := time.Duration(resetCfg.IdleMinutes) * time.Minute
		if now.Sub(lastModified) >= idleFor {
			shouldReset = true
		}
	case "daily":
		atHour := resetCfg.AtHour
		if atHour < 0 || atHour > 23 {
			atHour = 4
		}
		resetTime := time.Date(now.Year(), now.Month(), now.Day(), atHour, 0, 0, 0, now.Location())
		if now.After(resetTime) && lastModified.Before(resetTime) {
			shouldReset = true
		}
	default:
		return nil
	}

	if !shouldReset {
		return nil
	}

	logger := tracing.LoggerFromContext(ctx, d.logger.GetZerolog())
	logger.Info().
		Str("session_key", sessionKey).
		Str("mode", mode).
		Time("last_modified", lastModified).
		Msg("session reset triggered")

	return d.sessionMgr.ClearSessionWithContext(ctx, sessionKey)
}
