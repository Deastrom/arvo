package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	cloudFlag  string // --cloud overrides config cloud_id
)

var rootCmd = &cobra.Command{
	Use:   "arvo",
	Short: "Atlassian CLI for AI agents — proxies to the Atlassian MCP",
	Long: `arvo bridges AI coding agents to Atlassian (Jira + Confluence) via the
Atlassian MCP, without requiring the MCP to be loaded into agent context.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// SetVersion wires the build-time version into the root command.
// Must be called before Execute().
func SetVersion(v string) {
	rootCmd.Version = v
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output raw JSON")
	rootCmd.PersistentFlags().StringVar(&cloudFlag, "cloud", "", "Atlassian cloud ID (overrides config default)")
}
