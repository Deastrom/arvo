# arvo

CLI tool that bridges AI coding agents to Atlassian (Jira + Confluence) via the Atlassian MCP, without requiring the MCP to be loaded into agent context.

## Problem

The Atlassian MCP works well but consumes significant context window when loaded into an AI agent. AI agents already interact effectively with CLI tools (like `glab` for GitLab) via bash. `arvo` acts as a thin CLI that proxies to the Atlassian MCP — keeping the agent's context lean.

## Architecture

```
Agent (Claude Code / OpenCode)
  │ bash
  ▼
arvo (Go binary)
  │ MCP Streamable HTTP + OAuth 2.1
  ▼
mcp.atlassian.com/v1/mcp
```

## Installation

### From release

Download the binary for your platform from [Releases](https://github.com/Deastrom/arvo/releases) and place it on your `$PATH`.

### From source

```bash
go install github.com/Deastrom/arvo@latest
```

### With mise

```bash
mise use github:Deastrom/arvo
```

## Setup

```bash
arvo auth login     # OAuth 2.1 browser flow — picks your default Atlassian site
arvo auth status    # Verify credentials
```

## Usage

```bash
# Jira
arvo jira get EA-123
arvo jira search "project = EA AND status = 'In Progress' ORDER BY updated DESC"
arvo jira create --project EA --type Task --summary "Fix the thing" --description "..."
arvo jira comment EA-123 --body "Investigated — root cause is X"
arvo jira transition EA-123 --to <transition-id>

# Confluence
arvo confluence get <page-id>
arvo confluence search "space = ENG AND title ~ 'runbook'"

# Raw MCP escape hatch
arvo tools                              # list all available MCP tools
arvo call <tool-name> '{"key":"val"}'   # call any tool directly

# Flags
arvo --json jira get EA-123             # output raw JSON
arvo --cloud <cloud-id> jira get EA-123 # override default site
```

## Agent skill

Once installed, add this to your agent's skill config:

````markdown
# Skill: atlassian

Use `arvo` to interact with Jira and Confluence.

## Commands
- `arvo jira get EA-123` — get issue details
- `arvo jira search "project = EA AND assignee = currentUser()"` — search with JQL
- `arvo jira create --project EA --type Task --summary "..."` — create issue
- `arvo jira comment EA-123 --body "..."` — add comment
- `arvo confluence get <page-id>` — get page content
- `arvo confluence search "title ~ 'meeting'"` — search pages
- `arvo --json <cmd>` — get raw JSON output
- `arvo call <tool> '{}'` — call any MCP tool directly
````

## Development

```bash
mise install          # install Go + convco
mise run build        # build ./arvo
mise run test:unit    # unit tests
mise run test:contract # MCP protocol contract tests
mise run lint         # go vet
mise run release      # cross-compile all platforms → dist/
mise run changelog    # preview changelog
```

## Auth details

- OAuth 2.1 with PKCE (S256) via dynamic client registration
- Tokens cached at `~/.config/arvo/tokens.json` (0600)
- Access token auto-refreshed on expiry
- Default site stored at `~/.config/arvo/config.json`

## License

MIT
