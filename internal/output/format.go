package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Print writes text or JSON to w based on the jsonMode flag.
// data is written as-is in JSON mode; fields are printed as key: value lines in text mode.
func Print(w io.Writer, data any, jsonMode bool) error {
	if jsonMode {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}
	return printText(w, data)
}

// PrintRaw writes raw JSON bytes, pretty-printing in JSON mode or as plain text otherwise.
func PrintRaw(w io.Writer, raw json.RawMessage, jsonMode bool) error {
	if jsonMode {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			_, err2 := fmt.Fprintln(w, string(raw))
			return err2
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	// In text mode, print as indented JSON anyway — raw blobs have no better representation.
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		_, err2 := fmt.Fprintln(w, string(raw))
		return err2
	}
	return printText(w, v)
}

// KV prints a single key: value line.
func KV(w io.Writer, key, value string) {
	fmt.Fprintf(w, "%-20s %s\n", key+":", value)
}

// Table prints a simple table with a header row and rows of equal-length string slices.
func Table(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	printRow := func(row []string) {
		parts := make([]string, len(row))
		for i, cell := range row {
			if i < len(widths) {
				parts[i] = fmt.Sprintf("%-*s", widths[i], cell)
			} else {
				parts[i] = cell
			}
		}
		fmt.Fprintln(w, strings.Join(parts, "  "))
	}

	printRow(headers)
	sep := make([]string, len(headers))
	for i, w := range widths {
		sep[i] = strings.Repeat("-", w)
	}
	printRow(sep)
	for _, row := range rows {
		printRow(row)
	}
}

// printText recursively prints a value in key: value format.
func printText(w io.Writer, data any) error {
	switch v := data.(type) {
	case map[string]any:
		for key, val := range v {
			switch inner := val.(type) {
			case string:
				KV(w, key, inner)
			case nil:
				KV(w, key, "<nil>")
			default:
				b, _ := json.Marshal(inner)
				KV(w, key, string(b))
			}
		}
	case []any:
		for i, item := range v {
			fmt.Fprintf(w, "[%d]\n", i)
			_ = printText(w, item)
		}
	case string:
		fmt.Fprintln(w, v)
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(w, string(b))
	}
	return nil
}
