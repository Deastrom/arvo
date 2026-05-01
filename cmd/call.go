package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Deastrom/arvo/internal/mcp"
	"github.com/Deastrom/arvo/internal/output"
	"github.com/spf13/cobra"
)

var callCmd = &cobra.Command{
	Use:   "call <tool> [json-args]",
	Short: "Raw MCP tool call (escape hatch)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]
		var toolArgs map[string]any
		if len(args) == 2 {
			if err := json.Unmarshal([]byte(args[1]), &toolArgs); err != nil {
				return fmt.Errorf("invalid JSON args: %w", err)
			}
		}
		if toolArgs == nil {
			toolArgs = map[string]any{}
		}

		s, err := session()
		if err != nil {
			return err
		}

		// Inject cloudId if not already provided by the caller.
		if _, ok := toolArgs["cloudId"]; !ok {
			toolArgs["cloudId"] = s.cloudID
		}

		result, err := s.client.CallTool(toolName, toolArgs)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.Print(os.Stdout, result, true)
		}

		text := mcp.TextContent(result)
		if text == "" {
			return output.Print(os.Stdout, result, false)
		}
		fmt.Fprintln(os.Stdout, text)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(callCmd)
}
