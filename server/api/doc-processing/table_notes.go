package docprocessing

import (
	"regexp"
	"strings"
)

// TableNote is one explicitly labelled note within a cell. Splitting notes does
// not change the canonical row IDs, cell contents, or their hashes.
type TableNote struct {
	Label string
	Text  string
}

var tableNoteMarker = regexp.MustCompile(`\$\s*\^\s*\{?([a-z])\}?\s*\$`)

// TableCellNotes recognizes a block of superscript-labelled notes, rather than a
// superscript reference following a value. Ambiguous or duplicate labels fail closed.
func TableCellNotes(text string) []TableNote {
	markers := tableNoteMarker.FindAllStringSubmatchIndex(text, -1)
	if len(markers) == 0 || strings.TrimSpace(text[:markers[0][0]]) != "" {
		return nil
	}
	seen := map[string]bool{}
	var notes []TableNote
	for i, marker := range markers {
		label := text[marker[2]:marker[3]]
		end := len(text)
		if i+1 < len(markers) {
			end = markers[i+1][0]
		}
		body := strings.TrimSpace(text[marker[1]:end])
		if seen[label] || body == "" {
			return nil
		}
		seen[label] = true
		notes = append(notes, TableNote{Label: label, Text: body})
	}
	return notes
}

// tableMetricNotes requires the quantity and its condition in the SAME note.
// Whole-cell context is not evidence for choosing between neighboring notes.
func tableMetricNotes(row TableRow, metric map[string]any) []TableNote {
	text, ok := row.FullWidthText()
	if !ok {
		return nil
	}
	matches := tableMetricFootnoteMatcher(metric)
	var found []TableNote
	for _, note := range TableCellNotes(text) {
		if matches(note.Text) {
			found = append(found, note)
		}
	}
	if len(found) != 1 {
		return nil
	}
	return found
}
