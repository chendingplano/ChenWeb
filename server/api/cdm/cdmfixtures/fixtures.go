// Package cdmfixtures provides shared CDM document fixtures for validator,
// store, and renderer tests, so every Phase 1 block and inline type is
// exercised from one definition instead of being redefined per test.
package cdmfixtures

import (
	"time"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

// JaroWinkler is the worked example from CDM spec §1, covering paragraph,
// emphasis/strong/text inline, list, table, and an equation using the
// Phase 1 fallback path (parse_status "skipped", no normalized AST).
func JaroWinkler() model.Document {
	return model.Document{
		Key:            "doc:jaro-winkler",
		Title:          "Jaro-Winkler Similarity",
		Language:       "en",
		SchemaVersion:  model.SchemaVersion,
		ContentVersion: 1,
		Metadata: model.Metadata{
			DocType:       "technical-doc",
			RenderingType: "user-manual",
			Authors:       []string{"Chen Ding"},
			Version:       "v1.0",
			CreateTime:    time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
			ModifyTime:    time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
		},
		Blocks: []model.Block{
			{
				ID:   "intro",
				Type: "paragraph",
				Content: []model.Inline{
					{Type: "text", Text: "Jaro-Winkler similarity is a string similarity algorithm designed for "},
					{Type: "emphasis", Content: []model.Inline{{Type: "text", Text: "short strings"}}},
					{Type: "text", Text: ", especially names."},
				},
			},
			{
				ID:      "score-range",
				Type:    "list",
				Ordered: false,
				Items: [][]model.Block{
					{
						{
							ID:   "score-range-1",
							Type: "paragraph",
							Content: []model.Inline{
								{Type: "strong", Content: []model.Inline{{Type: "text", Text: "1.0"}}},
								{Type: "text", Text: " means identical strings."},
							},
						},
					},
					{
						{
							ID:   "score-range-2",
							Type: "paragraph",
							Content: []model.Inline{
								{Type: "strong", Content: []model.Inline{{Type: "text", Text: "0.0"}}},
								{Type: "text", Text: " means completely different strings."},
							},
						},
					},
				},
			},
			{
				ID:   "example-table",
				Type: "table",
				Columns: []model.TableColumn{
					{Key: "left", Title: "String 1", Align: "left"},
					{Key: "right", Title: "String 2", Align: "left"},
					{Key: "score", Title: "Similarity", Align: "right"},
				},
				Rows: []model.TableRow{
					{Cells: map[string][]model.Inline{
						"left":  {{Type: "text", Text: "John"}},
						"right": {{Type: "text", Text: "Jon"}},
						"score": {{Type: "text", Text: "0.93"}},
					}},
					{Cells: map[string][]model.Inline{
						"left":  {{Type: "text", Text: "Michael"}},
						"right": {{Type: "text", Text: "Micheal"}},
						"score": {{Type: "text", Text: "0.97"}},
					}},
				},
			},
			{
				ID:   "jaro-formula",
				Type: "equation",
				Math: &model.Equation{
					Display:     true,
					ParseStatus: "skipped",
					Original: &model.MathSource{
						Format: "latex",
						Source: `J = \frac{1}{3}\left(\frac{m}{|s_1|} + \frac{m}{|s_2|} + \frac{m-t}{m}\right)`,
					},
				},
			},
		},
	}
}

// AllBlockTypes returns a document exercising every Phase 1 block type
// (heading, paragraph, list, table, code, equation, image, quote, callout)
// and every inline type (text, strong, emphasis, code, link, math, citation,
// cross_reference), for validator and renderer coverage.
func AllBlockTypes() model.Document {
	return model.Document{
		Key:            "doc:phase1-all-types",
		Title:          "Phase 1 Block Type Coverage",
		Language:       "en",
		SchemaVersion:  model.SchemaVersion,
		ContentVersion: 1,
		Metadata: model.Metadata{
			DocType:       "technical-doc",
			RenderingType: "user-manual",
			Authors:       []string{"Chen Ding"},
		},
		Blocks: []model.Block{
			{
				ID:    "h1",
				Type:  "heading",
				Level: 1,
				Content: []model.Inline{
					{Type: "text", Text: "Title Heading"},
				},
			},
			{
				ID:   "p1",
				Type: "paragraph",
				Content: []model.Inline{
					{Type: "text", Text: "See "},
					{Type: "strong", Content: []model.Inline{{Type: "text", Text: "bold"}}},
					{Type: "text", Text: " and "},
					{Type: "emphasis", Content: []model.Inline{{Type: "text", Text: "italic"}}},
					{Type: "text", Text: ", inline "},
					{Type: "code", Text: "x := 1"},
					{Type: "text", Text: ", a "},
					{Type: "link", URL: "https://example.com", Content: []model.Inline{{Type: "text", Text: "link"}}},
					{Type: "text", Text: ", a citation "},
					{Type: "citation", CitationKey: "winkler1990", Locator: "p. 354"},
					{Type: "text", Text: ", and a "},
					{
						Type:   "cross_reference",
						Target: &model.RefTarget{BlockID: "eq1"},
						Content: []model.Inline{
							{Type: "text", Text: "cross-reference"},
						},
					},
					{Type: "text", Text: "."},
				},
			},
			{
				ID:      "list1",
				Type:    "list",
				Ordered: true,
				Items: [][]model.Block{
					{{ID: "list1-1", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "First"}}}},
					{{ID: "list1-2", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "Second"}}}},
				},
			},
			{
				ID:   "table1",
				Type: "table",
				Columns: []model.TableColumn{
					{Key: "a", Title: "A"},
					{Key: "b", Title: "B"},
				},
				Rows: []model.TableRow{
					{Cells: map[string][]model.Inline{
						"a": {{Type: "text", Text: "1"}},
						"b": {{Type: "text", Text: "2"}},
					}},
				},
			},
			{
				ID:   "code1",
				Type: "code",
				Lang: "go",
				Text: "func main() {}\n",
			},
			{
				ID:   "eq1",
				Type: "equation",
				Math: &model.Equation{
					Display:     false,
					ParseStatus: "skipped",
					Original:    &model.MathSource{Format: "typst", Source: "a + b = c"},
				},
			},
			{
				ID:  "img1",
				Type: "image",
				Src: "diagrams/flow.png",
				Alt: "Flow diagram",
				Caption: []model.Inline{
					{Type: "text", Text: "Figure: process flow"},
				},
			},
			{
				ID:   "quote1",
				Type: "quote",
				Content: []model.Inline{
					{Type: "text", Text: "A quoted passage."},
				},
			},
			{
				ID:    "callout1",
				Type:  "callout",
				Role:  "warning",
				Title: "Limitation",
				Children: []model.Block{
					{
						ID:   "callout1-body",
						Type: "paragraph",
						Content: []model.Inline{
							{Type: "text", Text: "Scores are not comparable across locales."},
						},
					},
				},
			},
		},
	}
}
