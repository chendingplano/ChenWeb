package docprocessing

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestDedupeRoutingAlarmsKeepsExactlyOnePerKind(t *testing.T) {
	alarms := []RoutingAlarm{
		{Kind: RoutingAlarmKindFallbackWarning, Message: "first fallback"},
		{Kind: RoutingAlarmKindFallbackWarning, Message: "second fallback"},
		{Kind: RoutingAlarmKindGateConflict, Message: "gate conflict"},
	}
	got := DedupeRoutingAlarms(alarms)
	if len(got) != 2 {
		t.Fatalf("deduped=%v, want 2 alarms (one per kind)", got)
	}
	if got[0].Kind != RoutingAlarmKindFallbackWarning || got[0].Message != "first fallback" {
		t.Fatalf("first alarm=%+v, want the first fallback_warning occurrence kept", got[0])
	}
	if got[1].Kind != RoutingAlarmKindGateConflict {
		t.Fatalf("second alarm=%+v, want gate_conflict", got[1])
	}
}

func TestAlarmForPlanErrorClassifiesGateVersusBindingVersusStructuralFailure(t *testing.T) {
	gateAlarm := AlarmForPlanError(&PipelineGateResolutionError{Processor: "extract_metrics", Reason: "decision_relevant_indeterminate"})
	if gateAlarm.Kind != RoutingAlarmKindGateConflict || gateAlarm.Severity != RoutingAlarmSeverityError {
		t.Fatalf("gate alarm=%+v, want kind=%s severity=%s", gateAlarm, RoutingAlarmKindGateConflict, RoutingAlarmSeverityError)
	}
	if !IsDecisionRelevantPlanConflict(&PipelineGateResolutionError{Processor: "extract_metrics", Reason: "x"}) {
		t.Fatal("gate resolution errors must be decision-relevant conflicts")
	}

	bindingAlarm := AlarmForPlanError(&PipelineBindingConflictError{Priority: 10, Reason: "indeterminate"})
	if bindingAlarm.Kind != RoutingAlarmKindBindingConflict {
		t.Fatalf("binding alarm=%+v, want kind=%s", bindingAlarm, RoutingAlarmKindBindingConflict)
	}
	if !IsDecisionRelevantPlanConflict(&PipelineBindingConflictError{Priority: 10, Reason: "conflicting"}) {
		t.Fatal("binding conflict errors must be decision-relevant conflicts")
	}

	// A structural/configuration error (unknown pipeline, unknown processor,
	// etc.) is NOT a DR7 applicability conflict: it must not be classified as
	// binding_conflict/gate_conflict, and must not be decision-relevant --
	// otherwise every test fixture using a non-production processor/pipeline
	// name would incorrectly block processing (see control.go's planErr
	// handling).
	structuralErr := errors.New(`unknown requested pipeline "typo_pipeline"`)
	structuralAlarm := AlarmForPlanError(structuralErr)
	if structuralAlarm.Kind != RoutingAlarmKindPlanBuildFailure {
		t.Fatalf("structural alarm=%+v, want kind=%s", structuralAlarm, RoutingAlarmKindPlanBuildFailure)
	}
	if IsDecisionRelevantPlanConflict(structuralErr) {
		t.Fatal("a structural/configuration error must not be decision-relevant")
	}
}

func TestRoutingAlarmSQLWriterInsertsIntoAlarmsErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors").
		WithArgs(RoutingAlarmSeverityWarning, "uncleared suppressive decision stays shadow-only", int64(99), RoutingAlarmKindFallbackWarning).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = (RoutingAlarmSQLWriter{DB: db}).WriteAlarm(context.Background(), RoutingAlarm{
		Kind: RoutingAlarmKindFallbackWarning, Severity: RoutingAlarmSeverityWarning,
		Message: "uncleared suppressive decision stays shadow-only", RunID: 99,
	})
	if err != nil {
		t.Fatalf("WriteAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingAlarmSQLWriterDefaultsMissingSeverityToError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors").
		WithArgs(RoutingAlarmSeverityError, "policy load failure", int64(99), RoutingAlarmKindPolicyIntegrity).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = (RoutingAlarmSQLWriter{DB: db}).WriteAlarm(context.Background(), RoutingAlarm{
		Kind: RoutingAlarmKindPolicyIntegrity, Message: "policy load failure", RunID: 99,
	})
	if err != nil {
		t.Fatalf("WriteAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRoutingAlarmSQLWriterUsesConflictClauseForRunScopedDedup proves the
// INSERT relies on uq_alarms_errors_run_id_kind (migration 20260801000019)
// plus ON CONFLICT DO NOTHING for cross-invocation dedup by (run_id, kind) --
// a retry of the same run raising the same alarm kind produces no second
// row. A live-Postgres proof that the conflict actually no-ops belongs to
// the plan's live-DB devdoc task; this proves the SQL shape the writer sends.
func TestRoutingAlarmSQLWriterUsesConflictClauseForRunScopedDedup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors \\(severity, message, run_id, kind\\) VALUES \\(\\$1,\\$2,\\$3,\\$4\\)\\s*\nON CONFLICT \\(run_id, kind\\) WHERE run_id IS NOT NULL AND kind IS NOT NULL DO NOTHING").
		WithArgs(RoutingAlarmSeverityError, "duplicate retry", int64(7), RoutingAlarmKindBindingConflict).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected: the conflict no-op path
	if err := (RoutingAlarmSQLWriter{DB: db}).WriteAlarm(context.Background(), RoutingAlarm{
		Kind: RoutingAlarmKindBindingConflict, Message: "duplicate retry", RunID: 7,
	}); err != nil {
		t.Fatalf("WriteAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRoutingAlarmSQLWriterUsesConflictClauseForRecordScopedDedup proves the
// RunID==0 && RecordID!=0 branch (e.g. AlarmForPlanError's block-mode-
// conflict alarm, raised before any kb.doc_process_runs row exists) relies
// on uq_alarms_errors_record_id_kind plus ON CONFLICT (record_id, kind)
// WHERE run_id IS NULL DO NOTHING -- a retry/redelivery of the same record
// raising the same alarm kind produces no second row, even though no run_id
// is ever available at that call site.
func TestRoutingAlarmSQLWriterUsesConflictClauseForRecordScopedDedup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors \\(severity, message, record_id, kind\\) VALUES \\(\\$1,\\$2,\\$3,\\$4\\)\\s*\nON CONFLICT \\(record_id, kind\\) WHERE run_id IS NULL AND record_id IS NOT NULL AND kind IS NOT NULL DO NOTHING").
		WithArgs(RoutingAlarmSeverityError, "duplicate retry, no run yet", int64(4821), RoutingAlarmKindBindingConflict).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected: the conflict no-op path
	if err := (RoutingAlarmSQLWriter{DB: db}).WriteAlarm(context.Background(), RoutingAlarm{
		Kind: RoutingAlarmKindBindingConflict, Message: "duplicate retry, no run yet", RecordID: 4821,
	}); err != nil {
		t.Fatalf("WriteAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingAlarmSQLWriterPrefersRunIDOverRecordIDWhenBothSet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors \\(severity, message, run_id, kind\\) VALUES").
		WithArgs(RoutingAlarmSeverityError, "run exists", int64(99), RoutingAlarmKindGateConflict).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := (RoutingAlarmSQLWriter{DB: db}).WriteAlarm(context.Background(), RoutingAlarm{
		Kind: RoutingAlarmKindGateConflict, Message: "run exists", RunID: 99, RecordID: 4821,
	}); err != nil {
		t.Fatalf("WriteAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRoutingAlarmSQLWriterInsertsUnconditionallyWithNoCorrelator covers the
// residual case where neither RunID nor RecordID is set (should not happen
// for a real event, which always carries a record id): the alarm inserts
// without any run_id/record_id/kind columns, matching pre-dedup behavior.
func TestRoutingAlarmSQLWriterInsertsUnconditionallyWithNoCorrelator(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors \\(severity, message\\) VALUES \\(\\$1,\\$2\\)").
		WithArgs(RoutingAlarmSeverityError, "no correlator available").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := (RoutingAlarmSQLWriter{DB: db}).WriteAlarm(context.Background(), RoutingAlarm{
		Message: "no correlator available",
	}); err != nil {
		t.Fatalf("WriteAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// fakeAlarmWriter records every WriteAlarm call so tests can assert exactly
// one alarm was persisted per kind before processor execution.
type fakeAlarmWriter struct {
	written []RoutingAlarm
	failFor string
}

func (w *fakeAlarmWriter) WriteAlarm(_ context.Context, alarm RoutingAlarm) error {
	if w.failFor != "" && alarm.Kind == w.failFor {
		return errors.New("simulated alarms_errors outage")
	}
	w.written = append(w.written, alarm)
	return nil
}

func TestWriteRoutingAlarmsDedupesBeforeWriting(t *testing.T) {
	writer := &fakeAlarmWriter{}
	errs := WriteRoutingAlarms(context.Background(), writer, []RoutingAlarm{
		{Kind: RoutingAlarmKindOperatorFailure, Message: "first"},
		{Kind: RoutingAlarmKindOperatorFailure, Message: "duplicate, must not be written"},
	})
	if len(errs) != 0 {
		t.Fatalf("errs=%v, want none", errs)
	}
	if len(writer.written) != 1 || writer.written[0].Message != "first" {
		t.Fatalf("written=%v, want exactly one alarm (the first occurrence)", writer.written)
	}
}

func TestWriteRoutingAlarmsSurvivesWriterOutageWithoutPanicking(t *testing.T) {
	writer := &fakeAlarmWriter{failFor: RoutingAlarmKindOperatorFailure}
	errs := WriteRoutingAlarms(context.Background(), writer, []RoutingAlarm{
		{Kind: RoutingAlarmKindOperatorFailure, Message: "will fail to persist"},
	})
	if len(errs) != 1 {
		t.Fatalf("errs=%v, want exactly one write failure surfaced", errs)
	}
}

func TestWriteRoutingAlarmsNilWriterIsNoop(t *testing.T) {
	if errs := WriteRoutingAlarms(context.Background(), nil, []RoutingAlarm{{Kind: RoutingAlarmKindGateConflict}}); errs != nil {
		t.Fatalf("errs=%v, want nil for nil writer", errs)
	}
}
