package classfoundation

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestObservedProfileReaderLabelsEvidenceNonAuthoritativeAndCapsExamples(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_observed_class_profiles p")).
		WithArgs("metric:amount").
		WillReturnRows(sqlmock.NewRows([]string{"id", "class_term_id", "aggregation_method", "method_version", "confidence"}).
			AddRow(int64(3), "metric:amount", "lossless_metric", "v1", 0.7))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_observed_class_profile_examples")).
		WithArgs(int64(3), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "observation_state", "raw_value", "source_excerpt", "method", "confidence", "create_time"}).
			AddRow(int64(1), "represented", "10", "first", "lossless_metric", 0.7, now).
			AddRow(int64(2), "datatype_mismatch", "ten", "second", "lossless_metric", nil, now))

	reader := ObservedProfileReader{DB: db, Caps: Caps{ProfileExamples: 2}}
	profile, err := reader.Get(context.Background(), "metric:amount")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if profile.Authoritative || profile.Authority != "observed evidence; non-authoritative" {
		t.Fatalf("profile authority = %#v", profile)
	}
	if len(profile.Examples) != 2 || profile.Examples[1].ObservationState != "datatype_mismatch" {
		t.Fatalf("examples = %#v", profile.Examples)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
