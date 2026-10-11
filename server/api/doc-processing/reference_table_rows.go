package docprocessing

import (
	"fmt"
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
			logger.Warn("reference table rows: no row match", "record_id", recordID,
				"cited_raw", m.CitedRaw, "source_line_spans", m.SourceLineSpans)
		}
	}
	if len(counts) > 0 && logger != nil {
		logger.Info("reference table rows applied", "record_id", recordID, "outcomes", counts)
	}
}
