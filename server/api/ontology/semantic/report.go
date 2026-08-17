package semantic

import (
	"fmt"
	"sort"
	"strings"
)

// RunReport is DR11's per-run report. Its shape is the decision: findings are
// grouped and counted rather than emitted one alarm at a time, and actual
// system failures are a separate section so an operator can tell a document
// that failed to process from one that processed with findings.
type RunReport struct {
	ProcessorName string
	InputRecordID int64

	ArtifactsExamined int
	// InstancesByDisposition counts semantic instances by governed
	// disposition term (normalized, raw_preserved, ...).
	InstancesByDisposition map[string]int
	// FindingsByTerm counts child findings by governed finding term.
	FindingsByTerm       map[string]int
	FindingsBySeverity   map[string]int
	FindingsByRetryState map[string]int
	NewFindings          int
	ReusedFindings       int
	// DownstreamSkipped maps a downstream operation name to the explicit
	// reason it produced no result. DR8 forbids "do nothing" as a terminal
	// behavior, so an operation that could not run must appear here.
	DownstreamSkipped   map[string]string
	DownstreamCompleted []string
	// SystemFailures is kept separate from findings. This is the line DR11
	// draws: a semantic finding must never be displayed as a failed document.
	SystemFailures []string

	ExecutionStatus ExecutionStatus
}

// NewRunReport returns an initialized report.
func NewRunReport(processorName string, inputRecordID int64) *RunReport {
	return &RunReport{
		ProcessorName:          processorName,
		InputRecordID:          inputRecordID,
		InstancesByDisposition: map[string]int{},
		FindingsByTerm:         map[string]int{},
		FindingsBySeverity:     map[string]int{},
		FindingsByRetryState:   map[string]int{},
		DownstreamSkipped:      map[string]string{},
		ExecutionStatus:        ExecutionCompleted,
	}
}

// ObserveOutcome folds one recorded attempt into the report.
func (r *RunReport) ObserveOutcome(res RecordResult) {
	r.ArtifactsExamined++
	if res.Outcome.DispositionTermID != "" {
		r.InstancesByDisposition[res.Outcome.DispositionTermID]++
	}
	for _, f := range res.Findings {
		r.FindingsByTerm[f.FindingTermID]++
		r.FindingsBySeverity[f.SeverityTermID]++
		if f.RetryStateTermID != "" {
			r.FindingsByRetryState[f.RetryStateTermID]++
		}
		if res.Reused {
			r.ReusedFindings++
		} else {
			r.NewFindings++
		}
	}
	if res.Outcome.ExecutionStatus == ExecutionFailed {
		r.ExecutionStatus = ExecutionFailed
	}
}

// SkipDownstream records that a downstream operation could not run, with the
// explicit reason DR8 requires.
func (r *RunReport) SkipDownstream(operation, reason string) {
	r.DownstreamSkipped[operation] = reason
}

// CompleteDownstream records a downstream operation that ran.
func (r *RunReport) CompleteDownstream(operation string) {
	r.DownstreamCompleted = append(r.DownstreamCompleted, operation)
}

// RecordSystemFailure records a genuine execution failure and fails the run.
func (r *RunReport) RecordSystemFailure(cause string) {
	r.SystemFailures = append(r.SystemFailures, cause)
	r.ExecutionStatus = ExecutionFailed
}

// TotalFindings counts every finding observed.
func (r *RunReport) TotalFindings() int {
	n := 0
	for _, c := range r.FindingsByTerm {
		n += c
	}
	return n
}

// LegacyStatus is the value to write to the legacy proc_status field.
func (r *RunReport) LegacyStatus() string { return LegacyProcStatus(r.ExecutionStatus) }

// SetsHasFailedProc reports whether this run should set has_failed_proc.
// A run with thousands of findings and no system failure returns false: that
// is the concrete behavior DR12 changes.
func (r *RunReport) SetsHasFailedProc() bool { return SetsHasFailedProc(r.ExecutionStatus) }

// Summary renders a single log line per run rather than one alarm per
// artifact, which is what DR11 means by "logs summarize per record/run".
func (r *RunReport) Summary() string {
	var sb strings.Builder
	// status is the PERSISTED binary value; display is the derived phrase DR11
	// allows a UI to show. Logging both keeps an operator from reading
	// "completed with findings" as a failed document while still making the
	// stored value greppable.
	display := FindingSummary{Count: r.TotalFindings()}.DisplayStatus(r.ExecutionStatus)
	fmt.Fprintf(&sb, "%s record=%d status=%s display=%q examined=%d findings=%d new=%d reused=%d",
		r.ProcessorName, r.InputRecordID, r.ExecutionStatus, display,
		r.ArtifactsExamined, r.TotalFindings(), r.NewFindings, r.ReusedFindings)
	if len(r.InstancesByDisposition) > 0 {
		fmt.Fprintf(&sb, " dispositions=[%s]", renderCounts(r.InstancesByDisposition))
	}
	if len(r.FindingsByTerm) > 0 {
		fmt.Fprintf(&sb, " findings_by_term=[%s]", renderCounts(r.FindingsByTerm))
	}
	if len(r.DownstreamSkipped) > 0 {
		keys := sortedKeys(r.DownstreamSkipped)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", k, r.DownstreamSkipped[k]))
		}
		fmt.Fprintf(&sb, " skipped=[%s]", strings.Join(parts, " "))
	}
	if len(r.SystemFailures) > 0 {
		fmt.Fprintf(&sb, " system_failures=%d", len(r.SystemFailures))
	}
	return sb.String()
}

func renderCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
