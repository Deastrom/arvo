package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeFileAtomic writes data to path atomically via temp file + rename.
// This prevents partial writes from corrupting the file on crash.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".arvo-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

const configDir = ".config/arvo"

// Tokens holds OAuth tokens and the registered client_id.
type Tokens struct {
	ClientID     string    `json:"client_id"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	// TokenURL overrides the default token endpoint (used in tests).
	TokenURL string `json:"token_url,omitempty"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, configDir), nil
}

func tokenPath() (string, error) {
	dir, err := configPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tokens.json"), nil
}

// LoadTokens reads cached tokens. Returns nil, nil if no token file exists.
func LoadTokens() (*Tokens, error) {
	path, err := tokenPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read tokens: %w", err)
	}
	var t Tokens
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse tokens: %w", err)
	}
	return &t, nil
}

// SaveTokens writes tokens to disk with 0600 permissions.
func SaveTokens(t *Tokens) error {
	dir, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tokens: %w", err)
	}
	path := filepath.Join(dir, "tokens.json")
	return writeFileAtomic(path, data, 0600)
}

// ClearTokens deletes the token file.
func ClearTokens() error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// IsExpired reports whether the access token has expired (with 30s buffer).
func (t *Tokens) IsExpired() bool {
	return time.Now().Add(30 * time.Second).After(t.ExpiresAt)
}
