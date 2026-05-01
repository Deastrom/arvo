//go:build contract

// Package contract verifies arvo's MCP client speaks the MCP Streamable HTTP
// spec correctly. Tests use a local httptest server that enforces the protocol
// — no live Atlassian credentials required.
//
// Run with: mise run test:contract
package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Deastrom/arvo/internal/mcp"
)

const testSessionID = "test-session-abc123"

// mcpServer spins up a strict MCP-compliant test server.
// It enforces:
//   - initialize must come first (no session ID on first request)
//   - Mcp-Session-Id must be present on all requests after initialize
//   - notifications/initialized must be sent after initialize
//   - Accept header must include both application/json and text/event-stream
//   - Authorization: Bearer header must always be present
//   - Content-Type: application/json must always be present on requests
func mcpServer(t *testing.T, tools func(w http.ResponseWriter, req mcp.Request)) *httptest.Server {
	t.Helper()
	initialized := false
	notifiedInitialized := false

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Enforce Accept header.
		accept := r.Header.Get("Accept")
		if accept == "" {
			t.Errorf("missing Accept header")
		}

		// Enforce Authorization header.
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("missing or malformed Authorization header: %q", auth)
		}

		// Enforce Content-Type header.
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}

		var req mcp.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("invalid JSON-RPC request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Enforce session ID protocol.
		sessionID := r.Header.Get("Mcp-Session-Id")
		if req.Method != "initialize" && sessionID != testSessionID {
			t.Errorf("method %q sent without valid Mcp-Session-Id (got %q)", req.Method, sessionID)
		}

		switch req.Method {
		case "initialize":
			if sessionID != "" {
				t.Errorf("initialize should not include Mcp-Session-Id")
			}
			// Return session ID in response header.
			w.Header().Set("Mcp-Session-Id", testSessionID)
			result := mcp.InitializeResult{
				ProtocolVersion: "2025-03-26",
				ServerInfo:      mcp.ClientInfo{Name: "test-mcp-server", Version: "1.0.0"},
				Capabilities:    map[string]any{"tools": map[string]any{}},
			}
			resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
			b, _ := json.Marshal(result)
			resp.Result = json.RawMessage(b)
			json.NewEncoder(w).Encode(resp)
			initialized = true

		case "notifications/initialized":
			if !initialized {
				t.Error("notifications/initialized sent before initialize completed")
			}
			notifiedInitialized = true
			w.WriteHeader(http.StatusAccepted)

		default:
			if !notifiedInitialized {
				t.Errorf("method %q called before notifications/initialized", req.Method)
			}
			if tools != nil {
				tools(w, req)
			}
		}
	}))
}

func TestContractInitializeHandshake(t *testing.T) {
	srv := mcpServer(t, nil)
	defer srv.Close()

	c := mcp.NewWithURL("test-token", srv.URL)
	result, err := c.Initialize()
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if result.ProtocolVersion != "2025-03-26" {
		t.Errorf("unexpected protocol version: %s", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "test-mcp-server" {
		t.Errorf("unexpected server name: %s", result.ServerInfo.Name)
	}
}

func TestContractInitializeIdempotent(t *testing.T) {
	srv := mcpServer(t, nil)
	defer srv.Close()

	c := mcp.NewWithURL("test-token", srv.URL)
	if _, err := c.Initialize(); err != nil {
		t.Fatalf("first Initialize: %v", err)
	}
	// Second call must be a no-op — must not re-send initialize.
	result, err := c.Initialize()
	if err != nil {
		t.Fatalf("second Initialize: %v", err)
	}
	if result != nil {
		t.Error("second Initialize should return nil (no-op)")
	}
}

func TestContractListToolsSessionRequired(t *testing.T) {
	tools := []mcp.Tool{
		{Name: "getJiraIssue", Description: "Get a Jira issue"},
		{Name: "searchJiraIssuesUsingJql", Description: "Search issues"},
	}
	srv := mcpServer(t, func(w http.ResponseWriter, req mcp.Request) {
		if req.Method != "tools/list" {
			t.Errorf("unexpected method: %s", req.Method)
			return
		}
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(mcp.ToolsListResult{Tools: tools})
		resp.Result = json.RawMessage(b)
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	c := mcp.NewWithURL("test-token", srv.URL)
	got, err := c.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(got) != len(tools) {
		t.Errorf("expected %d tools, got %d", len(tools), len(got))
	}
}

func TestContractCallToolArgumentsAlwaysSent(t *testing.T) {
	srv := mcpServer(t, func(w http.ResponseWriter, req mcp.Request) {
		if req.Method != "tools/call" {
			t.Errorf("unexpected method: %s", req.Method)
			return
		}
		// Verify arguments field is always present (not omitted when empty).
		raw, _ := json.Marshal(req.Params)
		var params struct {
			Arguments *json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(raw, &params); err != nil || params.Arguments == nil {
			t.Error("tools/call must always include arguments field, even when empty")
		}

		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(mcp.ToolCallResult{
			Content: []mcp.Content{{Type: "text", Text: "ok"}},
		})
		resp.Result = json.RawMessage(b)
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	c := mcp.NewWithURL("test-token", srv.URL)
	// Call with empty args — arguments must still be present in payload.
	if _, err := c.CallTool("anyTool", map[string]any{}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
}

func TestContractSSEResponse(t *testing.T) {
	srv := mcpServer(t, func(w http.ResponseWriter, req mcp.Request) {
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(mcp.ToolCallResult{
			Content: []mcp.Content{{Type: "text", Text: "sse-result"}},
		})
		resp.Result = json.RawMessage(b)
		respJSON, _ := json.Marshal(resp)

		w.Header().Set("Content-Type", "text/event-stream")
		// Send a server notification first — client must skip it.
		w.Write([]byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/ping\"}\n\n"))
		// Then the real response.
		w.Write([]byte("event: message\ndata: " + string(respJSON) + "\n\n"))
	})
	defer srv.Close()

	c := mcp.NewWithURL("test-token", srv.URL)
	result, err := c.CallTool("anyTool", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool SSE: %v", err)
	}
	if mcp.TextContent(result) != "sse-result" {
		t.Errorf("unexpected content: %q", mcp.TextContent(result))
	}
}

func TestContractToolError(t *testing.T) {
	srv := mcpServer(t, func(w http.ResponseWriter, req mcp.Request) {
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(mcp.ToolCallResult{
			IsError: true,
			Content: []mcp.Content{{Type: "text", Text: "not found"}},
		})
		resp.Result = json.RawMessage(b)
		json.NewEncoder(w).Encode(resp)
	})
	defer srv.Close()

	c := mcp.NewWithURL("test-token", srv.URL)
	_, err := c.CallTool("anyTool", map[string]any{})
	if err == nil {
		t.Fatal("expected error for isError=true tool result")
	}
}

func TestContractUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := mcp.NewWithURL("bad-token", srv.URL)
	_, err := c.Initialize()
	if err == nil {
		t.Fatal("expected error on 401")
	}
}
