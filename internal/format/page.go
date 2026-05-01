package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const bodySnipLen = 1000

// PageSummary is the curated default view of a Confluence page.
type PageSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Space        string `json:"space,omitempty"`
	Status       string `json:"status,omitempty"`
	Version      int    `json:"version,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	ModifiedBy   string `json:"modified_by,omitempty"`
	URL          string `json:"url,omitempty"`
	BodySnip     string `json:"body_snip,omitempty"`
	CommentCount int    `json:"comment_count"`
}

// PageDetail extends PageSummary with the full body and comments.
type PageDetail struct {
	PageSummary
	Body     string    `json:"body,omitempty"`
	Comments []Comment `json:"comments,omitempty"`
}

// ParsePage extracts a curated PageSummary from the MCP text response.
func ParsePage(text string) (*PageSummary, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse page: %w", err)
	}
	return extractPageSummary(raw), nil
}

// ParsePageDetail extracts a PageDetail (full body + comments).
func ParsePageDetail(text string) (*PageDetail, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse page detail: %w", err)
	}
	summary := extractPageSummary(raw)
	detail := &PageDetail{PageSummary: *summary}
	detail.Body = extractPageBody(raw)
	detail.Comments = extractPageComments(raw)
	return detail, nil
}

// PrintPageSummary writes the default curated text view to w.
func PrintPageSummary(w io.Writer, p *PageSummary) {
	fmt.Fprintf(w, "%q  (Page %s)\n", p.Title, p.ID)
	meta := []string{}
	if p.Space != "" {
		meta = append(meta, "Space: "+p.Space)
	}
	if p.Status != "" {
		meta = append(meta, "Status: "+p.Status)
	}
	if p.Version > 0 {
		meta = append(meta, fmt.Sprintf("Version: %d", p.Version))
	}
	if len(meta) > 0 {
		fmt.Fprintln(w, strings.Join(meta, "  "))
	}
	if p.LastModified != "" {
		line := "Last modified: " + p.LastModified
		if p.ModifiedBy != "" {
			line += " by " + p.ModifiedBy
		}
		fmt.Fprintln(w, line)
	}
	if p.URL != "" {
		fmt.Fprintln(w, "URL: "+p.URL)
	}
	if p.BodySnip != "" {
		fmt.Fprintf(w, "\nBody (%d chars):\n  %s\n", utf8.RuneCountInString(p.BodySnip), wordWrap(p.BodySnip, 76, "  "))
	}
	if p.CommentCount > 0 {
		fmt.Fprintf(w, "\nComments: %d\n", p.CommentCount)
	}
}

// PrintPageDetail writes the full page view (body + comments) to w.
func PrintPageDetail(w io.Writer, d *PageDetail) {
	// Print summary without the snip — full body follows below.
	p := d.PageSummary
	savedSnip := p.BodySnip
	p.BodySnip = ""
	PrintPageSummary(w, &p)
	if d.Body != "" {
		fmt.Fprintf(w, "\nBody:\n%s\n", indent(d.Body, "  "))
	} else if savedSnip != "" {
		fmt.Fprintf(w, "\nBody:\n%s\n", indent(savedSnip, "  "))
	}
	if d.CommentCount > 0 && len(d.Comments) == 0 {
		fmt.Fprintf(w, "\n(comments not expanded — %d known; use --raw to fetch)\n", d.CommentCount)
	}
	if len(d.Comments) > 0 {
		fmt.Fprintf(w, "\n--- Comments (%d) ---\n", len(d.Comments))
		for i, c := range d.Comments {
			fmt.Fprintf(w, "[%d] %s  %s\n%s\n\n", i+1, c.Author, c.Date, indent(c.Body, "  "))
		}
	}
}

// --- extraction helpers ---

func extractPageSummary(raw map[string]any) *PageSummary {
	p := &PageSummary{}
	p.ID = strVal(raw, "id")
	p.Title = strVal(raw, "title")
	p.Status = strVal(raw, "status")

	// Space: v1 API has space.key; v2 has spaceId (no name without expansion).
	if space := mapGet[map[string]any](raw, "space"); space != nil {
		p.Space = strVal(space, "key")
	}

	// Version: v1 has version.when + version.by.displayName;
	//          v2 has version.createdAt + version.authorId (no displayName).
	if ver := mapGet[map[string]any](raw, "version"); ver != nil {
		p.Version = int(numVal(ver, "number"))
		// v1 field
		when := strVal(ver, "when")
		if when == "" {
			// v2 field
			when = strVal(ver, "createdAt")
		}
		p.LastModified = fmtDate(when)
		if by := mapGet[map[string]any](ver, "by"); by != nil {
			p.ModifiedBy = strVal(by, "displayName")
		}
		// v2 authorId only — we don't have displayName without another call; leave empty.
	}

	// URL: v1 provides _links.base + _links.webui.
	// v2 does not include _links; construct from spaceId/id if possible.
	if links := mapGet[map[string]any](raw, "_links"); links != nil {
		if base := strVal(links, "base"); base != "" {
			p.URL = base + strVal(links, "webui")
		} else {
			p.URL = strVal(links, "webui")
		}
	}

	body := extractPageBody(raw)
	p.BodySnip = runeSnip(body, bodySnipLen)

	// Comment count: prefer children.comment.size (v1), fall back to metadata.
	if children := mapGet[map[string]any](raw, "children"); children != nil {
		if cc := mapGet[map[string]any](children, "comment"); cc != nil {
			p.CommentCount = int(numVal(cc, "size"))
		}
	}
	if p.CommentCount == 0 {
		if meta := mapGet[map[string]any](raw, "metadata"); meta != nil {
			if fc := mapGet[map[string]any](meta, "frontend"); fc != nil {
				p.CommentCount = int(numVal(fc, "comments"))
			}
		}
	}

	return p
}

func extractPageBody(raw map[string]any) string {
	body := mapGet[map[string]any](raw, "body")
	if body == nil {
		return ""
	}

	// v2 API: body is a top-level ADF document {type:"doc", content:[...]}.
	if strVal(body, "type") == "doc" {
		return adfToText(body)
	}

	// v1 API: body has named format sub-objects.
	for _, bodyFmt := range []string{"storage", "atlas_doc_format", "view", "export_view"} {
		f := mapGet[map[string]any](body, bodyFmt)
		if f == nil {
			continue
		}
		value := strVal(f, "value")
		if value == "" {
			continue
		}
		// atlas_doc_format value is a JSON-serialised ADF string, not HTML.
		if bodyFmt == "atlas_doc_format" {
			var adf map[string]any
			if err := json.Unmarshal([]byte(value), &adf); err == nil {
				return adfToText(adf)
			}
			// Malformed ADF JSON — fall through and strip tags as best-effort.
		}
		return stripTags(value)
	}
	return ""
}

func extractPageComments(raw map[string]any) []Comment {
	// Comments may be nested under children.comment.results.
	children := mapGet[map[string]any](raw, "children")
	if children == nil {
		return nil
	}
	comment := mapGet[map[string]any](children, "comment")
	if comment == nil {
		return nil
	}
	results, ok := comment["results"].([]any)
	if !ok {
		return nil
	}
	out := make([]Comment, 0, len(results))
	for _, item := range results {
		cm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		body := extractPageBody(cm) // comments have same body structure
		ver := mapGet[map[string]any](cm, "version")
		author := ""
		date := ""
		if ver != nil {
			date = fmtDate(strVal(ver, "when"))
			if by := mapGet[map[string]any](ver, "by"); by != nil {
				author = strVal(by, "displayName")
			}
		}
		out = append(out, Comment{Author: author, Date: date, Body: body})
	}
	return out
}

// stripTags removes HTML tags from a string, leaving plain text.
func stripTags(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			sb.WriteRune(' ')
		case !inTag:
			sb.WriteRune(r)
		}
	}
	// Collapse whitespace.
	return strings.Join(strings.Fields(sb.String()), " ")
}
