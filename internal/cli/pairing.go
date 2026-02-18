package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/pkg/pairing"
	"github.com/spf13/cobra"
)

var pairingCmd = &cobra.Command{
	Use:   "pairing",
	Short: "Manage channel pairing requests",
}

var pairingListCmd = &cobra.Command{
	Use:   "list <channel>",
	Short: "List pending pairing requests for a channel",
	Args:  cobra.ExactArgs(1),
	RunE:  runPairingList,
}

var pairingApproveCmd = &cobra.Command{
	Use:   "approve <channel> <code>",
	Short: "Approve a pending pairing request by code",
	Args:  cobra.ExactArgs(2),
	RunE:  runPairingApprove,
}

var pairingRejectCmd = &cobra.Command{
	Use:   "reject <channel> <code>",
	Short: "Reject a pending pairing request by code",
	Args:  cobra.ExactArgs(2),
	RunE:  runPairingReject,
}

func init() {
	pairingCmd.AddCommand(pairingListCmd)
	pairingCmd.AddCommand(pairingApproveCmd)
	pairingCmd.AddCommand(pairingRejectCmd)
	rootCmd.AddCommand(pairingCmd)
}

func runPairingList(cmd *cobra.Command, args []string) error {
	channel := strings.ToLower(strings.TrimSpace(args[0]))
	manager, err := loadPairingManager(channel)
	if err != nil {
		return err
	}

	pending := manager.ListPending()
	if len(pending) == 0 {
		cmd.Println("No pending pairing requests.")
		return nil
	}

	cmd.Printf("Pending pairing requests for %s:\n", channel)
	for _, req := range pending {
		remaining := time.Until(req.ExpiresAt).Round(time.Second)
		if remaining < 0 {
			remaining = 0
		}
		cmd.Printf("- code: %s | peer: %s | expires in: %s\n", req.Code, req.PeerID, remaining)
	}
	return nil
}

func runPairingApprove(cmd *cobra.Command, args []string) error {
	channel := strings.ToLower(strings.TrimSpace(args[0]))
	code := strings.TrimSpace(args[1])
	manager, err := loadPairingManager(channel)
	if err != nil {
		return err
	}

	req, err := manager.Approve(code)
	if err != nil {
		return err
	}

	cmd.Printf("Approved pairing for %s (peer %s).\n", channel, req.PeerID)
	return nil
}

func runPairingReject(cmd *cobra.Command, args []string) error {
	channel := strings.ToLower(strings.TrimSpace(args[0]))
	code := strings.TrimSpace(args[1])
	manager, err := loadPairingManager(channel)
	if err != nil {
		return err
	}

	req, err := manager.Reject(code)
	if err != nil {
		return err
	}

	cmd.Printf("Rejected pairing for %s (peer %s).\n", channel, req.PeerID)
	return nil
}

func loadPairingManager(channel string) (*pairing.Manager, error) {
	if channel == "" {
		return nil, fmt.Errorf("channel is required")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	pendingPath, allowlistPath := pairing.DefaultPaths(cfg.DataDir, channel)
	bootstrap := []string{}
	if channel == "telegram" {
		for _, id := range cfg.Telegram.Allowlist {
			bootstrap = append(bootstrap, fmt.Sprintf("%d", id))
		}
	}

	return pairing.NewManager(pairing.ManagerOptions{
		Channel:            channel,
		PendingPath:        pendingPath,
		AllowlistPath:      allowlistPath,
		BootstrapAllowlist: bootstrap,
	})
}
