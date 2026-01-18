package cmd

import (
	"fmt"
	"os"

	"github.com/kreigan/powerdns-zone-manager/pkg/config"
	"github.com/kreigan/powerdns-zone-manager/pkg/logger"
	"github.com/kreigan/powerdns-zone-manager/pkg/manager"
	"github.com/kreigan/powerdns-zone-manager/pkg/powerdns"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply [config-file]",
	Short: "Apply zone configuration from YAML file",
	Long: `Apply zone and record configuration from a YAML file.

This command:
1. Creates absent zones (marked as managed with the configured account)
2. Creates, updates, or deletes managed RRsets in the zones
3. Does not touch records that are not managed

A record set is considered managed if it has at least one comment where its
'account' property value matches the configured account name.`,
	Args: cobra.ExactArgs(1),
	RunE: runApply,
}

var dryRun bool

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be changed without applying")
}

func runApply(cmd *cobra.Command, args []string) error {
	apiURL, _ := cmd.Flags().GetString("api-url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	verbose, _ := cmd.Flags().GetBool("verbose")
	configFile := args[0]
	accountName := getAccountName()

	// Initialize logger
	log := logger.New(verbose)
	log.SetDryRun(dryRun)

	log.Info("Loading configuration from %s", configFile)
	log.Debug("API URL: %s", apiURL)
	log.Debug("API Key: %s", logger.MaskSecret(apiKey))
	log.Debug("Account name: %s", accountName)

	// Load configuration
	cfg, err := config.LoadFromFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	log.Info("Loaded %d zone(s) from configuration", len(cfg.Zones))

	// Create PowerDNS client
	client := powerdns.NewClient(apiURL, apiKey, log)

	// Create manager
	mgr := manager.NewManager(client, accountName, log)

	// Apply configuration
	opts := manager.ApplyOptions{
		DryRun: dryRun,
	}

	log.Info("Applying configuration...")
	result, err := mgr.Apply(cmd.Context(), cfg, opts)
	if err != nil {
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	// Print results
	printApplyResult(result, dryRun)

	if len(result.Errors) > 0 {
		return fmt.Errorf("completed with %d error(s)", len(result.Errors))
	}

	log.Info("Configuration applied successfully")
	return nil
}

func printApplyResult(result *manager.ApplyResult, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "[DRY RUN] "
	}

	fmt.Fprintf(os.Stdout, "%s=== Apply Summary ===\n", prefix)
	fmt.Fprintf(os.Stdout, "Zones created: %d\n", result.ZonesCreated)
	fmt.Fprintf(os.Stdout, "RRsets created: %d\n", result.RRsetsCreated)
	fmt.Fprintf(os.Stdout, "RRsets updated: %d\n", result.RRsetsUpdated)
	fmt.Fprintf(os.Stdout, "RRsets deleted: %d\n", result.RRsetsDeleted)

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\n%sErrors:\n", prefix)
		for _, err := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
	}
}
