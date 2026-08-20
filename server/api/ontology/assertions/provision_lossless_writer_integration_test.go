package assertions

import (
	"context"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// Task 7.6, gate on, resolved subject, parseable modality: processProvision
// takes the real writer path and produces a represented assertion, one
// supporting evidence link, and one associate-stage outcome envelope.
func TestIntegrationProcessProvisionLosslessMaterializesRepresentedAssertion(t *testing.T) {
	db := freshAssertionsTestDB(t)
	t.Setenv(semantic.GateProvisionLosslessWrites, "true")
	ctx := context.Background()
	seedGovernedTerm(t, db, "prov:has_provision", "property", "provision")
	seedGovernedTerm(t, db, "prov:required", "property", "provision")
	seedObjectNode(t, db, "obj-widget")

	dcStore := DecisionCandidateStore{DB: db}
	dc := proposeAndReviewProvisionCandidate(t, dcStore, "prov-lossless-1", 300, provisionCandidatePayload{
		ProvID:          "prov-lossless-1",
		RawText:         "widgets shall be labeled with a serial number",
		Modality:        "required",
		AssertionKind:   "required",
		SubjectObjectID: "obj-widget",
	})

	a := AssociateSemantics{DB: db}
	outcome, err := a.processProvision(ctx, dcStore, dc)
	if err != nil {
		t.Fatalf("processProvision: %v", err)
	}
	if outcome != "represented" {
		t.Fatalf("outcome = %q, want represented", outcome)
	}

	assertion, err := (AssertionStore{DB: db}).GetLatest(ctx, dc.LogicalIdentityKey)
	if err != nil {
		t.Fatalf("load assertion: %v", err)
	}
	if assertion.Status != StatusRepresented {
		t.Fatalf("status = %q, want represented", assertion.Status)
	}
	if assertion.PredicateTermID != ProvisionPredicateTermID {
		t.Errorf("predicate = %q, want %q", assertion.PredicateTermID, ProvisionPredicateTermID)
	}
	if assertion.AssertionKindTermID != "prov:required" {
		t.Errorf("assertion kind = %q, want prov:required", assertion.AssertionKindTermID)
	}
	if assertion.ValueStateTermID != semantic.ValuePresent {
		t.Errorf("value_state = %q, want present", assertion.ValueStateTermID)
	}

	var supportCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.assertion_evidence
WHERE artifact_type='provision' AND artifact_id='prov-lossless-1' AND input_record_id=300
  AND evidence_role='supports' AND NOT deleted`).Scan(&supportCount); err != nil {
		t.Fatalf("count evidence: %v", err)
	}
	if supportCount != 1 {
		t.Fatalf("active supporting evidence links = %d, want 1", supportCount)
	}

	var outcomeCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.semantic_processing_outcomes
WHERE artifact_type='provision' AND artifact_id='prov-lossless-1' AND active`).Scan(&outcomeCount); err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	if outcomeCount != 1 {
		t.Fatalf("active outcome envelopes = %d, want 1 (associate only)", outcomeCount)
	}

	var candStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM kb.semantic_decision_candidates WHERE id=$1`, dc.ID).Scan(&candStatus); err != nil {
		t.Fatalf("load candidate status: %v", err)
	}
	if candStatus != StatusAccepted {
		t.Errorf("candidate status = %q, want accepted", candStatus)
	}
}

// DR13: "successful materialization supersedes the occurrence." A provision
// that already carries an active fallback occurrence (task 7.4's path, from
// before this family had a real writer) must have that occurrence marked
// inactive and pointed at the new assertion once the real writer runs --
// otherwise the occurrence is left stale-active alongside the assertion it
// now duplicates, discovered live against real corpus data during task 7.6.
func TestIntegrationProcessProvisionLosslessSupersedesPreexistingFallbackOccurrence(t *testing.T) {
	db := freshAssertionsTestDB(t)
	ctx := context.Background()
	seedGovernedTerm(t, db, "prov:has_provision", "property", "provision")
	seedGovernedTerm(t, db, "prov:required", "property", "provision")
	seedObjectNode(t, db, "obj-widget-4")

	dcStore := DecisionCandidateStore{DB: db}
	dc := proposeAndReviewProvisionCandidate(t, dcStore, "prov-lossless-supersede", 304, provisionCandidatePayload{
		ProvID:          "prov-lossless-supersede",
		RawText:         "widgets shall be labeled",
		Modality:        "required",
		AssertionKind:   "required",
		SubjectObjectID: "obj-widget-4",
	})

	a := AssociateSemantics{DB: db}
	// First: gate off, fallback on -- reproduces the pre-7.6 state a real
	// corpus artifact could already be in.
	t.Setenv(semantic.GateFallbackWrites, "true")
	if err := a.writeProvisionFallbackOccurrence(ctx, dc, "no_governed_deontic_predicate"); err != nil {
		t.Fatalf("seed fallback occurrence: %v", err)
	}
	var occIDBefore int64
	var activeBefore bool
	if err := db.QueryRowContext(ctx, `
SELECT id, active FROM kb.unresolved_semantic_occurrences
WHERE artifact_type='provision' AND artifact_id='prov-lossless-supersede'`).Scan(&occIDBefore, &activeBefore); err != nil {
		t.Fatalf("load seeded occurrence: %v", err)
	}
	if !activeBefore {
		t.Fatal("seeded occurrence must start active")
	}

	// Now: the real writer runs for the same artifact.
	t.Setenv(semantic.GateProvisionLosslessWrites, "true")
	outcome, err := a.processProvision(ctx, dcStore, dc)
	if err != nil {
		t.Fatalf("processProvision: %v", err)
	}
	if outcome != "represented" {
		t.Fatalf("outcome = %q, want represented", outcome)
	}

	var activeAfter bool
	var resultingAssertionID, currentOutcomeID *int64
	if err := db.QueryRowContext(ctx, `
SELECT active, resulting_assertion_id, current_outcome_id FROM kb.unresolved_semantic_occurrences WHERE id=$1`,
		occIDBefore).Scan(&activeAfter, &resultingAssertionID, &currentOutcomeID); err != nil {
		t.Fatalf("reload occurrence: %v", err)
	}
	if activeAfter {
		t.Error("occurrence must be superseded (inactive) once the real writer materializes an assertion")
	}
	if resultingAssertionID == nil {
		t.Error("occurrence must record the resulting assertion id")
	}
	if currentOutcomeID == nil {
		t.Error("occurrence must record the current outcome id")
	}
}

// Task 7.6, gate on, unresolved subject: defers as unresolved_referent, the
// same reason metrics use, and writes nothing to the fallback store (DR1:
// a SupportsInstances family never uses the unresolved-occurrence path).
func TestIntegrationProcessProvisionLosslessDefersOnUnresolvedSubject(t *testing.T) {
	db := freshAssertionsTestDB(t)
	t.Setenv(semantic.GateProvisionLosslessWrites, "true")
	t.Setenv(semantic.GateFallbackWrites, "true") // prove it's still not used
	ctx := context.Background()
	seedGovernedTerm(t, db, "prov:has_provision", "property", "provision")
	seedGovernedTerm(t, db, "prov:required", "property", "provision")

	dcStore := DecisionCandidateStore{DB: db}
	dc := proposeAndReviewProvisionCandidate(t, dcStore, "prov-lossless-unresolved", 301, provisionCandidatePayload{
		ProvID:        "prov-lossless-unresolved",
		RawText:       "widgets shall be labeled",
		Modality:      "required",
		AssertionKind: "required",
		// SubjectObjectID intentionally empty.
	})

	a := AssociateSemantics{DB: db}
	outcome, err := a.processProvision(ctx, dcStore, dc)
	if err != nil {
		t.Fatalf("processProvision: %v", err)
	}
	if outcome != "deferred" {
		t.Fatalf("outcome = %q, want deferred", outcome)
	}

	var candReason string
	if err := db.QueryRowContext(ctx, `SELECT decision_reason FROM kb.semantic_decision_candidates WHERE id=$1`, dc.ID).Scan(&candReason); err != nil {
		t.Fatalf("load candidate reason: %v", err)
	}
	if candReason != "unresolved_referent" {
		t.Errorf("decision_reason = %q, want unresolved_referent", candReason)
	}

	var occCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.unresolved_semantic_occurrences
WHERE artifact_type='provision' AND artifact_id='prov-lossless-unresolved'`).Scan(&occCount); err != nil {
		t.Fatalf("count occurrences: %v", err)
	}
	if occCount != 0 {
		t.Fatalf("unresolved occurrences = %d, want 0 (DR1: a SupportsInstances family never falls back)", occCount)
	}
}

// Task 7.6, gate on, resolved subject, unparseable modality: defers with the
// accurate "unparsed_modality" reason (distinct from the pre-7.6
// "no_governed_deontic_predicate" gap) and still uses the generic fallback
// store when its own gate authorizes it.
func TestIntegrationProcessProvisionLosslessDefersOnUnparsedModality(t *testing.T) {
	db := freshAssertionsTestDB(t)
	t.Setenv(semantic.GateProvisionLosslessWrites, "true")
	t.Setenv(semantic.GateFallbackWrites, "true")
	ctx := context.Background()
	seedGovernedTerm(t, db, "prov:has_provision", "property", "provision")
	seedObjectNode(t, db, "obj-widget-2")

	dcStore := DecisionCandidateStore{DB: db}
	dc := proposeAndReviewProvisionCandidate(t, dcStore, "prov-lossless-unparsed", 302, provisionCandidatePayload{
		ProvID:          "prov-lossless-unparsed",
		RawText:         "ambiguous text",
		Modality:        "unparsed",
		AssertionKind:   "unparsed",
		SubjectObjectID: "obj-widget-2",
	})

	a := AssociateSemantics{DB: db}
	outcome, err := a.processProvision(ctx, dcStore, dc)
	if err != nil {
		t.Fatalf("processProvision: %v", err)
	}
	if outcome != "deferred" {
		t.Fatalf("outcome = %q, want deferred", outcome)
	}

	var candReason string
	if err := db.QueryRowContext(ctx, `SELECT decision_reason FROM kb.semantic_decision_candidates WHERE id=$1`, dc.ID).Scan(&candReason); err != nil {
		t.Fatalf("load candidate reason: %v", err)
	}
	if candReason != "unparsed_modality" {
		t.Errorf("decision_reason = %q, want unparsed_modality", candReason)
	}

	var occCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.unresolved_semantic_occurrences
WHERE artifact_type='provision' AND artifact_id='prov-lossless-unparsed' AND active`).Scan(&occCount); err != nil {
		t.Fatalf("count occurrences: %v", err)
	}
	if occCount != 1 {
		t.Fatalf("unresolved occurrences = %d, want 1 (fallback gate is on)", occCount)
	}
}

// Idempotent re-entry: once a candidate is accepted, a second Run over the
// same input record must not touch it again -- Run's own status filter
// (only 'candidate'/'in_review') excludes it, the same guarantee metric's
// writer relies on (no metric test calls writeMetricLossless twice on one
// candidate ID either; re-entry safety lives in Run's selection, not the
// writer). This proves that at the level real reprocessing actually uses:
// AssociateSemantics.Run, not a direct second writer call.
func TestIntegrationProcessProvisionLosslessSecondRunDoesNotReexamineAcceptedCandidate(t *testing.T) {
	db := freshAssertionsTestDB(t)
	t.Setenv(semantic.GateProvisionLosslessWrites, "true")
	ctx := context.Background()
	seedGovernedTerm(t, db, "prov:has_provision", "property", "provision")
	seedGovernedTerm(t, db, "prov:required", "property", "provision")
	seedObjectNode(t, db, "obj-widget-3")

	dcStore := DecisionCandidateStore{DB: db}
	proposeAndReviewProvisionCandidate(t, dcStore, "prov-lossless-replay", 303, provisionCandidatePayload{
		ProvID:          "prov-lossless-replay",
		RawText:         "widgets shall be labeled",
		Modality:        "required",
		AssertionKind:   "required",
		SubjectObjectID: "obj-widget-3",
	})

	a := AssociateSemantics{DB: db}
	first, err := a.Run(ctx, 303)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.Represented != 1 {
		t.Fatalf("first run represented = %d, want 1", first.Represented)
	}

	second, err := a.Run(ctx, 303)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if second.Examined != 0 {
		t.Fatalf("second run examined = %d, want 0 (accepted candidate must not be re-selected)", second.Examined)
	}

	var assertionCount, supportCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.semantic_assertions WHERE logical_identity_key='provision:prov-lossless-replay'`).Scan(&assertionCount); err != nil {
		t.Fatalf("count assertions: %v", err)
	}
	if assertionCount != 1 {
		t.Fatalf("assertions for logical identity = %d, want 1", assertionCount)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.assertion_evidence
WHERE artifact_type='provision' AND artifact_id='prov-lossless-replay' AND input_record_id=303
  AND evidence_role='supports' AND NOT deleted`).Scan(&supportCount); err != nil {
		t.Fatalf("count evidence: %v", err)
	}
	if supportCount != 1 {
		t.Fatalf("active supporting evidence links = %d, want 1", supportCount)
	}
}
