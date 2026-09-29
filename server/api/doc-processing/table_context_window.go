package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// TableContextWindow is the read-time view of a table metric's context: the header
// rows, the matched data rows, and `radius` data rows either side of each match. It is
// never stored — kb.metrics.metric_context (and so search_document) holds only the
// matched rows, so a search for a neighbor row's text does not hit this metric.
type TableContextWindow struct {
	Line    int               `json:"line"`
	Caption string            `json:"caption,omitempty"`
	Columns []string          `json:"columns"`
	Rows    []TableContextRow `json:"rows"`
}

// TableContextRow is one row of a TableContextWindow.
type TableContextRow struct {
	ID    string   `json:"id"`
	Cells []string `json:"cells"`
	// FullWidth: the row is one cell spanning every column; Cells has one entry.
	FullWidth bool `json:"full_width,omitempty"`
	Header    bool `json:"header,omitempty"`
	Matched   bool `json:"matched,omitempty"`
}

func newTableContextRow(r TableRow) TableContextRow {
	if text, ok := r.FullWidthText(); ok {
		return TableContextRow{ID: r.ID, Cells: []string{text}, FullWidth: true}
	}
	return TableContextRow{ID: r.ID, Cells: r.Cells}
}

// TableContextRadius is the number of neighbor data rows shown on each side.
const TableContextRadius = 1

// BuildTableContextWindow returns the window for the rows `refs` cites on table line
// lineNo. ok is false when the content is not a parseable table or no cited data row
// exists in it.
func BuildTableContextWindow(lineNo int, content, caption string, refs []TableRowRef, radius int) (*TableContextWindow, bool) {
	g, err := ParseTableGrid(content)
	if err != nil {
		return nil, false
	}
	matched := map[int]bool{}
	for _, ref := range refs {
		if ref.Line != lineNo {
			continue
		}
		for _, id := range ref.Rows {
			if i := g.DataRowIndex(id); i >= 0 {
				matched[i] = true
			}
		}
	}
	if len(matched) == 0 {
		return nil, false
	}
	w := &TableContextWindow{Line: lineNo, Caption: caption, Columns: g.Columns}
	for _, h := range g.Headers {
		row := newTableContextRow(h)
		row.Header = true
		w.Rows = append(w.Rows, row)
	}
	for i, r := range g.Rows {
		near := false
		for m := range matched {
			if i >= m-radius && i <= m+radius {
				near = true
				break
			}
		}
		if near {
			row := newTableContextRow(r)
			row.Matched = matched[i]
			w.Rows = append(w.Rows, row)
		}
	}
	return w, true
}

// RenderNumbered renders the window in the same "<line>#<row id>: a | b" form the
// extraction LLM sees, with matched rows tagged "(matched)".
func (w *TableContextWindow) RenderNumbered() string {
	var b strings.Builder
	if w.Caption != "" {
		b.WriteString(w.Caption)
	}
	for _, r := range w.Rows {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strconv.Itoa(w.Line))
		b.WriteByte('#')
		b.WriteString(r.ID)
		if r.Matched {
			b.WriteString(" (matched)")
		}
		b.WriteString(": ")
		b.WriteString(strings.Join(r.Cells, " | "))
	}
	return b.String()
}

// TableCaptionBefore returns the table-caption line just before a table line in
// lines, if any (MinerU writes table-caption, table-image, table).
func TableCaptionBefore(lines []Line, tableLineNo int) string {
	return newTableLineIndex(lines).caption(tableLineNo)
}

// ParseTableRowRefs decodes a stored kb.metrics.source_table_rows value.
func ParseTableRowRefs(raw string) []TableRowRef { return parseTableRowRefs(raw) }

// LLMLineContent is the content an LLM should see for a line (tables as numbered
// rows, everything else unchanged).
func LLMLineContent(lineType string, lineNo int, content string) string {
	return llmLineContent(lineType, lineNo, content)
}

// LoadRecordLinesByID reads a record's line file (used to build table context
// windows at read time).
func LoadRecordLinesByID(ctx context.Context, db *sql.DB, recordID int64) ([]Line, error) {
	rec, err := (DocMetadataSQLStore{DB: db}).GetInputRecord(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("load record %d: %w", recordID, err)
	}
	return loadRecordLinesForTables(rec, recordID)
}

// TableContextWindows builds one window per table line cited by refs, with the
// table's caption, using TableContextRadius neighbor rows.
func TableContextWindows(lines []Line, refs []TableRowRef) []TableContextWindow {
	if len(refs) == 0 || len(lines) == 0 {
		return nil
	}
	idx := newTableLineIndex(lines)
	var out []TableContextWindow
	for _, ref := range refs {
		l, ok := idx.byNo[ref.Line]
		if !ok || l.LineType != "table" {
			continue
		}
		if w, ok := BuildTableContextWindow(ref.Line, l.Content, idx.caption(ref.Line), []TableRowRef{ref}, TableContextRadius); ok {
			out = append(out, *w)
		}
	}
	return out
}
