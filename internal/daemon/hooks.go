package daemon

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/pkg/commandqueue"
	"github.com/harun/ranya/pkg/hooks"
	"github.com/rs/zerolog"
)

func newHookManager(cfg config.HooksConfig, logger zerolog.Logger) (*hooks.Manager, error) {
	hookDefs := make([]hooks.Hook, 0, len(cfg.Entries))
	for _, entry := range cfg.Entries {
		timeout := time.Duration(entry.TimeoutMs) * time.Millisecond
		if entry.TimeoutMs <= 0 {
			timeout = 5 * time.Second
		}
		hookDefs = append(hookDefs, hooks.Hook{
			ID:      strings.TrimSpace(entry.ID),
			Event:   strings.TrimSpace(entry.Event),
			Script:  strings.TrimSpace(entry.Script),
			Timeout: timeout,
			Enabled: entry.Enabled,
		})
	}

	return hooks.NewManager(hooks.Config{
		Enabled: cfg.Enabled,
		Hooks:   hookDefs,
		Logger:  logger,
	})
}

func (d *Daemon) triggerHookEvent(ctx context.Context, event string, data map[string]interface{}) error {
	if d.hookManager == nil {
		return nil
	}
	if err := d.hookManager.Trigger(ctx, event, data); err != nil {
		return err
	}
	return nil
}

func (d *Daemon) bindQueueHooks() {
	if d.queue == nil || d.hookManager == nil {
		return
	}

	d.queue.On("enqueued", func(evt commandqueue.Event) {
		data := map[string]interface{}{
			"lane":    evt.Lane,
			"task_id": evt.TaskID,
		}
		for k, v := range evt.Data {
			data[k] = v
		}
		go func() {
			if err := d.triggerHookEvent(context.Background(), "command:new", data); err != nil {
				d.logger.Warn().Err(err).Msg("command:new hooks failed")
			}
		}()
	})
}

func (d *Daemon) triggerStartupHooks() {
	if err := d.triggerHookEvent(context.Background(), "daemon:startup", map[string]interface{}{
		"pid": os.Getpid(),
	}); err != nil {
		d.logger.Warn().Err(err).Msg("daemon:startup hooks failed")
	}
}

func (d *Daemon) triggerAgentBootstrapHook(ctx context.Context, req RuntimeRequest, agentID string) error {
	data := map[string]interface{}{
		"session_key": req.SessionKey,
		"source":      req.Source,
		"agent_id":    agentID,
		"prompt":      req.Prompt,
	}
	if err := d.triggerHookEvent(ctx, "agent:bootstrap", data); err != nil {
		return fmt.Errorf("agent:bootstrap hooks failed: %w", err)
	}
	return nil
}
