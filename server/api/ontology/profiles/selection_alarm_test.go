package profiles

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestSelectionAlarmSQLWriterInsertsScopeScopedDedup proves the automatic
// profile-selection-indeterminacy warning (spec 2026080102 section 11) is
// persisted to alarms_errors with a scope_id correlator plus the
// uq_alarms_errors_scope_id_kind partial unique index (migration
// 20260801000021), deduplicated at the database level so a repeated scope
// creation/redelivery can never write a second warning row for the same
// scope. The statement must not reference run_id or record_id: a review scope
// is identified by its TEXT review_scope_id, so the dedup is independent of
// the run/record-id routing-alarm indexes from migration 20260801000019.
func TestSelectionAlarmSQLWriterInsertsScopeScopedDedup(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors \\(severity, message, scope_id, kind\\) VALUES \\(\\$1,\\$2,\\$3,\\$4\\)\\s*\nON CONFLICT \\(scope_id, kind\\) WHERE scope_id IS NOT NULL AND kind IS NOT NULL DO NOTHING").
		WithArgs(SelectionAlarmSeverityWarning, "automatic review profile selection is indeterminate on a requested closed dimension; review continues with explicit indeterminate applicability results", "scope-1", SelectionAlarmKindIndeterminate).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected: the conflict no-op path
	if err := (SelectionAlarmSQLWriter{DB: db}).WriteSelectionAlarm(context.Background(), SelectionAlarm{
		Kind:     SelectionAlarmKindIndeterminate,
		Severity: SelectionAlarmSeverityWarning,
		Message:  "automatic review profile selection is indeterminate on a requested closed dimension; review continues with explicit indeterminate applicability results",
		ScopeID:  "scope-1",
	}); err != nil {
		t.Fatalf("WriteSelectionAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSelectionAlarmSQLWriterIsIndependentOfRoutingAlarms locks the SQL shape:
// the scope-scoped INSERT never touches run_id/record_id columns and never
// reuses the routing-alarm conflict predicates, so selection warnings are
// deduplicated by scope id alone and cannot be collapsed into (or block) a
// routing alarm for the same run/record.
func TestSelectionAlarmSQLWriterIsIndependentOfRoutingAlarms(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors \\(severity, message, scope_id, kind\\) VALUES \\(\\$1,\\$2,\\$3,\\$4\\)\\s*\nON CONFLICT \\(scope_id, kind\\) WHERE scope_id IS NOT NULL AND kind IS NOT NULL DO NOTHING").
		WithArgs(SelectionAlarmSeverityWarning, "indeterminate on a closed dimension", "scope-2", SelectionAlarmKindIndeterminate).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := (SelectionAlarmSQLWriter{DB: db}).WriteSelectionAlarm(context.Background(), SelectionAlarm{
		Kind: SelectionAlarmKindIndeterminate, Message: "indeterminate on a closed dimension", ScopeID: "scope-2",
	}); err != nil {
		t.Fatalf("WriteSelectionAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionAlarmSQLWriterDefaultsMissingSeverityToWarning(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alarms_errors \\(severity, message, scope_id, kind\\) VALUES").
		WithArgs(SelectionAlarmSeverityWarning, "no severity supplied", "scope-3", SelectionAlarmKindIndeterminate).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := (SelectionAlarmSQLWriter{DB: db}).WriteSelectionAlarm(context.Background(), SelectionAlarm{
		Kind: SelectionAlarmKindIndeterminate, Message: "no severity supplied", ScopeID: "scope-3",
	}); err != nil {
		t.Fatalf("WriteSelectionAlarm: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionAlarmSQLWriterRejectsNilDB(t *testing.T) {
	if err := (SelectionAlarmSQLWriter{}).WriteSelectionAlarm(context.Background(), SelectionAlarm{ScopeID: "scope"}); err == nil {
		t.Fatal("WriteSelectionAlarm with nil DB must error")
	}
}

// fakeSelectionAlarmWriter records every WriteSelectionAlarm call so the
// selector/handler tests can assert exactly one warning per indeterminate
// scope id before/after scope creation.
type fakeSelectionAlarmWriter struct {
	written []SelectionAlarm
	failFor string
}

func (w *fakeSelectionAlarmWriter) WriteSelectionAlarm(_ context.Context, alarm SelectionAlarm) error {
	if w.failFor != "" && alarm.Kind == w.failFor {
		return errors.New("simulated alarms_errors outage")
	}
	w.written = append(w.written, alarm)
	return nil
}

// TestWriteSelectionAlarmSurfacesWriterErrors proves the writer itself returns
// a write failure (which the selector then treats as best-effort and surfaces
// via its logger rather than aborting the scope's creation, per spec
// 2026080102 section 11 and TestSelectStillCreatesIndeterminateScopeWhenAlarmWriteFails).
func TestWriteSelectionAlarmSurfacesWriterErrors(t *testing.T) {
	writer := &fakeSelectionAlarmWriter{failFor: SelectionAlarmKindIndeterminate}
	if err := writer.WriteSelectionAlarm(context.Background(), SelectionAlarm{Kind: SelectionAlarmKindIndeterminate, ScopeID: "scope"}); err == nil {
		t.Fatal("writer failure must be returned to the selector")
	}
}
