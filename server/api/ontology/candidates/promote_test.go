package candidates

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPromoteToContentRequiresApprovedStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDraft)).
			AddRow(int64(1), "term", []byte(`{"term_id":"core:assertion"}`), "core", "llm", nil,
				[]byte("null"), nil, nil, "fp", []byte("null"), StatusDraft, nil, nil, nil,
				now, nil, now, nil, nil))

	store := CandidateStore{DB: db}
	if _, err := store.PromoteToContent(context.Background(), 1, "curator"); err == nil {
		t.Fatal("expected error for non-approved candidate")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPromoteToContentMaterializesTerm(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payload := []byte(`{"term_id":"core:assertion","term_kind":"class","module_id":"core","definition":"An assertion is a qualified claim."}`)
	now := time.Now()

	// GetCandidate: approved term candidate.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusApproved)).
			AddRow(int64(1), "term", payload, "core", "llm", nil,
				[]byte("null"), nil, nil, "fp", []byte("null"), StatusApproved, nil, nil, nil,
				now, nil, now, nil, nil))
	// contentExists on kb.ontology_terms -> false (nothing materialized yet).
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM kb.ontology_terms WHERE source_candidate_id = $1)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	// CreateTerm inserts version 1 with source_candidate_id = candidate id.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs("core:assertion", "class", "core", "approved", "An assertion is a qualified claim.", nil, int64(1), "curator", "curator").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "create_time", "create_by",
			"modify_time", "modify_by",
		}).AddRow(int64(10), "core:assertion", 1, "class", "core", "approved",
			"An assertion is a qualified claim.", nil, int64(1), now, "curator", now, "curator"))

	store := CandidateStore{DB: db}
	got, err := store.PromoteToContent(context.Background(), 1, "curator")
	if err != nil {
		t.Fatalf("PromoteToContent: %v", err)
	}
	// Candidate stays approved; included_in_release is chunk B's release act.
	if got.Status != StatusApproved {
		t.Fatalf("candidate status should remain approved after promotion, got %q", got.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPromoteToContentSkipsWhenAlreadyMaterialized(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payload := []byte(`{"term_id":"core:assertion","term_kind":"class","module_id":"core"}`)
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusApproved)).
			AddRow(int64(1), "term", payload, "core", "llm", nil,
				[]byte("null"), nil, nil, "fp", []byte("null"), StatusApproved, nil, nil, nil,
				now, nil, now, nil, nil))
	// Content already exists for this candidate -> no INSERT at all.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (SELECT 1 FROM kb.ontology_terms WHERE source_candidate_id = $1)")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	store := CandidateStore{DB: db}
	if _, err := store.PromoteToContent(context.Background(), 1, "curator"); err != nil {
		t.Fatalf("PromoteToContent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
