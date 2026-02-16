package cli

import (
	"github.com/spf13/cobra"
)

const version = "0.1.0"

var (
	cfgFile  string
	logLevel string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ranya",
	Short: "Ranya - AI Agent Orchestration System",
	Long: `Ranya is an AI agent orchestration system with Telegram integration.
It provides always-on daemon service, multi-agent coordination, and powerful
tool execution capabilities.`,
	Version: version,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ranya/ranya.json)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	// Version template
	rootCmd.SetVersionTemplate(`{{with .Name}}{{printf "%s " .}}{{end}}{{printf "version %s" .Version}}
`)
}

// GetRootCmd returns the root command for testing
func GetRootCmd() *cobra.Command {
	return rootCmd
}

// GetVersion returns the current version
func GetVersion() string {
	return version
}
