package semantic

// ProvisionAdapterName and ProvisionAdapterVersion identify the provision
// family adapter in the runtime compliance registry.
const (
	ProvisionAdapterName    = "provision_lossless_adapter"
	ProvisionAdapterVersion = "0.1.0"
	// ProvisionArtifactType matches kb.assertion_evidence.artifact_type and
	// kb.artifact_objects.artifact_type for provisions.
	ProvisionArtifactType = "provision"
	// ProvisionOccurrenceScope is the family-declared source scope that
	// participates in OccurrenceKey.
	ProvisionOccurrenceScope = "provision_occurrence:v1"
)

// ProvisionAdapter is the provision family's DR13 declaration (task 7.6),
// the second vertical slice after MetricAdapter. Unlike metrics, a
// provision clause needs no cross-document convergence -- each is already
// its own atomic, uniquely-identified source claim (prov_id) -- so it
// declares only the associate stage as required: there is no separate
// normalize or class-resolution phase to report for this family.
type ProvisionAdapter struct{}

func (ProvisionAdapter) ArtifactType() string    { return ProvisionArtifactType }
func (ProvisionAdapter) AdapterName() string     { return ProvisionAdapterName }
func (ProvisionAdapter) AdapterVersion() string  { return ProvisionAdapterVersion }
func (ProvisionAdapter) OccurrenceScope() string { return ProvisionOccurrenceScope }

// SupportsInstances is true: task 7.6 gives provisions a real writer
// (writeProvisionLossless), gated behind LOSSLESS_SEMANTIC_WRITES_PROVISION
// (on by default as of task 7.7). DR1's "must produce an instance"
// obligation is enforced by the gate and completeness projection at
// activation time, not by this declaration alone (the same sequencing
// metrics used from task 3.8 through task 6.9).
func (ProvisionAdapter) SupportsInstances() bool { return true }

// RawIdentityFields are the kb.provisions columns that constitute the raw
// occurrence identity and payload (DR2).
func (ProvisionAdapter) RawIdentityFields() []string {
	return []string{
		"prov_id", "input_record_id", "provision", "prov_desc",
		"provision_type", "provision_subject", "source_line_spans",
	}
}

// RequiredStages is the provision adapter's declared required stage set.
// Only associate is required (DR13: an undeclared stage's absence does not
// fail completeness) -- classifying the deontic modality and associating it
// with a resolved subject happen together in one step for this family.
func (ProvisionAdapter) RequiredStages() []StageContract {
	return []StageContract{
		{
			StageTermID:         StageAssociate,
			DecisionScopes:      []string{"deontic_predicate"},
			AllowedDispositions: []string{DispositionNormalized, DispositionRawPreserved, DispositionNoResult},
		},
	}
}

// ValueStates is every value state a provision instance may carry. A
// provision's raw descriptive text IS its complete normalized
// representation (there is no further structured form to parse it into), so
// only "present" is used by the writer today.
func (ProvisionAdapter) ValueStates() []string { return []string{ValuePresent} }

// ConformanceStates is every conformance state a provision instance may
// carry. No class-contract validation is implemented for this vertical
// slice, matching the same honesty MetricAdapter's associate stage uses.
func (ProvisionAdapter) ConformanceStates() []string { return []string{ConformanceNotEvaluated} }

// DependencyAxes names the axes a provision outcome's dependency fingerprint
// covers. There is no governed-mapping or class-contract axis for this
// family (its only real dependency is the modality parser's own version).
func (ProvisionAdapter) DependencyAxes() []string { return []string{"parser_version"} }

// ProvisionArtifactSourceSQL is the family's current-occurrence query for
// the completeness projection, mirroring MetricArtifactSourceSQL.
const ProvisionArtifactSourceSQL = `
SELECT p.prov_id AS artifact_id, p.input_record_id AS input_record_id
FROM kb.provisions p
WHERE p.prov_id IS NOT NULL AND btrim(p.prov_id) <> ''`

// The provision adapter registers itself so LookupAdapter, the completeness
// projection, and AuthorizeWriterActivation can find it. Registration is not
// activation on its own -- LOSSLESS_SEMANTIC_WRITES_PROVISION is what gates
// the writer (it defaults on as of task 7.7, but can still be explicitly
// disabled per-environment).
func init() { RegisterAdapter(ProvisionAdapter{}) }
