package keywords

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

var testNow = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

func TestCreateConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO kb.keyword_concepts (concept_id, pref_label, gloss, scope, status, gloss_source)`)).
		WithArgs("kw:test", "Test Concept", nil, "_", "active", "none").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:test", "Test Concept", nil, "_", "active", nil, "none", testNow, testNow))

	c, err := store.CreateConcept(ctx, Concept{ConceptID: "kw:test", PrefLabel: "Test Concept"})
	if err != nil {
		t.Fatalf("CreateConcept: %v", err)
	}
	if c.ConceptID != "kw:test" {
		t.Errorf("expected concept_id kw:test, got %s", c.ConceptID)
	}
	if c.Status != "active" {
		t.Errorf("expected status active, got %s", c.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCreateConceptValidation(t *testing.T) {
	store := ConceptStore{DB: nil}
	ctx := context.Background()

	tests := []struct {
		name string
		c    Concept
	}{
		{"missing concept_id", Concept{PrefLabel: "Test", Scope: "_", Status: "active"}},
		{"missing pref_label", Concept{ConceptID: "kw:x", Scope: "_", Status: "active"}},
		{"invalid status", Concept{ConceptID: "kw:x", PrefLabel: "Test", Scope: "_", Status: "bogus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.CreateConcept(ctx, tt.c)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestGetConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id = $1`)).
		WithArgs("kw:test").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:test", "Test Concept", "A test concept", "_", "active", nil, "human", testNow, testNow))

	c, err := store.GetConcept(ctx, "kw:test")
	if err != nil {
		t.Fatalf("GetConcept: %v", err)
	}
	if c.ConceptID != "kw:test" {
		t.Errorf("expected kw:test, got %s", c.ConceptID)
	}
	if c.Gloss == nil || *c.Gloss != "A test concept" {
		t.Errorf("expected gloss 'A test concept', got %v", c.Gloss)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListConcepts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE status IN ('active', 'provisional') ORDER BY concept_id`)).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:a", "Alpha", nil, "_", "active", nil, "none", testNow, testNow).
			AddRow("kw:b", "Beta", nil, "_", "provisional", nil, "none", testNow, testNow))

	concepts, err := store.ListConcepts(ctx, "")
	if err != nil {
		t.Fatalf("ListConcepts: %v", err)
	}
	if len(concepts) != 2 {
		t.Fatalf("expected 2 concepts, got %d", len(concepts))
	}
	if concepts[0].ConceptID != "kw:a" || concepts[1].ConceptID != "kw:b" {
		t.Errorf("unexpected sort order: %v", concepts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestListConceptsByScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE scope = $1 AND status IN ('active', 'provisional') ORDER BY concept_id`)).
		WithArgs("ventilator").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:v", "Vent", nil, "ventilator", "active", nil, "none", testNow, testNow))

	concepts, err := store.ListConcepts(ctx, "ventilator")
	if err != nil {
		t.Fatalf("ListConcepts: %v", err)
	}
	if len(concepts) != 1 || concepts[0].Scope != "ventilator" {
		t.Errorf("expected 1 ventilator-scoped concept, got %d", len(concepts))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpdateConceptLabel(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`UPDATE kb.keyword_concepts SET pref_label = $2, gloss = $3, modify_time = NOW() WHERE concept_id = $1 RETURNING ` + conceptColumns)).
		WithArgs("kw:test", "Updated", nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:test", "Updated", nil, "_", "active", nil, "none", testNow, testNow))

	c, err := store.UpdateConceptLabel(ctx, "kw:test", "Updated", "")
	if err != nil {
		t.Fatalf("UpdateConceptLabel: %v", err)
	}
	if c.PrefLabel != "Updated" {
		t.Errorf("expected Updated, got %s", c.PrefLabel)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransitionConceptStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	// First query: GetConcept for current state.
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id = $1`)).
		WithArgs("kw:test").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:test", "Test", nil, "_", "active", nil, "none", testNow, testNow))

	// Second query: UPDATE with status transition.
	mock.ExpectQuery(regexp.QuoteMeta(
		`UPDATE kb.keyword_concepts SET status = $2, modify_time = NOW() WHERE concept_id = $1 RETURNING ` + conceptColumns)).
		WithArgs("kw:test", "provisional").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:test", "Test", nil, "_", "provisional", nil, "none", testNow, testNow))

	c, err := store.TransitionStatus(ctx, "kw:test", "provisional")
	if err != nil {
		t.Fatalf("TransitionStatus: %v", err)
	}
	if c.Status != "provisional" {
		t.Errorf("expected provisional, got %s", c.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTransitionConceptStatusIllegal(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	// GetConcept: current status is 'merged' (cannot transition further).
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id = $1`)).
		WithArgs("kw:test").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:test", "Test", nil, "_", "merged", nil, "none", testNow, testNow))

	_, err = store.TransitionStatus(ctx, "kw:test", "deprecated")
	if err == nil {
		t.Error("expected illegal transition error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMergeConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	// Verify target exists.
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id = $1`)).
		WithArgs("kw:target").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:target", "Target", nil, "_", "active", nil, "none", testNow, testNow))

	// Update fromID: set merged_into.
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE kb.keyword_concepts SET status = 'merged', merged_into = $2, modify_time = NOW() WHERE concept_id = $1`)).
		WithArgs("kw:from", "kw:target").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Return the target.
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id = $1`)).
		WithArgs("kw:target").
		WillReturnRows(sqlmock.NewRows([]string{
			"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
		}).AddRow("kw:target", "Target", nil, "_", "active", nil, "none", testNow, testNow))

	c, err := store.MergeConcept(ctx, "kw:from", "kw:target")
	if err != nil {
		t.Fatalf("MergeConcept: %v", err)
	}
	if c.ConceptID != "kw:target" {
		t.Errorf("expected kw:target, got %s", c.ConceptID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMergeConceptSelfMerge(t *testing.T) {
	store := ConceptStore{DB: nil}
	ctx := context.Background()

	_, err := store.MergeConcept(ctx, "kw:x", "kw:x")
	if err == nil {
		t.Error("expected self-merge error")
	}
}

func TestDuplicateConceptID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(
		`INSERT INTO kb.keyword_concepts (concept_id, pref_label, gloss, scope, status, gloss_source)`)).
		WithArgs("kw:dup", "Duplicate", nil, "_", "active", "none").
		WillReturnError(fmt.Errorf(`duplicate key value violates unique constraint "keyword_concepts_pkey"`))

	_, err = store.CreateConcept(ctx, Concept{ConceptID: "kw:dup", PrefLabel: "Duplicate"})
	if err == nil {
		t.Error("expected duplicate key error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
