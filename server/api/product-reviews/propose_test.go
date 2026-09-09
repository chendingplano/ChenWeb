package productreviews

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type fakeExtractor struct {
	resp  map[string]any
	err   error
	calls int
}

func (f *fakeExtractor) ExtractJSON(_ context.Context, _ llmclients.JSONExtractionInput) (map[string]any, error) {
	f.calls++
	return f.resp, f.err
}

func node(path []string, kind string) map[string]any {
	return map[string]any{
		"path": toAny(path), "label": path[len(path)-1], "node_kind": kind,
		"confidence": 0.8, "rationale": "because",
	}
}

func toAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// insertArgs matches an insertNode call, pinning only parent / label / depth /
// origin / status and treating the rest as noise.
func insertArgs(parent any, label string, depth int, origin, status string) []driver.Value {
	a := parent
	if p, ok := parent.(int64); ok {
		a = p
	}
	return []driver.Value{
		sqlmock.AnyArg(), a, sqlmock.AnyArg(), label, sqlmock.AnyArg(),
		sqlmock.AnyArg(), int64(depth), origin, status, sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
	}
}

func idRow(id int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id"}).AddRow(id)
}

// Scenario: Proposal produces a multi-level tree (spec: Decomposition proposal
// pass) — Display Backlight lands at depth 2 with its parent set to Display.
func TestProposeMultiLevelTree(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	fx := &fakeExtractor{resp: map[string]any{"nodes": []any{
		node([]string{"Ventilator", "Display"}, "module"),
		node([]string{"Ventilator", "Breathing Circuit"}, "module"),
		node([]string{"Ventilator", "Battery"}, "part"),
		node([]string{"Ventilator", "Display", "Display Backlight"}, "part"),
	}}}
	p := Proposer{Store: store, Extractor: fx, ModelName: "m", PromptText: "prompt",
		Budgets: BudgetsConfig{MaxDepth: 3, MaxNodes: 200}}

	mock.ExpectQuery(rx("FROM kb.product_profiles WHERE id = $1")).WithArgs(int64(1)).
		WillReturnRows(profileRows(1, "Ventilator", 1, "draft", false, 0))
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator"}))
	mock.ExpectBegin()
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(insertArgs(int64(1), "Display", 1, OriginLLMProposed, StatusProposed)...).
		WillReturnRows(idRow(10))
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(insertArgs(int64(1), "Breathing Circuit", 1, OriginLLMProposed, StatusProposed)...).
		WillReturnRows(idRow(11))
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(insertArgs(int64(1), "Battery", 1, OriginLLMProposed, StatusProposed)...).
		WillReturnRows(idRow(12))
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).
		WithArgs(insertArgs(int64(10), "Display Backlight", 2, OriginLLMProposed, StatusProposed)...).
		WillReturnRows(idRow(13))
	mock.ExpectExec(rx("SET version = version + 1")).WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := p.Propose(context.Background(), 1, ProposeInput{}); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if fx.calls != 1 {
		t.Fatalf("extractor calls = %d, want exactly 1", fx.calls)
	}
}

// Scenario: Node budget exceeded (spec) — persist up to the budget breadth-first,
// record truncated + discarded count.
func TestProposeNodeBudgetExceeded(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	fx := &fakeExtractor{resp: map[string]any{"nodes": []any{
		node([]string{"Ventilator", "Display"}, "module"),
		node([]string{"Ventilator", "Breathing Circuit"}, "module"),
		node([]string{"Ventilator", "Battery"}, "part"),
	}}}
	p := Proposer{Store: store, Extractor: fx, ModelName: "m", PromptText: "prompt",
		Budgets: BudgetsConfig{MaxDepth: 3, MaxNodes: 2}}

	mock.ExpectQuery(rx("FROM kb.product_profiles WHERE id = $1")).WithArgs(int64(1)).
		WillReturnRows(profileRows(1, "Ventilator", 1, "draft", false, 0))
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator"}))
	mock.ExpectBegin()
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).WillReturnRows(idRow(10))
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).WillReturnRows(idRow(11))
	mock.ExpectExec(rx("SET truncated = TRUE")).WithArgs(int64(1), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(rx("SET version = version + 1")).WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := p.Propose(context.Background(), 1, ProposeInput{}); err != nil {
		t.Fatalf("Propose: %v", err)
	}
}

// Scenario: Proposal call fails (spec) — CWB_KB_PMR_011, profile untouched.
func TestProposeCallFails(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	fx := &fakeExtractor{err: errors.New("upstream 500")}
	p := Proposer{Store: store, Extractor: fx, ModelName: "m", PromptText: "prompt"}

	mock.ExpectQuery(rx("FROM kb.product_profiles WHERE id = $1")).WithArgs(int64(1)).
		WillReturnRows(profileRows(1, "Ventilator", 1, "draft", false, 0))
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator"}))
	// no BeginTx, no inserts — the profile is left exactly as it was.

	err := p.Propose(context.Background(), 1, ProposeInput{})
	var pmr *PMRError
	if !errors.As(err, &pmr) || pmr.Code != CodeProposalFailed {
		t.Fatalf("err = %v, want CWB_KB_PMR_011", err)
	}
}

func TestPlanNodes(t *testing.T) {
	in := []proposedNode{
		{Path: []string{"Ventilator", "Display"}},
		{Path: []string{"Ventilator", "Display"}}, // duplicate
		{Path: []string{"Ventilator", "Battery"}},
		{Path: []string{"Ventilator", "Display", "Backlight"}},
		{Path: []string{"Ventilator", "Display", "Backlight", "LED", "Die"}}, // depth 4 > budget
		{Path: []string{"Humidifier", "Chamber"}},                            // not rooted here
	}
	kept, discarded := planNodes(in, "Ventilator", BudgetsConfig{MaxDepth: 3, MaxNodes: 2})
	if discarded != 1 {
		t.Fatalf("discarded = %d, want 1", discarded)
	}
	if len(kept) != 2 {
		t.Fatalf("kept %d nodes, want 2 (breadth-first)", len(kept))
	}
	for _, k := range kept {
		if len(k.Path) != 2 {
			t.Fatalf("kept a depth-%d node before all depth-1 nodes: %v", len(k.Path)-1, k.Path)
		}
	}
}
