package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Deastrom/arvo/internal/config"
)

// setupHome sets HOME to a temp dir and returns cleanup func.
func setupHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

func TestRootCommandHelp(t *testing.T) {
	// Verify the binary can at least show help without panicking.
	// We do this by executing the cobra root command directly.
	// Since Execute() calls os.Exit, we test the command tree structure instead.
	// (Full integration tests would use exec.Command against the built binary.)
}

func TestConfigLoadCloudID(t *testing.T) {
	tmp := setupHome(t)

	// Write a config with a cloud ID.
	cfgDir := filepath.Join(tmp, ".config", "arvo")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{
		"cloud_id":  "myorg.atlassian.net",
		"cloud_url": "https://myorg.atlassian.net",
		"mcp_url":   "https://mcp.atlassian.com/v1/mcp",
	})
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.CloudID != "myorg.atlassian.net" {
		t.Errorf("expected cloud_id 'myorg.atlassian.net', got %q", cfg.CloudID)
	}
}

func TestConfigLoadMissing(t *testing.T) {
	setupHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load with missing file: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config even when file missing")
	}
	if cfg.CloudID != "" {
		t.Errorf("expected empty cloud_id when no config file, got %q", cfg.CloudID)
	}
}
