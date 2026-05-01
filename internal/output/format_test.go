package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Deastrom/arvo/internal/output"
)

func TestKV(t *testing.T) {
	var buf bytes.Buffer
	output.KV(&buf, "status", "open")
	line := buf.String()
	if !strings.Contains(line, "status:") {
		t.Errorf("expected 'status:' in output, got %q", line)
	}
	if !strings.Contains(line, "open") {
		t.Errorf("expected 'open' in output, got %q", line)
	}
}

func TestTable(t *testing.T) {
	var buf bytes.Buffer
	output.Table(&buf, []string{"KEY", "VALUE"}, [][]string{
		{"PROJ-1", "Fix the bug"},
		{"PROJ-2", "Add feature"},
	})
	out := buf.String()
	if !strings.Contains(out, "KEY") || !strings.Contains(out, "VALUE") {
		t.Errorf("missing header in table output: %q", out)
	}
	if !strings.Contains(out, "PROJ-1") {
		t.Errorf("missing row data in table output: %q", out)
	}
}

func TestTableShortRow(t *testing.T) {
	// Row shorter than headers must not panic.
	var buf bytes.Buffer
	output.Table(&buf, []string{"A", "B", "C"}, [][]string{
		{"only-one"},
	})
	out := buf.String()
	if !strings.Contains(out, "only-one") {
		t.Errorf("missing partial row data: %q", out)
	}
}

func TestTableEmptyRows(t *testing.T) {
	var buf bytes.Buffer
	output.Table(&buf, []string{"KEY", "VALUE"}, nil)
	out := buf.String()
	if !strings.Contains(out, "KEY") {
		t.Errorf("expected header even with no rows: %q", out)
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"key": "value", "num": 42}
	if err := output.Print(&buf, data, true); err != nil {
		t.Fatalf("Print JSON: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"key"`) || !strings.Contains(out, `"value"`) {
		t.Errorf("unexpected JSON output: %q", out)
	}
}

func TestPrintText(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"summary": "Fix the bug", "status": "Open"}
	if err := output.Print(&buf, data, false); err != nil {
		t.Fatalf("Print text: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "summary:") {
		t.Errorf("expected 'summary:' in text output: %q", out)
	}
}

func TestPrintRawJSON(t *testing.T) {
	var buf bytes.Buffer
	raw := json.RawMessage(`{"foo":"bar"}`)
	if err := output.PrintRaw(&buf, raw, true); err != nil {
		t.Fatalf("PrintRaw JSON: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"foo"`) || !strings.Contains(out, `"bar"`) {
		t.Errorf("unexpected PrintRaw JSON output: %q", out)
	}
}

func TestPrintRawText(t *testing.T) {
	var buf bytes.Buffer
	raw := json.RawMessage(`{"foo":"bar"}`)
	if err := output.PrintRaw(&buf, raw, false); err != nil {
		t.Fatalf("PrintRaw text: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "foo") {
		t.Errorf("expected key 'foo' in text output: %q", out)
	}
}

func TestPrintRawMalformed(t *testing.T) {
	var buf bytes.Buffer
	raw := json.RawMessage(`not-json`)
	if err := output.PrintRaw(&buf, raw, true); err != nil {
		t.Fatalf("PrintRaw malformed: %v", err)
	}
	if !strings.Contains(buf.String(), "not-json") {
		t.Errorf("expected raw fallback: %q", buf.String())
	}
}
