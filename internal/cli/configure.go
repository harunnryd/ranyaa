package cli

import (
	"fmt"

	"github.com/harun/ranya/internal/config"
	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Run interactive configuration wizard",
	Long: `Run an interactive configuration wizard to set up Ranya.
The wizard will guide you through configuring API keys, Telegram, and other settings.`,
	RunE: runConfigure,
}

func init() {
	rootCmd.AddCommand(configureCmd)
}

func runConfigure(cmd *cobra.Command, args []string) error {
	// Create wizard
	wizard := config.NewWizard()

	// Run wizard
	cfg, err := wizard.Run()
	if err != nil {
		return fmt.Errorf("configuration failed: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Save configuration
	loader := config.NewLoader(cfgFile)
	if err := loader.Save(cfg); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	configPath := loader.GetConfigPath()
	fmt.Printf("\nConfiguration saved to: %s\n", configPath)
	fmt.Println("\nYou can now start Ranya with: ranya start")

	return nil
}
