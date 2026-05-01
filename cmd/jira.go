package cmd

import (
	"fmt"
	"os"

	"github.com/Deastrom/arvo/internal/format"
	"github.com/Deastrom/arvo/internal/mcp"
	"github.com/spf13/cobra"
)

// Per-command output flags.
var (
	jiraRaw  bool
	jiraJSON bool
	jiraFull bool
)

var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Interact with Jira",
}

var jiraGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a Jira issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := session()
		if err != nil {
			return err
		}
		result, err := s.client.CallTool("getJiraIssue", map[string]any{
			"cloudId":      s.cloudID,
			"issueIdOrKey": args[0],
		})
		if err != nil {
			return err
		}
		return printIssue(result, s.cloudURL)
	},
}

var jiraSearchCmd = &cobra.Command{
	Use:   "search <jql>",
	Short: "Search Jira issues with JQL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := session()
		if err != nil {
			return err
		}
		result, err := s.client.CallTool("searchJiraIssuesUsingJql", map[string]any{
			"cloudId": s.cloudID,
			"jql":     args[0],
		})
		if err != nil {
			return err
		}
		return printIssueSearch(result)
	},
}

var (
	jiraProject     string
	jiraIssueType   string
	jiraSummary     string
	jiraDescription string
)

var jiraCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a Jira issue",
	RunE: func(cmd *cobra.Command, args []string) error {
		if jiraProject == "" || jiraSummary == "" {
			return fmt.Errorf("--project and --summary are required")
		}
		s, err := session()
		if err != nil {
			return err
		}
		toolArgs := map[string]any{
			"cloudId":       s.cloudID,
			"projectKey":    jiraProject,
			"summary":       jiraSummary,
			"issueTypeName": jiraIssueType,
		}
		if jiraDescription != "" {
			toolArgs["description"] = jiraDescription
		}
		result, err := s.client.CallTool("createJiraIssue", toolArgs)
		if err != nil {
			return err
		}
		return printIssue(result, s.cloudURL)
	},
}

var jiraTransitionTo string

var jiraTransitionCmd = &cobra.Command{
	Use:   "transition <key>",
	Short: "Transition a Jira issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if jiraTransitionTo == "" {
			return fmt.Errorf("--to is required (transition ID)")
		}
		s, err := session()
		if err != nil {
			return err
		}
		result, err := s.client.CallTool("transitionJiraIssue", map[string]any{
			"cloudId":      s.cloudID,
			"issueIdOrKey": args[0],
			"transition":   map[string]any{"id": jiraTransitionTo},
		})
		if err != nil {
			return err
		}
		return printToolResult(result)
	},
}

var jiraCommentBody string

var jiraCommentCmd = &cobra.Command{
	Use:   "comment <key>",
	Short: "Add a comment to a Jira issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if jiraCommentBody == "" {
			return fmt.Errorf("--body is required")
		}
		s, err := session()
		if err != nil {
			return err
		}
		result, err := s.client.CallTool("addCommentToJiraIssue", map[string]any{
			"cloudId":      s.cloudID,
			"issueIdOrKey": args[0],
			"commentBody":  jiraCommentBody,
		})
		if err != nil {
			return err
		}
		wantRaw() // trigger deprecation warning if global --json used
		return printToolResult(result)
	},
}

// --- output routing ---

// wantRaw reports whether the caller wants the full raw MCP response.
// The legacy global --json flag is treated as --raw with a deprecation warning.
func wantRaw() bool {
	if jsonOutput {
		fmt.Fprintln(os.Stderr, "warning: global --json is deprecated; use --raw for raw output or --json on the subcommand for curated JSON")
		return true
	}
	return jiraRaw
}

// wantJSON reports whether the caller wants curated JSON output.
func wantJSON() bool { return jiraJSON }

func printIssue(result *mcp.ToolCallResult, cloudURL string) error {
	if wantRaw() {
		return printToolResult(result)
	}
	text := mcp.TextContent(result)
	if text == "" {
		return printToolResult(result)
	}
	if jiraFull {
		d, err := format.ParseIssueDetail(text)
		if err != nil {
			return printToolResult(result)
		}
		d.URL = format.IssueURL(cloudURL, d.Key)
		if wantJSON() {
			return printJSON(d)
		}
		format.PrintIssueDetail(os.Stdout, d)
		return nil
	}
	s, err := format.ParseIssue(text)
	if err != nil {
		return printToolResult(result)
	}
	s.URL = format.IssueURL(cloudURL, s.Key)
	if wantJSON() {
		return printJSON(s)
	}
	format.PrintIssueSummary(os.Stdout, s)
	return nil
}

func printIssueSearch(result *mcp.ToolCallResult) error {
	if wantRaw() {
		return printToolResult(result)
	}
	text := mcp.TextContent(result)
	if text == "" {
		return printToolResult(result)
	}
	// --full on search: return individual full-detail summaries is not practical
	// for search results (N API calls). Instead, --full includes the full summary
	// text rows (no-op currently; reserved for future per-issue expansion).
	r, err := format.ParseIssueSearch(text)
	if err != nil {
		return printToolResult(result)
	}
	if wantJSON() {
		return printJSON(r)
	}
	format.PrintIssueSearch(os.Stdout, r)
	return nil
}

func init() {
	// get and search share all three output flags.
	for _, c := range []*cobra.Command{jiraGetCmd, jiraSearchCmd} {
		c.Flags().BoolVar(&jiraRaw, "raw", false, "Print raw MCP response")
		c.Flags().BoolVar(&jiraJSON, "json", false, "Print curated JSON")
		c.Flags().BoolVar(&jiraFull, "full", false, "Include full description and comments")
	}

	// create: --full makes no sense on a create response; omit it.
	jiraCreateCmd.Flags().BoolVar(&jiraRaw, "raw", false, "Print raw MCP response")
	jiraCreateCmd.Flags().BoolVar(&jiraJSON, "json", false, "Print curated JSON")
	jiraCreateCmd.Flags().StringVar(&jiraProject, "project", "", "Project key (required)")
	jiraCreateCmd.Flags().StringVar(&jiraIssueType, "type", "Task", "Issue type")
	jiraCreateCmd.Flags().StringVar(&jiraSummary, "summary", "", "Issue summary (required)")
	jiraCreateCmd.Flags().StringVar(&jiraDescription, "description", "", "Issue description")

	// transition and comment: add --raw for escape hatch.
	jiraTransitionCmd.Flags().BoolVar(&jiraRaw, "raw", false, "Print raw MCP response")
	jiraTransitionCmd.Flags().StringVar(&jiraTransitionTo, "to", "", "Transition ID (required)")

	jiraCommentCmd.Flags().BoolVar(&jiraRaw, "raw", false, "Print raw MCP response")
	jiraCommentCmd.Flags().StringVar(&jiraCommentBody, "body", "", "Comment body (required)")

	jiraCmd.AddCommand(jiraGetCmd, jiraSearchCmd, jiraCreateCmd, jiraTransitionCmd, jiraCommentCmd)
	rootCmd.AddCommand(jiraCmd)
}
