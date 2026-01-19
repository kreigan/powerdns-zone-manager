// Package cmd provides CLI commands for the PowerDNS zone manager.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kreigan/powerdns-zone-manager/internal/config"
	"github.com/kreigan/powerdns-zone-manager/internal/logger"
	"github.com/kreigan/powerdns-zone-manager/internal/manager"
	"github.com/kreigan/powerdns-zone-manager/internal/powerdns"
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
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runApply,
}

var dryRun bool
var autoConfirm bool

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be changed without applying")
	applyCmd.Flags().BoolVarP(&autoConfirm, "auto-confirm", "y", false, "Skip confirmation prompt")
}

func runApply(cmd *cobra.Command, args []string) error {
	apiURL, err := cmd.Flags().GetString("api-url")
	if err != nil {
		return fmt.Errorf("failed to get api-url flag: %w", err)
	}

	apiKey, err := cmd.Flags().GetString("api-key")
	if err != nil {
		return fmt.Errorf("failed to get api-key flag: %w", err)
	}

	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return fmt.Errorf("failed to get verbose flag: %w", err)
	}

	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("failed to get json flag: %w", err)
	}

	noColor, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return fmt.Errorf("failed to get no-color flag: %w", err)
	}

	configFile := args[0]
	accountName := getAccountName()

	// Initialize logger
	log := logger.New(logger.Options{
		Verbose: verbose,
		JSON:    jsonOutput,
		NoColor: noColor,
	})
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

	// Set confirmation function (skip in JSON mode or auto-confirm)
	if !jsonOutput && !autoConfirm && !dryRun {
		mgr.SetConfirmFunc(func(prompt string) bool {
			fmt.Printf("%s [y/N]: ", prompt)
			reader := bufio.NewReader(os.Stdin)
			response, err := reader.ReadString('\n')
			if err != nil {
				return false
			}
			response = strings.TrimSpace(strings.ToLower(response))
			return response == "y" || response == "yes"
		})
	}

	// Apply configuration
	opts := manager.ApplyOptions{
		DryRun:      dryRun,
		AutoConfirm: jsonOutput || autoConfirm,
	}

	log.Info("Applying configuration...")
	result, err := mgr.Apply(cmd.Context(), cfg, opts)
	if err != nil {
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	// Print results
	printApplyResult(log, result, dryRun, jsonOutput)

	if len(result.Errors) > 0 {
		return fmt.Errorf("apply completed with %d error(s)", len(result.Errors))
	}

	return nil
}

func printApplyResult(log *logger.Logger, result *manager.ApplyResult, isDryRun, jsonOutput bool) {
	if jsonOutput {
		log.InfoWithData("Apply completed", map[string]interface{}{
			"zonesCreated":  result.ZonesCreated,
			"rrsetsCreated": result.RRsetsCreated,
			"rrsetsUpdated": result.RRsetsUpdated,
			"rrsetsDeleted": result.RRsetsDeleted,
			"errors":        len(result.Errors),
		})
		return
	}

	prefix := ""
	if isDryRun {
		prefix = "[DRY RUN] "
	}

	fmt.Printf("\n%sResults:\n", prefix)
	fmt.Printf("  Zones created:  %d\n", result.ZonesCreated)
	fmt.Printf("  RRsets created: %d\n", result.RRsetsCreated)
	fmt.Printf("  RRsets updated: %d\n", result.RRsetsUpdated)
	fmt.Printf("  RRsets deleted: %d\n", result.RRsetsDeleted)

	if len(result.Errors) > 0 {
		fmt.Fprintf(os.Stderr, "\nErrors:\n")
		for _, err := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", err)
		}
	}
}
