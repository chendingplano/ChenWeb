// Package semantic implements the shared, family-generic infrastructure
// decided by ADR 2026081801: semantic processing outcomes and findings, the
// unresolved-occurrence fallback, dependency fingerprints, dependency-driven
// retry, the family adapter contract, and the completeness projection.
//
// Family-specific semantics stay with each family's adapter. Nothing in this
// package knows what a metric is.
package semantic

import (
	"fmt"
	"strings"
)

// Governed disposition terms (DR4). Disposition describes what a stage managed
// to produce. It never selects the canonical identity branch: DR2 is explicit
// that a parsed-but-unmapped claim can be DispositionRawPreserved while its
// claim identity still uses the normalized-value branch.
const (
	DispositionNormalized    = "semantic:normalized"
	DispositionRawPreserved  = "semantic:raw_preserved"
	DispositionNotApplicable = "semantic:not_applicable"
	DispositionNoResult      = "semantic:no_result"
)

// Governed finding terms (DR4). This is the initial set; the vocabulary is
// extensible ontology terms, so a new finding needs a seed entry and no schema
// change.
const (
	FindingMappingUnresolved        = "semantic:mapping_unresolved"
	FindingMappingAmbiguous         = "semantic:mapping_ambiguous"
	FindingUnparsed                 = "semantic:unparsed"
	FindingValueMissing             = "semantic:value_missing"
	FindingValueUnknown             = "semantic:value_unknown"
	FindingDatatypeMismatch         = "semantic:datatype_mismatch"
	FindingContractViolation        = "semantic:contract_violation"
	FindingClassProvisional         = "semantic:class_provisional"
	FindingClassAmbiguous           = "semantic:class_ambiguous"
	FindingIdentityEvidenceConflict = "semantic:identity_evidence_conflict"
	FindingSourceConflict           = "semantic:source_conflict"
	FindingNoVerdict                = "semantic:no_verdict"
)

// Governed finding dimensions (DR4).
const (
	DimensionMapping     = "semantic:dimension_mapping"
	DimensionValue       = "semantic:dimension_value"
	DimensionClass       = "semantic:dimension_class"
	DimensionConformance = "semantic:dimension_conformance"
	DimensionIdentity    = "semantic:dimension_identity"
	DimensionConflict    = "semantic:dimension_conflict"
)

// Governed severities. DR3: severity is a user-facing signal only. An
// error-severity finding does not become an execution failure unless the
// system cannot safely persist or continue.
const (
	SeverityInfo    = "semantic:severity_info"
	SeverityWarning = "semantic:severity_warning"
	SeverityError   = "semantic:severity_error"
)

// severityRank orders severities so an outcome's highest severity can be
// derived from its children.
var severityRank = map[string]int{
	SeverityInfo:    1,
	SeverityWarning: 2,
	SeverityError:   3,
}

// Governed retry states (DR10).
const (
	RetryPending      = "semantic:retry_pending"
	RetryScheduled    = "semantic:retry_scheduled"
	RetryNotRetryable = "semantic:retry_not_retryable"
	RetryStale        = "semantic:retry_stale"
)

// Governed semantic stages (DR5, DR13). An adapter declares which of these it
// requires; the completeness projection reports against that declaration.
const (
	StageNormalize       = "semantic:stage_normalize"
	StageClassResolution = "semantic:stage_class_resolution"
	StageAssociate       = "semantic:stage_associate"
)

// Governed class identity states (DR9). ClassCandidateEvidenceConflict is the
// STATE; FindingIdentityEvidenceConflict is the corresponding processing
// finding and is never stored in the state field.
const (
	ClassResolvedExisting          = "semantic:resolved_existing"
	ClassProvisionalNew            = "semantic:provisional_new"
	ClassAmbiguousCandidates       = "semantic:ambiguous_candidates"
	ClassCandidateEvidenceConflict = "semantic:candidate_evidence_conflict"
)

// Governed mapping resolution states (DR9).
const (
	MappingResolved    = "semantic:mapping_resolved"
	MappingUnresolved  = "semantic:mapping_state_unresolved"
	MappingAmbiguous   = "semantic:mapping_state_ambiguous"
	MappingNotRequired = "semantic:mapping_not_required"
)

// Governed value states (DR9; ADR 2026081701 DR8's table). ValuePresent means
// a usable normalized value exists -- it is independent of whether a governed
// mapping resolved, which is why DR6 keeps a parsed literal 'present' even
// when its bucket fields are empty.
const (
	ValuePresent          = "semantic:value_present"
	ValueMissing          = "semantic:value_state_missing"
	ValueUnparsed         = "semantic:value_state_unparsed"
	ValueDatatypeMismatch = "semantic:value_state_datatype_mismatch"
	ValueUnknown          = "semantic:value_state_unknown"
	ValueNotApplicable    = "semantic:value_state_not_applicable"
)

// Governed conformance states (DR9).
const (
	Conforms                     = "semantic:conforms"
	ConformanceContractViolation = "semantic:conformance_contract_violation"
	ConformanceNotEvaluated      = "semantic:not_evaluated"
)

// Governed evidence-role term IDs (DR9). The persisted and API identifiers
// remain the bare strings "supports" and "contradicts"; these are their
// governed term IDs, used in ontology references and fingerprints.
const (
	EvidenceSupports    = "semantic:evidence_supports"
	EvidenceContradicts = "semantic:evidence_contradicts"
)

// governedTerms is every machine identifier this package may persist. It backs
// ValidateGovernedIdentifier, which is what enforces DR9's rule that
// "persisted state, API payloads, dependency fingerprints, and canonical
// serialization use the governed identifiers exactly".
var governedTerms = func() map[string]bool {
	all := []string{
		DispositionNormalized, DispositionRawPreserved, DispositionNotApplicable, DispositionNoResult,
		FindingMappingUnresolved, FindingMappingAmbiguous, FindingUnparsed, FindingValueMissing,
		FindingValueUnknown, FindingDatatypeMismatch, FindingContractViolation,
		FindingClassProvisional, FindingClassAmbiguous, FindingIdentityEvidenceConflict,
		FindingSourceConflict, FindingNoVerdict,
		DimensionMapping, DimensionValue, DimensionClass, DimensionConformance,
		DimensionIdentity, DimensionConflict,
		SeverityInfo, SeverityWarning, SeverityError,
		RetryPending, RetryScheduled, RetryNotRetryable, RetryStale,
		StageNormalize, StageClassResolution, StageAssociate,
		ClassResolvedExisting, ClassProvisionalNew, ClassAmbiguousCandidates,
		ClassCandidateEvidenceConflict,
		MappingResolved, MappingUnresolved, MappingAmbiguous, MappingNotRequired,
		ValuePresent, ValueMissing, ValueUnparsed, ValueDatatypeMismatch,
		ValueUnknown, ValueNotApplicable,
		Conforms, ConformanceContractViolation, ConformanceNotEvaluated,
		EvidenceSupports, EvidenceContradicts,
	}
	m := make(map[string]bool, len(all))
	for _, t := range all {
		m[t] = true
	}
	return m
}()

// ValidateGovernedIdentifier rejects anything that is not an exact governed
// machine identifier.
//
// The hyphen check is deliberately separate and reported separately: DR9
// permits user interfaces to render hyphenated or natural-language labels, so
// a hyphenated value reaching a persisted field is almost always a UI label
// leaking into storage rather than an unknown concept. Naming that case
// specifically is what makes the failure actionable.
func ValidateGovernedIdentifier(value string) error {
	if value == "" {
		return fmt.Errorf("governed identifier is empty")
	}
	if strings.ContainsAny(value, "- ") {
		return fmt.Errorf("governed identifier %q is a display label, not a machine identifier: persisted state uses the underscore form", value)
	}
	if !governedTerms[value] {
		return fmt.Errorf("governed identifier %q is not a known semantic term", value)
	}
	return nil
}

// IsGovernedTerm reports whether value is an exact governed machine identifier.
func IsGovernedTerm(value string) bool { return governedTerms[value] }

// HighestSeverity returns the highest-ranked severity in the given set, or ""
// for an empty set. It backs the transactional derivation of an outcome's
// highest_severity_term_id from its current child findings (DR4).
func HighestSeverity(severities []string) string {
	best, bestRank := "", 0
	for _, s := range severities {
		if r := severityRank[s]; r > bestRank {
			best, bestRank = s, r
		}
	}
	return best
}
