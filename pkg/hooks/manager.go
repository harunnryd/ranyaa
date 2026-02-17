package hooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Hook defines a lifecycle event hook.
type Hook struct {
	ID      string
	Event   string
	Script  string
	Timeout time.Duration
	Enabled bool
}

// Config configures a Hook manager.
type Config struct {
	Enabled bool
	Hooks   []Hook
	Logger  zerolog.Logger
}

// Manager executes configured hooks for lifecycle events.
type Manager struct {
	enabled bool
	logger  zerolog.Logger

	mu          sync.RWMutex
	hooksByEvent map[string][]Hook
}

// NewManager creates a hook manager.
func NewManager(cfg Config) (*Manager, error) {
	manager := &Manager{
		enabled:      cfg.Enabled,
		logger:       cfg.Logger.With().Str("component", "hooks").Logger(),
		hooksByEvent: make(map[string][]Hook),
	}

	if !cfg.Enabled {
		return manager, nil
	}

	for _, hook := range cfg.Hooks {
		if !hook.Enabled {
			continue
		}
		event := strings.TrimSpace(hook.Event)
		if event == "" {
			return nil, fmt.Errorf("hook event is required")
		}
		if strings.TrimSpace(hook.Script) == "" {
			return nil, fmt.Errorf("hook script is required for event %q", event)
		}
		manager.hooksByEvent[event] = append(manager.hooksByEvent[event], hook)
	}

	return manager, nil
}

// Trigger executes hooks registered for an event.
func (m *Manager) Trigger(ctx context.Context, event string, data map[string]interface{}) error {
	if m == nil || !m.enabled {
		return nil
	}
	event = strings.TrimSpace(event)
	if event == "" {
		return fmt.Errorf("event is required")
	}

	m.mu.RLock()
	hooks := append([]Hook(nil), m.hooksByEvent[event]...)
	m.mu.RUnlock()
	if len(hooks) == 0 {
		return nil
	}

	var errs []error
	for _, hook := range hooks {
		if err := m.executeHook(ctx, event, hook, data); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (m *Manager) executeHook(ctx context.Context, event string, hook Hook, data map[string]interface{}) error {
	if ctx == nil {
		ctx = context.Background()
	}

	hookID := hook.ID
	if strings.TrimSpace(hookID) == "" {
		hookID = event
	}

	runCtx := ctx
	cancel := func() {}
	if hook.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, hook.Timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", hook.Script)
	cmd.Env = buildHookEnvironment(event, data)

	output, err := cmd.CombinedOutput()
	outputText := strings.TrimSpace(string(output))
	if err != nil {
		if outputText != "" {
			return fmt.Errorf("hook %s failed: %w: %s", hookID, err, outputText)
		}
		return fmt.Errorf("hook %s failed: %w", hookID, err)
	}

	if outputText != "" {
		m.logger.Debug().
			Str("event", event).
			Str("hook_id", hookID).
			Str("output", outputText).
			Msg("Hook executed")
	}

	return nil
}

func buildHookEnvironment(event string, data map[string]interface{}) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env, "RANYA_HOOK_EVENT="+event)

	if len(data) == 0 {
		return env
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		envKey := "RANYA_HOOK_DATA_" + normalizeEnvKey(key)
		env = append(env, envKey+"="+fmt.Sprintf("%v", data[key]))
	}
	return env
}

func normalizeEnvKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return "UNKNOWN"
	}

	upper := strings.ToUpper(key)
	builder := strings.Builder{}
	builder.Grow(len(upper))
	for _, r := range upper {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			continue
		}
		builder.WriteRune('_')
	}
	return builder.String()
}

