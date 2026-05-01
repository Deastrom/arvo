package mcp_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Deastrom/arvo/internal/mcp"
)

func makeServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *mcp.Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := mcp.NewWithURL("test-token", srv.URL)
	return srv, client
}

// standardHandler handles initialize + notifications/initialized for any test,
// then delegates to extraHandler for subsequent methods.
func standardHandler(t *testing.T, extra func(w http.ResponseWriter, req mcp.Request)) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch req.Method {
		case "initialize":
			result := mcp.InitializeResult{
				ProtocolVersion: "2025-03-26",
				ServerInfo:      mcp.ClientInfo{Name: "atlassian-mcp", Version: "1.0"},
			}
			resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
			b, _ := json.Marshal(result)
			resp.Result = json.RawMessage(b)
			json.NewEncoder(w).Encode(resp)

		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)

		default:
			if extra != nil {
				extra(w, req)
			}
		}
	}
}

func TestInitialize(t *testing.T) {
	_, client := makeServer(t, standardHandler(t, nil))

	res, err := client.Initialize()
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if res.ServerInfo.Name != "atlassian-mcp" {
		t.Errorf("unexpected server name: %s", res.ServerInfo.Name)
	}

	res2, err := client.Initialize()
	if err != nil {
		t.Fatalf("second Initialize: %v", err)
	}
	if res2 != nil {
		t.Error("expected nil on second Initialize call")
	}
}

func TestListTools(t *testing.T) {
	tools := []mcp.Tool{
		{Name: "getJiraIssue", Description: "Get a Jira issue"},
		{Name: "searchJiraIssuesUsingJql", Description: "Search issues"},
	}

	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		if req.Method != "tools/list" {
			t.Errorf("unexpected method: %s", req.Method)
			return
		}
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(mcp.ToolsListResult{Tools: tools})
		resp.Result = json.RawMessage(b)
		json.NewEncoder(w).Encode(resp)
	}))

	got, err := client.ListTools()
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(got) != len(tools) {
		t.Errorf("expected %d tools, got %d", len(tools), len(got))
	}
	if got[0].Name != "getJiraIssue" {
		t.Errorf("unexpected first tool: %s", got[0].Name)
	}
}

func TestListToolsPaginated(t *testing.T) {
	page1 := []mcp.Tool{{Name: "tool1"}}
	page2 := []mcp.Tool{{Name: "tool2"}, {Name: "tool3"}}
	calls := 0

	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		calls++
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		var result mcp.ToolsListResult
		if calls == 1 {
			result = mcp.ToolsListResult{Tools: page1, NextCursor: "cursor-2"}
		} else {
			result = mcp.ToolsListResult{Tools: page2}
		}
		b, _ := json.Marshal(result)
		resp.Result = json.RawMessage(b)
		json.NewEncoder(w).Encode(resp)
	}))

	got, err := client.ListTools()
	if err != nil {
		t.Fatalf("ListTools paginated: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 tools across 2 pages, got %d", len(got))
	}
	if calls != 2 {
		t.Errorf("expected 2 tools/list calls, got %d", calls)
	}
}

func TestCallTool(t *testing.T) {
	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		if req.Method != "tools/call" {
			t.Errorf("unexpected method: %s", req.Method)
			return
		}
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(mcp.ToolCallResult{
			Content: []mcp.Content{{Type: "text", Text: "PROJ-1: Fix the bug"}},
		})
		resp.Result = json.RawMessage(b)
		json.NewEncoder(w).Encode(resp)
	}))

	res, err := client.CallTool("getJiraIssue", map[string]any{"issueIdOrKey": "PROJ-1"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if mcp.TextContent(res) != "PROJ-1: Fix the bug" {
		t.Errorf("unexpected text: %q", mcp.TextContent(res))
	}
}

func TestCallToolError(t *testing.T) {
	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(mcp.ToolCallResult{
			IsError: true,
			Content: []mcp.Content{{Type: "text", Text: "issue not found"}},
		})
		resp.Result = json.RawMessage(b)
		json.NewEncoder(w).Encode(resp)
	}))

	_, err := client.CallTool("getJiraIssue", map[string]any{"issueIdOrKey": "NOPE-999"})
	if err == nil {
		t.Fatal("expected error for isError=true result")
	}
}

func TestUnauthorized(t *testing.T) {
	_, client := makeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := client.Initialize()
	if err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestSSEResponse(t *testing.T) {
	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		result := mcp.ToolCallResult{
			Content: []mcp.Content{{Type: "text", Text: "SSE result"}},
		}
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(result)
		resp.Result = json.RawMessage(b)
		respJSON, _ := json.Marshal(resp)

		w.Header().Set("Content-Type", "text/event-stream")
		notif := `{"jsonrpc":"2.0","method":"notifications/something"}`
		w.Write([]byte("event: message\ndata: " + notif + "\n\n"))
		w.Write([]byte("event: message\ndata: " + string(respJSON) + "\n\n"))
	}))

	res, err := client.CallTool("anyTool", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool SSE: %v", err)
	}
	if mcp.TextContent(res) != "SSE result" {
		t.Errorf("unexpected text: %q", mcp.TextContent(res))
	}
}

func TestSSENonMessageEventSkipped(t *testing.T) {
	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		result := mcp.ToolCallResult{Content: []mcp.Content{{Type: "text", Text: "ok"}}}
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(result)
		resp.Result = json.RawMessage(b)
		wrappedJSON, _ := json.Marshal(resp)

		w.Header().Set("Content-Type", "text/event-stream")
		// Non-message event should be skipped.
		w.Write([]byte("event: ping\ndata: keepalive\n\n"))
		w.Write([]byte("data: " + string(wrappedJSON) + "\n\n"))
	}))

	res, err := client.CallTool("anyTool", map[string]any{})
	if err != nil {
		t.Fatalf("SSE non-message event: %v", err)
	}
	if mcp.TextContent(res) != "ok" {
		t.Errorf("unexpected text: %q", mcp.TextContent(res))
	}
}

func TestSSEDoneTerminator(t *testing.T) {
	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		result := mcp.ToolCallResult{Content: []mcp.Content{{Type: "text", Text: "done-test"}}}
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(result)
		resp.Result = json.RawMessage(b)
		wrappedJSON, _ := json.Marshal(resp)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: " + string(wrappedJSON) + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))

	res, err := client.CallTool("anyTool", map[string]any{})
	if err != nil {
		t.Fatalf("SSE DONE: %v", err)
	}
	if mcp.TextContent(res) != "done-test" {
		t.Errorf("unexpected text: %q", mcp.TextContent(res))
	}
}

func TestSSEMalformedJSON(t *testing.T) {
	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(fmt.Sprintf("data: {not valid json, id: %v}\n\n", req.ID)))
	}))

	_, err := client.CallTool("anyTool", map[string]any{})
	if err == nil {
		t.Fatal("expected error for malformed JSON in SSE data")
	}
}

func TestRetryOn503(t *testing.T) {
	attempts := 0
	_, client := makeServer(t, standardHandler(t, func(w http.ResponseWriter, req mcp.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		resp := mcp.Response{JSONRPC: "2.0", ID: req.ID}
		b, _ := json.Marshal(mcp.ToolCallResult{Content: []mcp.Content{{Type: "text", Text: "ok"}}})
		resp.Result = json.RawMessage(b)
		json.NewEncoder(w).Encode(resp)
	}))

	res, err := client.CallTool("anyTool", map[string]any{})
	if err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if mcp.TextContent(res) != "ok" {
		t.Errorf("unexpected text: %q", mcp.TextContent(res))
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}
