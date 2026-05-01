package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Deastrom/arvo/internal/mcp"
)

// printToolResult prints the raw MCP TextContent, or falls back to JSON-marshalled Content.
func printToolResult(result *mcp.ToolCallResult) error {
	text := mcp.TextContent(result)
	if text != "" {
		fmt.Fprintln(os.Stdout, text)
		return nil
	}
	b, err := json.MarshalIndent(result.Content, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(b))
	return nil
}

// printJSON marshals v as indented JSON to stdout.
func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(b))
	return nil
}
