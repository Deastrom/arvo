package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// IssueRow is a single row in a Jira search result table.
type IssueRow struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Assignee string `json:"assignee,omitempty"`
	Summary  string `json:"summary"`
}

// PageRow is a single row in a Confluence search result table.
type PageRow struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Space        string `json:"space,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	ModifiedBy   string `json:"modified_by,omitempty"`
}

// IssueSearchResult holds parsed Jira search results.
type IssueSearchResult struct {
	Total  int        `json:"total"`
	Issues []IssueRow `json:"issues"`
}

// PageSearchResult holds parsed Confluence search results.
type PageSearchResult struct {
	Total int       `json:"total"`
	Pages []PageRow `json:"pages"`
}

// ParseIssueSearch parses a JQL search result from MCP TextContent.
func ParseIssueSearch(text string) (*IssueSearchResult, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse issue search: %w", err)
	}

	result := &IssueSearchResult{}

	issues, ok := raw["issues"].([]any)
	if !ok {
		issues, _ = raw["results"].([]any)
	}

	for _, item := range issues {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fields := mapGet[map[string]any](m, "fields")
		row := IssueRow{
			Key:      strVal(m, "key"),
			Type:     strPath(fields, "issuetype", "name"),
			Status:   strPath(fields, "status", "name"),
			Priority: strPath(fields, "priority", "name"),
			Assignee: strPath(fields, "assignee", "displayName"),
			Summary:  strVal(fields, "summary"),
		}
		result.Issues = append(result.Issues, row)
	}

	// total may be absent (MCP pagination); derive from result count.
	result.Total = int(numVal(raw, "total"))
	if result.Total == 0 {
		result.Total = len(result.Issues)
	}

	return result, nil
}

// ParsePageSearch parses a CQL search result from MCP TextContent.
// The Confluence search API wraps each result under a "content" key;
// metadata (title, lastModified, space) is on the result itself.
func ParsePageSearch(text string) (*PageSearchResult, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse page search: %w", err)
	}

	result := &PageSearchResult{}
	result.Total = int(numVal(raw, "totalSize"))
	if result.Total == 0 {
		result.Total = int(numVal(raw, "total"))
	}

	results, ok := raw["results"].([]any)
	if !ok {
		results, _ = raw["pages"].([]any)
	}

	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Content object holds id and type.
		content := mapGet[map[string]any](m, "content")

		row := PageRow{
			Title:        strVal(m, "title"),
			LastModified: fmtDate(strVal(m, "lastModified")),
		}

		if content != nil {
			row.ID = strVal(content, "id")
		} else {
			// Fallback: some endpoints put id at root.
			row.ID = strVal(m, "id")
		}

		// Space: prefer resultGlobalContainer.title, fall back to space.key.
		if gc := mapGet[map[string]any](m, "resultGlobalContainer"); gc != nil {
			row.Space = strVal(gc, "title")
		} else if space := mapGet[map[string]any](m, "space"); space != nil {
			row.Space = strVal(space, "key")
		}

		// Author: from content.history.createdBy or version.by.
		if content != nil {
			if hist := mapGet[map[string]any](content, "history"); hist != nil {
				row.ModifiedBy = strPath(hist, "createdBy", "displayName")
			}
		}
		if row.ModifiedBy == "" {
			if ver := mapGet[map[string]any](m, "version"); ver != nil {
				row.ModifiedBy = strPath(ver, "by", "displayName")
			}
		}

		result.Pages = append(result.Pages, row)
	}

	return result, nil
}

// PrintIssueSearch writes a table of issue search results to w.
func PrintIssueSearch(w io.Writer, r *IssueSearchResult) {
	fmt.Fprintf(w, "Total: %d\n\n", r.Total)
	if len(r.Issues) == 0 {
		fmt.Fprintln(w, "(no results)")
		return
	}

	// Column widths. NOTE: len() counts bytes; columns may misalign for
	// non-ASCII values (CJK etc). Acceptable for the ASCII-dominant Jira case.
	wKey, wType, wStatus, wPri, wAss := 10, 8, 12, 8, 12
	for _, row := range r.Issues {
		if l := len(row.Key); l > wKey {
			wKey = l
		}
		if l := len(row.Type); l > wType {
			wType = l
		}
		if l := len(row.Status); l > wStatus {
			wStatus = l
		}
		if l := len(row.Priority); l > wPri {
			wPri = l
		}
		if l := len(row.Assignee); l > wAss {
			wAss = l
		}
	}

	hdr := fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %-*s  %s",
		wKey, "KEY", wType, "TYPE", wStatus, "STATUS", wPri, "PRIORITY", wAss, "ASSIGNEE", "SUMMARY")
	fmt.Fprintln(w, hdr)
	fmt.Fprintln(w, strings.Repeat("-", len(hdr)+20))

	for _, row := range r.Issues {
		summary := runeSnip(row.Summary, 60)
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %-*s  %s\n",
			wKey, row.Key,
			wType, row.Type,
			wStatus, row.Status,
			wPri, row.Priority,
			wAss, row.Assignee,
			summary)
	}
}

// PrintPageSearch writes a table of page search results to w.
func PrintPageSearch(w io.Writer, r *PageSearchResult) {
	fmt.Fprintf(w, "Total: %d\n\n", r.Total)
	if len(r.Pages) == 0 {
		fmt.Fprintln(w, "(no results)")
		return
	}

	wID, wSpace, wDate, wBy := 12, 20, 12, 16
	for _, row := range r.Pages {
		if l := len(row.ID); l > wID {
			wID = l
		}
		if l := len(row.Space); l > wSpace && l <= 30 {
			wSpace = l
		}
		if l := len(row.LastModified); l > wDate {
			wDate = l
		}
		if l := len(row.ModifiedBy); l > wBy && l <= 25 {
			wBy = l
		}
	}

	hdr := fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %s",
		wID, "ID", wSpace, "SPACE", wDate, "MODIFIED", wBy, "BY", "TITLE")
	fmt.Fprintln(w, hdr)
	fmt.Fprintln(w, strings.Repeat("-", len(hdr)+20))

	for _, row := range r.Pages {
		space := runeSnip(row.Space, wSpace)
		by := runeSnip(row.ModifiedBy, wBy)
		title := runeSnip(row.Title, 60)
		fmt.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %s\n",
			wID, row.ID,
			wSpace, space,
			wDate, row.LastModified,
			wBy, by,
			title)
	}
}
