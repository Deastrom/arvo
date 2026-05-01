# arvo — Slim Output Plan

## Problem

MCP responses from Atlassian are enormous. A single `arvo jira get EA-123` can
return hundreds of KB of nested JSON — full issue schema, all fields, complete
comment history with author metadata, renderedBody, schema noise, etc.

The primary consumer is an AI agent. Every byte of output costs context window.
The current default (MCP text content as-is) is unusable at scale.

## Goal

Aggressive default truncation. The agent sees only what it needs to decide what
to do next. Full data available on demand via flags.

## Prior Art

| Tool | Default | Expand |
|------|---------|--------|
| `gh issue view` | Curated text summary | `--json field,...`, `--comments` |
| `kubectl get` | Table | `-o json`, `-o jsonpath=...` |
| `glab issue view` | Curated text | `--json`, `--comments` |
| **arvo (proposed)** | Curated text | `--fields`, `--comments`, `--description`, `--body`, `--full`, `--json [fields]`, `--raw` |

## Output Modes

### `arvo jira get EA-123` (default)

```
EA-123  Story  In Progress  High
"Implement dark mode toggle"
Assignee:    ben.cedar
Sprint:      Sprint 24
Labels:      frontend, ui
Parent:      EA-100
Links:       blocks EA-130 · blocked by EA-99

Description (500 chars):
  As a user I want to toggle dark mode so that I can use the app
  comfortably at night...

Comments: 3 (latest: 2026-04-28 by jane.doe)
```

~300 bytes. Agent decides: do I need more? It asks.

### `arvo jira search "project = EA AND status = 'In Progress'"` (default)

```
KEY      TYPE   STATUS       PRI   ASSIGNEE    SUMMARY
EA-123   Story  In Progress  High  ben.cedar   Implement dark mode toggle
EA-124   Bug    In Progress  Med   jane.doe    Login timeout on Safari
2 issues
```

### `arvo confluence get <id>` (default)

```
"Engineering Runbook"  (Page 12345)
Space: EA  Status: current  Version: 14
Last modified: 2026-04-20 by ben.cedar

Body (1000 chars):
  ## On-call process
  When an alert fires...

Comments: 2
```

## Flags

All subcommands that return content support:

| Flag | Effect |
|------|--------|
| `--fields key,status,assignee` | Print only named fields (comma-separated) |
| `--comments` | Append last 10 comments (author + date + body, no metadata) |
| `--comments-limit N` | Change comment limit (default 10) |
| `--description` | Full untruncated description |
| `--body` | Full untruncated page body (confluence) |
| `--full` | All of the above combined |
| `--json` | Curated summary as JSON |
| `--json key,status,description` | Named fields as JSON (like `gh --json`) |
| `--raw` | Full MCP response, unmodified — for debugging |

`--raw` replaces current `--json` behaviour. `--json` becomes the curated JSON
summary. Agents that need raw can use `arvo call <tool> <args>` instead.

## Implementation

### New package: `internal/format`

Typed Go structs per entity. The full MCP response is parsed into these — raw
JSON never reaches stdout unless `--raw`.

```go
// IssueSummary is the curated view of a Jira issue.
type IssueSummary struct {
    Key         string
    IssueType   string
    Status      string
    Priority    string
    Summary     string
    Assignee    string
    Sprint      string
    StoryPoints float64
    Labels      []string
    Parent      string
    Links       []IssueLink
    DescSnip    string // first 500 chars of description
    CommentCount int
    LatestComment *CommentSnip
}

// IssueDetail extends IssueSummary with full fields for --full/--description/--comments.
type IssueDetail struct {
    IssueSummary
    Description string
    Comments    []Comment
}

type Comment struct {
    Author string
    Date   string
    Body   string // plain text, no HTML/schema noise
}

type IssueLink struct {
    Direction string // "blocks", "blocked by", "relates to"
    Key       string
}
```

Similar structs for `PageSummary`, `PageDetail`, `SearchResult`.

### Extraction

Each command handler calls a `format.ParseIssue(raw json.RawMessage) (*IssueSummary, error)`
function that extracts the curated fields from the MCP response JSON. No struct
field survives extraction unless it was explicitly mapped.

### `--fields` implementation

Parse comma-separated field names. Use reflection or a field map to print only
those keys. Start simple: just filter the output lines, not the extraction.

### `--json [fields]` implementation

If `--json` has no argument: marshal the curated struct. If it has a field list:
marshal only those keys (map[string]any subset).

### Breaking change: `--json` → `--raw`

Current `--json` emits full MCP JSON. This becomes `--raw`. `--json` will now
emit the curated summary as JSON. This is a breaking change for any scripts
using `--json` today.

Migration: if any agent skills reference `arvo --json`, update them to `arvo --raw`.

## Truncation Defaults

| Field | Default limit | Flag to expand |
|-------|--------------|----------------|
| Description | 500 chars | `--description` |
| Confluence body | 1000 chars | `--body` |
| Comments shown | 10 most recent | `--comments-limit N` |
| Comment body | 300 chars each | `--full` |
| Search results | 50 rows (MCP default) | `--limit N` already on search |

## Files Changed

```
cmd/jira.go        — add flags, call format.ParseIssue / format.PrintIssue
cmd/confluence.go  — add flags, call format.ParsePage / format.PrintPage
internal/
  format/
    issue.go       — IssueSummary, IssueDetail, ParseIssue, PrintIssue
    page.go        — PageSummary, PageDetail, ParsePage, PrintPage
    search.go      — SearchResult, ParseSearch, PrintSearch
    fields.go      — --fields flag parsing and filtering
```

`internal/output/format.go` remains for generic KV/Table/Print helpers used
elsewhere. The new `internal/format/` package is Jira/Confluence-specific.

## Out of Scope

- `arvo call` — raw escape hatch, always full output
- `arvo tools` — already compact
- `arvo auth *` — no payload to truncate
