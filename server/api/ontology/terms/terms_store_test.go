package terms

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTermStoreCreateTermInsertsVersionOne(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs("core:assertion", "class", "core", "draft", "An assertion is a qualified claim.", nil, nil, nil, nil, nil, "tester", "tester").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "value_type", "range_type",
			"permitted_unit_term_ids", "create_time", "create_by",
			"modify_time", "modify_by",
		}).AddRow(int64(1), "core:assertion", 1, "class", "core", "draft",
			"An assertion is a qualified claim.", nil, nil, nil, nil, nil, now, "tester", now, "tester"))

	store := TermStore{DB: db}
	got, err := store.CreateTerm(context.Background(), Term{
		TermID:     "core:assertion",
		TermKind:   "class",
		ModuleID:   "core",
		Status:     "draft",
		Definition: "An assertion is a qualified claim.",
		CreateBy:   "tester",
		ModifyBy:   "tester",
	})
	if err != nil {
		t.Fatalf("CreateTerm: %v", err)
	}
	if got.TermID != "core:assertion" || got.Version != 1 || got.TermKind != "class" {
		t.Fatalf("unexpected term: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTermStoreCreateTermRejectsUnsupportedKind(t *testing.T) {
	store := TermStore{DB: nil}
	_, err := store.CreateTerm(context.Background(), Term{TermID: "x", TermKind: "owl:Class", ModuleID: "core"})
	if err == nil {
		t.Fatal("expected error for unsupported term_kind")
	}
}

func TestTermStoreCreateTermRequiresModule(t *testing.T) {
	store := TermStore{DB: nil}
	_, err := store.CreateTerm(context.Background(), Term{TermID: "core:assertion", TermKind: "class"})
	if err == nil {
		t.Fatal("expected error for missing module_id")
	}
}

func TestTermStoreGetTermLatestReadsCompatibilityCurrentView(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms_current\nWHERE term_id = $1")).
		WithArgs("core:assertion").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "value_type", "range_type",
			"permitted_unit_term_ids", "create_time", "create_by",
			"modify_time", "modify_by",
		}).AddRow(int64(3), "core:assertion", 2, "class", "core", "approved",
			"An assertion is a qualified claim.", nil, nil, nil, nil, nil, now, "tester", now, "tester"))

	store := TermStore{DB: db}
	got, err := store.GetTermLatest(context.Background(), "core:assertion")
	if err != nil {
		t.Fatalf("GetTermLatest: %v", err)
	}
	if got.Version != 2 || got.Status != "approved" {
		t.Fatalf("unexpected latest term: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTermStoreGetTermReadsAppendOnlyRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_revisions\nWHERE term_id = $1 AND revision = $2")).
		WithArgs("core:assertion", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "value_type", "range_type",
			"permitted_unit_term_ids", "create_time", "create_by",
			"modify_time", "modify_by",
		}).AddRow(int64(1), "core:assertion", 1, "class", "core", "draft",
			"An assertion is a qualified claim.", nil, nil, nil, nil, nil, now, "tester", now, "tester"))

	store := TermStore{DB: db}
	got, err := store.GetTerm(context.Background(), "core:assertion", 1)
	if err != nil {
		t.Fatalf("GetTerm: %v", err)
	}
	if got.ID != 1 || got.Version != 1 || got.Status != "draft" {
		t.Fatalf("unexpected historic term: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTermStoreTransitionStatusEnforcesStateMachine(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	// GetTerm reads the immutable prior revision; draft -> rejected appends v2.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_revisions\nWHERE term_id = $1 AND revision = $2")).
		WithArgs("core:assertion", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "value_type", "range_type",
			"permitted_unit_term_ids", "create_time", "create_by",
			"modify_time", "modify_by",
		}).AddRow(int64(1), "core:assertion", 1, "class", "core", "draft",
			"", nil, nil, nil, nil, nil, now, "", now, ""))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs("core:assertion", "class", "core", "rejected", nil, nil, nil, nil, nil, nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "value_type", "range_type",
			"permitted_unit_term_ids", "create_time", "create_by",
			"modify_time", "modify_by",
		}).AddRow(int64(2), "core:assertion", 2, "class", "core", "rejected",
			"", nil, nil, nil, nil, nil, now, "", now, ""))

	store := TermStore{DB: db}
	got, err := store.TransitionStatus(context.Background(), "core:assertion", 1, "rejected", "")
	if err != nil {
		t.Fatalf("TransitionStatus: %v", err)
	}
	if got.Status != "rejected" || got.Version != 2 {
		t.Fatalf("unexpected status: %q", got.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTermStoreTransitionStatusRejectsIllegalArc(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_revisions\nWHERE term_id = $1 AND revision = $2")).
		WithArgs("core:assertion", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "value_type", "range_type",
			"permitted_unit_term_ids", "create_time", "create_by",
			"modify_time", "modify_by",
		}).AddRow(int64(1), "core:assertion", 1, "class", "core", "approved",
			"", nil, nil, nil, nil, nil, now, "", now, ""))

	store := TermStore{DB: db}
	_, err = store.TransitionStatus(context.Background(), "core:assertion", 1, "draft", "")
	if err == nil {
		t.Fatal("expected error for approved -> draft transition")
	}
	if !errors.Is(err, errIllegalTermTransition) {
		t.Fatalf("expected errIllegalTermTransition, got %v", err)
	}
}

func TestTermStoreNilDBErrors(t *testing.T) {
	store := TermStore{DB: nil}
	if _, err := store.GetTermLatest(context.Background(), "core:assertion"); err == nil {
		t.Fatal("expected error for nil db")
	}
	if _, err := store.GetTerm(context.Background(), "core:assertion", 0); err == nil {
		t.Fatal("expected error for version < 1")
	}
}
