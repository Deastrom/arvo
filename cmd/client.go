package cmd

import (
	"fmt"
	"os"

	"github.com/Deastrom/arvo/internal/auth"
	"github.com/Deastrom/arvo/internal/config"
	"github.com/Deastrom/arvo/internal/mcp"
)

// mcpSession holds a ready MCP client and the resolved cloudId.
type mcpSession struct {
	client   *mcp.Client
	cloudID  string
	cloudURL string // e.g. https://example.atlassian.net
}

// session loads cached tokens, refreshes if needed, resolves cloudId, and
// returns a ready MCP client. Commands call this instead of mcpClient().
func session() (*mcpSession, error) {
	t, err := auth.LoadTokens()
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("not authenticated — run `arvo auth login`")
	}
	t, refreshed, err := auth.EnsureValid(t)
	if err != nil {
		return nil, err
	}
	if refreshed {
		if err := auth.SaveTokens(t); err != nil {
			return nil, fmt.Errorf("save refreshed tokens: %w", err)
		}
	}

	cloudID, cloudURL, err := resolveCloud()
	if err != nil {
		return nil, err
	}

	mcpURL, err := resolveMCPURL()
	if err != nil {
		return nil, err
	}

	return &mcpSession{
		client:   mcp.New(t.AccessToken, mcpURL),
		cloudID:  cloudID,
		cloudURL: cloudURL,
	}, nil
}

// resolveCloud returns the cloudId and cloudURL to use.
// Note: when --cloud is specified, cloudURL is empty and issue browse URLs
// will not be rendered. Use `arvo auth login` to persist cloudURL automatically.
func resolveCloud() (string, string, error) {
	if cloudFlag != "" {
		return cloudFlag, "", nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", "", fmt.Errorf("load config: %w", err)
	}
	if cfg.CloudID == "" {
		return "", "", fmt.Errorf("no cloud ID configured — run `arvo auth login` or pass --cloud <id>")
	}
	return cfg.CloudID, cfg.CloudURL, nil
}

// resolveMCPURL returns the MCP server endpoint to use, checked in order:
// the ARVO_MCP_URL env var, the persisted config's mcp_url, then the
// built-in default (mcp.DefaultURL). The env var lets an override be
// applied without a rebuild or config edit — useful when Atlassian
// changes the endpoint or during an edge-side incident.
func resolveMCPURL() (string, error) {
	if v := os.Getenv("ARVO_MCP_URL"); v != "" {
		return v, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	if cfg.MCPURL != "" {
		return cfg.MCPURL, nil
	}
	return mcp.DefaultURL, nil
}
