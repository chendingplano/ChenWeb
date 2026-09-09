package productreviews

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func groundOneNode(mock sqlmock.Sqlmock, n nodeRow) {
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, n))
}

// Scenario: Node grounds to a reconciled object (spec: Grounding pass).
func TestGroundToObjectNode(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	g := Grounder{Store: store, DB: store.DB}

	groundOneNode(mock, nodeRow{ID: 2, Kind: KindPart, Label: "Display", Grounding: GroundingUngrounded})
	mock.ExpectQuery(rx("FROM kb.object_nodes")).
		WillReturnRows(sqlmock.NewRows([]string{"object_id", "reconcile_status"}).AddRow("obj:display", "active"))
	mock.ExpectExec(rx("SET grounding = $2, object_id = $3, reconcile_status = $4")).
		WithArgs(int64(2), GroundingObjectNode, "obj:display", "active").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := g.Ground(context.Background(), 1); err != nil {
		t.Fatalf("Ground: %v", err)
	}
}

// Scenario: Node grounds to a metric-subject concept (spec: Grounding pass).
func TestGroundToKeywordConcept(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	g := Grounder{Store: store, DB: store.DB}

	groundOneNode(mock, nodeRow{ID: 2, Kind: KindPart, Label: "Airway Pressure", Grounding: GroundingUngrounded})
	mock.ExpectQuery(rx("FROM kb.object_nodes")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(rx("FROM kb.keyword_concepts")).
		WithArgs("Airway Pressure").
		WillReturnRows(sqlmock.NewRows([]string{"concept_id"}).AddRow("kc:airway_pressure"))
	mock.ExpectExec(rx("SET grounding = $2, concept_id = $3, reconcile_status = $4")).
		WithArgs(int64(2), GroundingKeywordConcept, "kc:airway_pressure", "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := g.Ground(context.Background(), 1); err != nil {
		t.Fatalf("Ground: %v", err)
	}
}

// Scenario: Node cannot be grounded (spec) — stays `ungrounded`, no write.
func TestGroundUngrounded(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	g := Grounder{Store: store, DB: store.DB}

	groundOneNode(mock, nodeRow{ID: 2, Kind: KindPart, Label: "Nonexistent Widget", Grounding: GroundingUngrounded})
	mock.ExpectQuery(rx("FROM kb.object_nodes")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(rx("FROM kb.keyword_concepts")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(rx("FROM kb.ontology_term_labels")).WillReturnError(sql.ErrNoRows)

	if err := g.Ground(context.Background(), 1); err != nil {
		t.Fatalf("Ground: %v", err)
	}
}

// Scenario: Grounded object is ambiguous (spec) — the reconcile state is
// recorded on the node.
func TestGroundAmbiguousObject(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()
	g := Grounder{Store: store, DB: store.DB}

	groundOneNode(mock, nodeRow{ID: 2, Kind: KindPart, Label: "Valve", Grounding: GroundingUngrounded})
	mock.ExpectQuery(rx("FROM kb.object_nodes")).
		WillReturnRows(sqlmock.NewRows([]string{"object_id", "reconcile_status"}).AddRow("obj:valve", "ambiguous"))
	mock.ExpectExec(rx("SET grounding = $2, object_id = $3, reconcile_status = $4")).
		WithArgs(int64(2), GroundingObjectNode, "obj:valve", "ambiguous").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := g.Ground(context.Background(), 1); err != nil {
		t.Fatalf("Ground: %v", err)
	}
}

// Scenario: Semantic search is disabled (spec) — lexical only, no embed call,
// no vector query, completes without error.
func TestGroundSemanticDisabled(t *testing.T) {
	store, mock, done := newMockStore(t)
	defer done()

	embedCalls := 0
	g := Grounder{
		Store: store, DB: store.DB, SemanticEnabled: false,
		Embed: func(context.Context, string) ([]float64, bool) { embedCalls++; return nil, false },
	}

	groundOneNode(mock, nodeRow{ID: 2, Kind: KindPart, Label: "Display", Grounding: GroundingUngrounded})
	mock.ExpectQuery(rx("FROM kb.object_nodes")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(rx("FROM kb.keyword_concepts")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(rx("FROM kb.ontology_term_labels")).WillReturnError(sql.ErrNoRows)

	if err := g.Ground(context.Background(), 1); err != nil {
		t.Fatalf("Ground: %v", err)
	}
	if embedCalls != 0 {
		t.Fatalf("embedder was called %d times with semantic search disabled", embedCalls)
	}
}
