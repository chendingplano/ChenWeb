// Package documentgeometry maps physical PDF table borders to canonical line IDs.
package documentgeometry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"golang.org/x/text/unicode/norm"
)

const Version = 1
const Algorithm = "canonical-table-geometry-v2"

type Box struct {
	Rotation int       `json:"rotation,omitempty"`
	Page     int       `json:"page"`
	Coords   []float64 `json:"coords"`
}
type Cell struct {
	ID    string `json:"id"`
	Boxes []Box  `json:"boxes"`
}
type Row struct {
	ID    string `json:"id"`
	Hash  string `json:"hash"`
	Boxes []Box  `json:"boxes"`
	Cells []Cell `json:"cells"`
}
type Table struct {
	Line int   `json:"line"`
	Rows []Row `json:"rows"`
}
type Document struct {
	PhysicalHash    string  `json:"physical_sha256"`
	Version         int     `json:"version"`
	Algorithm       string  `json:"algorithm"`
	CoordinateSpace string  `json:"coordinate_space"`
	LineHash        string  `json:"line_sha256"`
	PDFHash         string  `json:"pdf_sha256"`
	Tables          []Table `json:"tables"`
}
type PhysicalCell struct {
	Text   string    `json:"text"`
	Coords []float64 `json:"coords"`
}
type PhysicalRow struct {
	Coords []float64      `json:"coords"`
	Cells  []PhysicalCell `json:"cells"`
}
type PhysicalTable struct {
	Rotation int           `json:"rotation,omitempty"`
	Page     int           `json:"page"`
	Coords   []float64     `json:"coords"`
	Rows     []PhysicalRow `json:"rows"`
}
type PhysicalDocument struct {
	Version   int             `json:"version"`
	Extractor string          `json:"extractor"`
	PDFHash   string          `json:"pdf_sha256"`
	Tables    []PhysicalTable `json:"tables"`
}

func hash(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
func normalized(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSymbol(r) || strings.ContainsRune(".-/%", r) {
			return unicode.ToLower(r)
		}
		return -1
	}, norm.NFKC.String(s))
}
func validBox(c []float64) bool {
	if len(c) != 4 {
		return false
	}
	for _, v := range c {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1000 {
			return false
		}
	}
	return c[2] > c[0] && c[3] > c[1]
}

// score permits a physical fragment's text to be part of its canonical cell.
// Every nonempty column must agree, so a repeated short value cannot select a row.
func score(row docprocessing.TableRow, physical PhysicalRow) int {
	if len(row.Cells) != len(physical.Cells) {
		return 0
	}
	total := 0
	for i, c := range physical.Cells {
		a, b := normalized(row.Cells[i]), normalized(c.Text)
		if b == "" {
			continue
		}
		if a != b && !(len([]rune(b)) >= 8 && strings.Contains(a, b)) {
			return 0
		}
		total += len([]rune(b))
	}
	if total < 4 {
		return 0
	}
	return total
}

type canonicalTable struct {
	line, page, anchor int
	coords             []float64
	grid               *docprocessing.TableGrid
}

func Build(lineData []byte, physical PhysicalDocument) (*Document, error) {
	physicalBytes, err := json.Marshal(physical)
	if err != nil {
		return nil, err
	}
	out := &Document{PhysicalHash: hash(physicalBytes), Version: Version, Algorithm: Algorithm, CoordinateSpace: "page-normalized-1000", LineHash: hash(lineData), PDFHash: physical.PDFHash, Tables: []Table{}}
	if physical.Version != Version || physical.Extractor != "pymupdf-tables-v1" {
		return nil, fmt.Errorf("unsupported physical table geometry version/extractor")
	}
	physical.Tables = append([]PhysicalTable(nil), physical.Tables...)
	sort.SliceStable(physical.Tables, func(i, j int) bool {
		a, b := physical.Tables[i], physical.Tables[j]
		if a.Page != b.Page {
			return a.Page < b.Page
		}
		return len(a.Coords) == 4 && len(b.Coords) == 4 && a.Coords[1] < b.Coords[1]
	})
	var canonical []canonicalTable
	for _, line := range strings.Split(string(lineData), "\n") {
		fields := strings.SplitN(line, "\t", 7)
		if len(fields) != 7 || fields[2] != "table" {
			continue
		}
		grid, err := docprocessing.ParseTableGrid(fields[6])
		if err != nil {
			continue
		}
		n, e1 := strconv.Atoi(fields[0])
		p, e2 := strconv.Atoi(fields[1])
		if e1 != nil || e2 != nil {
			continue
		}
		c := canonicalTable{line: n, page: p, anchor: -1, grid: grid}
		json.Unmarshal([]byte(fields[5]), &c.coords)
		distance := math.Inf(1)
		for i, t := range physical.Tables {
			if t.Page != p || len(t.Coords) != 4 || len(t.Rows) == 0 {
				continue
			}
			// A first-page fragment must have at least one recognizable canonical row.
			recognizable := false
			for _, r := range append(append([]docprocessing.TableRow{}, grid.Headers...), grid.Rows...) {
				for _, pr := range t.Rows {
					if score(r, pr) > 0 {
						recognizable = true
					}
				}
			}
			if !recognizable {
				continue
			}
			d := 0.0
			if len(c.coords) == 4 {
				for k := range c.coords {
					d += math.Abs(c.coords[k] - t.Coords[k])
				}
			}
			if d < distance {
				distance = d
				c.anchor = i
			}
		}
		canonical = append(canonical, c)
	}
	for index, c := range canonical {
		if c.anchor < 0 {
			continue
		}
		end := len(physical.Tables)
		for _, next := range canonical[index+1:] {
			if next.anchor > c.anchor {
				end = next.anchor
				break
			}
		}
		// A physical table may continue across adjacent pages. Accept repeated
		// headers or a strongly matching data fragment; stop at the next anchor.
		rows := append(append([]docprocessing.TableRow{}, c.grid.Headers...), c.grid.Rows...)
		mapped := map[string]*Row{}
		ambiguous := map[string]bool{}
		seenText := map[string]map[string]bool{}
		selected := []PhysicalTable{physical.Tables[c.anchor]}
		continuations := map[int][]PhysicalTable{}
		for _, t := range physical.Tables[c.anchor+1 : end] {
			if t.Page <= c.page || len(t.Rows) == 0 {
				continue
			}
			associated := false
			for _, h := range c.grid.Headers {
				if score(h, t.Rows[0]) > 0 {
					associated = true
				}
			}
			if !associated {
				for _, r := range c.grid.Rows {
					for _, pr := range t.Rows {
						if score(r, pr) >= 20 {
							associated = true
						}
					}
				}
			}
			if associated {
				continuations[t.Page] = append(continuations[t.Page], t)
			}
		}
		// The first fragment is anchored by its canonical page and box. A later
		// page must have exactly one associated table; competing copies are not
		// evidence for a continuation. Do not bridge pages without a fragment.
		for page := c.page + 1; len(continuations[page]) == 1; page++ {
			selected = append(selected, continuations[page][0])
		}
		for _, t := range selected {
			for _, pr := range t.Rows {
				best, ties, winner := 0, 0, -1
				for i, r := range rows {
					s := score(r, pr)
					if s > best {
						best = s
						ties = 1
						winner = i
					} else if s > 0 && s == best {
						ties++
					}
				}
				if winner < 0 || ties != 1 || !validBox(pr.Coords) {
					continue
				}
				r := rows[winner]
				if ambiguous[r.ID] {
					continue
				}
				if !strings.HasPrefix(r.ID, "h") {
					var parts []string
					for _, cell := range pr.Cells {
						parts = append(parts, normalized(cell.Text))
					}
					key := strings.Join(parts, "\x1f")
					if seenText[r.ID] == nil {
						seenText[r.ID] = map[string]bool{}
					}
					if seenText[r.ID][key] {
						delete(mapped, r.ID)
						ambiguous[r.ID] = true
						continue
					}
					seenText[r.ID][key] = true
				}
				dest := mapped[r.ID]
				if dest == nil {
					dest = &Row{ID: r.ID, Hash: r.Hash, Boxes: []Box{}, Cells: make([]Cell, len(r.Cells))}
					for col := range r.Cells {
						dest.Cells[col] = Cell{ID: fmt.Sprintf("%s:c%d", r.ID, col+1), Boxes: []Box{}}
					}
					mapped[r.ID] = dest
				}
				dest.Boxes = append(dest.Boxes, Box{Page: t.Page, Rotation: t.Rotation, Coords: pr.Coords})
				for col, cell := range pr.Cells {
					if validBox(cell.Coords) {
						dest.Cells[col].Boxes = append(dest.Cells[col].Boxes, Box{Page: t.Page, Rotation: t.Rotation, Coords: cell.Coords})
					}
				}
			}
		}
		table := Table{Line: c.line, Rows: []Row{}}
		for _, r := range rows {
			if m := mapped[r.ID]; m != nil {
				table.Rows = append(table.Rows, *m)
			}
		}
		out.Tables = append(out.Tables, table)
	}
	return out, nil
}

func CompanionPath(linePath string) string {
	return strings.TrimSuffix(linePath, filepath.Ext(linePath)) + ".table-geometry.json"
}
func PhysicalPath(pdfPath string) string {
	return strings.TrimSuffix(pdfPath, filepath.Ext(pdfPath)) + ".pdf-table-geometry.json"
}

// Ensure returns a current companion, rebuilding atomically when inputs changed.
// Missing physical geometry returns an empty document, never a stale companion.
func Ensure(linePath, pdfPath string) (*Document, error) {
	lineData, err := os.ReadFile(linePath)
	if err != nil {
		return nil, err
	}
	empty := &Document{Version: Version, Algorithm: Algorithm, CoordinateSpace: "page-normalized-1000", LineHash: hash(lineData), Tables: []Table{}}
	if strings.TrimSpace(pdfPath) == "" {
		return empty, nil
	}
	raw, err := os.ReadFile(PhysicalPath(pdfPath))
	if os.IsNotExist(err) {
		return empty, nil
	}
	if err != nil {
		return nil, err
	}
	var physical PhysicalDocument
	if err = json.Unmarshal(raw, &physical); err != nil {
		return nil, err
	}
	pdf, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, err
	}
	if physical.PDFHash != hash(pdf) {
		return empty, nil
	}
	empty.PDFHash = physical.PDFHash
	physicalBytes, err := json.Marshal(physical)
	if err != nil {
		return nil, err
	}
	empty.PhysicalHash = hash(physicalBytes)
	path := CompanionPath(linePath)
	if saved, e := os.ReadFile(path); e == nil {
		var current Document
		if json.Unmarshal(saved, &current) == nil && current.Version == Version && current.Algorithm == Algorithm && current.LineHash == empty.LineHash && current.PDFHash == empty.PDFHash && current.PhysicalHash == empty.PhysicalHash {
			return &current, nil
		}
	}
	result, err := Build(lineData, physical)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".table-geometry-*.tmp")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return nil, err
	}
	if _, err = tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return nil, err
	}
	if err = tmp.Close(); err != nil {
		return nil, err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return nil, err
	}
	return result, nil
}
