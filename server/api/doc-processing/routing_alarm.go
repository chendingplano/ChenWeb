package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Routing alarm kinds are stable identifiers, also persisted verbatim into
// alarms_errors' optional `kind` column (migration 20260801000019) so a
// run-scoped alarm can be deduplicated at the database level across
// separate handleEvent invocations (e.g. a JetStream redelivery), not just
// within one Go call. The kind is additionally folded into Message text for
// operators reading alarms_errors directly, since that table predates this
// column and other producers don't set it.
const (
	RoutingAlarmKindBindingConflict  = "binding_conflict"
	RoutingAlarmKindGateConflict     = "gate_conflict"
	RoutingAlarmKindOperatorFailure  = "operator_failure"
	RoutingAlarmKindPolicyIntegrity  = "policy_integrity_failure"
	RoutingAlarmKindFallbackWarning  = "fallback_warning"
	// RoutingAlarmKindPlanBuildFailure covers a BuildProductionProcessorPlanFromFacts
	// error that is NOT a decision-relevant DR7 binding/gate conflict (e.g. an
	// unknown pipeline/processor name, a broken spec registration) -- a
	// structural/configuration problem, not an applicability conflict. It is
	// alarmed but does not block processing (see IsDecisionRelevantPlanConflict).
	RoutingAlarmKindPlanBuildFailure = "plan_build_failure"
)

const (
	RoutingAlarmSeverityError   = "error"
	RoutingAlarmSeverityWarning = "warning"
)

// RoutingAlarm is one plan-level operator alarm raised by routing
// resolution/enforcement. Message is the human-readable text persisted
// verbatim into alarms_errors.message; it must stay content-safe (ids,
// checksums, processor/pipeline names only -- never document content).
// RunID correlates the alarm to kb.doc_process_runs.id for per-run dedup
// (spec 2026080102 section 11); zero/unset means the alarm was raised
// before a run row exists (e.g. a block-mode conflict that fails before
// processing starts) and is therefore never deduped against other alarms.
type RoutingAlarm struct {
	Kind     string
	Severity string
	Message  string
	RunID    int64
}

// DedupeRoutingAlarms collapses repeated alarms of the same kind into their
// first occurrence. One FinalizeRoutingPlan call is one routing decision for
// one run, so this gives "exactly one alarm per run" per kind, matching spec
// 2026080102 section 11's "raise exactly one error alarm for the plan,
// deduplicated by run id".
func DedupeRoutingAlarms(alarms []RoutingAlarm) []RoutingAlarm {
	seen := make(map[string]bool, len(alarms))
	out := make([]RoutingAlarm, 0, len(alarms))
	for _, alarm := range alarms {
		if seen[alarm.Kind] {
			continue
		}
		seen[alarm.Kind] = true
		out = append(out, alarm)
	}
	return out
}

// AlarmForPlanError classifies a BuildProductionProcessorPlanFromFacts
// failure as an operator alarm. A decision-relevant DR7 binding/gate
// conflict (see IsDecisionRelevantPlanConflict) is classified as
// binding_conflict/gate_conflict; every other plan-build failure (unknown
// pipeline/processor, broken spec registration) is a plan_build_failure --
// still alarmed, but not a policy applicability conflict.
func AlarmForPlanError(planErr error) RoutingAlarm {
	var gateErr *PipelineGateResolutionError
	if errors.As(planErr, &gateErr) {
		return RoutingAlarm{
			Kind: RoutingAlarmKindGateConflict, Severity: RoutingAlarmSeverityError,
			Message: fmt.Sprintf("processor gate resolution failed: %s", planErr.Error()),
		}
	}
	var bindingErr *PipelineBindingConflictError
	if errors.As(planErr, &bindingErr) {
		return RoutingAlarm{
			Kind: RoutingAlarmKindBindingConflict, Severity: RoutingAlarmSeverityError,
			Message: fmt.Sprintf("pipeline binding resolution failed: %s", planErr.Error()),
		}
	}
	return RoutingAlarm{
		Kind: RoutingAlarmKindPlanBuildFailure, Severity: RoutingAlarmSeverityError,
		Message: fmt.Sprintf("routing plan build failed: %s", planErr.Error()),
	}
}

// IsDecisionRelevantPlanConflict reports whether planErr is a
// decision-relevant DR7 binding/gate conflict or indeterminacy -- the exact
// category spec 2026080102 section 11 requires to "fail before processors
// run" in block mode -- as opposed to a structural/configuration error
// (unknown pipeline/processor, broken spec registration) that is alarmed
// but does not block processing, preserving prior behavior for those cases.
func IsDecisionRelevantPlanConflict(planErr error) bool {
	var gateErr *PipelineGateResolutionError
	if errors.As(planErr, &gateErr) {
		return true
	}
	var bindingErr *PipelineBindingConflictError
	return errors.As(planErr, &bindingErr)
}

// RoutingAlarmWriter persists one operator alarm.
type RoutingAlarmWriter interface {
	WriteAlarm(ctx context.Context, alarm RoutingAlarm) error
}

// RoutingAlarmSQLWriter targets the existing generic alarms_errors table
// (workspacelists/alarms.go) so routing alarms surface alongside every other
// operator alarm on /semos/admin/alarms without a new admin surface.
type RoutingAlarmSQLWriter struct{ DB *sql.DB }

// WriteAlarm inserts one alarm. When both RunID and Kind are set, it relies
// on uq_alarms_errors_run_id_kind (migration 20260801000019) plus
// ON CONFLICT DO NOTHING to guarantee at most one row per (run_id, kind)
// even across separate process invocations retrying the same run -- an
// in-memory dedupe (DedupeRoutingAlarms) alone cannot survive a retry.
// Alarms without a RunID (raised before a run row exists) always insert;
// they have no run to dedupe against.
func (w RoutingAlarmSQLWriter) WriteAlarm(ctx context.Context, alarm RoutingAlarm) error {
	if w.DB == nil {
		return errors.New("db is nil")
	}
	severity := alarm.Severity
	if severity == "" {
		severity = RoutingAlarmSeverityError
	}
	var runID any
	if alarm.RunID != 0 {
		runID = alarm.RunID
	}
	var kind any
	if alarm.Kind != "" {
		kind = alarm.Kind
	}
	_, err := w.DB.ExecContext(ctx, `INSERT INTO alarms_errors (severity, message, run_id, kind) VALUES ($1,$2,$3,$4)
ON CONFLICT (run_id, kind) WHERE run_id IS NOT NULL AND kind IS NOT NULL DO NOTHING`, severity, alarm.Message, runID, kind)
	return err
}

// WriteRoutingAlarms dedupes and persists a batch of alarms. Write failures
// are returned but never block the caller's enforcement decision -- an
// alarms-table outage must not stop document processing.
func WriteRoutingAlarms(ctx context.Context, writer RoutingAlarmWriter, alarms []RoutingAlarm) []error {
	if writer == nil {
		return nil
	}
	var errs []error
	for _, alarm := range DedupeRoutingAlarms(alarms) {
		if err := writer.WriteAlarm(ctx, alarm); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
