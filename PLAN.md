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
| **arvo (proposed)** | Curated text | `--comments`, `--full`, `--json`, `--raw` |

## Output Modes

### `arvo jira get EA-123` (default)

```
EA-123  Story  In Progress  High
"Implement dark mode toggle"
Assignee:    ben.cedar
Reporter:    jane.doe
Created:     2026-03-01  Updated: 2026-04-28
Sprint:      Sprint 24
Labels:      frontend, ui
Parent:      EA-100
Links:       blocks EA-130 · blocked by EA-99
URL:         https://myorg.atlassian.net/browse/EA-123

Description (500 chars):
  As a user I want to toggle dark mode so that I can use the app
  comfortably at night...

Comments: 3 (latest: 2026-04-28 by jane.doe)
```

### `arvo jira get EA-123 --comments`

Appends full comment bodies (no truncation — if you asked for comments, you need them):

```
--- Comments (3) ---
[1] jane.doe  2026-04-20
  Investigated — root cause is the CSS variable not being applied to modals.

[2] ben.cedar  2026-04-28
  Fixed in commit abc123. Needs QA.
```

### `arvo jira get EA-123 --full`

Description untruncated + comments. Equivalent to `--comments` + full description.

### `arvo jira get EA-123 --json`

Curated summary as JSON (same fields as default text output). Structured for
agent consumption without the noise of the raw MCP response:

```json
{
  "key": "EA-123",
  "type": "Story",
  "status": "In Progress",
  "priority": "High",
  "summary": "Implement dark mode toggle",
  "assignee": "ben.cedar",
  "reporter": "jane.doe",
  "created": "2026-03-01",
  "updated": "2026-04-28",
  "labels": ["frontend", "ui"],
  "parent": "EA-100",
  "links": ["blocks EA-130", "blocked by EA-99"],
  "url": "https://myorg.atlassian.net/browse/EA-123",
  "description_snip": "As a user I want to toggle dark mode...",
  "comment_count": 3,
  "latest_comment": {"author": "jane.doe", "date": "2026-04-28", "body": "Fixed in commit abc123..."}
}
```

### `arvo jira get EA-123 --raw`

Full MCP response, unmodified. Current `--json` behaviour. For debugging or
when the agent needs fields not in the curated summary.

### `arvo jira search "project = EA AND status = 'In Progress'"` (default)

```
KEY      TYPE   STATUS       PRI   ASSIGNEE    SUMMARY
EA-123   Story  In Progress  High  ben.cedar   Implement dark mode toggle
EA-124   Bug    In Progress  Med   jane.doe    Login timeout on Safari
2 issues (limit 25 — use --limit N or refine JQL for more)
```

Default limit: 25. `--limit N` to change.

### `arvo confluence get <id>` (default)

```
"Engineering Runbook"  (Page 12345)
Space: EA  Status: current  Version: 14
Last modified: 2026-04-20 by ben.cedar
URL: https://myorg.atlassian.net/wiki/spaces/EA/pages/12345

Body (1000 chars):
  ## On-call process
  When an alert fires...

Comments: 2
```

### `arvo confluence search <cql>` (default)

```
ID       SPACE  TITLE                    LAST MODIFIED
12345    EA     Engineering Runbook      2026-04-20 by ben.cedar
12346    EA     Incident Response Guide  2026-04-18 by jane.doe
2 pages (limit 25)
```

## Flags

### `jira get` / `jira search`

| Flag | Effect |
|------|--------|
| `--comments` | Append full comment bodies (no truncation) |
| `--full` | Full description + full comments |
| `--limit N` | Max results for search (default 25) |
| `--json` | Curated summary as JSON |
| `--raw` | Full MCP response (replaces current `--json`) |

### `confluence get` / `confluence search`

| Flag | Effect |
|------|--------|
| `--comments` | Append full comment bodies |
| `--full` | Full page body + full comments |
| `--limit N` | Max results for search (default 25) |
| `--json` | Curated summary as JSON |
| `--raw` | Full MCP response |

### Intentionally omitted

- **`--fields`** — too complex (format ambiguity, text vs JSON) for the value it adds. Agents can use `--json` + `jq` if they need specific fields.
- **`--description` / `--body`** — covered by `--full`. Separate flags for description vs body vs comments adds surface area without much benefit; `--full` is the one flag an agent needs to remember.
- **`--comments-limit N`** — dropped. When you ask for comments, you get all of them (up to 50). Agents shouldn't need to tune this.

## Breaking Change: `--json` → `--raw`

Current `--json` (global flag) emits raw MCP JSON. This becomes `--raw`.
`--json` will emit the curated summary as JSON.

**Migration**: `--json` is kept as a hidden deprecated alias for `--raw` for
one release (v0.2.x), printing a stderr warning:
```
warning: --json now emits curated JSON; use --raw for full MCP response
```

The global `--json` bool on `rootCmd` is replaced by per-subcommand `--json`
and `--raw` string/bool flags.

## `--json` Flag Implementation

`--json` as a cobra `BoolVar` (current) conflicts with the proposed
`--json [fields]` (optional argument). Resolution: **`--json` is a plain bool**.
No optional field argument. Agents that need specific fields use `--json` output
piped to `jq`, which is idiomatic and well-understood.

This avoids the `NoOptDefVal` / custom `pflag.Value` complexity entirely.

## Go Structs

### `internal/format/issue.go`

```go
type IssueSummary struct {
    Key          string
    IssueType    string
    Status       string
    Priority     string
    Summary      string
    Assignee     string
    Reporter     string
    Created      string // "2006-01-02"
    Updated      string // "2006-01-02"
    Sprint       string
    Labels       []string
    Parent       string
    Links        []string // "blocks EA-130", "blocked by EA-99"
    URL          string
    DescSnip     string // first 500 chars
    CommentCount int
    LatestComment *CommentSnip
}

type CommentSnip struct {
    Author string
    Date   string
    Body   string // first 150 chars
}

type IssueDetail struct {
    IssueSummary
    Description string   // full
    Comments    []Comment
}

type Comment struct {
    Author string
    Date   string
    Body   string // full, no truncation
}

// StoryPoints intentionally omitted — it's a custom field with an
// instance-specific ID (customfield_XXXXX). Not reliably extractable.
```

### `internal/format/page.go`

```go
type PageSummary struct {
    ID           string
    Title        string
    Space        string
    Status       string
    Version      int
    LastModified string
    ModifiedBy   string
    URL          string
    BodySnip     string // first 1000 chars
    CommentCount int
}

type PageDetail struct {
    PageSummary
    Body     string    // full
    Comments []Comment // full, no truncation
}
```

### `internal/format/search.go`

```go
type IssueRow struct {
    Key      string
    Type     string
    Status   string
    Priority string
    Assignee string
    Summary  string
}

type PageRow struct {
    ID           string
    Space        string
    Title        string
    LastModified string
    ModifiedBy   string
}
```

## MCP Response Indirection

The MCP response shape is:

```
ToolCallResult.Content[].Text  →  JSON string  →  actual Jira/Confluence data
```

`ParseIssue` receives the extracted text string from `TextContent(result)`,
not `Response.Result` directly. Signature:

```go
func ParseIssue(text string) (*IssueSummary, error)
func ParseIssueDetail(text string) (*IssueDetail, error)
func ParseSearch(text string) ([]IssueRow, error)
func ParsePage(text string) (*PageSummary, error)
func ParsePageDetail(text string) (*PageDetail, error)
func ParsePageSearch(text string) ([]PageRow, error)
```

## Error / Empty State

- Issue not found: MCP returns a tool error → `CallTool` returns `error` →
  print to stderr, exit 1. No change needed.
- Search with 0 results: parse empty `issues` array → print `0 issues`.
- Auth failure: already handled upstream.

## Truncation Defaults

| Field | Default | Flag to expand |
|-------|---------|----------------|
| Description | 500 chars | `--full` |
| Confluence body | 1000 chars | `--full` |
| Comment body (snip in default) | 150 chars | n/a (snip only shown as "latest comment") |
| Comments (via `--comments`) | all, no truncation | — |
| Search results | 25 | `--limit N` |

## Files Changed

```
cmd/jira.go           — add --comments, --full, --limit, --json, --raw flags;
                        call format.Parse* and format.Print*
cmd/confluence.go     — same
internal/
  format/
    issue.go          — IssueSummary, IssueDetail, Parse*, Print*
    page.go           — PageSummary, PageDetail, Parse*, Print*
    search.go         — IssueRow, PageRow, Parse*, Print* (table)
internal/output/
  format.go           — unchanged (generic KV/Table/Print helpers)
```

## Out of Scope

- `arvo call` — raw escape hatch, always full MCP output
- `arvo tools` — already compact
- `arvo auth *` — no payload to truncate
- `arvo jira create / comment / transition` — write operations, no large response
