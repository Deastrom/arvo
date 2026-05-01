package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	mcpBaseURL   = "https://mcp.atlassian.com"
	tokenURL     = "https://cf.mcp.atlassian.com/v1/token"
	registerURL  = "https://cf.mcp.atlassian.com/v1/register"
	authorizeURL = "https://mcp.atlassian.com/v1/authorize"
	callbackPort = "19876"
	callbackPath = "/mcp/oauth/callback"
	redirectURI  = "http://127.0.0.1:19876/mcp/oauth/callback"
)

// Register performs RFC 7591 dynamic client registration.
// Returns a client_id on success.
func Register(clientName string) (string, error) {
	payload := map[string]any{
		"client_name":                clientName,
		"client_uri":                 "https://github.com/Deastrom/arvo",
		"redirect_uris":              []string{redirectURI},
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(registerURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("register: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("register: status %d: %s", resp.StatusCode, b)
	}
	var result struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("register decode: %w", err)
	}
	if result.ClientID == "" {
		return "", fmt.Errorf("register: empty client_id in response")
	}
	return result.ClientID, nil
}

// Login performs the full OAuth 2.1 + PKCE authorization code flow.
// clientID may be empty — if so, dynamic registration is attempted first.
func Login(clientID string) (*Tokens, error) {
	var err error
	if clientID == "" {
		clientID, err = Register("arvo")
		if err != nil {
			return nil, fmt.Errorf("dynamic registration failed (set client_id in ~/.config/arvo/config.json): %w", err)
		}
	}

	// PKCE
	verifier, challenge, err := pkce()
	if err != nil {
		return nil, err
	}

	// Spin up local callback server on the fixed port 127.0.0.1:19876.
	// This must match the redirect_uri registered with Atlassian exactly.
	listener, err := net.Listen("tcp", "127.0.0.1:"+callbackPort)
	if err != nil {
		return nil, fmt.Errorf("callback listener (port %s in use?): %w", callbackPort, err)
	}

	// Build authorization URL.
	state := randomState()
	// offline_access gets us a refresh token.
	// The MCP server advertises the Jira/Confluence scopes itself; requesting
	// an empty scope here lets Atlassian grant whatever the MCP app needs.
	scope := "offline_access"
	authURL := fmt.Sprintf(
		"%s?response_type=code&client_id=%s&redirect_uri=%s&code_challenge=%s&code_challenge_method=S256&state=%s&scope=%s",
		authorizeURL,
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		challenge,
		state,
		url.QueryEscape(scope),
	)

	fmt.Printf("Opening browser for Atlassian login...\nIf the browser does not open, visit:\n%s\n", authURL)
	_ = openBrowser(authURL)

	// Wait for callback.
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "Login successful. You can close this tab.")
		codeCh <- code
	})

	srv := &http.Server{
		Handler:     mux,
		ReadTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(listener) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case e := <-errCh:
		_ = srv.Shutdown(context.Background())
		return nil, e
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return nil, fmt.Errorf("login timed out waiting for browser callback")
	}
	_ = srv.Shutdown(context.Background())

	return exchangeCode(clientID, code, verifier, redirectURI)
}

// Refresh exchanges a refresh token for a new access token.
func Refresh(t *Tokens) (*Tokens, error) {
	endpoint := tokenURL
	if t.TokenURL != "" {
		endpoint = t.TokenURL
	}
	vals := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {t.RefreshToken},
		"client_id":     {t.ClientID},
	}
	resp, err := http.PostForm(endpoint, vals)
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}
	defer resp.Body.Close()
	return parseTokenResponse(t.ClientID, resp)
}

func exchangeCode(clientID, code, verifier, redirectURI string) (*Tokens, error) {
	vals := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	}
	resp, err := http.PostForm(tokenURL, vals)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	return parseTokenResponse(clientID, resp)
}

func parseTokenResponse(clientID string, resp *http.Response) (*Tokens, error) {
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint: status %d: %s", resp.StatusCode, b)
	}
	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("token decode: %w", err)
	}
	expiry := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	if tr.ExpiresIn == 0 {
		expiry = time.Now().Add(3600 * time.Second) // sensible default
	}
	return &Tokens{
		ClientID:     clientID,
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    expiry,
	}, nil
}

// EnsureValid returns a valid token, refreshing if necessary.
// Callers should SaveTokens after this if the returned token differs.
func EnsureValid(t *Tokens) (*Tokens, bool, error) {
	if !t.IsExpired() {
		return t, false, nil
	}
	if t.RefreshToken == "" {
		return nil, false, fmt.Errorf("access token expired and no refresh token — run `arvo auth login`")
	}
	refreshed, err := Refresh(t)
	if err != nil {
		return nil, false, fmt.Errorf("token refresh failed — run `arvo auth login`: %w", err)
	}
	return refreshed, true, nil
}

// pkce generates a code_verifier and S256 code_challenge.
func pkce() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("pkce rand: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomState() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "linux":
		return exec.Command("xdg-open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
