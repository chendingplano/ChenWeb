package assertions

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// ProvisionLosslessWriterVersion identifies this writer's dependency-
// fingerprint parser/writer axis (DR10), the same role
// MetricLosslessWriterVersion plays for metrics.
const ProvisionLosslessWriterVersion = "provision_lossless_writer/0.1.0"

// ProvisionPredicateTermID is the one governed predicate every provision
// assertion uses to bind its subject to the provision (task 7.6's prov:
// module) -- the provision-family equivalent of mea:measured_by.
const ProvisionPredicateTermID = "prov:has_provision"

// provisionAssertionKindTermID maps a normalized provision modality to its
// governed deontic assertion-kind term. Mirrors metricAssertionKindTermID:
// only "unparsed" (the modality could not be classified from the source
// text) is unsupported -- distinct from the pre-7.6 gap, where no term
// existed at all regardless of modality.
func provisionAssertionKindTermID(assertionKind string) (string, bool) {
	switch assertionKind {
	case "required", "prohibited", "permitted":
		return "prov:" + assertionKind, true
	default:
		return "", false
	}
}

// writeProvisionLossless is task 7.6's minimal DR5-style atomic transaction
// for provisions, gated behind LOSSLESS_SEMANTIC_WRITES_PROVISION.
//
// Unlike writeMetricLossless, it needs no claim registry: a provision clause
// is already its own atomic, uniquely-identified source claim (prov_id).
// Metrics need cross-document convergence because two documents can measure
// the "same" physical quantity in different words; two provisions are never
// the "same" provision merely because they share a modality, so
// dc.LogicalIdentityKey (already prov_id-scoped by ProvisionNormalizer) is
// the assertion's own logical identity directly, with no canonical-claim
// indirection.
//
// The caller (processProvisionLossless) has already checked that the
// subject is resolved and the assertion kind is governed; this function
// does the atomic write only.
func (a AssociateSemantics) writeProvisionLossless(ctx context.Context, dc DecisionCandidate, p provisionCandidatePayload, assertionKindTermID string) (string, error) {
	if dc.InputRecordID == nil {
		return "", fmt.Errorf("provision candidate %d has no input_record_id", dc.ID)
	}
	inputRecordID := *dc.InputRecordID

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin provision semantic transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	asStore := AssertionStore{DB: tx}
	evStore := EvidenceStore{DB: tx, Assertions: asStore}
	dcStoreTx := DecisionCandidateStore{DB: tx}

	assertion := Assertion{
		LogicalIdentityKey:     dc.LogicalIdentityKey,
		SubjectRefKind:         "object_node",
		SubjectRefID:           p.SubjectObjectID,
		SubjectObjectID:        p.SubjectObjectID,
		PredicateTermID:        ProvisionPredicateTermID,
		ObjectRefKind:          "literal",
		ObjectLiteral:          mustMarshal(map[string]string{"text": p.RawText}),
		AssertionKindTermID:    assertionKindTermID,
		RawText:                p.RawText,
		Confidence:             dc.Confidence,
		Status:                 StatusRepresented,
		// A provision's "value" is its raw descriptive text -- there is no
		// further structured form to parse it into, so the literal above IS
		// the complete normalized representation (unlike a metric's
		// value-unparsed case, where a number was expected and missing).
		ValueStateTermID:       semantic.ValuePresent,
		ConformanceStateTermID: semantic.ConformanceNotEvaluated,
		RawPayload:             mustMarshal(map[string]string{"raw_text": p.RawText, "modality": p.Modality}),
		RawSnapshotFingerprint: dc.PayloadFingerprint,
		CreateBy:               "provision_lossless_writer",
		ModifyBy:               "provision_lossless_writer",
	}
	created, err := a.persistAssertionTx(ctx, asStore, assertion)
	if err != nil {
		return "", fmt.Errorf("persist represented provision assertion: %w", err)
	}

	if err := supersedeProvisionSupportEvidence(ctx, evStore, dc, p, inputRecordID, created.ID); err != nil {
		return "", fmt.Errorf("supersede provision support evidence: %w", err)
	}

	outcomeID, err := recordProvisionStageOutcome(ctx, tx, dc, inputRecordID, created.ID)
	if err != nil {
		return "", err
	}

	// DR13: a real instance materializing must supersede any pre-existing
	// fallback occurrence for this artifact (e.g. task 7.4's generic-fallback
	// record, written before this family had a real writer) -- otherwise the
	// occurrence is left stale-active alongside the assertion it now
	// duplicates. A no-op when no such occurrence exists.
	if _, err := (semantic.OccurrenceStore{DB: a.DB}).SupersedeActiveForArtifactTx(ctx, tx, dc.SourceArtifactType, dc.SourceArtifactID, inputRecordID, created.ID, outcomeID); err != nil {
		return "", fmt.Errorf("supersede fallback occurrence: %w", err)
	}

	if _, err := dcStoreTx.SetResultingAssertion(ctx, dc.ID, created.ID); err != nil {
		return "", fmt.Errorf("set resulting assertion: %w", err)
	}
	if _, err := dcStoreTx.TransitionStatus(ctx, dc.ID, StatusAccepted, "persisted as kb.semantic_assertions (represented)", "associate_semantics"); err != nil {
		return "", fmt.Errorf("transition decision candidate: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit provision semantic transaction: %w", err)
	}
	return "represented", nil
}

// supersedeProvisionSupportEvidence enforces the same "at most one active
// support link" cardinality supersedeMetricSupportEvidence enforces for
// metrics, reusing its generic (artifact-type-parameterized) lookup helper.
// There is no uq_assertion_evidence_current_provision_support index yet
// (task 6.3's metric-only equivalent) -- this transaction's atomicity is
// what prevents a duplicate today; a schema-level partial index is a
// reasonable future hardening step, not required for this minimal writer.
func supersedeProvisionSupportEvidence(ctx context.Context, evStore EvidenceStore, dc DecisionCandidate, p provisionCandidatePayload, inputRecordID, newAssertionID int64) error {
	existing, err := activeMetricSupportEvidence(ctx, evStore, dc.SourceArtifactType, dc.SourceArtifactID, inputRecordID)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.AssertionID == newAssertionID {
			return nil
		}
		if err := evStore.DeleteEvidence(ctx, existing.ID, "provision_lossless_writer", "superseded by new provision semantic transaction"); err != nil {
			return err
		}
	}
	recordID := inputRecordID
	_, err = evStore.AddEvidence(ctx, Evidence{
		AssertionID:      newAssertionID,
		InputRecordID:    &recordID,
		ArtifactType:     dc.SourceArtifactType,
		ArtifactID:       dc.SourceArtifactID,
		ArtifactObjectID: p.SubjectObjectID,
		EvidenceQuote:    p.RawText,
		SourceLineSpans:  dc.SourceLineSpans,
		Confidence:       dc.Confidence,
		EvidenceRole:     "supports",
		ActorKind:        "processor",
		CreateBy:         "provision_lossless_writer",
	})
	return err
}

// recordProvisionStageOutcome writes the mandatory outcome envelope for the
// provision adapter's one required stage (task 7.6): unlike metrics,
// provisions have no separate normalize or class-resolution phase to report
// -- classifying the deontic modality and associating it with a subject
// happen together, so ProvisionAdapter declares only StageAssociate as
// required, and this is the single envelope the completeness projection
// checks for. Returns the outcome's row ID so the caller can supersede any
// pre-existing fallback occurrence with it.
func recordProvisionStageOutcome(ctx context.Context, tx *sql.Tx, dc DecisionCandidate, inputRecordID, assertionID int64) (int64, error) {
	inputFingerprint := dc.PayloadFingerprint
	if inputFingerprint == "" {
		inputFingerprint = "unknown"
	}
	stageDeps := semantic.Dependencies{ParserVersion: ProvisionLosslessWriterVersion}.Fingerprint()
	out := semantic.Outcome{
		OutcomeKey:            semantic.OutcomeKey(inputRecordID, dc.SourceArtifactType, dc.SourceArtifactID, semantic.StageAssociate),
		InputRecordID:         &inputRecordID,
		ArtifactType:          dc.SourceArtifactType,
		ArtifactID:            dc.SourceArtifactID,
		AssertionID:           &assertionID,
		StageTermID:           semantic.StageAssociate,
		DispositionTermID:     semantic.DispositionNormalized,
		ExecutionStatus:       semantic.ExecutionStatusFor(semantic.CategorySemanticSuccess),
		OutcomeCategory:       semantic.CategorySemanticSuccess,
		DependencyFingerprint: stageDeps,
		InputFingerprint:      inputFingerprint,
		ProcessorName:         "provision_lossless_writer",
		ProcessorVersion:      ProvisionLosslessWriterVersion,
		CreateBy:              "provision_lossless_writer",
	}
	result, err := semantic.RecordTx(ctx, tx, out, nil)
	if err != nil {
		return 0, err
	}
	return result.Outcome.ID, nil
}
