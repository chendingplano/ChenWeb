package productreviews

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func metricArtifactRows(rows ...[]driver.Value) *sqlmock.Rows {
	out := sqlmock.NewRows([]string{
		"artifact_id", "source_row_id", "input_record_id",
		"primary_label", "metric_subject", "subject_concept_id", "source_line_spans",
	})
	for _, r := range rows {
		out.AddRow(r...)
	}
	return out
}

func TestMetricListByDocuments(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(rx("FROM kb.search_artifacts_metric sa")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(metricArtifactRows(
			[]driver.Value{"42_mtc_1", int64(501), int64(42), "Display Luminance", "Display", "", []byte(`[[10,12]]`)},
			[]driver.Value{"42_mtc_2", int64(502), int64(42), "Battery Endurance", "Battery", "kc:battery", []byte(`[]`)},
		))

	got, err := (metricAdapter{}).ListByDocuments(context.Background(), db, []int64{42})
	if err != nil {
		t.Fatalf("ListByDocuments: %v", err)
	}
	if len(got) != 2 || got[0].ArtifactID != "42_mtc_1" || got[0].ArtifactType != "metric" {
		t.Fatalf("got %+v", got)
	}
	if got[1].SubjectConcept != "kc:battery" || got[1].SubjectText != "Battery" {
		t.Fatalf("row 2 = %+v", got[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestMetricMatchBySubjectConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(rx("WHERE m.subject_concept_id = ANY($1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(metricArtifactRows(
			[]driver.Value{"77_mtc_3", int64(900), int64(77), "Display Luminance", "Display", "kc:display", []byte(`[]`)},
		))

	got, err := (metricAdapter{}).MatchBySubjectConcept(context.Background(), db, []string{"kc:display", "kc:display"})
	if err != nil {
		t.Fatalf("MatchBySubjectConcept: %v", err)
	}
	if len(got) != 1 || got[0].InputRecordID != 77 {
		t.Fatalf("got %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// Empty id lists short-circuit without hitting the database.
func TestMetricAdapterEmptyInputsNoQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	if got, _ := (metricAdapter{}).ListByDocuments(context.Background(), db, nil); got != nil {
		t.Fatalf("ListByDocuments(nil) = %v, want nil", got)
	}
	if got, _ := (metricAdapter{}).Project(context.Background(), db, nil); got != nil {
		t.Fatalf("Project(nil) = %v, want nil", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected query for empty inputs: %v", err)
	}
}
