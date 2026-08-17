package semantic

import (
	"strings"
	"testing"
)

// Task 4.6 / DR9: "persisted state, API payloads, dependency fingerprints, and
// canonical serialization use the governed identifiers exactly". UIs may render
// hyphenated labels; storage may not accept them.
func TestValidateGovernedIdentifierRejectsHyphenatedAliases(t *testing.T) {
	aliases := []string{
		"semantic:mapping-unresolved",
		"semantic:raw-preserved",
		"mapping unresolved",
		"raw preserved",
		"semantic:value-state-missing",
	}
	for _, alias := range aliases {
		err := ValidateGovernedIdentifier(alias)
		if err == nil {
			t.Errorf("hyphenated/spaced alias %q was accepted", alias)
			continue
		}
		// The error must name the actual problem so an operator can fix the
		// call site rather than guessing the vocabulary is wrong.
		if !strings.Contains(err.Error(), "display label") {
			t.Errorf("alias %q rejected with unhelpful error: %v", alias, err)
		}
	}
}

func TestValidateGovernedIdentifierRejectsUnknownAndEmpty(t *testing.T) {
	if err := ValidateGovernedIdentifier(""); err == nil {
		t.Error("empty identifier was accepted")
	}
	if err := ValidateGovernedIdentifier("semantic:invented_term"); err == nil {
		t.Error("unknown identifier was accepted")
	}
}

func TestValidateGovernedIdentifierAcceptsEveryDeclaredTerm(t *testing.T) {
	// Every constant this package can persist must validate. If a constant is
	// added without registering it, this catches it before a runtime write does.
	for _, term := range []string{
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
	} {
		if err := ValidateGovernedIdentifier(term); err != nil {
			t.Errorf("declared term %q failed validation: %v", term, err)
		}
	}
}

// DR9: candidate_evidence_conflict is the class-identity STATE;
// semantic:identity_evidence_conflict is the corresponding processing FINDING
// and is never stored in the state field. Conflating them is easy and would
// make state queries silently wrong, so they must remain distinct values.
func TestIdentityConflictStateAndFindingAreDistinct(t *testing.T) {
	if ClassCandidateEvidenceConflict == FindingIdentityEvidenceConflict {
		t.Fatal("the class-identity state and the processing finding must be distinct terms (DR9)")
	}
}

func TestHighestSeverityOrdering(t *testing.T) {
	if got := HighestSeverity(nil); got != "" {
		t.Errorf("empty set = %q, want empty", got)
	}
	if got := HighestSeverity([]string{SeverityInfo, SeverityWarning}); got != SeverityWarning {
		t.Errorf("info+warning = %q, want warning", got)
	}
	if got := HighestSeverity([]string{SeverityError, SeverityInfo}); got != SeverityError {
		t.Errorf("error+info = %q, want error", got)
	}
	// An unrecognized severity must not outrank a real one.
	if got := HighestSeverity([]string{"bogus", SeverityInfo}); got != SeverityInfo {
		t.Errorf("bogus+info = %q, want info", got)
	}
}
