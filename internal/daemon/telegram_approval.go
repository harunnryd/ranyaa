package daemon

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/harun/ranya/internal/telegram"
	"github.com/harun/ranya/pkg/toolexecutor"
	"github.com/rs/zerolog"
)

type telegramApprovalForwarder struct {
	bot     *telegram.Bot
	targets []int64
	logger  zerolog.Logger
}

func (f *telegramApprovalForwarder) ForwardApproval(_ context.Context, pending toolexecutor.PendingApproval) error {
	if f.bot == nil {
		return fmt.Errorf("telegram bot is not configured")
	}
	if len(f.targets) == 0 {
		return fmt.Errorf("no telegram approval targets configured")
	}

	text := formatTelegramApprovalMessage(pending)
	sent := 0
	var lastErr error
	for _, chatID := range f.targets {
		if err := f.bot.SendMessage(chatID, text); err != nil {
			lastErr = err
			f.logger.Warn().
				Err(err).
				Int64("chat_id", chatID).
				Str("approval_id", pending.ID).
				Msg("Failed to forward approval request to Telegram target")
			continue
		}
		sent++
	}

	if sent == 0 {
		if lastErr != nil {
			return fmt.Errorf("failed to forward approval request: %w", lastErr)
		}
		return fmt.Errorf("failed to forward approval request")
	}

	return nil
}

func formatTelegramApprovalMessage(pending toolexecutor.PendingApproval) string {
	req := pending.Request
	parts := []string{
		"Approval required for tool execution.",
		fmt.Sprintf("id: %s", pending.ID),
		fmt.Sprintf("command: %s", req.Command),
	}
	if len(req.Args) > 0 {
		parts = append(parts, fmt.Sprintf("args: %s", strings.Join(req.Args, " ")))
	}
	if req.Cwd != "" {
		parts = append(parts, fmt.Sprintf("cwd: %s", req.Cwd))
	}
	if req.AgentID != "" {
		parts = append(parts, fmt.Sprintf("agent: %s", req.AgentID))
	}
	parts = append(parts, "Reply with: /approve <id> allow-once|allow-always|deny")
	return strings.Join(parts, "\n")
}

func (d *Daemon) configureTelegramApprovalWorkflow() error {
	if d.toolExecutor == nil || d.telegramBot == nil {
		return nil
	}
	if !d.config.Tools.ExecApprovals.Enabled {
		return nil
	}

	targets := append([]int64{}, d.config.Telegram.Allowlist...)
	if len(targets) == 0 {
		d.logger.Warn().Msg("exec approvals enabled but no Telegram allowlist targets configured; chat approvals disabled")
		return nil
	}

	allowlist, err := toolexecutor.NewAllowlistManager(d.config.Tools.ExecApprovals.AllowlistPath)
	if err != nil {
		return fmt.Errorf("failed to initialize approval allowlist manager: %w", err)
	}

	forwarder := &telegramApprovalForwarder{
		bot:     d.telegramBot,
		targets: targets,
		logger:  d.logger.GetZerolog(),
	}
	approvalHandler := toolexecutor.NewChatApprovalHandler(forwarder, allowlist)
	approvalManager := toolexecutor.NewApprovalManager(approvalHandler)
	d.toolExecutor.SetApprovalManager(approvalManager)

	if d.gatewayServer != nil {
		if err := d.gatewayServer.RegisterMethod("approvals.resolve", func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			id, _ := params["id"].(string)
			if strings.TrimSpace(id) == "" {
				return nil, fmt.Errorf("id is required")
			}

			actionRaw, _ := params["action"].(string)
			action, parseErr := toolexecutor.ParseApprovalAction(actionRaw)
			if parseErr != nil {
				return nil, parseErr
			}

			actor, _ := params["actor"].(string)
			if actor == "" {
				actor = "gateway"
			}

			if err := approvalManager.ResolveApproval(id, action, actor); err != nil {
				return nil, err
			}

			return map[string]interface{}{
				"resolved": true,
				"id":       id,
				"action":   string(action),
			}, nil
		}); err != nil {
			return fmt.Errorf("failed to register approvals.resolve method: %w", err)
		}
	}

	commands := d.telegramCmd
	if commands == nil {
		commands = telegram.NewCommands(d.telegramBot)
		d.telegramCmd = commands
		d.telegramBot.SetCommandHandler(commands)
	}
	commands.Register("approve", func(cmdCtx telegram.CommandContext) error {
		if len(cmdCtx.Args) != 2 {
			return commands.SendResponse(cmdCtx, "Usage: /approve <id> allow-once|allow-always|deny")
		}

		approvalID := strings.TrimSpace(cmdCtx.Args[0])
		action, parseErr := toolexecutor.ParseApprovalAction(cmdCtx.Args[1])
		if parseErr != nil {
			return commands.SendResponse(cmdCtx, "Invalid action. Use allow-once, allow-always, or deny")
		}

		actor := cmdCtx.Username
		if actor == "" {
			actor = "user:" + strconv.FormatInt(cmdCtx.UserID, 10)
		}

		if resolveErr := approvalManager.ResolveApproval(approvalID, action, actor); resolveErr != nil {
			return commands.SendResponse(cmdCtx, fmt.Sprintf("Failed to resolve approval: %v", resolveErr))
		}

		return commands.SendResponse(cmdCtx, fmt.Sprintf("Approval %s resolved: %s", approvalID, action))
	})

	d.logger.Info().
		Int("targets", len(targets)).
		Msg("Telegram chat approval workflow enabled")

	return nil
}
