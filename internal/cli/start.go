package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Ranya daemon service",
	Long: `Start the Ranya daemon service in the background.
The daemon will process messages from Telegram and other channels.`,
	RunE: runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Check if daemon is already running
	pidFile := getPIDFilePath()
	if isRunning(pidFile) {
		return fmt.Errorf("daemon is already running (PID file: %s)", pidFile)
	}

	// TODO: This will be implemented when daemon service is ready (Phase B)
	// For now, we just create a placeholder
	fmt.Println("Starting Ranya daemon...")
	fmt.Printf("Config: %s\n", cfgFile)
	fmt.Printf("Log level: %s\n", logLevel)

	return fmt.Errorf("daemon service not yet implemented (Phase B)")
}

func getPIDFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/ranya.pid"
	}
	return filepath.Join(home, ".ranya", "ranya.pid")
}

func isRunning(pidFile string) bool {
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		return false
	}

	// Read PID and check if process exists
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}

	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(os.Signal(nil))
	return err == nil
}
