package semantic

import "fmt"

// ExecutionStatus is canonically binary (ADR 2026081801 DR3). There is no
// third persisted value: "completed with findings" is a UI phrase derived from
// FindingSummary.Count > 0, never a stored status.
//
// This matters because the current pipeline conflates two axes. A proposed
// range-type mapping makes associate_semantics return an aggregate error, which
// puts the record in has_failed_proc and the failed-processor retry queue --
// operational machinery that cannot fix a missing governed mapping no matter
// how often it re-runs.
type ExecutionStatus string

const (
	ExecutionCompleted ExecutionStatus = "completed"
	ExecutionFailed    ExecutionStatus = "failed"
)

// OutcomeCategory is DR3's classification of why a run ended as it did. It is
// stored alongside ExecutionStatus so a reader can distinguish an
// infrastructure failure from unrecoverable output without re-deriving it.
type OutcomeCategory string

const (
	// CategorySystemFailure: required input unreadable, database unavailable,
	// transaction rollback, required model/service invocation with no usable
	// output, or an invariant violation preventing safe persistence.
	CategorySystemFailure OutcomeCategory = "system_failure"
	// CategorySourceOrOutputUnrecoverable: output so malformed that no
	// artifact can be identified or safely preserved. This is the ONLY
	// category permitted to record an outcome with no artifact_id, and DR13
	// forbids it from creating an unresolved semantic occurrence.
	CategorySourceOrOutputUnrecoverable OutcomeCategory = "source_or_output_unrecoverable"
	// CategorySemanticFinding: mapping missing, value unparsed, datatype
	// mismatch, class provisional/ambiguous, contract violation, missing
	// value, source conflict. Completes with a mandatory finding summary.
	CategorySemanticFinding OutcomeCategory = "semantic_finding"
	// CategorySemanticSuccess: required normalization and validation
	// capabilities produced usable output.
	CategorySemanticSuccess OutcomeCategory = "semantic_success"
)

// ExecutionStatusFor maps a category to its execution status. Only the two
// failure categories fail a processor; severity never converts a finding into
// a failure.
func ExecutionStatusFor(c OutcomeCategory) ExecutionStatus {
	switch c {
	case CategorySystemFailure, CategorySourceOrOutputUnrecoverable:
		return ExecutionFailed
	default:
		return ExecutionCompleted
	}
}

// LegacyProcStatus projects the binary status onto the legacy proc_status
// vocabulary (DR3: "The legacy schema projects completed to success and failed
// to failed").
//
// Every write of a legacy proc_status value goes through this function rather
// than through a literal, so no call site can invent a third value or map
// "completed with findings" to "failed".
func LegacyProcStatus(s ExecutionStatus) string {
	if s == ExecutionFailed {
		return "failed"
	}
	return "success"
}

// SetsHasFailedProc reports whether this status should set kb.inputs
// has_failed_proc and enter the failed-processor retry queue. DR3 and DR12:
// only genuinely failed execution does. A content-level finding never does,
// which is the concrete behavior change that stops mapping misses from
// producing retry storms.
func SetsHasFailedProc(s ExecutionStatus) bool { return s == ExecutionFailed }

// FindingSummary is the mandatory summary every completed run carries (DR3).
// ByTerm counts governed finding terms, not envelope dispositions, because
// DR4 requires reports to count child finding_term_id values.
type FindingSummary struct {
	Count           int
	HighestSeverity string
	ByTerm          map[string]int
	ByRetryState    map[string]int
	New             int
	Reused          int
}

// NewFindingSummary derives a summary from a finding set.
func NewFindingSummary(findings []Finding) FindingSummary {
	s := FindingSummary{
		Count:        len(findings),
		ByTerm:       make(map[string]int, len(findings)),
		ByRetryState: make(map[string]int, len(findings)),
	}
	severities := make([]string, 0, len(findings))
	for _, f := range findings {
		s.ByTerm[f.FindingTermID]++
		if f.RetryStateTermID != "" {
			s.ByRetryState[f.RetryStateTermID]++
		}
		severities = append(severities, f.SeverityTermID)
	}
	s.HighestSeverity = HighestSeverity(severities)
	return s
}

// DisplayStatus renders the derived phrase DR3 permits UIs to show. It is a
// pure function of persisted state; nothing stores its result.
func (s FindingSummary) DisplayStatus(status ExecutionStatus) string {
	if status == ExecutionFailed {
		return "failed"
	}
	if s.Count > 0 {
		return "completed with findings"
	}
	return "completed"
}

// Validate enforces DR3's rule that a completed run's summary is mandatory and
// internally consistent: a zero count carries no severity, a positive count
// must carry one. It mirrors chk_semantic_processing_outcomes_severity_summary
// so a caller fails before the database does, with a clearer message.
func (s FindingSummary) Validate() error {
	switch {
	case s.Count < 0:
		return fmt.Errorf("finding count %d is negative", s.Count)
	case s.Count == 0 && s.HighestSeverity != "":
		return fmt.Errorf("finding count is 0 but highest severity is %q", s.HighestSeverity)
	case s.Count > 0 && s.HighestSeverity == "":
		return fmt.Errorf("finding count is %d but no highest severity was derived", s.Count)
	}
	if s.HighestSeverity != "" {
		if err := ValidateGovernedIdentifier(s.HighestSeverity); err != nil {
			return fmt.Errorf("highest severity: %w", err)
		}
	}
	for term := range s.ByTerm {
		if err := ValidateGovernedIdentifier(term); err != nil {
			return fmt.Errorf("finding summary term: %w", err)
		}
	}
	return nil
}

// ContinuationDecision answers DR3's "cannot continue" question for one stage
// attempt. The narrow definition matters: inability to parse, classify,
// validate, compare, or project remains a FINDING as long as the input,
// outcome, provenance, and an explicit no-result reason can be durably
// committed.
type ContinuationDecision struct {
	Category OutcomeCategory
	Reason   string
}

// ClassifyServiceFailure implements DR3's optional-versus-required service
// rule: an optional enrichment service's failure is a finding when the stage
// contract declares and persists a deterministic fallback, while failure of a
// service the stage declares REQUIRED is an execution failure.
func ClassifyServiceFailure(serviceRequired bool, hasDeterministicFallback bool, serviceName string) ContinuationDecision {
	if serviceRequired {
		return ContinuationDecision{
			Category: CategorySystemFailure,
			Reason:   fmt.Sprintf("required service %q failed with no usable output", serviceName),
		}
	}
	if !hasDeterministicFallback {
		// An optional service with no declared fallback leaves the stage with
		// no defined result to persist, so it cannot honestly complete.
		return ContinuationDecision{
			Category: CategorySystemFailure,
			Reason:   fmt.Sprintf("optional service %q failed and the stage contract declares no deterministic fallback", serviceName),
		}
	}
	return ContinuationDecision{
		Category: CategorySemanticFinding,
		Reason:   fmt.Sprintf("optional service %q failed; deterministic fallback applied", serviceName),
	}
}

// ClassifyPersistenceCapability implements the rest of DR3's boundary: a stage
// that cannot determine a semantic result still COMPLETES when it can durably
// commit its input, outcome, provenance, and an explicit no-result reason. It
// fails only when that persistence set cannot complete.
func ClassifyPersistenceCapability(canPersistOutcome bool, canIdentifyArtifact bool) ContinuationDecision {
	if !canPersistOutcome {
		return ContinuationDecision{
			Category: CategorySystemFailure,
			Reason:   "required atomic persistence set cannot complete",
		}
	}
	if !canIdentifyArtifact {
		return ContinuationDecision{
			Category: CategorySourceOrOutputUnrecoverable,
			Reason:   "no artifact could be identified or safely preserved from processor output",
		}
	}
	return ContinuationDecision{Category: CategorySemanticFinding, Reason: "no semantic result; reason recorded"}
}
