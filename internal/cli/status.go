package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	Long:  `Show the current status of the Ranya daemon service.`,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	pidFile := getPIDFilePath()

	if !isRunning(pidFile) {
		fmt.Println("Status: stopped")
		return nil
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

	// Get PID file modification time for uptime calculation
	fileInfo, err := os.Stat(pidFile)
	if err == nil {
		uptime := time.Since(fileInfo.ModTime())
		fmt.Printf("Status: running\n")
		fmt.Printf("PID: %d\n", pid)
		fmt.Printf("Uptime: %s\n", formatDuration(uptime))
	} else {
		fmt.Printf("Status: running\n")
		fmt.Printf("PID: %d\n", pid)
	}

	return nil
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
