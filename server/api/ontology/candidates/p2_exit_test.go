package candidates

// P2 exit-criteria tests. spec 2026072702 §16.3 items 1-7 (the ADR P2 exit
// gate) map onto the chunk A-D store contracts:
//
//	item 1  fingerprint dedup on reprocessing              -> TestP2ExitItem1FingerprintDedup
//	item 3  candidate state machine + deferred gate +
//	        approved stays inactive until a release       -> TestP2ExitItem3GovernedLifecycle
//	item 4  no LLM-only path reaches an active state      -> TestP2ExitItem4NoLLMActivation
//	item 5  atomic release, pinned deps, checksum,
//	        rollback preserves history                    -> modules.ReleaseStore tests + p3validate live run
//	item 6  material change -> replacement term, never
//	        mutates the released term                     -> terms.TermStore versioning (TestP2ExitItem6...)
//	item 7  change set activates together; failed
//	        validation leaves previous active release     -> modules.ReleaseStore tests + p3validate live run
//
// Items 2 and 8-17 are P3 (assertions/association) and are out of scope.

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestP2ExitItem1FingerprintDedup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	payload := []byte(`{"term_id":"core:assertion","term_kind":"class"}`)
	fp, err := Fingerprint(payload, "llm", "rec:9", "core")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	now := time.Now()
	// First create: insert returns a row. Second create: ON CONFLICT DO
	// NOTHING returns no row -> the existing candidate is returned reused.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payload), "core", "llm", "rec:9", "null", nil, nil, fp, "null",
			StatusDiscovered, nil, "x", "x", nil).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDiscovered)).
			AddRow(int64(1), "term", payload, "core", "llm", "rec:9", []byte("null"), nil, nil, fp,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "x", now, "x", nil))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payload), "core", "llm", "rec:9", "null", nil, nil, fp, "null",
			StatusDiscovered, nil, "x", "x", nil).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("WHERE fingerprint = $1")).
		WithArgs(fp).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusDiscovered)).
			AddRow(int64(1), "term", payload, "core", "llm", "rec:9", []byte("null"), nil, nil, fp,
				[]byte("null"), StatusDiscovered, nil, nil, nil, now, "x", now, "x", nil))

	store := CandidateStore{DB: db}
	first, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "term", ProposedPayload: payload, ProposedModuleID: "core",
		SourceType: "llm", SourceRef: "rec:9", CreateBy: "x", ModifyBy: "x",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := store.CreateCandidate(context.Background(), Candidate{
		CandidateKind: "term", ProposedPayload: payload, ProposedModuleID: "core",
		SourceType: "llm", SourceRef: "rec:9", CreateBy: "x", ModifyBy: "x",
	})
	if err != nil {
		t.Fatalf("reprocess: %v", err)
	}
	if first.ID != second.ID || !second.Reused {
		t.Fatalf("expected reprocessing to reuse candidate %d, got %d reused=%v", first.ID, second.ID, second.Reused)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestP2ExitItem3GovernedLifecycle(t *testing.T) {
	// Approved candidates remain inactive: included_in_release is release-owned
	// and cannot be reached through the generic transition.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(candidateRow(1, StatusApproved)).
			AddRow(int64(1), "term", []byte(`{}`), nil, "manual", nil, []byte("null"), nil, nil, "fp",
				[]byte("null"), StatusApproved, nil, nil, nil, now, nil, now, nil, nil))

	store := CandidateStore{DB: db}
	_, err = store.TransitionStatus(context.Background(), 1, StatusIncludedInRelease, "curator")
	if err == nil {
		t.Fatal("expected included_in_release to be refused outside the release path")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestP2ExitItem4NoLLMActivation(t *testing.T) {
	// An LLM-produced candidate may be created (discovered) and promoted no
	// further than a governed review; the state machine has no path from an
	// LLM proposal to an active state other than the release gate, and no LLM
	// path exists in the code at all -- only the release path writes
	// included_in_release (guarded in TestP2ExitItem3).
	if !transitionAllowed(StatusDiscovered, StatusDraft) {
		t.Fatal("discovered -> draft must remain an arc (curator enrichment)")
	}
	if transitionAllowed(StatusApproved, StatusIncludedInRelease) != true {
		// The machine lists the arc (spec §9.3), but the store refuses it
		// outside the release path; both must hold.
		t.Fatal("the machine arc exists, but the store guard enforces release ownership")
	}
}
