# arvo

CLI tool that bridges AI coding agents to Atlassian (Jira + Confluence) via the Atlassian MCP, without requiring the MCP to be loaded into agent context.

## Problem

The Atlassian MCP works well but consumes significant context window when loaded. AI agents already interact effectively with CLI tools (like `glab` for GitLab) via bash. `arvo` acts as a CLI for agents (and humans) that proxies to the Atlassian MCP under the hood.

## Architecture

```
Agent (Claude Code / OpenCode)
  │ bash
  ▼
arvo (Go binary)
  │ HTTP/SSE + OAuth 2.1
  ▼
mcp.atlassian.com/v1/mcp
```

## OAuth Flow

Atlassian's MCP exposes standard OAuth 2.1 discovery:

- **AS Metadata**: `https://mcp.atlassian.com/.well-known/oauth-authorization-server`
- **Authorization**: `https://mcp.atlassian.com/v1/authorize`
- **Token**: `https://cf.mcp.atlassian.com/v1/token`
- **Dynamic Registration**: `https://cf.mcp.atlassian.com/v1/register` (RFC 7591)
- **Grant types**: `authorization_code`, `refresh_token`
- **Public clients**: Supported (`"none"` auth method)
- **PKCE**: S256

### First-run auth sequence

1. POST to registration endpoint with client metadata -> get `client_id` (cache it)
2. Generate PKCE code_verifier + code_challenge (S256)
3. Open browser to authorization endpoint
4. Listen on localhost callback for auth code
5. Exchange code at token endpoint -> `access_token` + `refresh_token`
6. Cache tokens at `~/.config/arvo/tokens.json` (0600)

### Subsequent runs

1. Load cached tokens
2. If access token expired, use refresh token silently
3. If refresh token expired, print "run `arvo auth login`" and exit 1

## MCP Protocol

Transport: Streamable HTTP (POST JSON-RPC to `https://mcp.atlassian.com/v1/mcp`)

Each CLI invocation:
1. POST `initialize` request (with session reuse if cached session ID)
2. POST `tools/call` with tool name + params
3. Parse response, output as structured text (or JSON with `--json`)
4. Clean up session

## CLI Surface

```
arvo auth login              # OAuth flow, cache tokens
arvo auth status             # Show auth state
arvo auth logout             # Clear cached tokens

arvo call <tool> [json]      # Raw MCP tool call (escape hatch)
arvo tools                   # List available MCP tools

arvo jira get <key>          # Get issue
arvo jira search <jql>       # Search issues
arvo jira create             # Create issue (flags: --project, --type, --summary, --description)
arvo jira transition <key>   # Transition issue
arvo jira comment <key>      # Add comment

arvo confluence get <id>     # Get page
arvo confluence search <cql> # Search pages
```

### Output modes

- Default: human/agent-readable text (summaries, tables)
- `--json`: raw JSON from MCP response
- Errors to stderr, data to stdout

## Language & Dependencies

- **Go** (single binary, no runtime)
- `golang.org/x/oauth2` or raw HTTP (the flow is simple enough)
- No MCP SDK needed - it's just JSON-RPC over HTTP POST

## File Structure

```
arvo/
  cmd/
    root.go          # cobra root command
    auth.go          # auth login/status/logout
    call.go          # raw tool call
    tools.go         # list tools
    jira.go          # jira subcommands
    confluence.go    # confluence subcommands
  internal/
    auth/
      oauth.go       # OAuth flow (register, authorize, token, refresh)
      store.go       # Token/client storage (~/.config/arvo/)
    mcp/
      client.go      # MCP client (initialize, call tool, parse response)
      jsonrpc.go     # JSON-RPC types
    output/
      format.go      # Text/JSON output formatting
  main.go
  go.mod
  go.sum
```

## Config

`~/.config/arvo/config.json` (optional):
```json
{
  "mcp_url": "https://mcp.atlassian.com/v1/mcp",
  "default_output": "text"
}
```

`~/.config/arvo/tokens.json` (managed by arvo):
```json
{
  "client_id": "...",
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": "..."
}
```

## Skill Integration

Once built, create a Claude Code / OpenCode skill that references `arvo`:

```markdown
# Skill: atlassian

Use `arvo` to interact with Jira and Confluence.

## Commands
- `arvo jira search "project = FOO AND status = Open"` - search issues
- `arvo jira get FOO-123` - get issue details
- `arvo confluence search "title ~ 'meeting'"` - search pages
...
```

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Dynamic registration blocked by Atlassian | Fall back to manual client ID config |
| Token refresh fails silently | Clear error message: "run `arvo auth login`" |
| MCP protocol version changes | Pin `MCP-Protocol-Version` header, handle gracefully |
| Session management overhead per invocation | Cache session ID, reuse across calls within TTL |

## License

MIT
