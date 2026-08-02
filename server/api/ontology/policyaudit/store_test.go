package policyaudit

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestSQLStoreWriteEventInsertsContentSafeRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO kb.pipeline_policy_events").
		WithArgs("decision_enforced", int64(7), 3, "processor_rule", int64(12), int64(99), int64(4821), "owner@example.com", `{"effect":"skip","processor":"extract_metrics"}`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = SQLStore{DB: db}.WriteEvent(context.Background(), Event{
		Kind: EventDecisionEnforced, PolicyID: 7, PolicyVersion: 3,
		SubjectKind: "processor_rule", SubjectID: 12, RunID: 99, RecordID: 4821,
		Actor: "owner@example.com", Detail: map[string]any{"effect": "skip", "processor": "extract_metrics"},
	})
	if err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLStoreWriteEventOmitsZeroIdentifiersAsNull(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO kb.pipeline_policy_events").
		WithArgs("binding_authored", nil, nil, nil, nil, nil, nil, nil, `{}`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := (SQLStore{DB: db}).WriteEvent(context.Background(), Event{Kind: EventBindingAuthored}); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLStoreWriteEventRequiresKind(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := (SQLStore{DB: db}).WriteEvent(context.Background(), Event{}); err == nil {
		t.Fatal("expected error for empty event kind")
	}
}

func TestSQLStoreWriteEventRequiresDB(t *testing.T) {
	if err := (SQLStore{}).WriteEvent(context.Background(), Event{Kind: EventRuleAuthored}); err == nil {
		t.Fatal("expected error for nil db")
	}
}
