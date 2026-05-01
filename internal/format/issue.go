package format

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	descSnipLen    = 500
	commentSnipLen = 150
)

// IssueSummary is the curated default view of a Jira issue.
type IssueSummary struct {
	Key           string       `json:"key"`
	IssueType     string       `json:"type"`
	Status        string       `json:"status"`
	Priority      string       `json:"priority"`
	Summary       string       `json:"summary"`
	Assignee      string       `json:"assignee,omitempty"`
	Reporter      string       `json:"reporter,omitempty"`
	Created       string       `json:"created,omitempty"`
	Updated       string       `json:"updated,omitempty"`
	Sprint        string       `json:"sprint,omitempty"`
	Labels        []string     `json:"labels,omitempty"`
	Parent        string       `json:"parent,omitempty"`
	Links         []string     `json:"links,omitempty"`
	URL           string       `json:"url,omitempty"`
	DescSnip      string       `json:"description_snip,omitempty"`
	CommentCount  int          `json:"comment_count"`
	LatestComment *CommentSnip `json:"latest_comment,omitempty"`
}

// CommentSnip is a truncated comment for the default summary view.
type CommentSnip struct {
	Author string `json:"author"`
	Date   string `json:"date"`
	Body   string `json:"body"`
}

// IssueDetail extends IssueSummary with full description and comments.
type IssueDetail struct {
	IssueSummary
	Description string    `json:"description,omitempty"`
	Comments    []Comment `json:"comments,omitempty"`
}

// Comment is a full comment body.
type Comment struct {
	Author string `json:"author"`
	Date   string `json:"date"`
	Body   string `json:"body"`
}

// ParseIssue extracts a curated IssueSummary from the MCP text response.
// text is the string from mcp.TextContent(result).
func ParseIssue(text string) (*IssueSummary, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse issue: %w", err)
	}
	return extractSummary(raw), nil
}

// ParseIssueDetail extracts an IssueDetail (full description + comments).
func ParseIssueDetail(text string) (*IssueDetail, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse issue detail: %w", err)
	}
	summary := extractSummary(raw)
	detail := &IssueDetail{IssueSummary: *summary}

	fields := mapGet[map[string]any](raw, "fields")

	// Full description.
	detail.Description = extractDescription(fields)

	// Full comments.
	if comments := extractComments(fields); len(comments) > 0 {
		detail.Comments = comments
	}

	return detail, nil
}

// PrintIssueSummary writes the default curated text view to w.
func PrintIssueSummary(w io.Writer, s *IssueSummary) {
	fmt.Fprintf(w, "%s  %s  %s  %s\n", s.Key, s.IssueType, s.Status, s.Priority)
	fmt.Fprintf(w, "%q\n", s.Summary)
	if s.Assignee != "" {
		fmt.Fprintf(w, "%-12s %s\n", "Assignee:", s.Assignee)
	}
	if s.Reporter != "" {
		fmt.Fprintf(w, "%-12s %s\n", "Reporter:", s.Reporter)
	}
	if s.Created != "" || s.Updated != "" {
		fmt.Fprintf(w, "%-12s %s  Updated: %s\n", "Created:", s.Created, s.Updated)
	}
	if s.Sprint != "" {
		fmt.Fprintf(w, "%-12s %s\n", "Sprint:", s.Sprint)
	}
	if len(s.Labels) > 0 {
		fmt.Fprintf(w, "%-12s %s\n", "Labels:", strings.Join(s.Labels, ", "))
	}
	if s.Parent != "" {
		fmt.Fprintf(w, "%-12s %s\n", "Parent:", s.Parent)
	}
	if len(s.Links) > 0 {
		fmt.Fprintf(w, "%-12s %s\n", "Links:", strings.Join(s.Links, " · "))
	}
	if s.URL != "" {
		fmt.Fprintf(w, "%-12s %s\n", "URL:", s.URL)
	}
	if s.DescSnip != "" {
		fmt.Fprintf(w, "\nDescription (%d chars):\n  %s\n", utf8.RuneCountInString(s.DescSnip), wordWrap(s.DescSnip, 76, "  "))
	}
	if s.CommentCount > 0 {
		line := fmt.Sprintf("\nComments: %d", s.CommentCount)
		if s.LatestComment != nil {
			line += fmt.Sprintf(" (latest: %s by %s)", s.LatestComment.Date, s.LatestComment.Author)
		}
		fmt.Fprintln(w, line)
	}
}

// PrintIssueDetail writes the full detail view (description + comments) to w.
func PrintIssueDetail(w io.Writer, d *IssueDetail) {
	// Print the summary without the snip — full description follows below.
	s := d.IssueSummary
	savedSnip := s.DescSnip
	s.DescSnip = ""
	PrintIssueSummary(w, &s)
	if d.Description != "" {
		fmt.Fprintf(w, "\nDescription:\n%s\n", indent(d.Description, "  "))
	} else if savedSnip != "" {
		fmt.Fprintf(w, "\nDescription:\n%s\n", indent(savedSnip, "  "))
	}
	if len(d.Comments) > 0 {
		fmt.Fprintf(w, "\n--- Comments (%d) ---\n", len(d.Comments))
		for i, c := range d.Comments {
			fmt.Fprintf(w, "[%d] %s  %s\n%s\n\n", i+1, c.Author, c.Date, indent(c.Body, "  "))
		}
	}
}

// --- extraction helpers ---

func extractSummary(raw map[string]any) *IssueSummary {
	s := &IssueSummary{}
	s.Key = strVal(raw, "key")
	fields := mapGet[map[string]any](raw, "fields")

	s.IssueType = strPath(fields, "issuetype", "name")
	s.Status = strPath(fields, "status", "name")
	s.Priority = strPath(fields, "priority", "name")
	s.Summary = strVal(fields, "summary")
	s.Assignee = strPath(fields, "assignee", "displayName")
	s.Reporter = strPath(fields, "reporter", "displayName")
	s.Created = fmtDate(strVal(fields, "created"))
	s.Updated = fmtDate(strVal(fields, "updated"))
	s.Parent = strPath(fields, "parent", "key")
	s.Labels = stringSlice(fields, "labels")
	s.Sprint = extractSprint(fields)
	s.Links = extractLinks(fields)

	desc := extractDescription(fields)
	s.DescSnip = runeSnip(desc, descSnipLen)

	comments := mapGet[map[string]any](fields, "comment")
	if comments != nil {
		total := int(numVal(comments, "total"))
		s.CommentCount = total
		if items, ok := comments["comments"].([]any); ok && len(items) > 0 {
			last := items[len(items)-1]
			if cm, ok := last.(map[string]any); ok {
				body := extractCommentBody(cm)
				snip := runeSnip(body, commentSnipLen)
				s.LatestComment = &CommentSnip{
					Author: strPath(cm, "author", "displayName"),
					Date:   fmtDate(strVal(cm, "created")),
					Body:   snip,
				}
			}
		}
	}

	return s
}

func extractDescription(fields map[string]any) string {
	if fields == nil {
		return ""
	}
	// Description may be a plain string or an Atlassian Document Format object.
	switch v := fields["description"].(type) {
	case string:
		return v
	case map[string]any:
		return adfToText(v)
	}
	return ""
}

func extractComments(fields map[string]any) []Comment {
	if fields == nil {
		return nil
	}
	comments := mapGet[map[string]any](fields, "comment")
	if comments == nil {
		return nil
	}
	items, ok := comments["comments"].([]any)
	if !ok {
		return nil
	}
	out := make([]Comment, 0, len(items))
	for _, item := range items {
		cm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, Comment{
			Author: strPath(cm, "author", "displayName"),
			Date:   fmtDate(strVal(cm, "created")),
			Body:   extractCommentBody(cm),
		})
	}
	return out
}

func extractCommentBody(cm map[string]any) string {
	switch v := cm["body"].(type) {
	case string:
		return v
	case map[string]any:
		return adfToText(v)
	}
	return ""
}

func extractSprint(fields map[string]any) string {
	if fields == nil {
		return ""
	}
	// Sprint may be under various custom field names; MCP normalises to "sprint".
	for _, key := range []string{"sprint", "customfield_10020"} {
		switch v := fields[key].(type) {
		case map[string]any:
			return strVal(v, "name")
		case []any:
			if len(v) > 0 {
				if m, ok := v[0].(map[string]any); ok {
					return strVal(m, "name")
				}
			}
		}
	}
	return ""
}

func extractLinks(fields map[string]any) []string {
	if fields == nil {
		return nil
	}
	items, ok := fields["issuelinks"].([]any)
	if !ok {
		return nil
	}
	var links []string
	for _, item := range items {
		link, ok := item.(map[string]any)
		if !ok {
			continue
		}
		lt := mapGet[map[string]any](link, "type")
		if lt == nil {
			continue
		}
		if out := mapGet[map[string]any](link, "outwardIssue"); out != nil {
			links = append(links, strVal(lt, "outward")+" "+strVal(out, "key"))
		}
		if in := mapGet[map[string]any](link, "inwardIssue"); in != nil {
			links = append(links, strVal(lt, "inward")+" "+strVal(in, "key"))
		}
	}
	return links
}

// adfToText converts an Atlassian Document Format node to plain text.
func adfToText(node map[string]any) string {
	var sb strings.Builder
	adfNode(&sb, node)
	return strings.TrimSpace(sb.String())
}

func adfNode(sb *strings.Builder, node map[string]any) {
	nodeType, _ := node["type"].(string)

	// Emit inline text.
	if text, ok := node["text"].(string); ok {
		sb.WriteString(text)
	}

	// Synthesise text for non-text leaf nodes.
	switch nodeType {
	case "mention":
		// attrs.text is the display name; fall back to attrs.id.
		if attrs := mapGet[map[string]any](node, "attrs"); attrs != nil {
			if t := strVal(attrs, "text"); t != "" {
				sb.WriteString("@" + t)
			} else if id := strVal(attrs, "id"); id != "" {
				sb.WriteString("@" + id)
			}
		}
	case "emoji":
		if attrs := mapGet[map[string]any](node, "attrs"); attrs != nil {
			if text := strVal(attrs, "text"); text != "" {
				sb.WriteString(text)
			} else if shortName := strVal(attrs, "shortName"); shortName != "" {
				sb.WriteString(shortName)
			}
		}
	case "inlineCard", "blockCard":
		if attrs := mapGet[map[string]any](node, "attrs"); attrs != nil {
			if url := strVal(attrs, "url"); url != "" {
				sb.WriteString("[" + url + "]")
			}
		}
	case "hardBreak":
		sb.WriteString("\n")
	case "rule":
		sb.WriteString("\n---\n")
	}

	// Recurse into children.
	if children, ok := node["content"].([]any); ok {
		for i, child := range children {
			if m, ok := child.(map[string]any); ok {
				// Prefix list items.
				childType, _ := m["type"].(string)
				if nodeType == "bulletList" && childType == "listItem" {
					sb.WriteString("• ")
				} else if nodeType == "orderedList" && childType == "listItem" {
					sb.WriteString(fmt.Sprintf("%d. ", i+1))
				}
				adfNode(sb, m)
			}
		}
	}

	// Table row/cell separators.
	switch nodeType {
	case "tableCell", "tableHeader":
		sb.WriteString("\t")
	case "tableRow":
		sb.WriteString("\n")
	}

	// Newlines after block nodes.
	switch nodeType {
	case "paragraph", "heading", "codeBlock", "blockquote", "listItem", "table":
		sb.WriteString("\n")
	}
}

// IssueURL constructs a browser browse URL for a Jira issue given the
// tenant base URL (e.g. "https://example.atlassian.net") and the issue key.
// Returns empty string if either argument is empty.
func IssueURL(cloudURL, key string) string {
	if cloudURL == "" || key == "" {
		return ""
	}
	return strings.TrimRight(cloudURL, "/") + "/browse/" + key
}

// runeSnip truncates s to at most maxRunes runes, appending "…" if truncated.
func runeSnip(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

// --- generic helpers ---

func strVal(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func strPath(m map[string]any, keys ...string) string {
	cur := m
	for i, k := range keys {
		if cur == nil {
			return ""
		}
		if i == len(keys)-1 {
			return strVal(cur, k)
		}
		cur, _ = cur[k].(map[string]any)
	}
	return ""
}

func numVal(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	f, _ := m[key].(float64)
	return f
}

func mapGet[T any](m map[string]any, key string) T {
	if m == nil {
		var zero T
		return zero
	}
	v, _ := m[key].(T)
	return v
}

func stringSlice(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func fmtDate(s string) string {
	if s == "" {
		return ""
	}
	// Try layouts in order. RFC3339 requires no fractional seconds; add the
	// millisecond variants that Jira/Confluence commonly emit.
	for _, layout := range []string{
		time.RFC3339,                    // 2006-01-02T15:04:05Z07:00
		"2006-01-02T15:04:05.000Z07:00", // RFC3339 + millis (colon offset)
		"2006-01-02T15:04:05.000-0700",  // no colon offset
		"2006-01-02T15:04:05Z",          // UTC no offset
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}

func wordWrap(s string, width int, prefix string) string {
	words := strings.Fields(s)
	var lines []string
	line := ""
	for _, w := range words {
		wLen := utf8.RuneCountInString(w)
		lineLen := utf8.RuneCountInString(line)
		if lineLen+wLen+1 > width && line != "" {
			lines = append(lines, line)
			line = w
		} else {
			if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"+prefix)
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}
