package format

import (
	"encoding/json"
	"fmt"
	"io"
)

// Transition is a single valid Jira workflow transition.
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ParseTransitions extracts transitions from the MCP text response of
// getTransitionsForJiraIssue.
func ParseTransitions(text string) ([]Transition, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("parse transitions: %w", err)
	}
	items, ok := raw["transitions"].([]any)
	if !ok {
		return nil, fmt.Errorf("parse transitions: no transitions array in response")
	}
	out := make([]Transition, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := strVal(m, "id")
		if id == "" {
			continue // skip malformed transitions with no ID
		}
		out = append(out, Transition{
			ID:   id,
			Name: strVal(m, "name"),
		})
	}
	return out, nil
}

// PrintTransitions writes a simple ID → Name table to w.
func PrintTransitions(w io.Writer, transitions []Transition) {
	fmt.Fprintf(w, "%-8s  %s\n", "ID", "NAME")
	fmt.Fprintf(w, "%-8s  %s\n", "--------", "----")
	for _, t := range transitions {
		fmt.Fprintf(w, "%-8s  %s\n", t.ID, t.Name)
	}
}
