package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Deastrom/arvo/internal/auth"
	"github.com/Deastrom/arvo/internal/config"
	"github.com/Deastrom/arvo/internal/mcp"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Atlassian authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with Atlassian via OAuth 2.1",
	RunE: func(cmd *cobra.Command, args []string) error {
		existing, _ := auth.LoadTokens()
		clientID := ""
		if existing != nil {
			clientID = existing.ClientID
		}

		tokens, err := auth.Login(clientID)
		if err != nil {
			return err
		}
		if err := auth.SaveTokens(tokens); err != nil {
			return fmt.Errorf("save tokens: %w", err)
		}

		// Fetch accessible sites via the MCP tool (not the REST API — the MCP
		// token is not valid for api.atlassian.com).
		mcpClient := mcp.New(tokens.AccessToken)
		sites, err := fetchSitesViaMCP(mcpClient)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not fetch sites: %v\n", err)
			fmt.Fprintln(os.Stderr, "Set cloud_id manually in ~/.config/arvo/config.json")
		} else if len(sites) == 0 {
			fmt.Fprintln(os.Stderr, "warning: no accessible Atlassian sites found")
			fmt.Fprintln(os.Stderr, "Set cloud_id manually in ~/.config/arvo/config.json")
		} else {
			site, err := pickSite(sites)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cfg.CloudID = site.ID
			cfg.CloudURL = site.URL
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintf(os.Stdout, "Default site set to: %s (%s)\n", site.Name, site.URL)
		}

		fmt.Fprintln(os.Stdout, "Login successful.")
		return nil
	},
}

// mcpSite is a site entry returned by getAccessibleAtlassianResources.
type mcpSite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// fetchSitesViaMCP calls the MCP tool getAccessibleAtlassianResources to get
// the list of sites. This works with the MCP token (unlike the REST API).
func fetchSitesViaMCP(client *mcp.Client) ([]mcpSite, error) {
	result, err := client.CallTool("getAccessibleAtlassianResources", map[string]any{})
	if err != nil {
		return nil, err
	}
	text := mcp.TextContent(result)

	// The MCP tool returns JSON embedded in a text block.
	// Try to parse it directly first.
	var sites []mcpSite
	if err := json.Unmarshal([]byte(text), &sites); err == nil {
		return sites, nil
	}

	// Some responses wrap in a content block — try to find a JSON array.
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(text[start:end+1]), &sites); err == nil {
			return sites, nil
		}
	}

	// Fall back: return a single synthesized entry from whatever text we got,
	// so the user at least sees something useful.
	return nil, fmt.Errorf("could not parse sites from MCP response: %s", text)
}

// pickSite prompts the user to choose a site when multiple are available.
func pickSite(sites []mcpSite) (mcpSite, error) {
	if len(sites) == 1 {
		fmt.Fprintf(os.Stdout, "One site found: %s (%s)\n", sites[0].Name, sites[0].URL)
		return sites[0], nil
	}

	fmt.Fprintln(os.Stdout, "\nMultiple Atlassian sites found. Pick one:")
	for i, s := range sites {
		fmt.Fprintf(os.Stdout, "  [%d] %s  %s\n", i+1, s.Name, s.URL)
	}
	fmt.Fprint(os.Stdout, "Enter number: ")
	var choice int
	if _, err := fmt.Fscan(os.Stdin, &choice); err != nil || choice < 1 || choice > len(sites) {
		return mcpSite{}, fmt.Errorf("invalid selection")
	}
	return sites[choice-1], nil
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication state",
	RunE: func(cmd *cobra.Command, args []string) error {
		t, err := auth.LoadTokens()
		if err != nil {
			return err
		}
		if t == nil {
			fmt.Fprintln(os.Stdout, "status:              not logged in")
			fmt.Fprintln(os.Stdout, "run:                 arvo auth login")
			return nil
		}
		status := "valid"
		if t.IsExpired() {
			if t.RefreshToken != "" {
				status = "expired (refresh token available)"
			} else {
				status = "expired (run `arvo auth login`)"
			}
		}
		fmt.Fprintf(os.Stdout, "%-20s %s\n", "status:", status)
		fmt.Fprintf(os.Stdout, "%-20s %s\n", "client_id:", t.ClientID)
		fmt.Fprintf(os.Stdout, "%-20s %s\n", "expires_at:", t.ExpiresAt.Format(time.RFC3339))

		cfg, err := config.Load()
		if err == nil && cfg.CloudID != "" {
			fmt.Fprintf(os.Stdout, "%-20s %s\n", "cloud_id:", cfg.CloudID)
			fmt.Fprintf(os.Stdout, "%-20s %s\n", "cloud_url:", cfg.CloudURL)
		}
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear cached tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := auth.ClearTokens(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "Logged out. Tokens cleared.")
		return nil
	},
}

func init() {
	authCmd.AddCommand(authLoginCmd, authStatusCmd, authLogoutCmd)
	rootCmd.AddCommand(authCmd)
}
