package assertions

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func assertionRow(cols []string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(cols).AddRow(
		int64(1), "p3test:metric:001", 1, "object_node", "obj_1",
		nil, "mea:measured_by", "literal", nil,
		nil, []byte(`{"value":250}`), nil, "positive",
		nil, []byte("null"), nil, "", nil, nil,
		nil, nil, nil, nil, nil,
		nil, "", StatusCandidate, nil,
		nil, nil, nil, nil,
		now, now, "tester", now, "tester",
	)
}

func assertionColumnNames() []string {
	return []string{
		"id", "logical_identity_key", "revision", "subject_ref_kind", "subject_ref_id",
		"subject_object_id", "predicate_term_id", "object_ref_kind", "object_ref_id",
		"object_object_id", "object_literal", "assertion_kind_term_id", "polarity",
		"modality", "qualifiers", "confidence", "value_form", "numeric_value", "lower_value",
		"upper_value", "lower_inclusive", "upper_inclusive", "comparator", "unit_term_id",
		"quantity_kind_term_id", "raw_text", "status", "decision_reason",
		"dependency_fingerprint", "superseded_by", "valid_time_start", "valid_time_end",
		"transaction_time", "create_time", "create_by", "modify_time", "modify_by",
	}
}

func TestAssertionStoreCreateAssertionInsertsRevisionOne(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_assertions")).
		WillReturnRows(assertionRow(assertionColumnNames()))

	store := AssertionStore{DB: db}
	got, err := store.CreateAssertion(context.Background(), Assertion{
		LogicalIdentityKey: "p3test:metric:001",
		SubjectRefKind:     "object_node",
		SubjectRefID:       "obj_1",
		PredicateTermID:    "mea:measured_by",
		ObjectRefKind:      "literal",
		ObjectLiteral:      []byte(`{"value":250}`),
	})
	if err != nil {
		t.Fatalf("CreateAssertion: %v", err)
	}
	if got.LogicalIdentityKey != "p3test:metric:001" || got.Revision != 1 || got.Status != StatusCandidate {
		t.Fatalf("unexpected assertion: %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestAssertionStoreCreateAssertionRequiresObjectRefOrLiteral(t *testing.T) {
	store := AssertionStore{DB: nil}
	_, err := store.CreateAssertion(context.Background(), Assertion{
		LogicalIdentityKey: "x",
		SubjectRefKind:     "object_node",
		SubjectRefID:       "obj_1",
		PredicateTermID:    "mea:measured_by",
	})
	if err == nil {
		t.Fatal("expected error when neither object_ref_id nor object_literal is set")
	}
}

func TestAssertionStoreCreateAssertionRejectsUnsupportedSubjectKind(t *testing.T) {
	store := AssertionStore{DB: nil}
	_, err := store.CreateAssertion(context.Background(), Assertion{
		LogicalIdentityKey: "x",
		SubjectRefKind:     "widget",
		SubjectRefID:       "obj_1",
		PredicateTermID:    "mea:measured_by",
		ObjectRefKind:      "literal",
		ObjectLiteral:      []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected error for unsupported subject_ref_kind")
	}
}

func TestAssertionStoreTransitionStatusRejectsIllegalTransition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := assertionRow(assertionColumnNames())
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(rows)

	store := AssertionStore{DB: db}
	// current status from the fixture row is StatusCandidate; candidate ->
	// accepted skips in_review and must be rejected.
	_, err = store.TransitionStatus(context.Background(), 1, StatusAccepted, "", "tester")
	if err == nil {
		t.Fatal("expected illegal transition error")
	}
}

func TestAssertionStoreDeferAssertionRequiresDependencyFingerprint(t *testing.T) {
	store := AssertionStore{DB: nil}
	_, err := store.DeferAssertion(context.Background(), 1, "", "reason", "tester")
	if err == nil {
		t.Fatal("expected error when dependency fingerprint is empty")
	}
}

func TestAssertionStoreRetryDeferredRejectsUnchangedFingerprint(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	cols := assertionColumnNames()
	now := time.Now()
	deferredRow := sqlmock.NewRows(cols).AddRow(
		int64(1), "p3test:metric:001", 1, "object_node", "obj_1",
		nil, "mea:measured_by", "literal", nil,
		nil, []byte(`{}`), nil, "positive",
		nil, []byte("null"), nil, "", nil, nil,
		nil, nil, nil, nil, nil,
		nil, "", StatusDeferred, nil,
		"fp-v1", nil, nil, nil,
		now, now, "tester", now, "tester",
	)
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(deferredRow)

	store := AssertionStore{DB: db}
	_, err = store.RetryDeferred(context.Background(), 1, "fp-v1", "tester")
	if err != errNoRetryWithoutDependencyChange {
		t.Fatalf("expected errNoRetryWithoutDependencyChange, got %v", err)
	}
}
