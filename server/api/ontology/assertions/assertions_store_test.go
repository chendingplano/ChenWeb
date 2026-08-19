package assertions

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

func TestValidateAssertionAllowsMissingValueWithoutObjectPayload(t *testing.T) {
	err := validateAssertion(Assertion{
		LogicalIdentityKey: "claim:missing-value",
		SubjectRefKind:     "object_node",
		SubjectRefID:       "obj-1",
		PredicateTermID:    "mea:measured_by",
		ValueStateTermID:   semantic.ValueMissing,
	})
	if err != nil {
		t.Fatalf("validateAssertion: %v", err)
	}
}

func TestAssertionColumnsIncludeLosslessStateFields(t *testing.T) {
	for _, column := range []string{
		"unsupported_prior_status",
		"class_identity_state_term_id",
		"mapping_resolution_state_term_id",
		"value_state_term_id",
		"conformance_state_term_id",
		"raw_payload",
		"raw_snapshot_fingerprint",
		"processing_error_details",
		"normalized_against_contract_revision_id",
	} {
		if !strings.Contains(assertionColumns, column) {
			t.Errorf("assertion read projection omits %q", column)
		}
	}
}

func TestListAdminFiltersDocumentScopedDiagnosticsThroughActiveEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	recordID := int64(7)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.semantic_assertions WHERE 1=1 AND EXISTS").WithArgs(recordID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT ").WillReturnRows(sqlmock.NewRows(assertionColumnNames()))
	_, _, err = (AssertionStore{DB: db}).ListAdmin(context.Background(), AssertionListFilter{InputRecordID: &recordID})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAssertionJSONExposesLosslessStateFields(t *testing.T) {
	contractRevision := int64(42)
	payload, err := json.Marshal(Assertion{
		UnsupportedPriorStatus:              StatusRepresented,
		ClassIdentityStateTermID:            "semantic:class_identity_resolved_existing",
		MappingResolutionStateTermID:        "semantic:mapping_resolution_unresolved",
		ValueStateTermID:                    "semantic:value_state_unparsed",
		ConformanceStateTermID:              "semantic:conformance_not_evaluated",
		RawPayload:                          json.RawMessage(`{"raw":"n/a"}`),
		RawSnapshotFingerprint:              "v1:raw",
		ProcessingErrorDetails:              json.RawMessage(`{"finding":"mapping_unresolved"}`),
		NormalizedAgainstContractRevisionID: &contractRevision,
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	for _, field := range []string{
		"unsupported_prior_status",
		"class_identity_state_term_id",
		"mapping_resolution_state_term_id",
		"value_state_term_id",
		"conformance_state_term_id",
		"raw_payload",
		"raw_snapshot_fingerprint",
		"processing_error_details",
		"normalized_against_contract_revision_id",
	} {
		if !strings.Contains(string(payload), `"`+field+`"`) {
			t.Errorf("assertion JSON omits %q: %s", field, payload)
		}
	}
}

func assertionRow(cols []string) *sqlmock.Rows {
	return assertionRowWithStatus(cols, StatusCandidate)
}

func assertionRowWithStatus(cols []string, status string) *sqlmock.Rows {
	return assertionRowWithStatusAndPrior(cols, status, nil)
}

func assertionRowWithStatusAndPrior(cols []string, status string, prior any) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows(cols).AddRow(
		int64(1), "p3test:metric:001", 1, "object_node", "obj_1",
		nil, "mea:measured_by", "literal", nil,
		nil, []byte(`{"value":250}`), nil, "positive",
		nil, []byte("null"), nil, "", nil, nil,
		nil, nil, nil, nil, nil,
		nil, "", status,
		prior, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil,
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
		"quantity_kind_term_id", "raw_text", "status", "unsupported_prior_status",
		"instance_of_term_id",
		"class_identity_state_term_id", "mapping_resolution_state_term_id",
		"value_state_term_id", "conformance_state_term_id", "raw_payload",
		"raw_snapshot_fingerprint", "processing_error_details",
		"normalized_against_contract_revision_id", "decision_reason",
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

// TestAssertionStoreCreateRevisionSupersedesNonAcceptedPrior locks in the
// fix for the P3-review finding mirroring log §F3: CreateRevision must
// supersede the prior revision whenever it is not already superseded, not
// only when it was accepted. An 'unsupported' prior (restored-from-evidence
// state) left un-superseded by the old accepted-only guard would otherwise
// remain live alongside the new revision.
func TestAssertionStoreCreateRevisionSupersedesNonAcceptedPrior(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions\nWHERE logical_identity_key = $1")).
		WithArgs("p3test:metric:001").
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusUnsupported))

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_assertions")).
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusCandidate))

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.semantic_assertions")).
		WithArgs(int64(1), StatusSuperseded, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := AssertionStore{DB: db}
	_, err = store.CreateRevision(context.Background(), Assertion{
		LogicalIdentityKey: "p3test:metric:001",
		SubjectRefKind:     "object_node",
		SubjectRefID:       "obj_1",
		PredicateTermID:    "mea:measured_by",
		ObjectRefKind:      "literal",
		ObjectLiteral:      []byte(`{"value":250}`),
	})
	if err != nil {
		t.Fatalf("CreateRevision: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v (supersede UPDATE was not issued for an unsupported prior)", err)
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
		nil, "", StatusDeferred,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil,
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

func TestHighestAcceptedAssertionIDReturnsMaxForSubjectObject(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(id), 0) FROM kb.semantic_assertions")).
		WithArgs("obj-1").
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(int64(90)))
	got, err := (AssertionStore{DB: db}).HighestAcceptedAssertionID(context.Background(), "obj-1")
	if err != nil {
		t.Fatalf("HighestAcceptedAssertionID: %v", err)
	}
	if got != 90 {
		t.Fatalf("HighestAcceptedAssertionID = %d, want 90", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHighestAcceptedAssertionIDReturnsZeroWhenNone(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(id), 0) FROM kb.semantic_assertions")).
		WithArgs("obj-none").
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(int64(0)))
	got, err := (AssertionStore{DB: db}).HighestAcceptedAssertionID(context.Background(), "obj-none")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("HighestAcceptedAssertionID = %d, want 0", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Task 5.7 reader compatibility certification: the two tests above match only
// the SELECT prefix, so a regression that dropped "AND status = 'accepted'"
// from the WHERE clause would not be caught. This test independently types
// the full WHERE clause (not copied from the source constant) so it fails if
// the accepted-only guarantee is ever weakened -- this is the accepted-only
// watermark named in consumer-lifecycle-policy.md.
func TestHighestAcceptedAssertionIDQueryFiltersToAcceptedStatusOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(id), 0) FROM kb.semantic_assertions
WHERE subject_object_id = $1 AND status = 'accepted'`)).
		WithArgs("obj-1").
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(int64(1)))
	if _, err := (AssertionStore{DB: db}).HighestAcceptedAssertionID(context.Background(), "obj-1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("query text no longer contains the accepted-only literal: %v", err)
	}
}

// Task 5.7: ListBySubjectObject backs the profile-rule assertion loader
// (api/kbhandler/ontology_review_assertion_loader.go), which always passes
// status="accepted" -- it must never see a represented or unsupported row.
// This has no prior direct test: the loader's own test only exercises a fake
// implementation of the lister interface, never this store method's SQL.
func TestListBySubjectObjectFiltersByRequestedStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`WHERE subject_object_id = $1
  AND ($2 = '' OR status = $2)`)).
		WithArgs("obj-1", "accepted").
		WillReturnRows(assertionRowWithStatus(assertionColumnNames(), StatusAccepted))
	got, err := (AssertionStore{DB: db}).ListBySubjectObject(context.Background(), "obj-1", "accepted")
	if err != nil {
		t.Fatalf("ListBySubjectObject: %v", err)
	}
	if len(got) != 1 || got[0].Status != StatusAccepted {
		t.Fatalf("ListBySubjectObject = %+v, want one accepted row", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("query no longer filters by the requested status: %v", err)
	}
}
