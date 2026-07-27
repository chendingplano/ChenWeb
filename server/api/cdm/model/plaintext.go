package model

import "strings"

// PlainText flattens inline content to plain text, dropping all markup
// (emphasis, links, citations render as their visible text only). Used by
// line-file generation (spec §10.1) and, later, retrieval projections
// (spec §8).
func PlainText(inlines []Inline) string {
	var out strings.Builder
	writePlainText(&out, inlines)
	return out.String()
}

func writePlainText(out *strings.Builder, inlines []Inline) {
	for _, n := range inlines {
		switch n.Type {
		case "text", "code":
			out.WriteString(n.Text)
		case "line_break":
			out.WriteByte('\n')
		case "citation":
			out.WriteString(n.CitationKey)
		default:
			writePlainText(out, n.Content)
		}
	}
}
