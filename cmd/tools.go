package cmd

import (
	"fmt"
	"os"

	"github.com/Deastrom/arvo/internal/output"
	"github.com/spf13/cobra"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "List available MCP tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := session()
		if err != nil {
			return err
		}
		tools, err := s.client.ListTools()
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.Print(os.Stdout, tools, true)
		}

		rows := make([][]string, len(tools))
		for i, t := range tools {
			desc := t.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			rows[i] = []string{t.Name, desc}
		}
		output.Table(os.Stdout, []string{"TOOL", "DESCRIPTION"}, rows)
		fmt.Fprintf(os.Stderr, "\n%d tools available\n", len(tools))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(toolsCmd)
}
