package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	mcpURL          = "https://mcp.atlassian.com/v1/mcp"
	protocolVersion = "2025-03-26"

	// supportedProtocols lists the protocol versions this client understands.
	// The server's version must match one of these.
	supportedProtocols = "2025-03-26"
)

// Version is injected at build time via -ldflags. Defaults to "dev".
var Version = "dev"

// Client makes JSON-RPC calls to the Atlassian MCP endpoint.
// It is safe for concurrent use after initialization.
type Client struct {
	httpClient  *http.Client
	accessToken string
	baseURL     string

	// idSeq is incremented atomically; each call gets a unique ID.
	idSeq atomic.Int64

	mu          sync.Mutex
	sessionID   string // Mcp-Session-Id returned by initialize
	initialized bool   // true after initialize + notifications/initialized sent
}

// New creates a new MCP client with the given access token.
func New(accessToken string) *Client {
	return newClient(accessToken, mcpURL)
}

// NewWithURL creates a client with a custom base URL (for testing).
func NewWithURL(accessToken, baseURL string) *Client {
	return newClient(accessToken, baseURL)
}

func newClient(accessToken, baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		accessToken: accessToken,
		baseURL:     baseURL,
	}
}

func (c *Client) nextID() int64 {
	return c.idSeq.Add(1)
}

func (c *Client) getSessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

func (c *Client) setSessionID(sid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = sid
}

// do executes a JSON-RPC request and returns the response.
// For notifications (req.ID == nil) a nil response is returned on success.
// It retries once on 429/503 with a short backoff.
func (c *Client) do(req Request) (*Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	var (
		rpcResp *Response
		lastErr error
	)
	for attempt := range 2 {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		rpcResp, lastErr = c.doOnce(body, req.ID)
		if lastErr == nil {
			return rpcResp, nil
		}
		// Only retry on retryable sentinel errors wrapped by doOnce.
		if !isRetryable(lastErr) {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

// retryableSentinel marks an error as safe to retry.
type retryableSentinel struct{ err error }

func (e *retryableSentinel) Error() string { return e.err.Error() }
func (e *retryableSentinel) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var r *retryableSentinel
	return err != nil && func() bool {
		e := err
		for e != nil {
			if _, ok := e.(*retryableSentinel); ok {
				return true
			}
			_ = r
			type unwrapper interface{ Unwrap() error }
			if u, ok := e.(unwrapper); ok {
				e = u.Unwrap()
			} else {
				break
			}
		}
		return false
	}()
}

// doOnce performs a single HTTP round-trip and returns a *Response or error.
// It returns *retryableSentinel for 429/503 so the caller can retry.
func (c *Client) doOnce(body []byte, reqID any) (*Response, error) {
	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	if sid := c.getSessionID(); sid != "" {
		httpReq.Header.Set("Mcp-Session-Id", sid)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	// Capture session ID from any response.
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.setSessionID(sid)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorized — run `arvo auth login`")
	case http.StatusAccepted:
		// Valid response for notifications (no body).
		return nil, nil
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		b, _ := io.ReadAll(resp.Body)
		return nil, &retryableSentinel{fmt.Errorf("mcp http %d: %s", resp.StatusCode, b)}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mcp http %d: %s", resp.StatusCode, b)
	}

	// Route to SSE or JSON parser based on Content-Type.
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		return parseSSE(resp.Body, reqID)
	}

	var rpcResp Response
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return &rpcResp, nil
}

// parseSSE reads a Server-Sent Events stream and returns the JSON-RPC response
// whose ID matches requestID. Server-initiated notifications are skipped.
func parseSSE(r io.Reader, requestID any) (*Response, error) {
	scanner := bufio.NewScanner(r)
	// Default 64KB buffer is too small for large MCP responses (e.g. Jira issues
	// with many comments). Set a 10MB max token size.
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	var (
		eventType string
		dataLines []string
	)

	flush := func() (*Response, error) {
		if len(dataLines) == 0 {
			return nil, nil
		}
		// Only process "message" events (the default event type).
		if eventType != "" && eventType != "message" {
			return nil, nil
		}
		// Per SSE spec, join multi-line data with U+000A.
		data := strings.Join(dataLines, "\n")
		if data == "" || data == "[DONE]" {
			return nil, nil
		}
		var rpcResp Response
		if err := json.Unmarshal([]byte(data), &rpcResp); err != nil {
			// Surface JSON errors rather than silently skipping.
			return nil, fmt.Errorf("sse: malformed JSON in data field: %w", err)
		}
		// Skip server-initiated notifications (no ID) or mismatched IDs.
		if requestID != nil {
			if rpcResp.ID == nil {
				return nil, nil
			}
			// Compare as JSON to handle string/number ID types.
			wantB, _ := json.Marshal(requestID)
			gotB, _ := json.Marshal(rpcResp.ID)
			if string(wantB) != string(gotB) {
				return nil, nil
			}
		}
		if rpcResp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
		}
		return &rpcResp, nil
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Blank line = dispatch the event.
		if line == "" {
			resp, err := flush()
			eventType = ""
			dataLines = nil
			if err != nil {
				return nil, err
			}
			if resp != nil {
				return resp, nil
			}
			continue
		}

		if strings.HasPrefix(line, "event:") {
			// TrimSpace is correct for the event field (no data payload concern).
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			// Per SSE spec §9.2.6: strip exactly one leading space if present.
			d := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(d, " ") {
				d = d[1:]
			}
			dataLines = append(dataLines, d)
		}
		// id: and retry: fields are intentionally ignored.
	}

	// Flush any trailing event without a trailing blank line.
	resp, err := flush()
	if err != nil {
		return nil, err
	}
	if resp != nil {
		return resp, nil
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("sse read: %w", err)
	}
	return nil, fmt.Errorf("no matching JSON-RPC response found in SSE stream")
}

// Initialize sends the MCP initialize handshake followed by
// notifications/initialized, as required by the spec.
// Safe to call multiple times — subsequent calls are no-ops.
func (c *Client) Initialize() (*InitializeResult, error) {
	c.mu.Lock()
	alreadyDone := c.initialized
	c.mu.Unlock()
	if alreadyDone {
		return nil, nil
	}

	id := c.nextID()
	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "initialize",
		Params: InitializeParams{
			ProtocolVersion: protocolVersion,
			ClientInfo:      ClientInfo{Name: "arvo", Version: Version},
			Capabilities:    map[string]any{},
		},
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("initialize: server returned 202 for a request (expected 200)")
	}
	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse initialize result: %w", err)
	}

	// Validate protocol version compatibility.
	if result.ProtocolVersion != supportedProtocols {
		return nil, fmt.Errorf("unsupported MCP protocol version %q (client supports %q)", result.ProtocolVersion, supportedProtocols)
	}

	// Send notifications/initialized (no ID — it's a notification).
	notif := Request{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if _, err := c.do(notif); err != nil {
		return nil, fmt.Errorf("send initialized notification: %w", err)
	}

	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()

	return &result, nil
}

// ListTools returns the list of tools available from the MCP server,
// following pagination cursors until all pages are fetched.
func (c *Client) ListTools() ([]Tool, error) {
	if _, err := c.Initialize(); err != nil {
		return nil, err
	}

	var all []Tool
	var cursor string
	for {
		id := c.nextID()
		var params any
		if cursor != "" {
			params = map[string]any{"cursor": cursor}
		}
		req := Request{
			JSONRPC: "2.0",
			ID:      id,
			Method:  "tools/list",
			Params:  params,
		}
		resp, err := c.do(req)
		if err != nil {
			return nil, err
		}
		var result ToolsListResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return nil, fmt.Errorf("parse tools/list result: %w", err)
		}
		all = append(all, result.Tools...)
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return all, nil
}

// CallTool sends a tools/call request and returns the result.
func (c *Client) CallTool(name string, args map[string]any) (*ToolCallResult, error) {
	if _, err := c.Initialize(); err != nil {
		return nil, err
	}
	id := c.nextID()
	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params: ToolCallParams{
			Name:      name,
			Arguments: args,
		},
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/call result: %w", err)
	}
	if result.IsError {
		text := ""
		for _, c := range result.Content {
			text += c.Text
		}
		return nil, fmt.Errorf("tool error: %s", text)
	}
	return &result, nil
}

// TextContent extracts the concatenated text from a tool call result.
func TextContent(r *ToolCallResult) string {
	var sb bytes.Buffer
	for _, c := range r.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}
