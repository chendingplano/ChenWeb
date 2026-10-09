package docprocessing

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

// TableGrid is the normalized form of a single-line HTML table (MinerU emits a whole
// table as one `table` line). rowspan/colspan cells are expanded so every row carries
// one value per column and reads on its own. Rows are addressed by logical IDs —
// header rows h0, h1, …; data rows r1, r2, … — so artifacts can reference a row
// without splitting the table into physical lines (which would renumber every line
// after it and invalidate stored source_line_spans).
type TableGrid struct {
	Columns []string   // one label per column: header cells joined top-down with "/"
	Headers []TableRow // IDs h0, h1, …
	Rows    []TableRow // data rows, IDs r1, r2, …
}

// TableRow is one expanded row of a TableGrid.
type TableRow struct {
	ID    string
	Cells []string
	// Spans[i] is how many columns the source cell starting at column i covers, or 0
	// when column i is a colspan copy of the cell to its left.
	Spans []int
	Hash  string // first 12 hex of SHA-1 over NFKC-normalized cells joined by \x1f
}

// FullWidthText returns the row's text when the whole row is one colspan cell (a
// footnote or section row), so renderers can show it once instead of per column.
func (r TableRow) FullWidthText() (string, bool) {
	if len(r.Cells) > 1 && len(r.Spans) > 0 && r.Spans[0] >= len(r.Cells) {
		return r.Cells[0], true
	}
	return "", false
}

// maxTableHeaderRows bounds the multi-level header heuristic so a table whose
// data rows use colspan is not swallowed into its header.
const maxTableHeaderRows = 3

var errNotATable = errors.New("table grid: no <tr> rows found")

type rawTableRow struct {
	cells      []string
	spans      []int
	hasTH      bool
	hasColspan bool
	maxRowspan int
}

// ParseTableGrid parses the HTML content of a table line into a TableGrid.
func ParseTableGrid(content string) (*TableGrid, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("table grid: parse html: %w", err)
	}
	var trs []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			trs = append(trs, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(trs) == 0 {
		return nil, errNotATable
	}

	// pending[col] carries a rowspan cell down into the following rows.
	type carry struct {
		text string
		left int
	}
	pending := map[int]*carry{}
	raws := make([]rawTableRow, 0, len(trs))
	width := 0
	for _, tr := range trs {
		var row rawTableRow
		col := 0
		place := func(text string, span int) {
			for len(row.cells) <= col {
				row.cells = append(row.cells, "")
				row.spans = append(row.spans, 1)
			}
			row.cells[col] = text
			row.spans[col] = span
			col++
		}
		fillCarried := func() {
			for {
				p, ok := pending[col]
				if !ok {
					return
				}
				place(p.text, 1)
				p.left--
				if p.left <= 0 {
					delete(pending, col-1)
				}
			}
		}
		for c := tr.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || (c.Data != "td" && c.Data != "th") {
				continue
			}
			fillCarried()
			if c.Data == "th" {
				row.hasTH = true
			}
			text := tableCellText(c)
			rs := tableSpanAttr(c, "rowspan")
			cs := tableSpanAttr(c, "colspan")
			if cs > 1 {
				row.hasColspan = true
			}
			if rs > row.maxRowspan {
				row.maxRowspan = rs
			}
			for i := 0; i < cs; i++ {
				if rs > 1 {
					pending[col] = &carry{text: text, left: rs - 1}
				}
				span := 0
				if i == 0 {
					span = cs
				}
				place(text, span)
			}
		}
		fillCarried() // rowspans that reach past the row's last explicit cell
		if len(row.cells) > width {
			width = len(row.cells)
		}
		raws = append(raws, row)
	}
	for i := range raws {
		for len(raws[i].cells) < width {
			raws[i].cells = append(raws[i].cells, "")
			raws[i].spans = append(raws[i].spans, 1)
		}
	}

	headerCount := 0
	for headerCount < len(raws) && raws[headerCount].hasTH {
		headerCount++
	}
	if headerCount == 0 {
		headerCount = 1
	}
	// A header row with colspan is followed by its sub-headers, and a header cell with
	// rowspan covers the rows below it (a unit row under "项目 | tA | Δt").
	spansPastHeader := func() bool {
		for i := 0; i < headerCount; i++ {
			if i+raws[i].maxRowspan > headerCount {
				return true
			}
		}
		return false
	}
	for headerCount < len(raws)-1 && headerCount < maxTableHeaderRows && (raws[headerCount-1].hasColspan || spansPastHeader()) {
		headerCount++
	}
	if headerCount > len(raws) {
		headerCount = len(raws)
	}

	g := &TableGrid{}
	for i, r := range raws {
		if i < headerCount {
			g.Headers = append(g.Headers, TableRow{ID: "h" + strconv.Itoa(i), Cells: r.cells, Spans: r.spans, Hash: tableRowHash(r.cells)})
		} else {
			g.Rows = append(g.Rows, TableRow{ID: "r" + strconv.Itoa(i-headerCount+1), Cells: r.cells, Spans: r.spans, Hash: tableRowHash(r.cells)})
		}
	}
	g.Columns = make([]string, width)
	for col := 0; col < width; col++ {
		var parts []string
		for _, h := range g.Headers {
			t := h.Cells[col]
			if t == "" || (len(parts) > 0 && parts[len(parts)-1] == t) {
				continue
			}
			parts = append(parts, t)
		}
		if len(parts) == 0 {
			g.Columns[col] = "#" + strconv.Itoa(col+1)
		} else {
			g.Columns[col] = strings.Join(parts, "/")
		}
	}
	return g, nil
}

// Row returns the header or data row with the given ID.
func (g *TableGrid) Row(id string) (TableRow, bool) {
	for _, set := range [][]TableRow{g.Headers, g.Rows} {
		for _, r := range set {
			if r.ID == id {
				return r, true
			}
		}
	}
	return TableRow{}, false
}

// dataRowByHash returns the first data row with the given hash.
func (g *TableGrid) dataRowByHash(hash string) (TableRow, bool) {
	for _, r := range g.Rows {
		if hash != "" && r.Hash == hash {
			return r, true
		}
	}
	return TableRow{}, false
}

// DataRowIndex returns the 0-based index of a data row ID in g.Rows, or -1.
func (g *TableGrid) DataRowIndex(id string) int {
	for i, r := range g.Rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}

// RenderNumbered renders the grid as one text line per row:
// "<line>#<row id>: cell | cell | …". This is the form sent to LLMs in place of the
// raw HTML (see canonicalChunkInputText); it is fully determined by the HTML.
func (g *TableGrid) RenderNumbered(lineNo int) string {
	var b strings.Builder
	write := func(r TableRow) {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strconv.Itoa(lineNo))
		b.WriteByte('#')
		b.WriteString(r.ID)
		b.WriteString(": ")
		if text, ok := r.FullWidthText(); ok {
			b.WriteString(text)
		} else {
			b.WriteString(strings.Join(r.Cells, " | "))
		}
	}
	for _, r := range g.Headers {
		write(r)
	}
	for _, r := range g.Rows {
		write(r)
	}
	return b.String()
}

// renderTableLineContent returns the numbered-row form of a table line's content,
// or the original content when it cannot be parsed as a table.
func renderTableLineContent(lineNo int, content string) string {
	g, err := ParseTableGrid(content)
	if err != nil {
		return content
	}
	return g.RenderNumbered(lineNo)
}

func tableCellText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(n.Data)
		case n.Type == html.ElementNode && (n.Data == "br" || n.Data == "p" || n.Data == "div"):
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func tableSpanAttr(n *html.Node, name string) int {
	for _, a := range n.Attr {
		if a.Key == name {
			v, err := strconv.Atoi(strings.TrimSpace(a.Val))
			if err != nil || v < 1 {
				return 1
			}
			return v
		}
	}
	return 1
}

func tableRowHash(cells []string) string {
	normalized := make([]string, len(cells))
	for i, c := range cells {
		normalized[i] = norm.NFKC.String(c)
	}
	sum := sha1.Sum([]byte(strings.Join(normalized, "\x1f")))
	return hex.EncodeToString(sum[:])[:12]
}

var tableRowRefSuffix = regexp.MustCompile(`#[hr]\d+`)

// stripTableRowRefs removes "#r1"/"#h0" row suffixes from a line span ("116#r1" →
// "116"). Chunk input labels table rows as "<line>#<row id>", so an LLM may echo that
// form into source_line_spans, which must stay plain line numbers.
func stripTableRowRefs(span string) string {
	if !strings.Contains(span, "#") {
		return span
	}
	return tableRowRefSuffix.ReplaceAllString(span, "")
}
