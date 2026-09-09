package productreviews

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func assertionRows(rows ...[]driver.Value) *sqlmock.Rows {
	out := sqlmock.NewRows([]string{
		"id", "predicate_term_id", "other_id", "canonical_name", "canonical_name_en", "reconcile_status",
	})
	for _, r := range rows {
		out.AddRow(r...)
	}
	return out
}

func emptyProductRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "relation_type", "related_products"})
}

func objectNameRow(name, nameEN string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"canonical_name", "canonical_name_en"}).AddRow(name, nameEN)
}

// Scenario: Expansion adds a part the model did not name (spec: Graph expansion
// pass) — a graph_expanded node with the assertion id recorded.
func TestExpandAddsPartFromAssertion(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	e := Expander{Store: store, DB: store.DB, Budgets: BudgetsConfig{MaxDepth: 3, MaxNodes: 200}}

	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{
			ID: 1, Kind: KindProduct, Label: "Ventilator",
			ObjectID: "obj:ventilator", Grounding: GroundingObjectNode,
		}))
	mock.ExpectBegin()
	mock.ExpectQuery(rx("FROM kb.semantic_assertions sa")).
		WithArgs("obj:ventilator", sqlmock.AnyArg()).
		WillReturnRows(assertionRows([]driver.Value{
			int64(99), "core:part_of", "obj:humidifier", "Humidifier", "Humidifier", "active",
		}))
	mock.ExpectQuery(rx("FROM kb.object_nodes WHERE object_id = $1")).WithArgs("obj:ventilator").
		WillReturnRows(objectNameRow("Ventilator", ""))
	mock.ExpectQuery(rx("FROM kb.products")).WillReturnRows(emptyProductRows())
	mock.ExpectQuery(rx("INSERT INTO kb.product_profile_nodes")).WillReturnRows(idRow(20))
	// the new node is itself grounded, so expansion recurses from it — no edges.
	mock.ExpectQuery(rx("FROM kb.semantic_assertions sa")).WithArgs("obj:humidifier", sqlmock.AnyArg()).
		WillReturnRows(assertionRows())
	mock.ExpectQuery(rx("FROM kb.object_nodes WHERE object_id = $1")).WithArgs("obj:humidifier").
		WillReturnRows(objectNameRow("Humidifier", ""))
	mock.ExpectQuery(rx("FROM kb.products")).WillReturnRows(emptyProductRows())
	mock.ExpectExec(rx("SET version = version + 1")).WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := e.Expand(context.Background(), 1); err != nil {
		t.Fatalf("Expand: %v", err)
	}
}

// Scenario: Expansion does not duplicate an existing node (spec) — the existing
// node records the additional supporting edge, no new node.
func TestExpandNoDuplicate(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	e := Expander{Store: store, DB: store.DB, Budgets: BudgetsConfig{MaxDepth: 3, MaxNodes: 200}}

	one := int64(1)
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1,
			nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator", ObjectID: "obj:ventilator", Grounding: GroundingObjectNode},
			nodeRow{ID: 2, ParentID: &one, Kind: KindPart, Label: "Humidifier", ObjectID: "obj:humidifier", Grounding: GroundingObjectNode, Depth: 1},
		))
	mock.ExpectBegin()
	// expand from the root: finds the humidifier edge, which already has a node.
	mock.ExpectQuery(rx("FROM kb.semantic_assertions sa")).WithArgs("obj:ventilator", sqlmock.AnyArg()).
		WillReturnRows(assertionRows([]driver.Value{
			int64(99), "core:part_of", "obj:humidifier", "Humidifier", "Humidifier", "active",
		}))
	mock.ExpectQuery(rx("FROM kb.object_nodes WHERE object_id = $1")).WithArgs("obj:ventilator").
		WillReturnRows(objectNameRow("Ventilator", ""))
	mock.ExpectQuery(rx("FROM kb.products")).WillReturnRows(emptyProductRows())
	mock.ExpectExec(rx("SET source_refs = COALESCE(source_refs")).
		WithArgs(int64(2), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// expand from the humidifier node: no further edges.
	mock.ExpectQuery(rx("FROM kb.semantic_assertions sa")).WithArgs("obj:humidifier", sqlmock.AnyArg()).
		WillReturnRows(assertionRows())
	mock.ExpectQuery(rx("FROM kb.object_nodes WHERE object_id = $1")).WithArgs("obj:humidifier").
		WillReturnRows(objectNameRow("Humidifier", ""))
	mock.ExpectQuery(rx("FROM kb.products")).WillReturnRows(emptyProductRows())
	mock.ExpectCommit()

	if err := e.Expand(context.Background(), 1); err != nil {
		t.Fatalf("Expand: %v", err)
	}
}

// Scenario: No graph edges exist (spec) — completes, adds no nodes, no version bump.
func TestExpandNoEdges(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	e := Expander{Store: store, DB: store.DB, Budgets: BudgetsConfig{MaxDepth: 3, MaxNodes: 200}}

	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{
			ID: 1, Kind: KindProduct, Label: "Ventilator", ObjectID: "obj:ventilator", Grounding: GroundingObjectNode,
		}))
	mock.ExpectBegin()
	mock.ExpectQuery(rx("FROM kb.semantic_assertions sa")).WithArgs("obj:ventilator", sqlmock.AnyArg()).
		WillReturnRows(assertionRows())
	mock.ExpectQuery(rx("FROM kb.object_nodes WHERE object_id = $1")).WithArgs("obj:ventilator").
		WillReturnRows(objectNameRow("Ventilator", ""))
	mock.ExpectQuery(rx("FROM kb.products")).WillReturnRows(emptyProductRows())
	mock.ExpectCommit()

	if err := e.Expand(context.Background(), 1); err != nil {
		t.Fatalf("Expand: %v", err)
	}
}
