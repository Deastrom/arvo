package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Deastrom/arvo/internal/mcp"
	"github.com/Deastrom/arvo/internal/output"
	"github.com/spf13/cobra"
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
		return printToolResult(result)
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
		return printToolResult(result)
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
		return printToolResult(result)
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
		return printToolResult(result)
	},
}

func printToolResult(result *mcp.ToolCallResult) error {
	if jsonOutput {
		return output.Print(os.Stdout, result, true)
	}
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

func init() {
	jiraCreateCmd.Flags().StringVar(&jiraProject, "project", "", "Project key (required)")
	jiraCreateCmd.Flags().StringVar(&jiraIssueType, "type", "Task", "Issue type")
	jiraCreateCmd.Flags().StringVar(&jiraSummary, "summary", "", "Issue summary (required)")
	jiraCreateCmd.Flags().StringVar(&jiraDescription, "description", "", "Issue description")

	jiraTransitionCmd.Flags().StringVar(&jiraTransitionTo, "to", "", "Transition ID (required)")

	jiraCommentCmd.Flags().StringVar(&jiraCommentBody, "body", "", "Comment body (required)")

	jiraCmd.AddCommand(jiraGetCmd, jiraSearchCmd, jiraCreateCmd, jiraTransitionCmd, jiraCommentCmd)
	rootCmd.AddCommand(jiraCmd)
}
