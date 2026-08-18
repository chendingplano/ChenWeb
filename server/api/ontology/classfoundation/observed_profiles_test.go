package classfoundation

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestObservedProfileStoreAggregatesMalformedAndConflictingObservations(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_profiles")).
		WithArgs("metric:amount", "lossless_metric", "v1", "aggregator").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(3)))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_attribute_observations")).
		WithArgs(int64(3), "value", "Amount", "decimal", "scalar", "unit:usd", "one", "datatype_mismatch", "lossless_metric", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_attribute_distributions")).
		WithArgs(int64(5), "document", "doc-17", "{}").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_profile_examples")).
		WithArgs(int64(3), int64(5), int64(11), int64(12), "datatype_mismatch", "unparseable", nil, "raw amount", "lossless_metric", nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_profile_exceptions")).
		WithArgs(int64(3), int64(5), int64(11), int64(12), "contradiction", "datatype_mismatch", `{"reason":"numeric expected"}`, "lossless_metric", nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := ObservedProfileStore{DB: db}
	err = store.Record(context.Background(), ObservedProfileObservation{
		ClassTermID:       "metric:amount",
		AttributeKey:      "value",
		ObservedName:      "Amount",
		LogicalDatatype:   "decimal",
		ValueForm:         "scalar",
		UnitTermID:        "unit:usd",
		Cardinality:       "one",
		ObservationState:  "datatype_mismatch",
		AggregationMethod: "lossless_metric",
		MethodVersion:     "v1",
		DocumentKey:       "doc-17",
		AssertionID:       int64Ptr(11),
		EvidenceID:        int64Ptr(12),
		RawValue:          "unparseable",
		SourceExcerpt:     "raw amount",
		ExceptionKind:     "contradiction",
		ExceptionDetails:  `{"reason":"numeric expected"}`,
		By:                "aggregator",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestObservedProfileStoreRejectsDroppedObservationState(t *testing.T) {
	store := ObservedProfileStore{}
	err := store.Record(context.Background(), ObservedProfileObservation{
		ClassTermID: "metric:amount", AttributeKey: "value", ObservationState: "", AggregationMethod: "lossless_metric", MethodVersion: "v1",
	})
	if err == nil {
		t.Fatal("expected observation state error")
	}
}

func TestObservedProfileStoreRetainsMalformedOutlierWithoutContractWrite(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_profiles")).
		WithArgs("metric:temperature", "lossless_metric", "v1", "aggregator").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_attribute_observations")).
		WithArgs(int64(9), "value", nil, "", "", "", nil, "unparsed", "lossless_metric", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_profile_examples")).
		WithArgs(int64(9), int64(10), nil, nil, "unparsed", "not-a-number", nil, nil, "lossless_metric", nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_observed_class_profile_exceptions")).
		WithArgs(int64(9), int64(10), nil, nil, "outlier", "unparsed", `{"reason":"malformed"}`, "lossless_metric", nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = (ObservedProfileStore{DB: db}).Record(context.Background(), ObservedProfileObservation{
		ClassTermID:       "metric:temperature",
		AttributeKey:      "value",
		ObservationState:  "unparsed",
		AggregationMethod: "lossless_metric",
		MethodVersion:     "v1",
		RawValue:          "not-a-number",
		ExceptionKind:     "outlier",
		ExceptionDetails:  `{"reason":"malformed"}`,
		By:                "aggregator",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	// Expectations cover only profile tables. Any attempted contract revision
	// write would be an unexpected query and fail this assertion.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func int64Ptr(value int64) *int64 { return &value }
