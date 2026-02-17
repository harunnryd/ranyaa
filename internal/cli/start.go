package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/harun/ranya/internal/config"
	"github.com/harun/ranya/internal/daemon"
	"github.com/harun/ranya/internal/logger"
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

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create logger
	log, err := logger.New(logger.Config{
		Level:   logLevel,
		File:    cfg.Logging.File,
		Console: false, // Daemon mode - no console output
	})
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	// Create daemon
	d, err := daemon.New(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to create daemon: %w", err)
	}

	// Fork to background
	if err := daemonize(pidFile); err != nil {
		return fmt.Errorf("failed to daemonize: %w", err)
	}

	// Start daemon
	if err := d.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Wait for shutdown signal
	d.Wait()

	// Remove PID file
	os.Remove(pidFile)

	return nil
}

func daemonize(pidFile string) error {
	// Check if we're already the child process
	if os.Getenv("RANYA_DAEMON_CHILD") == "1" {
		// We're the child, write PID file
		pid := os.Getpid()
		pidDir := filepath.Dir(pidFile)
		if err := os.MkdirAll(pidDir, 0755); err != nil {
			return fmt.Errorf("failed to create PID directory: %w", err)
		}
		if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
			return fmt.Errorf("failed to write PID file: %w", err)
		}
		return nil
	}

	// Fork the process
	env := append(os.Environ(), "RANYA_DAEMON_CHILD=1")
	procAttr := &os.ProcAttr{
		Env:   env,
		Files: []*os.File{nil, nil, nil}, // Detach from stdin/stdout/stderr
	}

	process, err := os.StartProcess(os.Args[0], os.Args, procAttr)
	if err != nil {
		return fmt.Errorf("failed to fork process: %w", err)
	}

	fmt.Printf("Ranya daemon started (PID: %d)\n", process.Pid)
	os.Exit(0)
	return nil
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

	// Check if process exists by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// stopDaemon sends SIGTERM to the daemon process
func stopDaemon(pidFile string) error {
	if !isRunning(pidFile) {
		return fmt.Errorf("daemon is not running")
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return fmt.Errorf("invalid PID in file: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	fmt.Printf("Stopping daemon (PID: %d)...\n", pid)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	return nil
}
