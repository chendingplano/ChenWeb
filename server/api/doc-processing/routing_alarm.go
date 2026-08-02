package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Routing alarm kinds are stable identifiers. alarms_errors
// (workspacelists/alarms.go) has no dedicated kind column -- it is a plain
// severity/message/status/notes operator alarm feed used across the
// codebase -- so the kind is folded into Message text; RoutingAlarm.Kind
// stays available to callers/tests for classification and dedupe.
const (
	RoutingAlarmKindBindingConflict = "binding_conflict"
	RoutingAlarmKindGateConflict    = "gate_conflict"
	RoutingAlarmKindOperatorFailure = "operator_failure"
	RoutingAlarmKindPolicyIntegrity = "policy_integrity_failure"
	RoutingAlarmKindFallbackWarning = "fallback_warning"
)

const (
	RoutingAlarmSeverityError   = "error"
	RoutingAlarmSeverityWarning = "warning"
)

// RoutingAlarm is one plan-level operator alarm raised by routing
// resolution/enforcement. Message is the human-readable text persisted
// verbatim into alarms_errors.message; it must stay content-safe (ids,
// checksums, processor/pipeline names only -- never document content).
type RoutingAlarm struct {
	Kind     string
	Severity string
	Message  string
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

// AlarmForPlanError classifies a BuildProductionProcessorPlanFromFacts /
// ResolveProductionPipelineResolution failure -- a decision-relevant
// binding/gate conflict or indeterminacy in block mode -- as an operator
// alarm.
func AlarmForPlanError(planErr error) RoutingAlarm {
	var gateErr *PipelineGateResolutionError
	if errors.As(planErr, &gateErr) {
		return RoutingAlarm{
			Kind: RoutingAlarmKindGateConflict, Severity: RoutingAlarmSeverityError,
			Message: fmt.Sprintf("processor gate resolution failed: %s", planErr.Error()),
		}
	}
	return RoutingAlarm{
		Kind: RoutingAlarmKindBindingConflict, Severity: RoutingAlarmSeverityError,
		Message: fmt.Sprintf("pipeline binding resolution failed: %s", planErr.Error()),
	}
}

// RoutingAlarmWriter persists one operator alarm.
type RoutingAlarmWriter interface {
	WriteAlarm(ctx context.Context, alarm RoutingAlarm) error
}

// RoutingAlarmSQLWriter targets the existing generic alarms_errors table
// (workspacelists/alarms.go) so routing alarms surface alongside every other
// operator alarm on /semos/admin/alarms without a new admin surface.
type RoutingAlarmSQLWriter struct{ DB *sql.DB }

func (w RoutingAlarmSQLWriter) WriteAlarm(ctx context.Context, alarm RoutingAlarm) error {
	if w.DB == nil {
		return errors.New("db is nil")
	}
	severity := alarm.Severity
	if severity == "" {
		severity = RoutingAlarmSeverityError
	}
	_, err := w.DB.ExecContext(ctx, "INSERT INTO alarms_errors (severity, message) VALUES ($1,$2)", severity, alarm.Message)
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
