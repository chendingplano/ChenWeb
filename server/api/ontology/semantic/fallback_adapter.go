package semantic

import "fmt"

// FallbackAdapterName and FallbackAdapterVersion identify the generic
// fallback adapter in the runtime compliance registry.
const (
	FallbackAdapterName    = "generic_fallback_adapter"
	FallbackAdapterVersion = "0.1.0"
)

// FallbackAdapter is DR13's stand-in declaration for a family that has a
// registered extractor/normalizer but no compliant semantic-instance adapter
// of its own. It never models ontology object instances (SupportsInstances
// is false): every occurrence of the family stays on the option-3
// unresolved-occurrence path (kb.unresolved_semantic_occurrences), which is
// exactly what "generic fallback" means under DR13.
//
// The declaration is deliberately minimal and honest about what the shared
// infrastructure alone can promise for a family it knows nothing family-
// specific about: the raw identity is only what
// kb.unresolved_semantic_occurrences itself requires (artifact_id,
// input_record_id), the value/conformance states are the governed
// "unknown"/"not evaluated" terms, and the required stages reuse the same
// shared Phase C stage vocabulary (StageNormalize, StageClassResolution,
// StageAssociate) every family's normalize_assertions -> associate_semantics
// -> project_semantics pipeline already runs through, restricted to
// dispositions that never claim a full normalized instance.
type FallbackAdapter struct {
	// ArtifactTypeValue is the kb artifact_type this fallback stands in for
	// (e.g. "provision") -- one FallbackAdapter value per family, so the
	// compliance registry and completeness projection can still tell
	// families apart even though they share one Go implementation.
	ArtifactTypeValue string
}

func (a FallbackAdapter) ArtifactType() string   { return a.ArtifactTypeValue }
func (a FallbackAdapter) AdapterName() string    { return FallbackAdapterName }
func (a FallbackAdapter) AdapterVersion() string { return FallbackAdapterVersion }

// OccurrenceScope is versioned per family so two families sharing this one
// adapter implementation can never collide on OccurrenceKey.
func (a FallbackAdapter) OccurrenceScope() string {
	return fmt.Sprintf("generic_fallback:%s:v1", a.ArtifactTypeValue)
}

// RawIdentityFields names only what kb.unresolved_semantic_occurrences
// itself requires (DR13: "artifact_id SHALL be required"). A family that
// later earns its own compliant adapter declares its real raw shape there
// instead of here.
func (a FallbackAdapter) RawIdentityFields() []string {
	return []string{"artifact_id", "input_record_id"}
}

// SupportsInstances is false: DR1 reserves the ontology-instance path for
// families with a compliant adapter. A fallback family stays on the
// unresolved-occurrence path for every occurrence.
func (a FallbackAdapter) SupportsInstances() bool { return false }

// RequiredStages reuses the shared Phase C stage vocabulary every family's
// normalize_assertions -> associate_semantics -> project_semantics pipeline
// already runs through, but restricts each stage to dispositions that never
// claim a full normalized instance -- a fallback family can preserve the raw
// occurrence or produce no result, never "semantic:normalized".
func (a FallbackAdapter) RequiredStages() []StageContract {
	dispositions := []string{DispositionRawPreserved, DispositionNoResult}
	return []StageContract{
		{StageTermID: StageNormalize, DecisionScopes: []string{"generic_fallback"}, AllowedDispositions: dispositions},
		{StageTermID: StageClassResolution, DecisionScopes: []string{"generic_fallback"}, AllowedDispositions: dispositions},
		{StageTermID: StageAssociate, DecisionScopes: []string{"generic_fallback"}, AllowedDispositions: dispositions},
	}
}

// ValueStates declares only the governed "unknown" state: the shared
// infrastructure has no family-specific parser, so it cannot claim present,
// missing, unparsed, or any other more specific value state on the family's
// behalf.
func (a FallbackAdapter) ValueStates() []string { return []string{ValueUnknown} }

// ConformanceStates declares only "not_evaluated": no class contract has
// been checked for a family with no compliant adapter.
func (a FallbackAdapter) ConformanceStates() []string { return []string{ConformanceNotEvaluated} }

// DependencyAxes: the only dependency signal the generic fallback store
// tracks for a family it knows nothing family-specific about is the raw
// source content itself changing.
func (a FallbackAdapter) DependencyAxes() []string { return []string{"source_revision"} }
