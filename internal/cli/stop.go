package cli

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	stopTimeout int
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Ranya daemon service",
	Long: `Stop the Ranya daemon service gracefully.
Sends SIGTERM to the daemon and waits for it to shut down.`,
	RunE: runStop,
}

func init() {
	stopCmd.Flags().IntVar(&stopTimeout, "timeout", 30, "timeout in seconds to wait for daemon to stop")
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	pidFile := getPIDFilePath()

	if !isRunning(pidFile) {
		return fmt.Errorf("daemon is not running")
	}

	// Read PID
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	if err != nil {
		return fmt.Errorf("invalid PID file: %w", err)
	}

	// Find process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM
	fmt.Printf("Stopping daemon (PID: %d)...\n", pid)
	err = process.Signal(syscall.SIGTERM)
	if err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait for process to stop
	deadline := time.Now().Add(time.Duration(stopTimeout) * time.Second)
	for time.Now().Before(deadline) {
		if !isRunning(pidFile) {
			fmt.Println("Daemon stopped successfully")
			os.Remove(pidFile)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill if timeout
	fmt.Println("Timeout reached, sending SIGKILL...")
	err = process.Signal(syscall.SIGKILL)
	if err != nil {
		return fmt.Errorf("failed to send SIGKILL: %w", err)
	}

	os.Remove(pidFile)
	fmt.Println("Daemon killed")
	return nil
}
