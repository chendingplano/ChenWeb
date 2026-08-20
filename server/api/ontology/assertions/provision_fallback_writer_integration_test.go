package assertions

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// proposeAndReviewProvisionCandidate mirrors proposeMetricCandidate: propose
// a provision decision candidate and transition it to in_review, the state
// processOne leaves a candidate in before handing it to a resolver.
func proposeAndReviewProvisionCandidate(t *testing.T, dbForCandidates DecisionCandidateStore, provID string, inputRecordID int64, p provisionCandidatePayload) DecisionCandidate {
	t.Helper()
	payload, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	dc, err := dbForCandidates.Propose(context.Background(), DecisionCandidate{
		LogicalIdentityKey: "provision:" + provID,
		CandidateKind:      "assertion",
		ProposedPayload:    payload,
		Method:             "explicit_structured",
		SourceArtifactType: "provision",
		SourceArtifactID:   provID,
		InputRecordID:      &inputRecordID,
		Status:             StatusCandidate,
		CreateBy:           "test",
		ModifyBy:           "test",
	})
	if err != nil {
		t.Fatalf("propose provision candidate: %v", err)
	}
	dc, err = dbForCandidates.TransitionStatus(context.Background(), dc.ID, StatusInReview, "", "test")
	if err != nil {
		t.Fatalf("transition candidate to in_review: %v", err)
	}
	return dc
}

// Task 7.4, gate explicitly off (ADR §6's rollback lever; the gate defaults
// ON as of this task): processProvision must remain byte-identical to its
// pre-gate behavior -- defer only, nothing written to the generic-fallback
// store.
func TestIntegrationProcessProvisionGateOffWritesNoFallbackRecord(t *testing.T) {
	db := freshAssertionsTestDB(t)
	t.Setenv(semantic.GateFallbackWrites, "false")
	ctx := context.Background()
	dcStore := DecisionCandidateStore{DB: db}

	dc := proposeAndReviewProvisionCandidate(t, dcStore, "prov-gate-off", 200, provisionCandidatePayload{
		ProvID:   "prov-gate-off",
		RawText:  "widgets shall be labeled",
		Modality: "required",
	})

	a := AssociateSemantics{DB: db}
	outcome, err := a.processProvision(ctx, dcStore, dc)
	if err != nil {
		t.Fatalf("processProvision: %v", err)
	}
	if outcome != "deferred" {
		t.Fatalf("outcome = %q, want deferred", outcome)
	}

	var occCount, outcomeCount int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.unresolved_semantic_occurrences
WHERE artifact_type='provision' AND artifact_id='prov-gate-off'`).Scan(&occCount); err != nil {
		t.Fatalf("count occurrences: %v", err)
	}
	if occCount != 0 {
		t.Fatalf("unresolved occurrences = %d, want 0 (gate is off)", occCount)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.semantic_processing_outcomes
WHERE artifact_type='provision' AND artifact_id='prov-gate-off'`).Scan(&outcomeCount); err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	if outcomeCount != 0 {
		t.Fatalf("outcome envelopes = %d, want 0 (gate is off)", outcomeCount)
	}
}

// Task 7.4, gate on: processProvision persists DR13's generic-fallback pair
// (one active unresolved occurrence, one outcome envelope with one finding)
// in addition to its unchanged defer behavior.
func TestIntegrationProcessProvisionGateOnWritesFallbackOccurrenceAndOutcome(t *testing.T) {
	db := freshAssertionsTestDB(t)
	t.Setenv(semantic.GateFallbackWrites, "true")
	ctx := context.Background()
	dcStore := DecisionCandidateStore{DB: db}

	dc := proposeAndReviewProvisionCandidate(t, dcStore, "prov-gate-on", 201, provisionCandidatePayload{
		ProvID:   "prov-gate-on",
		RawText:  "widgets shall be labeled",
		Modality: "required",
	})

	a := AssociateSemantics{DB: db}
	outcome, err := a.processProvision(ctx, dcStore, dc)
	if err != nil {
		t.Fatalf("processProvision: %v", err)
	}
	if outcome != "deferred" {
		t.Fatalf("outcome = %q, want deferred (the fallback write must not change the candidate's own outcome)", outcome)
	}

	var occCount int
	var occActive bool
	if err := db.QueryRowContext(ctx, `
SELECT count(*), bool_and(active) FROM kb.unresolved_semantic_occurrences
WHERE artifact_type='provision' AND artifact_id='prov-gate-on'`).Scan(&occCount, &occActive); err != nil {
		t.Fatalf("count occurrences: %v", err)
	}
	if occCount != 1 {
		t.Fatalf("unresolved occurrences = %d, want 1", occCount)
	}
	if !occActive {
		t.Fatal("unresolved occurrence is not active")
	}

	var outcomeCount int
	var stageTermID, disposition string
	if err := db.QueryRowContext(ctx, `
SELECT count(*), max(stage_term_id), max(disposition_term_id) FROM kb.semantic_processing_outcomes
WHERE artifact_type='provision' AND artifact_id='prov-gate-on' AND active`).Scan(&outcomeCount, &stageTermID, &disposition); err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	if outcomeCount != 1 {
		t.Fatalf("active outcome envelopes = %d, want 1", outcomeCount)
	}
	if stageTermID != semantic.StageAssociate {
		t.Errorf("stage_term_id = %q, want %q", stageTermID, semantic.StageAssociate)
	}
	if disposition != semantic.DispositionRawPreserved {
		t.Errorf("disposition_term_id = %q, want %q", disposition, semantic.DispositionRawPreserved)
	}

	var findingCount int
	var findingTermID string
	if err := db.QueryRowContext(ctx, `
SELECT count(*), max(f.finding_term_id) FROM kb.semantic_processing_findings f
JOIN kb.semantic_processing_outcomes o ON o.id = f.outcome_id
WHERE o.artifact_type='provision' AND o.artifact_id='prov-gate-on' AND f.active`).Scan(&findingCount, &findingTermID); err != nil {
		t.Fatalf("count findings: %v", err)
	}
	if findingCount != 1 {
		t.Fatalf("active findings = %d, want 1", findingCount)
	}
	if findingTermID != semantic.FindingMappingUnresolved {
		t.Errorf("finding_term_id = %q, want %q", findingTermID, semantic.FindingMappingUnresolved)
	}

	// Re-running the fallback write itself (independent of the decision
	// candidate's own state machine, which does not support re-entering
	// processProvision on an already-deferred candidate) must replay
	// idempotently, not duplicate rows (DR13/DR4).
	if err := a.writeProvisionFallbackOccurrence(ctx, dc, "no_governed_deontic_predicate"); err != nil {
		t.Fatalf("second writeProvisionFallbackOccurrence: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM kb.unresolved_semantic_occurrences
WHERE artifact_type='provision' AND artifact_id='prov-gate-on'`).Scan(&occCount); err != nil {
		t.Fatalf("re-count occurrences: %v", err)
	}
	if occCount != 1 {
		t.Fatalf("unresolved occurrences after replay = %d, want 1 (idempotent replay, not a duplicate)", occCount)
	}
}
