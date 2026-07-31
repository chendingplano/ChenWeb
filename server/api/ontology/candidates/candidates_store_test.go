package candidates

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func candidateRow(id int64, status string) []string {
	return []string{
		"id", "candidate_kind", "proposed_payload", "proposed_module_id", "source_type",
		"source_ref", "source_line_spans", "discovery_method", "confidence", "fingerprint",
		"candidate_matches", "status", "decision_reason", "dependency_fingerprint",
		"proposed_by", "create_time", "create_by", "modify_time", "modify_by",
	}
}

func TestCreateCandidateInsertsAndComputesFingerprint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payload := []byte(`{"term_id":"core:assertion","term_kind":"class"}`)
	fp, err := Fingerprint(payload, "llm", "rec:1", "core")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payload), "core", "llm", "rec:1", "null", nil, nil, fp, "null",
			StatusDiscovered, nil, "tester", "tester").
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDiscovered)).
			AddRow(int64(1), "term", payload, "core", "llm", "rec:1", []byte("null"), nil, nil, fp,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "tester", now, "tester"))

	store := CandidateStore{DB: db}
	got, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind:    "term",
		ProposedPayload:  payload,
		ProposedModuleID: "core",
		SourceType:       "llm",
		SourceRef:        "rec:1",
		CreateBy:         "tester",
		ModifyBy:         "tester",
	})
	if err != nil {
		t.Fatalf("CreateCandidate: %v", err)
	}
	if got.Reused || got.Fingerprint != fp || got.Status != StatusDiscovered {
		t.Fatalf("unexpected candidate: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestCreateCandidateReusesExistingOnIdenticalFingerprint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payload := []byte(`{"term_id":"core:assertion","term_kind":"class"}`)
	fp, err := Fingerprint(payload, "llm", "rec:1", "core")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	now := time.Now()

	// ON CONFLICT (fingerprint) DO NOTHING -> no row returned.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payload), "core", "llm", "rec:1", "null", nil, nil, fp, "null",
			StatusDiscovered, nil, "tester", "tester").
		WillReturnError(sql.ErrNoRows)
	// Existing candidate is fetched by fingerprint.
	mock.ExpectQuery(regexp.QuoteMeta("WHERE fingerprint = $1")).
		WithArgs(fp).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusApproved)).
			AddRow(int64(7), "term", payload, "core", "llm", "rec:1", []byte("null"), nil, nil, fp,
				[]byte("null"), StatusApproved, nil, nil, nil, now, "system", now, "system"))

	store := CandidateStore{DB: db}
	got, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind:    "term",
		ProposedPayload:  payload,
		ProposedModuleID: "core",
		SourceType:       "llm",
		SourceRef:        "rec:1",
		CreateBy:         "tester",
		ModifyBy:         "tester",
	})
	if err != nil {
		t.Fatalf("CreateCandidate: %v", err)
	}
	if !got.Reused || got.ID != 7 || got.Status != StatusApproved {
		t.Fatalf("expected reused existing candidate, got %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTransitionStatusMovesDraftToInReview(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDraft)).
			AddRow(int64(1), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusDraft, nil, nil, nil, now, nil, now, nil))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_candidates")).
		WithArgs(int64(1), StatusInReview, "curator").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusInReview)).
			AddRow(int64(1), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusInReview, nil, nil, nil, now, nil, now, nil))

	store := CandidateStore{DB: db}
	got, err := store.TransitionStatus(context.Background(), 1, StatusInReview, "curator")
	if err != nil {
		t.Fatalf("TransitionStatus: %v", err)
	}
	if got.Status != StatusInReview {
		t.Fatalf("unexpected status: %q", got.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTransitionStatusRejectsSkipToApproved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDiscovered)).
			AddRow(int64(1), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, nil, now, nil))

	store := CandidateStore{DB: db}
	_, err = store.TransitionStatus(context.Background(), 1, StatusApproved, "curator")
	if err == nil {
		t.Fatal("expected error for discovered -> approved")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestRetryDeferredRequiresChangedDependencyFingerprint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDeferred)).
			AddRow(int64(1), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusDeferred, nil, "dep-v1", nil, now, nil, now, nil))

	store := CandidateStore{DB: db}
	// Same dependency fingerprint -> not eligible.
	_, retryErr := store.RetryDeferred(context.Background(), 1, "dep-v1", "curator")
	if retryErr == nil {
		t.Fatal("expected error when dependency fingerprint is unchanged")
	}
	if !errors.Is(retryErr, errNoRetryWithoutDependencyChange) {
		t.Fatalf("expected errNoRetryWithoutDependencyChange, got %v", retryErr)
	}
	// Blank fingerprint -> refused.
	if _, err := store.RetryDeferred(context.Background(), 1, "", "curator"); err == nil {
		t.Fatal("expected error for blank new fingerprint")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestDeferCandidateRecordsDependencyFingerprint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDraft)).
			AddRow(int64(1), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusDraft, nil, nil, nil, now, nil, now, nil))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_candidates")).
		WithArgs(int64(1), "dep-v1", "blocked on dependency", "curator").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDeferred)).
			AddRow(int64(1), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusDeferred, "blocked on dependency", "dep-v1", nil, now, nil, now, nil))

	store := CandidateStore{DB: db}
	got, err := store.DeferCandidate(context.Background(), 1, "dep-v1", "blocked on dependency", "curator")
	if err != nil {
		t.Fatalf("DeferCandidate: %v", err)
	}
	if got.Status != StatusDeferred || got.DependencyFingerprint != "dep-v1" {
		t.Fatalf("unexpected deferred candidate: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestDeferCandidateRequiresFingerprintAndEditableState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	store := CandidateStore{DB: db}
	if _, err := store.DeferCandidate(context.Background(), 1, "", "blocked", "curator"); err == nil {
		t.Fatal("expected error for blank dependency fingerprint")
	}

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows(candidateRow(2, StatusApproved)).
			AddRow(int64(2), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusApproved, nil, nil, nil, now, nil, now, nil))
	if _, err := store.DeferCandidate(context.Background(), 2, "dep-v1", "blocked", "curator"); err == nil {
		t.Fatal("expected error deferring an approved candidate")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestUpdatePayloadFrozenAfterInReview(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusInReview)).
			AddRow(int64(1), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusInReview, nil, nil, nil, now, nil, now, nil))

	store := CandidateStore{DB: db}
	if _, err := store.UpdatePayload(context.Background(), 1, []byte(`{"x":1}`), "curator"); err == nil {
		t.Fatal("expected error for payload edit in in_review")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
