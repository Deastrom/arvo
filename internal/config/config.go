package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const configFile = "config.json"

// Config holds arvo user configuration.
type Config struct {
	CloudID    string `json:"cloud_id,omitempty"`
	CloudURL   string `json:"cloud_url,omitempty"`
	MCPURL     string `json:"mcp_url,omitempty"`
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "arvo"), nil
}

func path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, configFile), nil
}

// Load reads config from disk. Returns a zero-value Config if none exists.
func Load() (*Config, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &c, nil
}

// Save writes config to disk.
func Save(c *Config) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, configFile), data, 0600)
}
