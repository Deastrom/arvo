package format_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Deastrom/arvo/internal/format"
)

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// minimalIssue builds the minimal Jira issue JSON that the parser expects.
func minimalIssue(key string, extras map[string]any) string {
	fields := map[string]any{
		"summary":   "Test Summary",
		"issuetype": map[string]any{"name": "Story"},
		"status":    map[string]any{"name": "In Progress"},
		"priority":  map[string]any{"name": "High"},
		"assignee":  map[string]any{"displayName": "Alice"},
		"reporter":  map[string]any{"displayName": "Bob"},
		"created":   "2024-03-01T10:00:00.000+0000",
		"updated":   "2024-04-15T09:30:00.000+0000",
		"comment": map[string]any{
			"total": float64(2),
			"comments": []any{
				map[string]any{
					"author":  map[string]any{"displayName": "Charlie"},
					"created": "2024-04-14T08:00:00.000+0000",
					"body":    "First comment body",
				},
				map[string]any{
					"author":  map[string]any{"displayName": "Dana"},
					"created": "2024-04-15T07:00:00.000+0000",
					"body":    "Latest comment body",
				},
			},
		},
	}
	for k, v := range extras {
		fields[k] = v
	}
	return mustJSON(map[string]any{"key": key, "fields": fields})
}

// ---- ParseIssue / ParseIssueDetail -----------------------------------------

func TestParseIssue_BasicFields(t *testing.T) {
	text := minimalIssue("PROJ-42", nil)
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if s.Key != "PROJ-42" {
		t.Errorf("Key: got %q, want %q", s.Key, "PROJ-42")
	}
	if s.IssueType != "Story" {
		t.Errorf("IssueType: got %q", s.IssueType)
	}
	if s.Status != "In Progress" {
		t.Errorf("Status: got %q", s.Status)
	}
	if s.Priority != "High" {
		t.Errorf("Priority: got %q", s.Priority)
	}
	if s.Assignee != "Alice" {
		t.Errorf("Assignee: got %q", s.Assignee)
	}
	if s.Reporter != "Bob" {
		t.Errorf("Reporter: got %q", s.Reporter)
	}
	if s.Created != "2024-03-01" {
		t.Errorf("Created: got %q", s.Created)
	}
	if s.CommentCount != 2 {
		t.Errorf("CommentCount: got %d", s.CommentCount)
	}
	if s.LatestComment == nil {
		t.Fatal("LatestComment is nil")
	}
	if s.LatestComment.Author != "Dana" {
		t.Errorf("LatestComment.Author: got %q", s.LatestComment.Author)
	}
}

func TestParseIssue_MissingOptionalFields(t *testing.T) {
	text := mustJSON(map[string]any{
		"key": "MIN-1",
		"fields": map[string]any{
			"summary": "Minimal",
		},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if s.Key != "MIN-1" {
		t.Errorf("Key: got %q", s.Key)
	}
	if s.CommentCount != 0 {
		t.Errorf("CommentCount should be 0, got %d", s.CommentCount)
	}
	if s.LatestComment != nil {
		t.Errorf("LatestComment should be nil")
	}
}

func TestParseIssue_LongDescription_Snipped(t *testing.T) {
	long := strings.Repeat("x", 600)
	text := mustJSON(map[string]any{
		"key":    "PROJ-99",
		"fields": map[string]any{"summary": "s", "description": long},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if len(s.DescSnip) > 510 { // 500 + "…"
		t.Errorf("DescSnip too long: %d", len(s.DescSnip))
	}
	if !strings.HasSuffix(s.DescSnip, "…") {
		t.Errorf("DescSnip should end with ellipsis")
	}
}

func TestParseIssue_ADFDescription(t *testing.T) {
	adf := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Hello ADF"},
				},
			},
		},
	}
	text := mustJSON(map[string]any{
		"key":    "ADF-1",
		"fields": map[string]any{"summary": "s", "description": adf},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if !strings.Contains(s.DescSnip, "Hello ADF") {
		t.Errorf("ADF description not extracted, got: %q", s.DescSnip)
	}
}

func TestParseIssue_Sprint(t *testing.T) {
	text := minimalIssue("PROJ-10", map[string]any{
		"sprint": map[string]any{"name": "Sprint 5"},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if s.Sprint != "Sprint 5" {
		t.Errorf("Sprint: got %q", s.Sprint)
	}
}

func TestParseIssue_Labels(t *testing.T) {
	text := minimalIssue("PROJ-11", map[string]any{
		"labels": []any{"backend", "urgent"},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if len(s.Labels) != 2 || s.Labels[0] != "backend" {
		t.Errorf("Labels: got %v", s.Labels)
	}
}

func TestParseIssue_Links(t *testing.T) {
	text := minimalIssue("PROJ-12", map[string]any{
		"issuelinks": []any{
			map[string]any{
				"type":         map[string]any{"outward": "blocks", "inward": "is blocked by"},
				"outwardIssue": map[string]any{"key": "PROJ-99"},
			},
		},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if len(s.Links) == 0 {
		t.Errorf("Links empty")
	}
	if !strings.Contains(s.Links[0], "PROJ-99") {
		t.Errorf("Link missing PROJ-99: %v", s.Links)
	}
}

func TestParseIssueDetail_FullComments(t *testing.T) {
	text := minimalIssue("PROJ-20", nil)
	d, err := format.ParseIssueDetail(text)
	if err != nil {
		t.Fatalf("ParseIssueDetail error: %v", err)
	}
	if len(d.Comments) != 2 {
		t.Errorf("Comments: got %d, want 2", len(d.Comments))
	}
	if d.Comments[0].Author != "Charlie" {
		t.Errorf("First comment author: got %q", d.Comments[0].Author)
	}
}

func TestParseIssue_InvalidJSON(t *testing.T) {
	_, err := format.ParseIssue("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ---- PrintIssueSummary / PrintIssueDetail -----------------------------------

func TestPrintIssueSummary_ContainsKey(t *testing.T) {
	s, _ := format.ParseIssue(minimalIssue("PROJ-42", nil))
	var buf bytes.Buffer
	format.PrintIssueSummary(&buf, s)
	out := buf.String()
	for _, want := range []string{"PROJ-42", "In Progress", "Alice", "2024-03-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintIssueDetail_ContainsComments(t *testing.T) {
	d, _ := format.ParseIssueDetail(minimalIssue("PROJ-42", nil))
	var buf bytes.Buffer
	format.PrintIssueDetail(&buf, d)
	out := buf.String()
	if !strings.Contains(out, "Charlie") {
		t.Errorf("detail output missing first comment author:\n%s", out)
	}
	if !strings.Contains(out, "Latest comment body") {
		t.Errorf("detail output missing latest comment body:\n%s", out)
	}
}

// ---- ParsePage / ParsePageDetail --------------------------------------------

func minimalPage(id string) string {
	return mustJSON(map[string]any{
		"id":     id,
		"title":  "My Page Title",
		"status": "current",
		"space":  map[string]any{"key": "TEAM"},
		"version": map[string]any{
			"number": float64(3),
			"when":   "2024-04-01T12:00:00.000Z",
			"by":     map[string]any{"displayName": "Eve"},
		},
		"_links": map[string]any{
			"base":  "https://example.atlassian.net/wiki",
			"webui": "/spaces/TEAM/pages/12345/My+Page+Title",
		},
		"body": map[string]any{
			"storage": map[string]any{
				"value": "<p>Hello <strong>world</strong></p>",
			},
		},
	})
}

func TestParsePage_BasicFields(t *testing.T) {
	p, err := format.ParsePage(minimalPage("12345"))
	if err != nil {
		t.Fatalf("ParsePage error: %v", err)
	}
	if p.ID != "12345" {
		t.Errorf("ID: got %q", p.ID)
	}
	if p.Title != "My Page Title" {
		t.Errorf("Title: got %q", p.Title)
	}
	if p.Space != "TEAM" {
		t.Errorf("Space: got %q", p.Space)
	}
	if p.Version != 3 {
		t.Errorf("Version: got %d", p.Version)
	}
	if p.ModifiedBy != "Eve" {
		t.Errorf("ModifiedBy: got %q", p.ModifiedBy)
	}
	if p.LastModified != "2024-04-01" {
		t.Errorf("LastModified: got %q", p.LastModified)
	}
	if !strings.Contains(p.URL, "TEAM") {
		t.Errorf("URL: got %q", p.URL)
	}
}

func TestParsePage_HTMLBodyStripped(t *testing.T) {
	p, err := format.ParsePage(minimalPage("12345"))
	if err != nil {
		t.Fatalf("ParsePage error: %v", err)
	}
	if strings.Contains(p.BodySnip, "<p>") || strings.Contains(p.BodySnip, "<strong>") {
		t.Errorf("BodySnip contains HTML tags: %q", p.BodySnip)
	}
	if !strings.Contains(p.BodySnip, "Hello") {
		t.Errorf("BodySnip missing text: %q", p.BodySnip)
	}
}

func TestParsePage_LongBody_Snipped(t *testing.T) {
	long := "<p>" + strings.Repeat("a", 1100) + "</p>"
	raw := mustJSON(map[string]any{
		"id": "99", "title": "T",
		"body": map[string]any{"storage": map[string]any{"value": long}},
	})
	p, err := format.ParsePage(raw)
	if err != nil {
		t.Fatalf("ParsePage error: %v", err)
	}
	if len(p.BodySnip) > 1010 {
		t.Errorf("BodySnip too long: %d", len(p.BodySnip))
	}
	if !strings.HasSuffix(p.BodySnip, "…") {
		t.Errorf("BodySnip should end with ellipsis")
	}
}

func TestParsePage_InvalidJSON(t *testing.T) {
	_, err := format.ParsePage("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestPrintPageSummary_ContainsTitle(t *testing.T) {
	p, _ := format.ParsePage(minimalPage("12345"))
	var buf bytes.Buffer
	format.PrintPageSummary(&buf, p)
	out := buf.String()
	for _, want := range []string{"My Page Title", "TEAM", "Eve", "2024-04-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// ---- ParseIssueSearch / ParsePageSearch -------------------------------------

func minimalIssueSearchResult() string {
	return mustJSON(map[string]any{
		"total": float64(2),
		"issues": []any{
			map[string]any{
				"key": "PROJ-1",
				"fields": map[string]any{
					"summary":   "First issue",
					"issuetype": map[string]any{"name": "Bug"},
					"status":    map[string]any{"name": "Open"},
					"priority":  map[string]any{"name": "High"},
					"assignee":  map[string]any{"displayName": "Frank"},
				},
			},
			map[string]any{
				"key": "PROJ-2",
				"fields": map[string]any{
					"summary":   "Second issue",
					"issuetype": map[string]any{"name": "Task"},
					"status":    map[string]any{"name": "Done"},
					"priority":  map[string]any{"name": "Low"},
				},
			},
		},
	})
}

func TestParseIssueSearch_Basic(t *testing.T) {
	r, err := format.ParseIssueSearch(minimalIssueSearchResult())
	if err != nil {
		t.Fatalf("ParseIssueSearch error: %v", err)
	}
	if r.Total != 2 {
		t.Errorf("Total: got %d", r.Total)
	}
	if len(r.Issues) != 2 {
		t.Errorf("Issues count: got %d", len(r.Issues))
	}
	if r.Issues[0].Key != "PROJ-1" {
		t.Errorf("Issues[0].Key: got %q", r.Issues[0].Key)
	}
	if r.Issues[1].Assignee != "" {
		// Second issue has no assignee.
		t.Errorf("Issues[1].Assignee should be empty, got %q", r.Issues[1].Assignee)
	}
}

func TestParseIssueSearch_Empty(t *testing.T) {
	raw := mustJSON(map[string]any{"total": float64(0), "issues": []any{}})
	r, err := format.ParseIssueSearch(raw)
	if err != nil {
		t.Fatalf("ParseIssueSearch error: %v", err)
	}
	if r.Total != 0 || len(r.Issues) != 0 {
		t.Errorf("Expected empty result, got total=%d issues=%d", r.Total, len(r.Issues))
	}
}

func TestPrintIssueSearch_Table(t *testing.T) {
	r, _ := format.ParseIssueSearch(minimalIssueSearchResult())
	var buf bytes.Buffer
	format.PrintIssueSearch(&buf, r)
	out := buf.String()
	for _, want := range []string{"PROJ-1", "PROJ-2", "Frank", "Bug", "Done"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestParsePageSearch_Basic(t *testing.T) {
	raw := mustJSON(map[string]any{
		"totalSize": float64(1),
		"results": []any{
			map[string]any{
				"id":    "999",
				"title": "A Page",
				"space": map[string]any{"key": "ENG"},
				"version": map[string]any{
					"when": "2024-05-01T00:00:00.000Z",
					"by":   map[string]any{"displayName": "Grace"},
				},
			},
		},
	})
	r, err := format.ParsePageSearch(raw)
	if err != nil {
		t.Fatalf("ParsePageSearch error: %v", err)
	}
	if r.Total != 1 {
		t.Errorf("Total: got %d", r.Total)
	}
	if r.Pages[0].Space != "ENG" {
		t.Errorf("Space: got %q", r.Pages[0].Space)
	}
	if r.Pages[0].ModifiedBy != "Grace" {
		t.Errorf("ModifiedBy: got %q", r.Pages[0].ModifiedBy)
	}
}

func TestPrintPageSearch_Table(t *testing.T) {
	raw := mustJSON(map[string]any{
		"totalSize": float64(1),
		"results": []any{
			map[string]any{
				"id": "999", "title": "A Page",
				"space": map[string]any{"key": "ENG"},
			},
		},
	})
	r, _ := format.ParsePageSearch(raw)
	var buf bytes.Buffer
	format.PrintPageSearch(&buf, r)
	out := buf.String()
	for _, want := range []string{"999", "A Page", "ENG"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

// ---- edge cases -------------------------------------------------------------

func TestParseIssue_VeryLongComment_Snipped(t *testing.T) {
	longBody := strings.Repeat("z", 300)
	text := mustJSON(map[string]any{
		"key": "EDGE-1",
		"fields": map[string]any{
			"summary": "s",
			"comment": map[string]any{
				"total": float64(1),
				"comments": []any{
					map[string]any{
						"author":  map[string]any{"displayName": "H"},
						"created": "2024-01-01",
						"body":    longBody,
					},
				},
			},
		},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if s.LatestComment == nil {
		t.Fatal("LatestComment nil")
	}
	if len(s.LatestComment.Body) > 160 {
		t.Errorf("comment snip too long: %d", len(s.LatestComment.Body))
	}
}

func TestParseIssue_SprintAsSlice(t *testing.T) {
	text := minimalIssue("PROJ-30", map[string]any{
		"customfield_10020": []any{
			map[string]any{"name": "Sprint 7"},
		},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if s.Sprint != "Sprint 7" {
		t.Errorf("Sprint (slice): got %q", s.Sprint)
	}
}

// ---- regression tests for oracle-identified bugs ----------------------------

func TestFmtDate_MillisecondRFC3339(t *testing.T) {
	// Jira commonly returns timestamps with milliseconds and colon offset.
	cases := []struct {
		input string
		want  string
	}{
		{"2024-01-15T10:30:00.000+01:00", "2024-01-15"},
		{"2024-01-15T10:30:00.000+0100", "2024-01-15"},
		{"2024-01-15T10:30:00.000Z", "2024-01-15"},
		{"2024-01-15T10:30:00Z", "2024-01-15"},
		{"2024-01-15", "2024-01-15"},
		{"not-a-date", "not-a-date"}, // passthrough
	}
	// fmtDate is internal; exercise it via ParseIssue.
	for _, tc := range cases {
		text := mustJSON(map[string]any{
			"key":    "DATE-1",
			"fields": map[string]any{"summary": "s", "created": tc.input},
		})
		s, err := format.ParseIssue(text)
		if err != nil {
			t.Fatalf("ParseIssue error for %q: %v", tc.input, err)
		}
		if s.Created != tc.want {
			t.Errorf("fmtDate(%q): got %q, want %q", tc.input, s.Created, tc.want)
		}
	}
}

func TestParseIssue_UTF8DescriptionSnip(t *testing.T) {
	// Build a string of 600 2-byte runes (×). Byte length = 1200, rune count = 600.
	long := strings.Repeat("×", 600)
	text := mustJSON(map[string]any{
		"key":    "UTF-1",
		"fields": map[string]any{"summary": "s", "description": long},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	// Must end with ellipsis and be valid UTF-8.
	if !strings.HasSuffix(s.DescSnip, "…") {
		t.Errorf("DescSnip missing ellipsis: %q", s.DescSnip[:min(20, len(s.DescSnip))])
	}
	if !strings.HasPrefix(s.DescSnip, "×") {
		t.Errorf("DescSnip lost content")
	}
	// Byte length must be a multiple of 2 (each × is 2 bytes) + 3 bytes for …
	// Just verify the string is valid UTF-8.
	if !isValidUTF8(s.DescSnip) {
		t.Errorf("DescSnip is not valid UTF-8")
	}
}

func TestParseIssue_IssueURL(t *testing.T) {
	// URL is now set by the cmd layer via format.IssueURL; verify the helper directly.
	cases := []struct {
		cloudURL string
		key      string
		want     string
	}{
		{"https://example.atlassian.net", "PROJ-55", "https://example.atlassian.net/browse/PROJ-55"},
		{"https://example.atlassian.net/", "PROJ-55", "https://example.atlassian.net/browse/PROJ-55"},
		{"", "PROJ-55", ""},
		{"https://example.atlassian.net", "", ""},
	}
	for _, tc := range cases {
		got := format.IssueURL(tc.cloudURL, tc.key)
		if got != tc.want {
			t.Errorf("IssueURL(%q, %q) = %q, want %q", tc.cloudURL, tc.key, got, tc.want)
		}
	}
}

func TestParsePage_CommentCountFromChildren(t *testing.T) {
	raw := mustJSON(map[string]any{
		"id": "1", "title": "T",
		"children": map[string]any{
			"comment": map[string]any{"size": float64(7)},
		},
	})
	p, err := format.ParsePage(raw)
	if err != nil {
		t.Fatalf("ParsePage error: %v", err)
	}
	if p.CommentCount != 7 {
		t.Errorf("CommentCount: got %d, want 7", p.CommentCount)
	}
}

func TestParsePage_AtlasDocFormatBody(t *testing.T) {
	adf := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "ADF content"},
				},
			},
		},
	}
	adfJSON, _ := json.Marshal(adf)
	raw := mustJSON(map[string]any{
		"id": "2", "title": "T",
		"body": map[string]any{
			"atlas_doc_format": map[string]any{
				"value": string(adfJSON),
			},
		},
	})
	p, err := format.ParsePage(raw)
	if err != nil {
		t.Fatalf("ParsePage error: %v", err)
	}
	if !strings.Contains(p.BodySnip, "ADF content") {
		t.Errorf("atlas_doc_format body not extracted: %q", p.BodySnip)
	}
	// Must not contain JSON syntax.
	if strings.Contains(p.BodySnip, `{"type"`) {
		t.Errorf("BodySnip contains raw JSON: %q", p.BodySnip)
	}
}

func TestADF_MentionAndEmoji(t *testing.T) {
	adf := map[string]any{
		"type": "paragraph",
		"content": []any{
			map[string]any{
				"type":  "mention",
				"attrs": map[string]any{"text": "Alice"},
			},
			map[string]any{"type": "text", "text": " said "},
			map[string]any{
				"type":  "emoji",
				"attrs": map[string]any{"text": "👍"},
			},
		},
	}
	text := mustJSON(map[string]any{
		"key":    "ADF-2",
		"fields": map[string]any{"summary": "s", "description": adf},
	})
	s, err := format.ParseIssue(text)
	if err != nil {
		t.Fatalf("ParseIssue error: %v", err)
	}
	if !strings.Contains(s.DescSnip, "@Alice") {
		t.Errorf("mention not rendered: %q", s.DescSnip)
	}
	if !strings.Contains(s.DescSnip, "👍") {
		t.Errorf("emoji not rendered: %q", s.DescSnip)
	}
}

// isValidUTF8 checks that a string contains only valid UTF-8 sequences.
func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' {
			return false
		}
	}
	return true
}

