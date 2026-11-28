package docprocessing

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// refTableRowLocation identifies occurrences within a table independently of row hashes.
func refTableRowLocation(refs []TableRowRef) string {
	var parts []string
	for _, ref := range mergeTableRowRefs(refs) {
		for _, row := range ref.Rows {
			parts = append(parts, fmt.Sprintf("%d#%s", ref.Line, row))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "|" + strings.Join(parts, ",")
}

// applyReferenceTableRows shares metrics' grid, hashes and context renderer. Explicit row
// citations are validated and hashed before context selection, so a citation spanning several
// rows retains every cited row. Model-provided hashes are never trusted.
func applyReferenceTableRows(lines []Line, mentions []RefMention, logger ApiTypes.JimoLogger, recordID int64) {
	idx := newTableLineIndex(lines)
	counts := map[string]int{}
	for i := range mentions {
		m := &mentions[i]
		var refs []TableRowRef
		for _, ref := range m.SourceTableRows {
			g, ok := idx.grid(ref.Line)
			if !ok {
				continue
			}
			valid := TableRowRef{Line: ref.Line, RowHash: map[string]string{}}
			for _, id := range ref.Rows {
				if row, ok := g.Row(id); ok {
					valid.Rows = append(valid.Rows, id)
					valid.RowHash[id] = row.Hash
				}
			}
			if len(valid.Rows) > 0 {
				refs = append(refs, valid)
			}
		}
		// Without header markup the grid labels the first reference as h0.
		// A unique exact identifier can recover that row without renumbering
		// the table or changing an ambiguous citation (ChenWeb#59).
		if len(m.SourceLineSpans) == 1 {
			line, end, ok := parseMetricLineSpan(m.SourceLineSpans[0])
			if ok && line == end {
				if grid, ok := idx.grid(line); ok {
					if row, ok := listedReferenceHeaderRow(grid, m.CitedRaw); ok {
						refs = []TableRowRef{{Line: line, Rows: []string{row.ID}, RowHash: map[string]string{row.ID: row.Hash}}}
					}
				}
			}
		}
		res := buildTableMetricContext(idx, map[string]any{
			"source_line_spans": m.SourceLineSpans,
			"source_table_rows": refs,
			"context":           m.Evidence,
			"metric_name":       m.CitedRaw,
			"metric_value":      m.Evidence,
			"condition":         m.MetricName,
		})
		// The builder accepts only rows belonging to table lines in the source spans.
		m.SourceTableRows = res.Refs
		if !res.HasTable {
			continue
		}
		counts[res.Outcome]++
		if res.Context != "" {
			m.Evidence = res.Context
		} else if logger != nil {
			logger.Warn("reference table rows: no row match (ChenWeb#59)", "record_id", recordID,
				"cited_raw", m.CitedRaw, "source_line_spans", m.SourceLineSpans)
		}
	}
	if len(counts) > 0 && logger != nil {
		logger.Info("reference table rows applied", "record_id", recordID, "outcomes", counts)
	}
}

func listedReferenceHeaderRow(grid *TableGrid, cited string) (TableRow, bool) {
	id := normalizeTableMatchText(cited)
	if id == "" || len(grid.Headers) != 1 {
		return TableRow{}, false
	}
	matches := func(row TableRow) bool {
		return slices.ContainsFunc(row.Cells, func(cell string) bool {
			return normalizeTableMatchText(strings.TrimLeft(cell, " \t—–-•·")) == id
		})
	}
	header := grid.Headers[0]
	return header, matches(header) && !slices.ContainsFunc(grid.Rows, matches)
}
