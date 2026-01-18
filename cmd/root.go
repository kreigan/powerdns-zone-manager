package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	defaultAccountName = "zone-manager"
)

var (
	version string
	commit  string
	date    string
)

// SetVersionInfo sets the version information for the CLI
func SetVersionInfo(v, c, d string) {
	version, commit, date = v, c, d
}

var rootCmd = &cobra.Command{
	Use:   "powerdns-zone-manager",
	Short: "Manage PowerDNS zones and records",
	Long: `A CLI tool for managing PowerDNS zones and resource record sets (RRsets).

This tool creates absent zones and manages RRsets marked with a specific account
name in comments. Only managed records are modified; other records are left untouched.

A record set is considered managed if it has at least one comment where its
'account' property value matches the configured account name (default: zone-manager,
configurable via ACCOUNT_NAME environment variable).`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().String("api-url", "", "PowerDNS API base URL (e.g., http://localhost:8081/api/v1/servers/localhost)")
	rootCmd.PersistentFlags().String("api-key", "", "PowerDNS API key")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose/debug output")

	_ = rootCmd.MarkPersistentFlagRequired("api-url")
	_ = rootCmd.MarkPersistentFlagRequired("api-key")
}

// getAccountName returns the account name from environment or default
func getAccountName() string {
	if name := os.Getenv("ACCOUNT_NAME"); name != "" {
		return name
	}
	return defaultAccountName
}
