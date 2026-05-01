package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Deastrom/arvo/internal/auth"
)

func TestSaveAndLoadTokens(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	want := &auth.Tokens{
		ClientID:     "test-client",
		AccessToken:  "access-abc",
		RefreshToken: "refresh-xyz",
		ExpiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
	}

	if err := auth.SaveTokens(want); err != nil {
		t.Fatalf("SaveTokens: %v", err)
	}

	// File should have 0600 permissions.
	path := filepath.Join(tmp, ".config", "arvo", "tokens.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat tokens file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}

	got, err := auth.LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens: %v", err)
	}
	if got == nil {
		t.Fatal("LoadTokens returned nil")
	}
	if got.ClientID != want.ClientID {
		t.Errorf("ClientID: got %q, want %q", got.ClientID, want.ClientID)
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", got.AccessToken, want.AccessToken)
	}
}

func TestLoadTokensNotExist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	got, err := auth.LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when no file, got %+v", got)
	}
}

func TestClearTokens(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_ = auth.SaveTokens(&auth.Tokens{ClientID: "x", ExpiresAt: time.Now().Add(time.Hour)})
	if err := auth.ClearTokens(); err != nil {
		t.Fatalf("ClearTokens: %v", err)
	}
	got, err := auth.LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens after clear: %v", err)
	}
	if got != nil {
		t.Error("expected nil after clear")
	}
}

func TestIsExpired(t *testing.T) {
	future := &auth.Tokens{ExpiresAt: time.Now().Add(time.Hour)}
	if future.IsExpired() {
		t.Error("future token should not be expired")
	}
	past := &auth.Tokens{ExpiresAt: time.Now().Add(-time.Hour)}
	if !past.IsExpired() {
		t.Error("past token should be expired")
	}
}

func TestEnsureValidNotExpired(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tok := &auth.Tokens{
		ClientID:     "cid",
		AccessToken:  "original",
		RefreshToken: "rtoken",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	got, refreshed, err := auth.EnsureValid(tok)
	if err != nil {
		t.Fatalf("EnsureValid: %v", err)
	}
	if refreshed {
		t.Error("should not have refreshed a non-expired token")
	}
	if got.AccessToken != "original" {
		t.Errorf("expected original token, got %q", got.AccessToken)
	}
}

func TestEnsureValidRefreshes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Fake token endpoint that returns a new access token.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "refresh_token" {
			http.Error(w, "unexpected grant_type", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	// Expired token pointing at our fake token server.
	tok := &auth.Tokens{
		ClientID:     "cid",
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
		TokenURL:     srv.URL,
	}

	got, refreshed, err := auth.EnsureValid(tok)
	if err != nil {
		t.Fatalf("EnsureValid: %v", err)
	}
	if !refreshed {
		t.Error("expected token to be refreshed")
	}
	if got.AccessToken != "new-access-token" {
		t.Errorf("expected new access token, got %q", got.AccessToken)
	}
}

func TestEnsureValidNoRefreshToken(t *testing.T) {
	tok := &auth.Tokens{
		AccessToken: "old",
		ExpiresAt:   time.Now().Add(-time.Hour),
	}
	_, _, err := auth.EnsureValid(tok)
	if err == nil {
		t.Fatal("expected error when no refresh token")
	}
}
