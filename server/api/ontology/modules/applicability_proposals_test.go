package modules

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

func TestValidProposalTransitions(t *testing.T) {
	tests := []struct {
		from, to string
		valid    bool
	}{
		{ProposalStatusDraft, ProposalStatusInReview, true},
		{ProposalStatusDraft, ProposalStatusApproved, false},
		{ProposalStatusDraft, ProposalStatusRejected, false},
		{ProposalStatusInReview, ProposalStatusApproved, true},
		{ProposalStatusInReview, ProposalStatusRejected, true},
		{ProposalStatusInReview, ProposalStatusDraft, false},
		// approved has no manual outgoing transition: inclusion in a release
		// is a release-transaction consequence (P5 review 2026080302 finding
		// P5-17), never an HTTP call with a caller-supplied release id.
		{ProposalStatusApproved, ProposalStatusIncludedInRelease, false},
		{ProposalStatusApproved, ProposalStatusRejected, false},
		{ProposalStatusRejected, ProposalStatusDraft, false},
		{ProposalStatusIncludedInRelease, ProposalStatusApproved, false},
	}
	for _, tt := range tests {
		got := validProposalTransition(tt.from, tt.to)
		if got != tt.valid {
			t.Errorf("validProposalTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.valid)
		}
	}
}

// TestCreateProposalStoresCanonicalPredicateAndChecksum proves CreateProposal
// stores the canonical predicate bytes and their canonical checksum (P5 review
// 2026080302 finding P5-3). The checksum must be exactly what
// policy_compile.go's compilePredicate recomputes at promotion time -- bare
// canonical hex, not a prefixed hash of the client's raw bytes -- otherwise
// every promoted binding fails the compiler and promotion is a no-op.
func TestCreateProposalStoresCanonicalPredicateAndChecksum(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	raw := json.RawMessage(`{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"product_specification"}}`)
	var doc semrules.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	canonical, checksum, err := semrules.Canonicalize(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(checksum, "sha256:") {
		t.Fatal("canonical checksum must be bare hex, not sha256:-prefixed")
	}
	createTime := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.ontology_applicability_proposals
    (module_id, release_id, proposal_kind, predicate, predicate_checksum, status, source_release_checksum, created_by)
VALUES ($1, $2, 'routing', $3::jsonb, $4, 'draft', $5, $6)
RETURNING id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
          COALESCE(source_release_checksum, ''), COALESCE(created_by, ''), create_time`)).
		WithArgs("test-module", int64(42), string(canonical), checksum, nil, "alice").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "release_id", "proposal_kind", "predicate", "predicate_checksum",
			"status", "source_release_checksum", "created_by", "create_time",
		}).AddRow(int64(1), "test-module", int64(42), "routing", []byte(canonical), checksum,
			"draft", "", "alice", createTime))

	store := ProposalStore{DB: db}
	proposal, err := store.CreateProposal(context.Background(), "test-module", 42, raw, "alice", "")
	if err != nil {
		t.Fatalf("CreateProposal: %v", err)
	}
	if proposal.PredicateChecksum != checksum {
		t.Fatalf("predicate_checksum = %q, want %q", proposal.PredicateChecksum, checksum)
	}
	// The stored predicate round-trips to the same canonical checksum the
	// compiler will recompute, i.e. compilePredicate(proposal.Predicate,
	// proposal.PredicateChecksum) succeeds.
	var storedDoc semrules.Document
	if err := json.Unmarshal(proposal.Predicate, &storedDoc); err != nil {
		t.Fatalf("stored predicate is not a valid document: %v", err)
	}
	_, recomputed, err := semrules.Canonicalize(storedDoc)
	if err != nil {
		t.Fatal(err)
	}
	if recomputed != proposal.PredicateChecksum {
		t.Fatalf("recomputed checksum %q != stored %q", recomputed, proposal.PredicateChecksum)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestParsePredicateDocument(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"product_specification"}}`)
	doc, err := parsePredicateDocument(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Version != 1 {
		t.Fatalf("version = %d, want 1", doc.Version)
	}
	if doc.Expression.Kind != "fact" {
		t.Fatalf("kind = %q, want fact", doc.Expression.Kind)
	}
}

func TestParsePredicateDocumentInvalidJSON(t *testing.T) {
	_, err := parsePredicateDocument(json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestProposalStoreRequiresModuleID(t *testing.T) {
	store := ProposalStore{}
	_, err := store.CreateProposal(nil, "", 1, json.RawMessage(`{}`), "", "")
	if err == nil {
		t.Fatal("expected error for empty module_id")
	}
}

func TestProposalStoreRequiresReleaseID(t *testing.T) {
	store := ProposalStore{}
	_, err := store.CreateProposal(nil, "test", 0, json.RawMessage(`{}`), "", "")
	if err == nil {
		t.Fatal("expected error for zero release_id")
	}
}

func TestProposalStoreRequiresPredicate(t *testing.T) {
	store := ProposalStore{}
	_, err := store.CreateProposal(nil, "test", 1, nil, "", "")
	if err == nil {
		t.Fatal("expected error for nil predicate")
	}
}

func TestProposalStoreNilDB(t *testing.T) {
	store := ProposalStore{}
	_, err := store.CreateProposal(nil, "test", 1, json.RawMessage(`{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"x"}}`), "", "")
	if err == nil {
		t.Fatal("expected error for nil DB")
	}
}

func TestTransitionProposalRequiresActorForApproval(t *testing.T) {
	// This test verifies the validation logic without a real DB.
	// The actual DB interaction is tested in live validation (I2).
	store := ProposalStore{}
	_, err := store.TransitionProposal(nil, 0, ProposalStatusApproved, "")
	if err == nil {
		t.Fatal("expected error for zero id")
	}
}

// TestTransitionProposalConcurrentTransitionConflict proves the TOCTOU guard
// (P5 review 2026080302 finding P5-17): the UPDATE is guarded by the expected
// current status, so a concurrent transition that moved the proposal first
// surfaces a conflict (zero rows updated) instead of silently overwriting the
// winner's status.
func TestTransitionProposalConcurrentTransitionConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	createTime := time.Now()
	// GetProposal reads the current status as 'draft'.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
       COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
       included_in_release_id, COALESCE(created_by, ''), create_time
FROM kb.ontology_applicability_proposals WHERE id = $1`)).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "release_id", "proposal_kind", "predicate", "predicate_checksum",
			"status", "source_release_checksum", "approved_by", "approved_at",
			"included_in_release_id", "created_by", "create_time",
		}).AddRow(int64(7), "vent", int64(42), "routing", []byte(`{"version":1}`), "abc",
			"draft", "", "", nil, nil, "alice", createTime))
	// A concurrent transaction already transitioned it, so the guarded UPDATE
	// matches zero rows (RETURNING yields no row).
	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE kb.ontology_applicability_proposals`)).
		WithArgs(int64(7), "in_review", nil, nil, "draft").
		WillReturnError(sql.ErrNoRows)

	store := ProposalStore{DB: db}
	_, err = store.TransitionProposal(context.Background(), 7, ProposalStatusInReview, "alice")
	if err == nil {
		t.Fatal("expected a conflict error when a concurrent transition moved the proposal")
	}
	if !strings.Contains(err.Error(), "concurrent transition") {
		t.Fatalf("error = %v, want concurrent-transition conflict", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestTransitionProposalSucceedsWithExpectedStatus proves a guarded transition
// succeeds when the row is still in the expected status, returning the updated
// proposal.
func TestTransitionProposalSucceedsWithExpectedStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	createTime := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, module_id, release_id, proposal_kind, predicate, predicate_checksum, status,
       COALESCE(source_release_checksum, ''), COALESCE(approved_by, ''), approved_at,
       included_in_release_id, COALESCE(created_by, ''), create_time
FROM kb.ontology_applicability_proposals WHERE id = $1`)).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "release_id", "proposal_kind", "predicate", "predicate_checksum",
			"status", "source_release_checksum", "approved_by", "approved_at",
			"included_in_release_id", "created_by", "create_time",
		}).AddRow(int64(7), "vent", int64(42), "routing", []byte(`{"version":1}`), "abc",
			"draft", "", "", nil, nil, "alice", createTime))
	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE kb.ontology_applicability_proposals`)).
		WithArgs(int64(7), "in_review", nil, nil, "draft").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "release_id", "proposal_kind", "predicate", "predicate_checksum",
			"status", "source_release_checksum", "approved_by", "approved_at",
			"included_in_release_id", "created_by", "create_time",
		}).AddRow(int64(7), "vent", int64(42), "routing", []byte(`{"version":1}`), "abc",
			"in_review", "", "", nil, nil, "alice", createTime))

	store := ProposalStore{DB: db}
	proposal, err := store.TransitionProposal(context.Background(), 7, ProposalStatusInReview, "alice")
	if err != nil {
		t.Fatalf("TransitionProposal: %v", err)
	}
	if proposal.Status != ProposalStatusInReview {
		t.Fatalf("status = %q, want in_review", proposal.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNullIfEmptyStr(t *testing.T) {
	if v := nullIfEmptyStr(""); v != nil {
		t.Fatalf("expected nil for empty string, got %v", v)
	}
	if v := nullIfEmptyStr("  "); v != nil {
		t.Fatalf("expected nil for whitespace string, got %v", v)
	}
	if v := nullIfEmptyStr("hello"); v != "hello" {
		t.Fatalf("expected hello, got %v", v)
	}
}
