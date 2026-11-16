package docprocessing

import (
	"context"
	"errors"
	"fmt"
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
var errMetricSimpleRowCitation = errors.New("split table output requires valid data-row citations in t")

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
				} else if _, full := row.FullWidthText(); !full {
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
	refs := tableRowRefsFromValue(raw["t"])
	if len(refs) == 0 {
		return false, errMetricSimpleRowCitation
	}
	for _, ref := range refs {
		var grid *TableGrid
		for position, g := range s.work.tables {
			if s.work.source[position].Line.LineNo == ref.Line {
				grid = g
				break
			}
		}
		if grid == nil {
			// Originally-overlap tables supply legitimate qualifying citations,
			// but cannot own any assertion in this source chunk.
			for _, line := range s.work.source {
				if line.Line.LineNo == ref.Line && line.Line.LineType == "table" {
					grid, _ = ParseTableGrid(line.Line.Content)
					break
				}
			}
			if grid == nil {
				return false, errMetricSimpleRowCitation
			}
		}
		for _, id := range ref.Rows {
			if _, ok := grid.Row(id); !ok {
				return false, errMetricSimpleRowCitation
			}
		}
	}
	var noteOwner *metricSimpleUnit
	for _, unit := range s.work.units {
		for _, ref := range refs {
			if ref.Line == s.work.source[unit.position].Line.LineNo && containsString(ref.Rows, unit.rowID) {
				row, _ := s.work.tables[unit.position].Row(unit.rowID)
				if _, full := row.FullWidthText(); full {
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
	return false, errMetricSimpleRowCitation
}

func (p *MetricsProcessor) extractSimpleSelection(ctx context.Context, recordID int64, chunk Chunk, s metricSimpleSelection, docCtx string, total, depth int, suffix string) (metricSimpleResult, error) {
	if err := context.Cause(ctx); err != nil {
		return metricSimpleResult{}, err
	}
	res, err := p.callSimpleChunkSelection(ctx, recordID, chunk, s.lines(), docCtx, total, suffix, &s)
	if err := context.Cause(ctx); err != nil {
		return metricSimpleResult{}, err
	}
	if err == nil {
		return res, nil
	}
	truncated := errors.Is(err, llmclients.ErrStructuredOutputTruncated) || errors.Is(err, llmclients.ErrLLMResponseTruncated)
	if len(s.units) < 2 || depth >= metricSimpleMaxSplitDepth || (!truncated && depth > 0) || errors.Is(err, errMetricSimpleRowCitation) {
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
