package cmd

import (
	"github.com/spf13/cobra"
)

var confluenceCmd = &cobra.Command{
	Use:   "confluence",
	Short: "Interact with Confluence",
}

var confluenceGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a Confluence page by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := session()
		if err != nil {
			return err
		}
		result, err := s.client.CallTool("getConfluencePage", map[string]any{
			"cloudId": s.cloudID,
			"pageId":  args[0],
		})
		if err != nil {
			return err
		}
		return printToolResult(result)
	},
}

var confluenceSearchCmd = &cobra.Command{
	Use:   "search <cql>",
	Short: "Search Confluence pages with CQL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := session()
		if err != nil {
			return err
		}
		result, err := s.client.CallTool("searchConfluenceUsingCql", map[string]any{
			"cloudId": s.cloudID,
			"cql":     args[0],
		})
		if err != nil {
			return err
		}
		return printToolResult(result)
	},
}

func init() {
	confluenceCmd.AddCommand(confluenceGetCmd, confluenceSearchCmd)
	rootCmd.AddCommand(confluenceCmd)
}
