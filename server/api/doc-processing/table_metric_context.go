package docprocessing

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"golang.org/x/text/unicode/norm"
)

// TableRowRef records which logical rows of a table line an artifact came from
// (kb.metrics.source_table_rows). Row IDs are TableGrid IDs ("r1", "h0"); RowHash is
// always computed by the system from the grid, never taken from the LLM, so a stale
// reference (the table was reparsed and the row changed) can be detected.
type TableRowRef struct {
	Line    int               `json:"line"`
	Rows    []string          `json:"rows"`
	RowHash map[string]string `json:"row_hash,omitempty"`
}

// tableRowRefsFromValue normalizes the accepted shapes of source_table_rows into
// merged, sorted refs (one per line):
//   - LLM output: []any{"116#r1", "116#r2"}
//   - stored JSON: []any{map{"line":116,"rows":[...],"row_hash":{...}}}
//   - in-process: []TableRowRef
//
// Malformed entries are dropped.
func tableRowRefsFromValue(value any) []TableRowRef {
	var refs []TableRowRef
	switch v := value.(type) {
	case []TableRowRef:
		refs = append(refs, v...)
	case []string:
		for _, s := range v {
			if r, ok := parseTableRowCitation(s); ok {
				refs = append(refs, r)
			}
		}
	case []any:
		for _, item := range v {
			switch it := item.(type) {
			case string:
				if r, ok := parseTableRowCitation(it); ok {
					refs = append(refs, r)
				}
			case map[string]any:
				line := int(toFloat(it["line"]))
				if line <= 0 {
					continue
				}
				r := TableRowRef{Line: line}
				for _, id := range toStringSlice(it["rows"]) {
					if isTableRowID(id) {
						r.Rows = append(r.Rows, id)
					}
				}
				if hm, ok := it["row_hash"].(map[string]any); ok {
					r.RowHash = map[string]string{}
					for k, hv := range hm {
						r.RowHash[k] = asString(hv)
					}
				}
				if len(r.Rows) > 0 {
					refs = append(refs, r)
				}
			}
		}
	}
	return mergeTableRowRefs(refs)
}

// parseTableRowCitation parses "116#r1" into a single-row ref.
func parseTableRowCitation(s string) (TableRowRef, bool) {
	line, id, ok := strings.Cut(strings.TrimSpace(s), "#")
	if !ok {
		return TableRowRef{}, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	id = strings.TrimSpace(id)
	if err != nil || n <= 0 || !isTableRowID(id) {
		return TableRowRef{}, false
	}
	return TableRowRef{Line: n, Rows: []string{id}}, true
}

func isTableRowID(id string) bool {
	if len(id) < 2 || (id[0] != 'r' && id[0] != 'h') {
		return false
	}
	n, err := strconv.Atoi(id[1:])
	return err == nil && n >= 0 && (id[0] == 'h' || n >= 1)
}

// mergeTableRowRefs unions refs per line; row IDs are de-duplicated and sorted in
// grid order (h* before r*, numerically). Hashes are kept when present.
func mergeTableRowRefs(refs []TableRowRef) []TableRowRef {
	if len(refs) == 0 {
		return nil
	}
	byLine := map[int]*TableRowRef{}
	var lines []int
	for _, r := range refs {
		cur, ok := byLine[r.Line]
		if !ok {
			cur = &TableRowRef{Line: r.Line}
			byLine[r.Line] = cur
			lines = append(lines, r.Line)
		}
		for _, id := range r.Rows {
			if !containsString(cur.Rows, id) {
				cur.Rows = append(cur.Rows, id)
			}
			if h := r.RowHash[id]; h != "" {
				if cur.RowHash == nil {
					cur.RowHash = map[string]string{}
				}
				cur.RowHash[id] = h
			}
		}
	}
	sort.Ints(lines)
	out := make([]TableRowRef, 0, len(lines))
	for _, l := range lines {
		r := byLine[l]
		sort.Slice(r.Rows, func(i, j int) bool { return tableRowOrder(r.Rows[i]) < tableRowOrder(r.Rows[j]) })
		out = append(out, *r)
	}
	return out
}

func tableRowOrder(id string) int {
	n, _ := strconv.Atoi(id[1:])
	if id[0] == 'h' {
		return n
	}
	return 1000 + n
}

// parseTableRowRefs decodes the stored JSONB value; empty/invalid → nil.
func parseTableRowRefs(raw string) []TableRowRef {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return tableRowRefsFromValue(v)
}

// sourceTableRowsSQLValue is the bind value for kb.metrics.source_table_rows:
// NULL when the metric has no row references.
func sourceTableRowsSQLValue(value any) any {
	refs := tableRowRefsFromValue(value)
	if len(refs) == 0 {
		return nil
	}
	bs, err := json.Marshal(refs)
	if err != nil {
		return nil
	}
	return string(bs)
}

// Stored-context caps (design D5). Neighbor rows are never stored; see tableContextWindow.
const (
	tableContextMaxCellRunes  = 400
	tableContextMaxTotalRunes = 2000
	// tableContextSmallTable: when no row can be matched, a table with at most this
	// many data rows is included whole.
	tableContextSmallTable = 5
	// tableCaptionLookback is how many lines before a table line its captions may sit
	// (MinerU writes table-caption line(s), table-image, table).
	tableCaptionLookback = 5
)

// Row-selection outcomes, logged and counted by the backfill.
const (
	tableRowsFromStoredRefs = "stored_refs"
	tableRowsFromEvidence   = "evidence"
	tableRowsFromLLM        = "llm_rows"
	tableRowsWholeTable     = "whole_table"
	tableRowsUnmatched      = "unmatched"
)

// tableLineIndex gives the context builder access to a record's lines by number and
// caches parsed grids, so a record's tables are parsed once however many metrics
// point at them.
type tableLineIndex struct {
	byNo  map[int]Line
	grids map[int]*TableGrid
}

func newTableLineIndex(lines []Line) *tableLineIndex {
	idx := &tableLineIndex{byNo: make(map[int]Line, len(lines)), grids: map[int]*TableGrid{}}
	for _, l := range lines {
		idx.byNo[l.LineNo] = l
	}
	return idx
}

func (x *tableLineIndex) grid(lineNo int) (*TableGrid, bool) {
	if g, ok := x.grids[lineNo]; ok {
		return g, g != nil
	}
	l, ok := x.byNo[lineNo]
	if !ok || l.LineType != "table" {
		x.grids[lineNo] = nil
		return nil, false
	}
	g, err := ParseTableGrid(l.Content)
	if err != nil {
		g = nil
	}
	x.grids[lineNo] = g
	return g, g != nil
}

// caption returns the table-caption lines just before a table line, joined in
// document order. MinerU can emit several (e.g. "表 1 …" then "单位为毫米").
func (x *tableLineIndex) caption(tableLineNo int) string {
	var parts []string
	for n := tableLineNo - 1; n >= tableLineNo-tableCaptionLookback && n > 0; n-- {
		l, ok := x.byNo[n]
		if !ok {
			continue
		}
		if l.LineType == "table-caption" {
			if c := strings.TrimSpace(l.Content); c != "" {
				parts = append([]string{c}, parts...)
			}
			continue
		}
		if l.LineType != "table-image" {
			break
		}
	}
	return strings.Join(parts, " ")
}

// tableMetricContextResult is what buildTableMetricContext decided for one metric.
type tableMetricContextResult struct {
	Context  string        // "" when the metric has no parseable table line
	Refs     []TableRowRef // rows used, with system-computed hashes
	Outcome  string        // per the tableRowsFrom*/tableRows* constants, worst across tables
	HasTable bool
}

// buildTableMetricContext builds the stored metric_context for a metric whose
// source_line_spans cover a table line: the caption plus each matched row rendered as
// "[r1] column: value | …". Rows are chosen in this order (design D5): stored refs
// whose hashes still match → a single best evidence-matching row → LLM-cited rows that
// exist → the whole table when it has ≤5 data rows → no match (the LLM context is kept).
// When the spans also cover prose lines, the LLM context is kept and the table block
// is appended after it.
func buildTableMetricContext(idx *tableLineIndex, metric map[string]any) tableMetricContextResult {
	var res tableMetricContextResult
	var tableLines []int
	hasProse := false
	for _, span := range normalizeSourceLineSpans(metric["source_line_spans"]) {
		start, end, ok := parseMetricLineSpan(span)
		if !ok {
			continue
		}
		for n := start; n <= end; n++ {
			l, ok := idx.byNo[n]
			if !ok {
				continue
			}
			switch l.LineType {
			case "table":
				if _, ok := idx.grid(n); ok {
					tableLines = append(tableLines, n)
				}
			case "table-caption", "table-image", "image":
			default:
				hasProse = true
			}
		}
	}
	if len(tableLines) == 0 {
		return res
	}
	res.HasTable = true

	cited := map[int]TableRowRef{}
	for _, r := range tableRowRefsFromValue(metric["source_table_rows"]) {
		cited[r.Line] = r
	}
	evidence := tableMetricEvidence(metric)
	outcomeRank := map[string]int{tableRowsFromStoredRefs: 0, tableRowsFromEvidence: 1, tableRowsFromLLM: 2, tableRowsWholeTable: 3, tableRowsUnmatched: 4}
	res.Outcome = tableRowsFromStoredRefs

	var blocks []string
	for _, n := range tableLines {
		g, _ := idx.grid(n)
		rows, outcome := selectTableRows(g, cited[n], evidence)
		if outcomeRank[outcome] > outcomeRank[res.Outcome] {
			res.Outcome = outcome
		}
		if len(rows) == 0 {
			continue
		}
		ref := TableRowRef{Line: n, RowHash: map[string]string{}}
		var b strings.Builder
		if c := idx.caption(n); c != "" {
			b.WriteString(c)
		}
		for _, r := range rows {
			ref.Rows = append(ref.Rows, r.ID)
			ref.RowHash[r.ID] = r.Hash
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(renderTableRowPairs(g, r))
		}
		res.Refs = append(res.Refs, ref)
		blocks = append(blocks, b.String())
	}
	if len(blocks) == 0 {
		return res
	}
	ctx := truncateRunesEllipsis(strings.Join(blocks, "\n"), tableContextMaxTotalRunes)
	if hasProse {
		// Idempotent: a context that already carries this table block (a re-run, or an
		// existing row coming back through merge) is kept as is.
		if llmCtx := strings.TrimSpace(asString(metric["context"])); llmCtx != "" {
			if strings.Contains(llmCtx, ctx) {
				ctx = llmCtx
			} else {
				ctx = llmCtx + "\n" + ctx
			}
		}
	}
	res.Context = ctx
	return res
}

func selectTableRows(g *TableGrid, cited TableRowRef, evidence []string) ([]TableRow, string) {
	// 1. Stored refs whose hashes all still match the grid.
	if len(cited.Rows) > 0 && len(cited.RowHash) > 0 {
		var rows []TableRow
		for _, id := range cited.Rows {
			r, ok := g.Row(id)
			if !ok || cited.RowHash[id] != r.Hash {
				rows = nil
				break
			}
			rows = append(rows, r)
		}
		if len(rows) > 0 {
			return rows, tableRowsFromStoredRefs
		}
	}
	// 2. A single data row that matches the most evidence strings.
	if len(evidence) > 0 {
		best, bestScore, tie := -1, 0, false
		for i, r := range g.Rows {
			text := normalizeTableMatchText(strings.Join(r.Cells, ""))
			score := 0
			for _, e := range evidence {
				if strings.Contains(text, e) {
					score++
				}
			}
			switch {
			case score > bestScore:
				best, bestScore, tie = i, score, false
			case score == bestScore && score > 0:
				tie = true
			}
		}
		if best >= 0 && !tie {
			return []TableRow{g.Rows[best]}, tableRowsFromEvidence
		}
	}
	// 3. LLM-cited data rows that exist in the grid (hash not yet known or stale).
	var llmRows []TableRow
	for _, id := range cited.Rows {
		if r, ok := g.Row(id); ok && strings.HasPrefix(id, "r") {
			llmRows = append(llmRows, r)
		}
	}
	if len(llmRows) > 0 {
		return llmRows, tableRowsFromLLM
	}
	// 4. Small table: all data rows.
	if len(g.Rows) > 0 && len(g.Rows) <= tableContextSmallTable {
		return g.Rows, tableRowsWholeTable
	}
	return nil, tableRowsUnmatched
}

// tableMetricEvidence returns the normalized strings used to find a metric's row.
func tableMetricEvidence(metric map[string]any) []string {
	var out []string
	for _, key := range []string{"metric_name", "metric_value", "threshold_or_target", "condition"} {
		e := normalizeTableMatchText(asString(metric[key]))
		if utf8.RuneCountInString(e) >= 2 && !containsString(out, e) {
			out = append(out, e)
		}
	}
	return out
}

func normalizeTableMatchText(s string) string {
	s = strings.ToLower(norm.NFKC.String(s))
	return strings.Join(strings.Fields(s), "")
}

// renderTableRowPairs renders "[r1] column: value | column: value", skipping empty cells.
func renderTableRowPairs(g *TableGrid, r TableRow) string {
	if text, ok := r.FullWidthText(); ok {
		return "[" + r.ID + "] " + truncateRunesEllipsis(text, tableContextMaxCellRunes)
	}
	parts := make([]string, 0, len(r.Cells))
	for i, c := range r.Cells {
		if strings.TrimSpace(c) == "" || (i < len(r.Spans) && r.Spans[i] == 0) {
			continue // empty, or a colspan copy of the cell already rendered
		}
		label := ""
		if i < len(g.Columns) {
			label = g.Columns[i]
		}
		parts = append(parts, label+": "+truncateRunesEllipsis(c, tableContextMaxCellRunes))
	}
	return "[" + r.ID + "] " + strings.Join(parts, " | ")
}

func truncateRunesEllipsis(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// applyTableMetricContexts rebuilds context / source_table_rows for every metric that
// comes from a table line. Metrics without a table line, or whose rows cannot be
// matched, keep the LLM's context. Returns outcome counts.
func applyTableMetricContexts(lines []Line, metrics []map[string]any, logger ApiTypes.JimoLogger, recordID int64) map[string]int {
	counts := map[string]int{}
	if len(lines) == 0 || len(metrics) == 0 {
		return counts
	}
	idx := newTableLineIndex(lines)
	for _, m := range metrics {
		res := buildTableMetricContext(idx, m)
		if !res.HasTable {
			continue
		}
		counts[res.Outcome]++
		if res.Context == "" {
			if logger != nil {
				logger.Warn("table metric context: no row match",
					"record_id", recordID,
					"metric_id", asString(m["metric_id"]),
					"metric_name", asString(m["metric_name"]),
					"source_line_spans", normalizeSourceLineSpans(m["source_line_spans"]))
			}
			continue
		}
		m["context"] = res.Context
		m["metric_context"] = res.Context
		m["source_table_rows"] = res.Refs
	}
	return counts
}

// loadRecordLinesForTables reads a record's line file for the table context builder.
func loadRecordLinesForTables(rec DocMetadataInputRecord, recordID int64) ([]Line, error) {
	path, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("resolve line file: %w", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read line file: %w", err)
	}
	return ParseInputLines(body)
}

// applyTableMetricContextsForRecord is the processor hook run just before metrics are
// written. A failure to load lines is logged and leaves the LLM context in place.
func (p *MetricsProcessor) applyTableMetricContextsForRecord(rec DocMetadataInputRecord, recordID int64, metrics []map[string]any) {
	lines, err := loadRecordLinesForTables(rec, recordID)
	if err != nil {
		p.Logger.Warn("table metric context: lines unavailable; keeping LLM context",
			"record_id", recordID, "error", err)
		return
	}
	counts := applyTableMetricContexts(lines, metrics, p.Logger, recordID)
	if len(counts) > 0 {
		p.Logger.Info("table metric context applied", "record_id", recordID, "outcomes", counts)
	}
}
