package keywords

import (
	"context"
	"database/sql"
	"errors"
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
		`UPDATE kb.keyword_concepts SET pref_label = $2, gloss = $3, modify_time = NOW() WHERE concept_id = $1 RETURNING `+conceptColumns)).
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
		`UPDATE kb.keyword_concepts SET status = $2, modify_time = NOW() WHERE concept_id = $1 RETURNING `+conceptColumns)).
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

func conceptRow(id, status string, mergedInto any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"concept_id", "pref_label", "gloss", "scope", "status", "merged_into", "gloss_source", "create_time", "modify_time",
	}).AddRow(id, id, nil, "_", status, mergedInto, "none", testNow, testNow)
}

const getConceptSQL = `SELECT ` + conceptColumns + ` ` + conceptFrom + ` WHERE concept_id = $1`

const neverMergeSQL = `SELECT EXISTS ( SELECT 1 FROM kb.semid_never_merge WHERE family = $1 AND node_a = $2 AND node_b = $3 )`

// TestMergeConcept verifies the §14.1 merge: tombstone + surface re-point in
// one transaction, origin_concept recorded, survivor returned.
func TestMergeConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	// F10: the guardrail reads run inside the merge's transaction, with
	// FOR UPDATE holding a row lock through to the write.
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:from").
		WillReturnRows(conceptRow("kw:from", "active", nil))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:target").
		WillReturnRows(conceptRow("kw:target", "active", nil))
	mock.ExpectQuery(regexp.QuoteMeta(neverMergeSQL)).
		WithArgs("keyword", "kw:from", "kw:target").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE kb.keyword_concepts SET status = 'merged', merged_into = $2, modify_time = NOW() WHERE concept_id = $1`)).
		WithArgs("kw:from", "kw:target").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE kb.keyword_surfaces SET concept_id = $2, origin_concept = COALESCE(origin_concept, $1), modify_time = NOW() WHERE concept_id = $1`)).
		WithArgs("kw:from", "kw:target").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:target").
		WillReturnRows(conceptRow("kw:target", "active", nil))

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
	if !errors.Is(err, ErrMergeRejected) {
		t.Errorf("expected ErrMergeRejected for self-merge, got %v", err)
	}
}

// K8 guardrails: nonexistent source, already-merged source, non-live target,
// and the persisted never_merge assertion (ADR kernel test 21) are all
// refused before anything is written.
func TestMergeConceptGuardrails(t *testing.T) {
	ctx := context.Background()

	t.Run("nonexistent source", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
			WithArgs("kw:ghost").
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()
		_, err := (ConceptStore{DB: db}).MergeConcept(ctx, "kw:ghost", "kw:target")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("already-merged source", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
			WithArgs("kw:from").
			WillReturnRows(conceptRow("kw:from", "merged", "kw:other"))
		mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
			WithArgs("kw:target").
			WillReturnRows(conceptRow("kw:target", "active", nil))
		mock.ExpectRollback()
		_, err := (ConceptStore{DB: db}).MergeConcept(ctx, "kw:from", "kw:target")
		if !errors.Is(err, ErrMergeRejected) {
			t.Errorf("expected ErrMergeRejected, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("tombstone target", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
			WithArgs("kw:from").
			WillReturnRows(conceptRow("kw:from", "active", nil))
		mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
			WithArgs("kw:target").
			WillReturnRows(conceptRow("kw:target", "merged", "kw:x"))
		mock.ExpectRollback()
		_, err := (ConceptStore{DB: db}).MergeConcept(ctx, "kw:from", "kw:target")
		if !errors.Is(err, ErrMergeRejected) {
			t.Errorf("expected ErrMergeRejected, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("never_merge blocks the merge", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
			WithArgs("kw:a").
			WillReturnRows(conceptRow("kw:a", "active", nil))
		mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
			WithArgs("kw:b").
			WillReturnRows(conceptRow("kw:b", "active", nil))
		mock.ExpectQuery(regexp.QuoteMeta(neverMergeSQL)).
			WithArgs("keyword", "kw:a", "kw:b").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectRollback()
		_, err := (ConceptStore{DB: db}).MergeConcept(ctx, "kw:a", "kw:b")
		if !errors.Is(err, ErrMergeRejected) {
			t.Errorf("expected ErrMergeRejected for never_merge, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

// FollowMerge chases the merged_into chain without writing anything (ADR
// kernel test 20: no transitive closure — A→B and B→C resolve A to C, but no
// A→C decision is fabricated). The mock has no Exec expectations, so any
// write would fail the test.
func TestFollowMergeChasesChain(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:a").
		WillReturnRows(conceptRow("kw:a", "merged", "kw:b"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:b").
		WillReturnRows(conceptRow("kw:b", "merged", "kw:c"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:c").
		WillReturnRows(conceptRow("kw:c", "active", nil))

	got, err := store.FollowMerge(ctx, "kw:a")
	if err != nil {
		t.Fatalf("FollowMerge: %v", err)
	}
	if got != "kw:c" {
		t.Errorf("expected chain to resolve to kw:c, got %s", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestFollowMergeCycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:a").
		WillReturnRows(conceptRow("kw:a", "merged", "kw:b"))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:b").
		WillReturnRows(conceptRow("kw:b", "merged", "kw:a"))

	if _, err := store.FollowMerge(ctx, "kw:a"); err == nil {
		t.Error("expected cycle error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// UnmergeConcept restores the tombstoned concept (ADR kernel test 19):
// surfaces whose origin_concept is the concept move back, the tombstone is
// cleared, and the concept resolves to itself again.
func TestUnmergeConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := ConceptStore{DB: db}
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:from").
		WillReturnRows(conceptRow("kw:from", "merged", "kw:target"))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE kb.keyword_surfaces SET concept_id = $1, origin_concept = NULL, modify_time = NOW() WHERE origin_concept = $1`)).
		WithArgs("kw:from").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE kb.keyword_concepts SET status = $2, merged_into = NULL, modify_time = NOW() WHERE concept_id = $1`)).
		WithArgs("kw:from", "active").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:from").
		WillReturnRows(conceptRow("kw:from", "active", nil))

	c, err := store.UnmergeConcept(ctx, "kw:from", "active")
	if err != nil {
		t.Fatalf("UnmergeConcept: %v", err)
	}
	if c.Status != "active" || c.MergedInto != nil {
		t.Errorf("expected restored active concept with no tombstone, got status=%s merged_into=%v", c.Status, c.MergedInto)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUnmergeConceptGuardrails(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid restore status", func(t *testing.T) {
		_, err := (ConceptStore{DB: nil}).UnmergeConcept(ctx, "kw:from", "deprecated")
		if !errors.Is(err, ErrMergeRejected) {
			t.Errorf("expected ErrMergeRejected, got %v", err)
		}
	})

	t.Run("not merged", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
			WithArgs("kw:from").
			WillReturnRows(conceptRow("kw:from", "active", nil))
		_, err := (ConceptStore{DB: db}).UnmergeConcept(ctx, "kw:from", "active")
		if !errors.Is(err, ErrMergeRejected) {
			t.Errorf("expected ErrMergeRejected, got %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
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
