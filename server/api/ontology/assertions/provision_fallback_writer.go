package assertions

import (
	"context"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// ProvisionFallbackWriterVersion identifies this writer's own processing
// logic in the dependency fingerprint it writes. There is no governed
// mapping/class/parser axis for provisions yet -- no core module term
// represents a deontic predicate at all (see processProvision) -- so the
// writer's own version is the one axis that can change and should retrigger
// a write.
const ProvisionFallbackWriterVersion = "0.1.0"

// writeProvisionFallbackOccurrence persists task 7.4's generic-fallback
// record for one provision candidate that associate_semantics cannot yet
// turn into an assertion (DR13): one active kb.unresolved_semantic_occurrences
// row plus one kb.semantic_processing_outcomes envelope for the associate
// stage, both keyed off semantic.FallbackAdapter{ArtifactTypeValue:"provision"}'s
// declared occurrence scope so they line up with what task 7.2 registered.
//
// Only called when Gates.FallbackAllowedFor("provision") is true; the caller
// (processProvision) owns that gate check, so this function has none of its
// own -- mirroring how writeMetricLossless is only reached once processMetric
// has already checked MetricLosslessWritesEnabled.
func (a AssociateSemantics) writeProvisionFallbackOccurrence(ctx context.Context, dc DecisionCandidate, reason string) error {
	if dc.InputRecordID == nil {
		return fmt.Errorf("provision candidate %d has no input_record_id", dc.ID)
	}
	inputRecordID := *dc.InputRecordID
	artifactType, artifactID := dc.SourceArtifactType, dc.SourceArtifactID

	inputFingerprint := dc.PayloadFingerprint
	if inputFingerprint == "" {
		inputFingerprint = "unknown"
	}
	dependencyFingerprint := semantic.Dependencies{
		Extra: map[string]string{"source_revision": ProvisionFallbackWriterVersion},
	}.Fingerprint()

	scope := (semantic.FallbackAdapter{ArtifactTypeValue: artifactType}).OccurrenceScope()
	occ := semantic.UnresolvedOccurrence{
		OccurrenceKey:         semantic.OccurrenceKey(scope, inputRecordID, artifactType, artifactID),
		InputRecordID:         &inputRecordID,
		ArtifactType:          artifactType,
		ArtifactID:            artifactID,
		RawPayload:            dc.ProposedPayload,
		InputFingerprint:      inputFingerprint,
		DependencyFingerprint: dependencyFingerprint,
		CreateBy:              "associate_semantics",
	}
	if _, _, err := (semantic.OccurrenceStore{DB: a.DB}).Upsert(ctx, occ); err != nil {
		return fmt.Errorf("upsert unresolved occurrence: %w", err)
	}

	// One finding: the governed deontic-predicate mapping this provision needs
	// is missing entirely, the same shape as a metric's unresolved value-range
	// mapping (DR4's mapping dimension generalizes to any missing governed
	// vocabulary mapping, not only metrics').
	finding := semantic.Finding{
		FindingKey:            semantic.FindingKey("generic_fallback", semantic.DimensionMapping),
		DimensionTermID:       semantic.DimensionMapping,
		FindingTermID:         semantic.FindingMappingUnresolved,
		SeverityTermID:        semantic.SeverityWarning,
		ErrorCode:             reason,
		DependencyFingerprint: dependencyFingerprint,
		CreateBy:              "associate_semantics",
	}
	out := semantic.Outcome{
		OutcomeKey:            semantic.OutcomeKey(inputRecordID, artifactType, artifactID, semantic.StageAssociate),
		InputRecordID:         &inputRecordID,
		ArtifactType:          artifactType,
		ArtifactID:            artifactID,
		StageTermID:           semantic.StageAssociate,
		DispositionTermID:     semantic.DispositionRawPreserved,
		ExecutionStatus:       semantic.ExecutionStatusFor(semantic.CategorySemanticFinding),
		OutcomeCategory:       semantic.CategorySemanticFinding,
		DependencyFingerprint: dependencyFingerprint,
		InputFingerprint:      inputFingerprint,
		ProcessorName:         "provision_fallback_writer",
		ProcessorVersion:      ProvisionFallbackWriterVersion,
		CreateBy:              "associate_semantics",
	}
	if _, err := (semantic.OutcomeStore{DB: a.DB}).Record(ctx, out, []semantic.Finding{finding}); err != nil {
		return fmt.Errorf("record outcome envelope: %w", err)
	}
	return nil
}
