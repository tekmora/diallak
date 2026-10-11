package serverplan

import (
	"fmt"
	"io"
	"strings"
)

// Render writes the formatted dry-run plan text for r to w.
func Render(w io.Writer, r Result) error {
	if err := renderHeader(w, r); err != nil {
		return err
	}
	if err := renderChanges(w, r.Changes); err != nil {
		return err
	}
	return renderRefusals(w, r.Refused)
}

func renderHeader(w io.Writer, r Result) error {
	mode := "own"
	if r.Raised {
		mode = "raised"
	}
	if _, err := fmt.Fprintf(w, "mode: %s\n", mode); err != nil {
		return fmt.Errorf("render mode: %w", err)
	}

	rights := r.Rights()
	rightsText := "none"
	if len(rights) > 0 {
		rightsText = strings.Join(rights, ", ")
	}
	if _, err := fmt.Fprintf(w, "rights needed: %s\n", rightsText); err != nil {
		return fmt.Errorf("render rights needed: %w", err)
	}
	return nil
}

func renderChanges(w io.Writer, changes []Change) error {
	if _, err := fmt.Fprintf(w, "changes: %d\n", len(changes)); err != nil {
		return fmt.Errorf("render changes count: %w", err)
	}
	for _, c := range changes {
		line := fmt.Sprintf("  %d. %s %s needs %s", c.Step, c.Action, c.Object, strings.Join(c.Needs, ", "))
		if c.Detail != "" {
			line += "; " + c.Detail
		}
		line += "\n"
		if _, err := io.WriteString(w, line); err != nil {
			return fmt.Errorf("render change: %w", err)
		}
	}
	return nil
}

func renderRefusals(w io.Writer, refused []Refusal) error {
	if _, err := fmt.Fprintf(w, "refused: %d\n", len(refused)); err != nil {
		return fmt.Errorf("render refused count: %w", err)
	}
	for _, ref := range refused {
		line := fmt.Sprintf("  - %s %s: %s", ref.Action, ref.Object, ref.Reason)
		if ref.Detail != "" {
			line += "; " + ref.Detail
		}
		line += "\n"
		if _, err := io.WriteString(w, line); err != nil {
			return fmt.Errorf("render refusal: %w", err)
		}
	}
	return nil
}
