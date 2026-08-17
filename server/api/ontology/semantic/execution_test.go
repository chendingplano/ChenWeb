package semantic

import "testing"

// Task 4.7: execution status is binary and its legacy projection is a pure
// function. The bug this guards against is a caller writing "completed with
// findings" -- or worse, "failed" -- into proc_status because a run had
// findings.
func TestExecutionStatusIsBinaryAndProjectsToLegacy(t *testing.T) {
	if got := LegacyProcStatus(ExecutionCompleted); got != "success" {
		t.Errorf("LegacyProcStatus(completed) = %q, want success", got)
	}
	if got := LegacyProcStatus(ExecutionFailed); got != "failed" {
		t.Errorf("LegacyProcStatus(failed) = %q, want failed", got)
	}
}

func TestOnlyFailureCategoriesFailExecution(t *testing.T) {
	cases := map[OutcomeCategory]ExecutionStatus{
		CategorySystemFailure:               ExecutionFailed,
		CategorySourceOrOutputUnrecoverable: ExecutionFailed,
		CategorySemanticFinding:             ExecutionCompleted,
		CategorySemanticSuccess:             ExecutionCompleted,
	}
	for category, want := range cases {
		if got := ExecutionStatusFor(category); got != want {
			t.Errorf("ExecutionStatusFor(%s) = %s, want %s", category, got, want)
		}
	}
}

// DR3/DR12: a run full of content-level findings must not enter has_failed_proc.
// This is the concrete behavior change that stops mapping misses from creating
// retry storms no retry can resolve.
func TestFindingsDoNotSetHasFailedProc(t *testing.T) {
	report := NewRunReport("associate_semantics", 42)
	for i := 0; i < 500; i++ {
		report.ObserveOutcome(RecordResult{
			Outcome: Outcome{
				DispositionTermID: DispositionRawPreserved,
				ExecutionStatus:   ExecutionCompleted,
			},
			Findings: []Finding{{
				FindingTermID:  FindingMappingUnresolved,
				SeverityTermID: SeverityError, // error severity, still not a failure
			}},
		})
	}
	if report.SetsHasFailedProc() {
		t.Fatal("500 error-severity findings must not set has_failed_proc (DR3)")
	}
	if report.LegacyStatus() != "success" {
		t.Fatalf("legacy status = %q, want success", report.LegacyStatus())
	}
	if report.TotalFindings() != 500 {
		t.Fatalf("total findings = %d, want 500", report.TotalFindings())
	}
}

func TestSystemFailureSetsHasFailedProc(t *testing.T) {
	report := NewRunReport("extract_metrics", 42)
	report.RecordSystemFailure("database unavailable")
	if !report.SetsHasFailedProc() {
		t.Fatal("a genuine system failure must still set has_failed_proc")
	}
	if report.LegacyStatus() != "failed" {
		t.Fatalf("legacy status = %q, want failed", report.LegacyStatus())
	}
}

// DR3 permits "completed with findings" only as a derived display phrase.
func TestDisplayStatusIsDerivedNotPersisted(t *testing.T) {
	clean := FindingSummary{Count: 0}
	if got := clean.DisplayStatus(ExecutionCompleted); got != "completed" {
		t.Errorf("clean run display = %q, want completed", got)
	}
	withFindings := FindingSummary{Count: 3, HighestSeverity: SeverityWarning}
	if got := withFindings.DisplayStatus(ExecutionCompleted); got != "completed with findings" {
		t.Errorf("run with findings display = %q, want %q", got, "completed with findings")
	}
	if got := withFindings.DisplayStatus(ExecutionFailed); got != "failed" {
		t.Errorf("failed run display = %q, want failed", got)
	}
}

func TestFindingSummaryValidateRejectsInconsistentSummaries(t *testing.T) {
	cases := map[string]FindingSummary{
		"count without severity": {Count: 2},
		"severity without count": {HighestSeverity: SeverityWarning},
		"negative count":         {Count: -1},
		"ungoverned severity":    {Count: 1, HighestSeverity: "warning"},
	}
	for name, s := range cases {
		if err := s.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
	ok := FindingSummary{Count: 1, HighestSeverity: SeverityError, ByTerm: map[string]int{FindingUnparsed: 1}}
	if err := ok.Validate(); err != nil {
		t.Errorf("consistent summary rejected: %v", err)
	}
}

func TestNewFindingSummaryDerivesHighestSeverity(t *testing.T) {
	s := NewFindingSummary([]Finding{
		{FindingTermID: FindingUnparsed, SeverityTermID: SeverityInfo, RetryStateTermID: RetryPending},
		{FindingTermID: FindingMappingUnresolved, SeverityTermID: SeverityError, RetryStateTermID: RetryPending},
		{FindingTermID: FindingMappingUnresolved, SeverityTermID: SeverityWarning},
	})
	if s.Count != 3 {
		t.Fatalf("count = %d, want 3", s.Count)
	}
	if s.HighestSeverity != SeverityError {
		t.Fatalf("highest severity = %q, want %q", s.HighestSeverity, SeverityError)
	}
	// DR4: reports count child finding terms, not envelope dispositions.
	if s.ByTerm[FindingMappingUnresolved] != 2 {
		t.Fatalf("ByTerm[mapping_unresolved] = %d, want 2", s.ByTerm[FindingMappingUnresolved])
	}
	if s.ByRetryState[RetryPending] != 2 {
		t.Fatalf("ByRetryState[pending] = %d, want 2", s.ByRetryState[RetryPending])
	}
}

// DR3's required-versus-optional service rule.
func TestClassifyServiceFailure(t *testing.T) {
	if got := ClassifyServiceFailure(true, true, "llm"); got.Category != CategorySystemFailure {
		t.Errorf("required service failure = %s, want system_failure", got.Category)
	}
	if got := ClassifyServiceFailure(false, true, "enricher"); got.Category != CategorySemanticFinding {
		t.Errorf("optional service with fallback = %s, want semantic_finding", got.Category)
	}
	// An optional service with no declared fallback leaves the stage with no
	// defined result, so it cannot honestly complete.
	if got := ClassifyServiceFailure(false, false, "enricher"); got.Category != CategorySystemFailure {
		t.Errorf("optional service without fallback = %s, want system_failure", got.Category)
	}
}

// DR3's narrow "cannot continue" definition.
func TestClassifyPersistenceCapability(t *testing.T) {
	if got := ClassifyPersistenceCapability(false, true); got.Category != CategorySystemFailure {
		t.Errorf("unpersistable outcome = %s, want system_failure", got.Category)
	}
	if got := ClassifyPersistenceCapability(true, false); got.Category != CategorySourceOrOutputUnrecoverable {
		t.Errorf("unidentifiable artifact = %s, want source_or_output_unrecoverable", got.Category)
	}
	// The important case: cannot classify, but CAN commit input, outcome,
	// provenance, and a no-result reason. That is a finding, not a failure.
	if got := ClassifyPersistenceCapability(true, true); got.Category != CategorySemanticFinding {
		t.Errorf("persistable no-result = %s, want semantic_finding", got.Category)
	}
}

// Task 4.7 / DR8: a downstream operation that cannot run must record why.
// "Do nothing" is not an acceptable terminal behavior.
func TestRunReportRecordsDownstreamSkipReasons(t *testing.T) {
	report := NewRunReport("associate_semantics", 7)
	report.SkipDownstream("comparison", "no_verdict: required normalized interval is unparsed")
	report.CompleteDownstream("search_index")
	if report.DownstreamSkipped["comparison"] == "" {
		t.Fatal("skipped downstream operation must carry an explicit reason")
	}
	if len(report.DownstreamCompleted) != 1 {
		t.Fatalf("completed downstream count = %d, want 1", len(report.DownstreamCompleted))
	}
	summary := report.Summary()
	for _, want := range []string{"status=completed", "skipped=[comparison="} {
		if !contains(summary, want) {
			t.Errorf("summary %q missing %q", summary, want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
