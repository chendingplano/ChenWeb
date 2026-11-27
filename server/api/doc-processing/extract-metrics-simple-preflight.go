package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const (
	metricSimpleUnknownOutputLimit = 4096 // planning only; never overrides the provider request
	metricSimpleMaxSplitDepth      = 3
)

var metricSimpleNumbers = regexp.MustCompile(`[+-]?\d+(?:[.,]\d+)?`)
var errMetricSimpleRowCitation = errors.New("split table output requires valid value-row citations in t (ChenWeb#68)")

type metricSimpleOverlapCitation struct{ line int }

func (e *metricSimpleOverlapCitation) Error() string {
	return fmt.Sprintf("%v: table %d is overlap context; extract the value from a normal table row and cite that row in t",
		errMetricSimpleRowCitation, e.line)
}

func (e *metricSimpleOverlapCitation) Unwrap() error { return errMetricSimpleRowCitation }

// A unit is indivisible: one prose line or one original, expanded table row.
// position indexes the original source, not a synthetic line number.
type metricSimpleUnit struct {
	position int
	rowID    string
	tokens   int
}

type metricSimpleWork struct {
	source []MarkedLine
	tables map[int]*TableGrid
	units  []metricSimpleUnit
	// Recovery can limit which rows of one overlap table are visible for citation.
	overlapTableOverride map[int]*TableGrid
}

type metricSimpleSelection struct {
	work  *metricSimpleWork
	units []metricSimpleUnit
}

// This is a conservative output heuristic, not a tokenizer. Each numeric occurrence
// may become an assertion; count repeated values and range endpoints separately.
// Allow 128 tokens per assertion for JSON/metadata, two per source rune, and repeat
// the row's labels/column names in every assertion's name, subject and condition.
func estimateMetricSimpleOutput(text, labels string) int {
	assertions := max(1, len(metricSimpleNumbers.FindAllStringIndex(text, -1)))
	return assertions*(128+2*utf8.RuneCountInString(labels)) + 2*utf8.RuneCountInString(text)
}

func newMetricSimpleWork(lines []MarkedLine) *metricSimpleWork {
	w := &metricSimpleWork{source: lines, tables: map[int]*TableGrid{}}
	for i, line := range lines {
		if normalizeChunkFlag(line.Mark) != "n" {
			continue
		}
		if line.Line.LineType == "table" {
			if grid, err := ParseTableGrid(line.Line.Content); err == nil && len(grid.Rows) > 0 {
				addMetricTableParagraphs(grid)
				w.tables[i] = grid
				for _, row := range grid.Rows {
					text := strings.Join(row.Cells, " | ")
					if full, ok := row.FullWidthText(); ok {
						text = full
					}
					labels := strings.Join(grid.Columns, " | ")
					for _, cell := range row.Cells {
						if !metricSimpleNumbers.MatchString(cell) {
							labels += " | " + cell
						}
					}
					w.units = append(w.units, metricSimpleUnit{i, row.ID, estimateMetricSimpleOutput(text, labels)})
				}
				continue
			}
		}
		w.units = append(w.units, metricSimpleUnit{i, "", estimateMetricSimpleOutput(line.Line.Content, "")})
	}
	return w
}

// Numeric data and quoted requirements can be labelled as headers when blank
// cells span several rows. Give quantitative rows an owner without changing
// the stable row IDs used by the full table (ChenWeb#60, ChenWeb#68).
func addMetricTableParagraphs(grid *TableGrid) {
	var paragraphs []TableRow
	grid.Headers = slices.DeleteFunc(grid.Headers, func(row TableRow) bool {
		if !metricSimpleNumbers.MatchString(strings.Join(row.Cells, " ")) {
			return false
		}
		paragraphs = append(paragraphs, row)
		return true
	})
	if len(paragraphs) > 0 {
		grid.Rows = append(paragraphs, grid.Rows...)
	}
}

func metricTableParagraphText(row TableRow) string {
	merged := ""
	for i, cell := range row.Cells {
		if row.Spans[i] == 0 || strings.TrimSpace(cell) == "" {
			continue
		}
		if row.Spans[i] == 1 || merged != "" {
			return ""
		}
		merged = cell
	}
	if !strings.ContainsAny(merged, ";；。") || len(metricSimpleNumbers.FindAllStringIndex(merged, 2)) < 2 {
		return ""
	}
	return merged
}

func (w *metricSimpleWork) plan(limit int) ([]metricSimpleSelection, int) {
	if limit <= 0 {
		limit = metricSimpleUnknownOutputLimit
	}
	budget := max(1, limit/2)
	var parts []metricSimpleSelection
	start, tokens := 0, 0
	for i, unit := range w.units {
		if i > start && tokens+unit.tokens > budget {
			parts = append(parts, metricSimpleSelection{w, w.units[start:i]})
			start, tokens = i, 0
		}
		tokens += unit.tokens
	}
	// Keep the old one-call behavior even for an empty/overlap-only chunk.
	parts = append(parts, metricSimpleSelection{w, w.units[start:]})
	return parts, budget
}

func (s metricSimpleSelection) estimate() int {
	n := 0
	for _, unit := range s.units {
		n += unit.tokens
	}
	return n
}

func (s metricSimpleSelection) lines() []MarkedLine {
	if len(s.units) == len(s.work.units) {
		return s.work.source
	}
	selected := map[int]map[string]bool{}
	for _, unit := range s.units {
		if selected[unit.position] == nil {
			selected[unit.position] = map[string]bool{}
		}
		selected[unit.position][unit.rowID] = true
	}
	var lines []MarkedLine
	for i, original := range s.work.source {
		line := original
		line.Mark = "o"
		owned := selected[i]
		if len(owned) > 0 {
			line.Mark = "n"
		}
		if grid := s.work.tables[i]; grid != nil && len(owned) > 0 && len(owned) < len(grid.Rows) {
			part := *grid // retain original Columns, IDs, Cells and Spans; never reparse subset HTML
			part.Rows = nil
			mark := ""
			flush := func() {
				if len(part.Rows) == 0 {
					return
				}
				fragment := original
				fragment.Mark = mark
				fragment.Line.Content = part.RenderNumbered(original.Line.LineNo)
				lines = append(lines, fragment)
				part.Rows = nil
			}
			for _, row := range grid.Rows {
				rowMark := "o"
				if owned[row.ID] {
					rowMark = "n"
				} else if _, full := row.FullWidthText(); !full && !strings.HasPrefix(row.ID, "h") {
					continue
				}
				// Keep notes and section labels interleaved in source order. Moving
				// a later section before a value changes which section governs it.
				if rowMark != mark {
					flush()
					mark = rowMark
				}
				part.Rows = append(part.Rows, row)
			}
			flush()
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// Table fragments are rendered text on the wire; source expansion must still use
// the original HTML, otherwise table matching and saved context lose their grid.
func (s metricSimpleSelection) sourceLines(request []MarkedLine) []MarkedLine {
	normal := map[int]bool{}
	for _, line := range request {
		if normalizeChunkFlag(line.Mark) == "n" {
			normal[line.Line.LineNo] = true
		}
	}
	lines := append([]MarkedLine(nil), s.work.source...)
	for i := range lines {
		lines[i].Mark = "o"
		if normal[lines[i].Line.LineNo] {
			lines[i].Mark = "n"
		}
	}
	return lines
}

// checkCitations runs accepts on every metric (m) and set-aside item (x) the response
// assigns to normal lines, as callSimpleChunkSelection does, and names each item it
// rejects. It is the contract's CheckParsed, so a citation slip is retried on the
// primary with this text instead of failing the segment (ChenWeb#29).
func (s metricSimpleSelection) checkCitations(payload map[string]any, idx simpleChunkLines) error {
	var errs []error
	for _, key := range []string{"m", "x"} {
		items, _ := payload[key].([]any)
		for i, item := range items {
			raw, ok := item.(map[string]any)
			if !ok {
				continue
			}
			spans := normalizeSourceLineSpans(raw["l"])
			if !idx.hasNormalLine(spans) {
				continue
			}
			accept := s.accepts
			if key == "x" {
				accept = s.acceptsSetAside
			}
			if _, err := accept(maps.Clone(raw), spans); err != nil {
				t, _ := json.Marshal(raw["t"])
				errs = append(errs, fmt.Errorf("%s[%d] %q cites t=%s: %w", key, i, strings.TrimSpace(asString(raw["n"])), t, err))
			}
		}
	}
	return errors.Join(errs...)
}

// For a split table, require explicit row citations before full-source inference.
// A multi-row assertion belongs to the first cited data unit in source order;
// full-width qualifying notes do not take ownership from a data row. A standalone
// note belongs to its own unit. Invalid/missing citations fail the
// group rather than silently discarding a potentially valid metric.
func (s metricSimpleSelection) accepts(raw map[string]any, spans []string) (bool, error) {
	selected := map[metricSimpleUnit]bool{}
	for _, unit := range s.units {
		selected[unit] = true
	}
	idx := newSimpleChunkLines(s.work.source)
	citedLines := idx.lineNumbers(spans)
	needsRows := false
	for position, grid := range s.work.tables {
		lineNo := s.work.source[position].Line.LineNo
		if !slices.Contains(citedLines, lineNo) {
			continue
		}
		count := 0
		for _, unit := range s.units {
			if unit.position == position {
				count++
			}
		}
		if count < len(grid.Rows) {
			needsRows = true
		}
	}
	if !needsRows {
		return true, nil
	}
	refs := simpleMetricRowRefs(raw)
	if len(refs) == 0 {
		// An uncited value can belong to a quoted paragraph embedded in the
		// table. Recover only a unique paragraph containing the complete
		// quantity, never a bare number repeated in ordinary cells (ChenWeb#68).
		var recovered []TableRowRef
		for position, grid := range s.work.tables {
			line := s.work.source[position].Line.LineNo
			if !slices.Contains(citedLines, line) {
				continue
			}
			for _, row := range grid.Rows {
				if metricTableParagraphText(row) == "" {
					continue
				}
				value, unit := asString(raw["v"]), asString(raw["u"])
				if unit != "" {
					value = strings.TrimSuffix(value, unit)
				}
				cells, _ := tableMetricHeaderQuantity(row, map[string]any{"metric_value": value, "unit": unit})
				if len(cells) > 0 {
					recovered = append(recovered, TableRowRef{Line: line, Rows: []string{row.ID}})
				}
			}
		}
		if len(recovered) == 1 {
			refs = recovered
			raw["t"] = []any{fmt.Sprintf("%d#%s", refs[0].Line, refs[0].Rows[0])}
		}
	}
	if len(refs) == 0 {
		return false, errMetricSimpleRowCitation
	}
	var overlapDataLine int
	for _, ref := range refs {
		var grid *TableGrid
		for position, g := range s.work.tables {
			if s.work.source[position].Line.LineNo == ref.Line {
				grid = g
				break
			}
		}
		overlap := grid == nil
		if overlap {
			// Originally-overlap tables supply legitimate qualifying citations,
			// but cannot own any assertion in this source chunk.
			grid = s.work.overlapTableOverride[ref.Line]
			if grid == nil {
				for _, line := range s.work.source {
					if line.Line.LineNo == ref.Line && line.Line.LineType == "table" {
						grid, _ = ParseTableGrid(line.Line.Content)
						break
					}
				}
			}
			if grid == nil {
				return false, errMetricSimpleRowCitation
			}
		}
		for _, id := range ref.Rows {
			row, ok := grid.Row(id)
			if !ok {
				if s.work.overlapTableOverride[ref.Line] != nil {
					return false, fmt.Errorf("table %d row %s is outside the recovery request", ref.Line, id)
				}
				return false, errMetricSimpleRowCitation
			}
			if overlap && strings.HasPrefix(row.ID, "r") {
				if _, fullWidth := row.FullWidthText(); !fullWidth {
					overlapDataLine = ref.Line
				}
			}
		}
	}
	var noteOwner *metricSimpleUnit
	for _, unit := range s.work.units {
		for _, ref := range refs {
			if ref.Line == s.work.source[unit.position].Line.LineNo && containsString(ref.Rows, unit.rowID) {
				row, _ := s.work.tables[unit.position].Row(unit.rowID)
				if _, full := row.FullWidthText(); full || strings.HasPrefix(row.ID, "h") {
					if noteOwner == nil {
						owner := unit
						noteOwner = &owner
					}
					continue
				}
				return selected[unit], nil
			}
		}
	}
	if noteOwner != nil {
		return selected[*noteOwner], nil
	}
	if overlapDataLine != 0 {
		return false, &metricSimpleOverlapCitation{line: overlapDataLine}
	}
	return false, errMetricSimpleRowCitation
}

// A set-aside item may be about the table itself (its title, header or a column's range),
// so it can cite a header row or no row, and no data row owns it. The segment holding the
// first unit of the cited table keeps it and the others drop it, so it is recorded once.
// A metric needs a value, hence a data row, and never takes this path.
func (s metricSimpleSelection) acceptsSetAside(raw map[string]any, spans []string) (bool, error) {
	accept, err := s.accepts(raw, spans)
	if !errors.Is(err, errMetricSimpleRowCitation) {
		return accept, err
	}
	citedLines := newSimpleChunkLines(s.work.source).lineNumbers(spans)
	for _, unit := range s.work.units {
		if _, ok := s.work.tables[unit.position]; ok && slices.Contains(citedLines, s.work.source[unit.position].Line.LineNo) {
			return slices.Contains(s.units, unit), nil
		}
	}
	return false, err
}

func (p *MetricsProcessor) extractSimpleSelection(ctx context.Context, recordID int64, chunk Chunk, s metricSimpleSelection, docCtx string, total, depth int, suffix string) (metricSimpleResult, error) {
	if err := context.Cause(ctx); err != nil {
		return metricSimpleResult{}, err
	}
	lines := s.lines()
	res, err := p.callSimpleChunkSelection(ctx, recordID, chunk, lines, docCtx, total, suffix, &s)
	if err := context.Cause(ctx); err != nil {
		return metricSimpleResult{}, err
	}
	if err == nil {
		return res, nil
	}
	var overlap *metricSimpleOverlapCitation
	if errors.Is(err, llmclients.ErrStructuredOutputRetriesExhausted) && errors.As(err, &overlap) {
		// Keep the original context for the normal attempts. After those fail,
		// remove the confusing data rows while keeping headers, notes and IDs.
		for i, line := range lines {
			if line.Line.LineNo != overlap.line || line.Line.LineType != "table" || normalizeChunkFlag(line.Mark) != "o" {
				continue
			}
			grid, parseErr := ParseTableGrid(line.Line.Content)
			if parseErr != nil {
				break
			}
			grid.Rows = slices.DeleteFunc(grid.Rows, func(row TableRow) bool {
				_, fullWidth := row.FullWidthText()
				return !fullWidth
			})
			recoveryLines := slices.Clone(lines)
			recoveryLines[i].Line.Content = grid.RenderNumbered(overlap.line)
			work := *s.work
			work.overlapTableOverride = map[int]*TableGrid{overlap.line: grid}
			recovery := metricSimpleSelection{&work, s.units}
			p.Logger.Info("extract metrics simple: retrying without overlap table data rows (ChenWeb#46)",
				"record_id", recordID, "chunk", chunk.SeqNo, "segment", suffix, "overlap_table", overlap.line)
			res, err = p.callSimpleChunkSelection(ctx, recordID, chunk, recoveryLines, docCtx, total, suffix+"_context_retry", &recovery)
			if cause := context.Cause(ctx); cause != nil {
				return metricSimpleResult{}, cause
			}
			if err != nil {
				return metricSimpleResult{}, err
			}
			res.calls++ // the initial structured call exhausted its retries
			return res, nil
		}
	}
	truncated := errors.Is(err, llmclients.ErrStructuredOutputTruncated) || errors.Is(err, llmclients.ErrLLMResponseTruncated)
	if len(s.units) < 2 || depth >= metricSimpleMaxSplitDepth || (!truncated && depth > 0) ||
		errors.Is(err, errMetricSimpleRowCitation) || isProviderAccountError(err) {
		return metricSimpleResult{}, err
	}
	p.Logger.Warn("extract metrics simple: splitting failed segment", "record_id", recordID,
		"chunk", chunk.SeqNo, "segment", suffix, "depth", depth, "truncated", truncated, "error", err)
	mid := len(s.units) / 2
	a, errA := p.extractSimpleSelection(ctx, recordID, chunk, metricSimpleSelection{s.work, s.units[:mid]}, docCtx, total, depth+1, suffix+"_h1")
	if errA != nil {
		return metricSimpleResult{}, fmt.Errorf("(MID-20261116-701) segment split: %w", errA)
	}
	b, errB := p.extractSimpleSelection(ctx, recordID, chunk, metricSimpleSelection{s.work, s.units[mid:]}, docCtx, total, depth+1, suffix+"_h2")
	if errB != nil {
		return metricSimpleResult{}, fmt.Errorf("(MID-20261116-702) segment split: %w", errB)
	}
	a.add(b)
	a.calls++ // failed parent call
	return a, nil
}
