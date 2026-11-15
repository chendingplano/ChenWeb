package docprocessing

import (
	"regexp"
	"sort"
	"strings"
)

// TableNote is one explicitly labelled note within a cell. Splitting notes does
// not change the canonical row IDs, cell contents, or their hashes.
type TableNote struct {
	Label string
	Text  string
}

var tableNoteMarker = regexp.MustCompile(`\$\s*\^\s*\{?([a-z])\}?\s*\$`)
var flattenedTableNoteMarker = regexp.MustCompile(`[。；！？.!?;]\s*([a-z])\s*\p{Han}`)

// TableCellNotes recognizes a block of superscript-labelled notes, rather than a
// superscript reference following a value. Ambiguous or duplicate labels fail closed.
func TableCellNotes(text string) []TableNote {
	explicit := tableNoteMarker.FindAllStringSubmatchIndex(text, -1)
	if len(explicit) == 0 || strings.TrimSpace(text[:explicit[0][0]]) != "" {
		return nil
	}
	type marker struct {
		start, end int
		label      string
		explicit   bool
	}
	var candidates []marker
	for _, m := range explicit {
		candidates = append(candidates, marker{m[0], m[1], text[m[2]:m[3]], true})
	}
	// MinerU sometimes preserves only the first superscript and flattens the
	// remaining labels. Accept a plain label only after a sentence boundary,
	// before Chinese prose, and as the next letter in an anchored note sequence.
	for _, m := range flattenedTableNoteMarker.FindAllStringSubmatchIndex(text, -1) {
		candidates = append(candidates, marker{m[2], m[3], text[m[2]:m[3]], false})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].start < candidates[j].start })
	var markers []marker
	for _, m := range candidates {
		if !m.explicit && (len(markers) == 0 || m.label[0] != markers[len(markers)-1].label[0]+1) {
			continue
		}
		markers = append(markers, m)
	}
	seen := map[string]bool{}
	var notes []TableNote
	for i, marker := range markers {
		label := marker.label
		end := len(text)
		if i+1 < len(markers) {
			end = markers[i+1].start
		}
		body := strings.TrimSpace(text[marker.end:end])
		if seen[label] || body == "" {
			return nil
		}
		seen[label] = true
		notes = append(notes, TableNote{Label: label, Text: body})
	}
	return notes
}

// matchTableMetricNoteRow overrides broad saved/fallback rows only when one
// labelled note in the table independently corroborates the metric.
func matchTableMetricNoteRow(g *TableGrid, metric map[string]any) (TableRow, bool) {
	matches := tableMetricFootnoteMatcher(metric)
	var found TableRow
	matched := false
	for _, row := range g.Rows {
		text, ok := row.FullWidthText()
		if !ok {
			continue
		}
		for _, note := range TableCellNotes(text) {
			if !matches(note.Text) {
				continue
			}
			if matched {
				return TableRow{}, false
			}
			found, matched = row, true
		}
	}
	return found, matched
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
