// Package gold builds CDM documents (server/api/cdm/model) from the
// synthetic gold ontology in gold.toml, so the existing Typst rendering
// pipeline (server/api/cdm/rendering) can turn a hand-authored gold fixture
// into a grounded synthetic document (ADR 2026072901 DR25) without a second,
// parallel document-authoring path.
package gold

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

// File is the full gold.toml schema this package resolves: document
// generation (BuildDocuments, using AuthorityDocument/Clause identity and
// text only) and verdict resolution (Resolve, in resolve.go, additionally
// using MetricDefinition/ClosedDimension/ExpectedVerdict and each clause's
// value fields).
type File struct {
	MetricDefinition  []MetricDefinition `toml:"metric_definition"`
	AuthorityDocument []Document         `toml:"authority_document"`
	Clause            []Clause           `toml:"clause"`
	ClosedDimension   []ClosedDimension  `toml:"closed_dimension"`
	ExpectedVerdict   []ExpectedVerdict  `toml:"expected_verdict"`
}

type MetricDefinition struct {
	ID           string `toml:"id"`
	QuantityKind string `toml:"quantity_kind"`
}

type Document struct {
	ID     string `toml:"id"`
	Family string `toml:"family"`
	Title  string `toml:"title"`
}

type Clause struct {
	ID           string   `toml:"id"`
	Document     string   `toml:"document"`
	Metric       string   `toml:"metric"`
	Form         string   `toml:"form"`
	Value        *float64 `toml:"value"`
	Unit         *string  `toml:"unit"`
	LowerValue   *float64 `toml:"lower_value"`
	UpperValue   *float64 `toml:"upper_value"`
	TextTemplate string   `toml:"text_template"`

	// Expectation governs how a real pipeline's failure to extract this
	// clause is scored (see coverage.go). Empty means "required": a missing
	// extraction is an error. "best_effort" means the source statement is
	// inherently vague/non-verifiable prose (bug 2026073001) -- extracting
	// it is good when it happens, but its absence is not scored as a defect.
	// This is orthogonal to Form: a "qualitative" clause is usually
	// best_effort, but "limit_absent" clauses (e.g. "this standard does not
	// specify a limit") are decidable, checkable facts and stay required.
	Expectation string `toml:"expectation"`
}

type ClosedDimension struct {
	Metric string `toml:"metric"`
	Family string `toml:"family"`
	Closed bool   `toml:"closed"`
}

type ExpectedVerdict struct {
	Metric   string `toml:"metric"`
	VsFamily string `toml:"vs_family"`
	Verdict  string `toml:"verdict"`
	Object   string `toml:"object"`
}

// Load reads and parses a gold.toml file at path.
func Load(path string) (File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("gold: reading %s: %w", path, err)
	}
	f, err := Parse(raw)
	if err != nil {
		return File{}, fmt.Errorf("gold: parsing %s: %w", path, err)
	}
	return f, nil
}

// Parse decodes gold.toml content already read by the caller -- e.g. a
// dataset loader that read the bytes through its own path-safety-checked
// file access rather than a plain os.ReadFile.
func Parse(raw []byte) (File, error) {
	var f File
	if err := toml.Unmarshal(raw, &f); err != nil {
		return File{}, err
	}
	return f, nil
}

// BuildDocuments renders every authority_document in f as a CDM document: one
// paragraph block per clause belonging to that document, in the order
// clauses appear in the gold file. The document's own title is not emitted
// as a block here -- rendering.TypstRenderer.RenderDocument already emits
// Document.Title as the page heading.
//
// Each paragraph block's ID equals its clause ID, so a clause and its
// rendered block -- and therefore its anchor -- share one identifier end to
// end: a clause in gold.toml, a block in the generated Document, and an
// anchor mark from ExtractAnchors all resolve by the same string.
func BuildDocuments(f File) []model.Document {
	docs := make([]model.Document, 0, len(f.AuthorityDocument))
	for _, d := range f.AuthorityDocument {
		var blocks []model.Block
		for _, c := range f.Clause {
			if c.Document != d.ID {
				continue
			}
			blocks = append(blocks, model.Block{
				ID:      c.ID,
				Type:    "paragraph",
				Content: []model.Inline{{Type: "text", Text: c.TextTemplate}},
			})
		}
		docs = append(docs, model.Document{
			Key:            d.ID,
			Title:          d.Title,
			SchemaVersion:  model.SchemaVersion,
			ContentVersion: 1,
			Blocks:         blocks,
		})
	}
	return docs
}
