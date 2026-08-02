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

func TestAlarmForPlanErrorClassifiesGateVersusBindingConflict(t *testing.T) {
	gateAlarm := AlarmForPlanError(&PipelineGateResolutionError{Processor: "extract_metrics", Reason: "decision_relevant_indeterminate"})
	if gateAlarm.Kind != RoutingAlarmKindGateConflict || gateAlarm.Severity != RoutingAlarmSeverityError {
		t.Fatalf("gate alarm=%+v, want kind=%s severity=%s", gateAlarm, RoutingAlarmKindGateConflict, RoutingAlarmSeverityError)
	}

	bindingAlarm := AlarmForPlanError(errors.New("indeterminate conditional pipeline bindings at priority 10"))
	if bindingAlarm.Kind != RoutingAlarmKindBindingConflict {
		t.Fatalf("binding alarm=%+v, want kind=%s", bindingAlarm, RoutingAlarmKindBindingConflict)
	}
}

func TestRoutingAlarmSQLWriterInsertsIntoAlarmsErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors").
		WithArgs(RoutingAlarmSeverityWarning, "uncleared suppressive decision stays shadow-only").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = (RoutingAlarmSQLWriter{DB: db}).WriteAlarm(context.Background(), RoutingAlarm{
		Kind: RoutingAlarmKindFallbackWarning, Severity: RoutingAlarmSeverityWarning,
		Message: "uncleared suppressive decision stays shadow-only",
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
		WithArgs(RoutingAlarmSeverityError, "policy load failure").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = (RoutingAlarmSQLWriter{DB: db}).WriteAlarm(context.Background(), RoutingAlarm{
		Kind: RoutingAlarmKindPolicyIntegrity, Message: "policy load failure",
	})
	if err != nil {
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
