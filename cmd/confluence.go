package cmd

import (
	"fmt"
	"os"

	"github.com/Deastrom/arvo/internal/format"
	"github.com/Deastrom/arvo/internal/mcp"
	"github.com/spf13/cobra"
)

var (
	confluenceRaw  bool
	confluenceJSON bool
	confluenceFull bool
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
		return printPage(result)
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
		return printPageSearch(result)
	},
}

func confluenceWantRaw() bool {
	if jsonOutput {
		fmt.Fprintln(os.Stderr, "warning: global --json is deprecated; use --raw for raw output or --json on the subcommand for curated JSON")
		return true
	}
	return confluenceRaw
}

func confluenceWantJSON() bool { return confluenceJSON }

func printPage(result *mcp.ToolCallResult) error {
	if confluenceWantRaw() {
		return printToolResult(result)
	}
	text := mcp.TextContent(result)
	if text == "" {
		return printToolResult(result)
	}
	if confluenceFull {
		d, err := format.ParsePageDetail(text)
		if err != nil {
			return printToolResult(result)
		}
		if confluenceWantJSON() {
			return printJSON(d)
		}
		format.PrintPageDetail(os.Stdout, d)
		return nil
	}
	p, err := format.ParsePage(text)
	if err != nil {
		return printToolResult(result)
	}
	if confluenceWantJSON() {
		return printJSON(p)
	}
	format.PrintPageSummary(os.Stdout, p)
	return nil
}

func printPageSearch(result *mcp.ToolCallResult) error {
	if confluenceWantRaw() {
		return printToolResult(result)
	}
	text := mcp.TextContent(result)
	if text == "" {
		return printToolResult(result)
	}
	r, err := format.ParsePageSearch(text)
	if err != nil {
		return printToolResult(result)
	}
	if confluenceWantJSON() {
		return printJSON(r)
	}
	format.PrintPageSearch(os.Stdout, r)
	return nil
}

func init() {
	for _, c := range []*cobra.Command{confluenceGetCmd, confluenceSearchCmd} {
		c.Flags().BoolVar(&confluenceRaw, "raw", false, "Print raw MCP response")
		c.Flags().BoolVar(&confluenceJSON, "json", false, "Print curated JSON")
		c.Flags().BoolVar(&confluenceFull, "full", false, "Include full body and comments")
	}

	confluenceCmd.AddCommand(confluenceGetCmd, confluenceSearchCmd)
	rootCmd.AddCommand(confluenceCmd)
}
