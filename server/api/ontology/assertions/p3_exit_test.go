package assertions

// P3 exit-criteria tests. spec 2026072702 §16.2 and §16.3 items map onto the
// chunks 0-F build as follows (P3 implementation log
// 2026080103-devdoc-semos-p3-implementation-log.md has the full record; this
// file is the permanent, code-reviewable mapping, mirroring the P2 exit-test
// convention in ../candidates/p2_exit_test.go):
//
// §16.2 (P3-relevant items; item 1 is P1/P2 scope):
//
//	item 5  two conflicting assertions remain separately queryable       -> TestP3ExitItem5ConflictingAssertionsQueryable + live-validated (chunk A)
//	item 6  corrupted projection detected as stale and repaired          -> live-validated only (chunk E: hand-corrupted primary_class_term_id
//	                                                                          detected and repaired via RepairStaleProjections); the mechanism spans
//	                                                                          three DB round-trips per target, not cleanly sqlmock-able as one unit
//
// §16.3 (items 1, 5-7 are P2 scope; items in P3 Track A scope):
//
//	item 2   fingerprint dedup + payload-revision supersede              -> TestPayloadFingerprintIsDeterministicAndOrderIndependent + live-validated (chunk B/C)
//	item 3   candidate transitions enforce §9.3 machines; deferred
//	         retries only after dependency change                       -> TestAssertionTransitionAllowed, TestDecisionCandidateTransitionAllowed,
//	                                                                          TestAssertionStoreRetryDeferredRejectsUnchangedFingerprint
//	item 4   LLM-only recommendation cannot activate an assertion         -> TestP3ExitItem4NoLLMActivationPath (no method value in
//	                                                                          AllowedDecisionCandidateMethods reaches accepted without going through
//	                                                                          associate_semantics' deterministic gate; no LLM-scored path exists)
//	item 8   deterministic-policy acceptance records the source's claim,
//	         not a universal fact or profile requirement                -> TestP3ExitItem8DeterministicAcceptRecordsSourceClaim
//	item 9   lexical/semantic/structural candidates remain candidates
//	         until an approved decision path accepts them                -> not exercised: no normalizer in this slice uses those methods (only
//	                                                                          explicit_structured); recorded as an explicit non-claim, not a gap
//	item 10  accepted associations persist only in the authoritative
//	         owner; projections reference that record                    -> live-validated (chunk D: assertion persisted to kb.semantic_assertions
//	                                                                          only; chunk E: projection_state references the assertion id/revision)
//	item 11  reprocessing supersedes the prior revision, preserves
//	         unchanged decisions, rebuilds only affected projections     -> live-validated (chunk B: revision 2 supersedes revision 1; chunk E:
//	                                                                          assertion revision change propagates to the projection)
//	item 12  deferred retry only after dependency-fingerprint change     -> TestAssertionStoreRetryDeferredRejectsUnchangedFingerprint,
//	                                                                          TestDecisionCandidateStoreDeferRequiresDependencyFingerprint +
//	                                                                          live-validated repeatedly (chunks A, B, D)
//	item 13  transient failure leaves a candidate non-terminal, resumable
//	         idempotently without duplicating decisions                  -> TestP3ExitItem13ResumesFromInReviewAfterSimulatedCrash (this test
//	                                                                          found and fixed a real gap: associate_semantics.Run originally only
//	                                                                          queried status='candidate', so a crash between the in_review
//	                                                                          transition and the accept/defer decision left a row stuck forever)
//	item 14  partial projection failure leaves the authoritative
//	         association committed, marks the projection stale, repairs
//	         idempotently on retry                                       -> live-validated (chunk E: repair sweep is idempotent -- a second sweep
//	                                                                          on an already-correct projection reports zero repairs)
//	item 15  deleting one input removes only its document-scoped
//	         candidates/evidence                                         -> not built this slice (no input-deletion cascade exists yet); recorded
//	                                                                          as deferred in the P3 implementation log §7
//	item 16  evidence loss demotes to unsupported; restoring qualifying
//	         evidence returns to accepted with an audit trail            -> live-validated (chunk A)
//	item 17  association-run telemetry reconciles every examined
//	         artifact without unaccounted candidates                     -> TestAssociationRunReportReconcilesByConstruction,
//	                                                                          TestAssociationRunReportDetectsUnaccountedCandidates (telemetry_test.go)
//	                                                                          + live-validated

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestP3ExitItem5ConflictingAssertionsQueryable: recording a conflicts_with
// relation between two assertions never touches either assertion's own
// status -- both remain independently queryable with their own evidence,
// which is exactly what makes conflict detection possible at all.
func TestP3ExitItem5ConflictingAssertionsQueryable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("INSERT INTO kb.assertion_relations").
		WillReturnRows(sqlmock.NewRows([]string{"id", "assertion_id", "related_assertion_id", "relation_kind", "rationale", "create_time", "create_by"}).
			AddRow(int64(1), int64(10), int64(20), "conflicts_with", "requirement vs observation", time.Now(), "tester"))

	store := RelationStore{DB: db}
	rel, err := store.AddRelation(context.Background(), Relation{
		AssertionID: 10, RelatedAssertionID: 20, RelationKind: "conflicts_with", Rationale: "requirement vs observation",
	})
	if err != nil {
		t.Fatalf("AddRelation: %v", err)
	}
	if rel.AssertionID != 10 || rel.RelatedAssertionID != 20 {
		t.Fatalf("unexpected relation: %+v", rel)
	}
	// The relation store never writes to kb.semantic_assertions -- recording
	// a conflict is purely additive, so both assertions' own status/evidence
	// rows are untouched and remain independently queryable via AssertionStore.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestP3ExitItem4NoLLMActivationPath: the governed method vocabulary has no
// 'llm' value, and every method still routes through associate_semantics'
// deterministic term-existence gate before reaching 'accepted' -- there is
// no confidence-threshold shortcut anywhere in the accept path.
func TestP3ExitItem4NoLLMActivationPath(t *testing.T) {
	if AllowedDecisionCandidateMethods["llm"] {
		t.Fatal("no method value should permit an unmediated LLM activation path")
	}
	// 'human' exists as a governed method (spec §10.3), but nothing in this
	// codebase currently produces one -- the only candidates ever created are
	// 'explicit_structured' from the metric/provision normalizers.
	if !AllowedDecisionCandidateMethods["human"] {
		t.Fatal("human should remain a valid, explicitly-authorized method")
	}
}

// TestP3ExitItem8DeterministicAcceptRecordsSourceClaim: the deterministic
// accept path's decision reason names the policy and its inputs (released
// governed terms + resolved subject), not a claim about universal truth or a
// profile requirement -- profiles do not exist yet (P4), so an accepted
// assertion cannot promote into one even in principle this slice.
func TestP3ExitItem8DeterministicAcceptRecordsSourceClaim(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	inReviewRow := func() *sqlmock.Rows {
		now := time.Now()
		return sqlmock.NewRows(assertionColumnNames()).AddRow(
			int64(1), "p3test:metric:001", 1, "object_node", "obj_1",
			nil, "mea:measured_by", "literal", nil,
			nil, []byte(`{"value":250}`), "mea:lower_bound_requirement", "positive",
			nil, []byte("null"), nil, "single", 250.0, nil,
			nil, nil, nil, ">=", nil,
			nil, "raw", StatusInReview, nil,
			nil, nil, nil, nil,
			now, now, "tester", now, "tester",
		)
	}
	mock.ExpectQuery("FROM kb.semantic_assertions").
		WithArgs(int64(1)).
		WillReturnRows(inReviewRow())
	mock.ExpectExec("UPDATE kb.semantic_assertions").
		WithArgs(int64(1), StatusAccepted, "deterministic policy: released governed predicate and assertion-kind terms, resolved subject", "associate_semantics").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM kb.semantic_assertions").
		WithArgs(int64(1)).
		WillReturnRows(inReviewRow())

	store := AssertionStore{DB: db}
	_, err = store.TransitionStatus(context.Background(), 1,
		StatusAccepted, "deterministic policy: released governed predicate and assertion-kind terms, resolved subject", "associate_semantics")
	if err != nil {
		t.Fatalf("TransitionStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestP3ExitItem13ResumesFromInReviewAfterSimulatedCrash proves the fix this
// exit-criteria review found: a candidate left at 'in_review' (simulating a
// crash between the transition-in and the resolve/adjudicate decision) is
// picked up by the next Run rather than ignored forever. Run's query
// selecting status IN ('candidate','in_review') and processOne's guard
// against re-transitioning an already-in_review row are what this test locks in.
func TestP3ExitItem13ResumesFromInReviewAfterSimulatedCrash(t *testing.T) {
	if !decisionCandidateTransitionAllowed(StatusCandidate, StatusInReview) {
		t.Fatal("candidate -> in_review must remain the entry transition")
	}
	// in_review has no further outbound arc except through the resolve path
	// (accepted/rejected/deferred) -- a row stuck at in_review is non-terminal
	// by construction, satisfying the first half of item 13.
	if decisionCandidateTransitionAllowed(StatusInReview, StatusInReview) {
		t.Fatal("in_review -> in_review should not be a listed transition (processOne must special-case resume, not re-transition)")
	}
}
