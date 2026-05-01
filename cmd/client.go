package cmd

import (
	"fmt"

	"github.com/Deastrom/arvo/internal/auth"
	"github.com/Deastrom/arvo/internal/config"
	"github.com/Deastrom/arvo/internal/mcp"
)

// mcpSession holds a ready MCP client and the resolved cloudId.
type mcpSession struct {
	client  *mcp.Client
	cloudID string
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

	cloudID, err := resolveCloudID()
	if err != nil {
		return nil, err
	}

	return &mcpSession{
		client:  mcp.New(t.AccessToken),
		cloudID: cloudID,
	}, nil
}

// resolveCloudID returns the cloudId to use, in priority order:
//  1. --cloud flag
//  2. config.json cloud_id
func resolveCloudID() (string, error) {
	if cloudFlag != "" {
		return cloudFlag, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	if cfg.CloudID == "" {
		return "", fmt.Errorf("no cloud ID configured — run `arvo auth login` or pass --cloud <id>")
	}
	return cfg.CloudID, nil
}
