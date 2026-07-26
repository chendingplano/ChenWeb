package model_test

import (
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

func baseDoc() model.Document {
	return model.Document{
		Key:           "doc:test",
		Title:         "Test",
		SchemaVersion: model.SchemaVersion,
	}
}

func TestValidate_ValidDocumentsPass(t *testing.T) {
	for name, doc := range map[string]model.Document{
		"jaro-winkler":    cdmfixtures.JaroWinkler(),
		"all-block-types": cdmfixtures.AllBlockTypes(),
	} {
		t.Run(name, func(t *testing.T) {
			doc := doc
			if err := model.Validate(&doc); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}
}

func TestValidate_DuplicateBlockID(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{ID: "a", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "x"}}},
		{ID: "a", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "y"}}},
	}

	err := model.Validate(&doc)
	requireError(t, err, `duplicate block id "a"`)
}

func TestValidate_DuplicateBlockID_NestedDepth(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:   "callout1",
			Type: "callout",
			Children: []model.Block{
				{ID: "dup", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "x"}}},
			},
		},
		{ID: "dup", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "y"}}},
	}

	err := model.Validate(&doc)
	requireError(t, err, `duplicate block id "dup"`)
}

func TestValidate_EmptyBlockID(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{ID: "", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "x"}}},
	}

	err := model.Validate(&doc)
	requireError(t, err, "empty id")
}

func TestValidate_ContentAndChildrenExclusivity(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:      "bad",
			Type:    "callout",
			Content: []model.Inline{{Type: "text", Text: "x"}},
			Children: []model.Block{
				{ID: "bad-child", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "y"}}},
			},
		},
	}

	err := model.Validate(&doc)
	requireError(t, err, `block "bad" populates more than one`)
}

func TestValidate_ParagraphWithContentOnly(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{ID: "p1", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "hello"}}},
	}

	if err := model.Validate(&doc); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidate_ListItemsValidatedRecursively(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:   "list1",
			Type: "list",
			Items: [][]model.Block{
				{{ID: "item1", Type: "not-a-real-type"}},
			},
		},
	}

	err := model.Validate(&doc)
	requireError(t, err, `block "item1" has unsupported type "not-a-real-type"`)
}

func TestValidate_UnknownBlockType(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{{ID: "b1", Type: "horizontal_stack"}}

	err := model.Validate(&doc)
	requireError(t, err, `unsupported type "horizontal_stack"`)
}

func TestValidate_WarningAsBlockTypeRejected(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{{ID: "w1", Type: "warning"}}

	err := model.Validate(&doc)
	requireError(t, err, "callout")
	requireError(t, err, `type "warning"`)
}

func TestValidate_CalloutWithValidRoleAccepted(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:    "c1",
			Type:  "callout",
			Role:  "warning",
			Title: "Note",
			Children: []model.Block{
				{ID: "c1-body", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "x"}}},
			},
		},
	}

	if err := model.Validate(&doc); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidate_HeadingLevelBounded(t *testing.T) {
	for _, level := range []int{0, 7, -1} {
		doc := baseDoc()
		doc.Blocks = []model.Block{{ID: "h1", Type: "heading", Level: level}}

		err := model.Validate(&doc)
		requireError(t, err, "outside the range 1-6")
	}
}

func TestValidate_TableCellKeyNotDeclared(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:      "t1",
			Type:    "table",
			Columns: []model.TableColumn{{Key: "a", Title: "A"}},
			Rows: []model.TableRow{
				{Cells: map[string][]model.Inline{"total": {{Type: "text", Text: "1"}}}},
			},
		},
	}

	err := model.Validate(&doc)
	requireError(t, err, `cell key "total"`)
}

func TestValidate_TableMissingCellKeyAllowed(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:   "t1",
			Type: "table",
			Columns: []model.TableColumn{
				{Key: "a", Title: "A"},
				{Key: "b", Title: "B"},
				{Key: "c", Title: "C"},
			},
			Rows: []model.TableRow{
				{Cells: map[string][]model.Inline{
					"a": {{Type: "text", Text: "1"}},
					"b": {{Type: "text", Text: "2"}},
				}},
			},
		},
	}

	if err := model.Validate(&doc); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidate_DuplicateColumnKey(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:   "t1",
			Type: "table",
			Columns: []model.TableColumn{
				{Key: "a", Title: "A"},
				{Key: "a", Title: "A again"},
			},
		},
	}

	err := model.Validate(&doc)
	requireError(t, err, `duplicate column key "a"`)
}

func TestValidate_EquationNeitherRepresentation(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{ID: "eq1", Type: "equation", Math: &model.Equation{ParseStatus: "skipped"}},
	}

	err := model.Validate(&doc)
	requireError(t, err, `equation "eq1" has neither`)
}

func TestValidate_EquationUnknownParseStatus(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:   "eq1",
			Type: "equation",
			Math: &model.Equation{
				ParseStatus: "partial",
				Original:    &model.MathSource{Format: "latex", Source: "x"},
			},
		},
	}

	err := model.Validate(&doc)
	requireError(t, err, "unsupported parse_status")
}

func TestValidate_Phase1SkippedEquationAccepted(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{
			ID:   "eq1",
			Type: "equation",
			Math: &model.Equation{
				ParseStatus: "skipped",
				Original:    &model.MathSource{Format: "latex", Source: "J = 1"},
			},
		},
	}

	if err := model.Validate(&doc); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidate_MultipleViolationsAllReported(t *testing.T) {
	doc := baseDoc()
	doc.Blocks = []model.Block{
		{ID: "dup", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "x"}}},
		{ID: "dup", Type: "horizontal_stack"},
	}

	err := model.Validate(&doc)
	requireError(t, err, `duplicate block id "dup"`)
	requireError(t, err, `unsupported type "horizontal_stack"`)

	ve, ok := err.(*model.ValidationError)
	if !ok {
		t.Fatalf("expected *model.ValidationError, got %T", err)
	}
	if len(ve.Violations) < 2 {
		t.Fatalf("expected at least 2 violations, got %d: %v", len(ve.Violations), ve.Violations)
	}
}

func requireError(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}
