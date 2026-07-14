package docbenchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMetricAdapterUsesOrderedRowsAndReconcilesStableFields(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	file := filepath.Join(t.TempDir(), "x.metrics")
	rows := `[{"metric_id":"a","metric_name":"A","metric_subject":"S","metric_value":"1","metric_unit":"kg","is_explicit_metric":true,"source_line_spans":[1],"file_only":"x"},{"metric_id":"b","metric_name":"B","metric_subject":"T","metric_value":"2","metric_unit":"m","is_explicit_metric":false,"source_line_spans":[2]}]`
	if err := os.WriteFile(file, []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}
	const expectedMetricQuery = `SELECT to_jsonb(m) AS row
FROM kb.metrics AS m
WHERE m.input_record_id = $1
ORDER BY m.metric_id COLLATE "C" ASC NULLS LAST, m.id ASC`
	mock.ExpectQuery(regexp.QuoteMeta(expectedMetricQuery)).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"row"}).
		AddRow([]byte(`{"metric_id":"a","metric_name":"A","metric_subject":"S","metric_value":"1","metric_unit":"kg","is_explicit_metric":true,"source_line_spans":[1],"db_index":0}`)).
		AddRow([]byte(`{"metric_id":"b","metric_name":"B","metric_subject":"T","metric_value":"2","metric_unit":"m","is_explicit_metric":false,"source_line_spans":[2],"db_index":1}`)))
	a := MetricAdapter{DB: db, ArtifactPath: func(int64) string { return file }}
	v, err := a.Capture(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	gotAny, err := a.Reconcile(v)
	if err != nil {
		t.Fatal(err)
	}
	got := gotAny.(MetricActual)
	if !reflect.DeepEqual(got.Rows[0]["db_index"], float64(0)) || !reflect.DeepEqual(got.Rows[1]["db_index"], float64(1)) {
		t.Fatalf("order lost: %#v", got.Rows)
	}
	v2, _ := a.Reconcile(v)
	if !reflect.DeepEqual(gotAny, v2) {
		t.Fatalf("nondeterministic: %#v != %#v", gotAny, v2)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricAdapterMissingOrDisagreementIsInvalidOutput(t *testing.T) {
	for _, tc := range []struct {
		name, contents string
		write          bool
	}{
		{"missing", "", false},
		{"disagreement", `[{"metric_id":"wrong"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			file := filepath.Join(t.TempDir(), "x.metrics")
			if tc.write {
				_ = os.WriteFile(file, []byte(tc.contents), 0o600)
			}
			mock.ExpectQuery(`SELECT to_jsonb\(m\) AS row\s+FROM kb\.metrics AS m\s+WHERE m\.input_record_id = \$1\s+ORDER BY m\.metric_id COLLATE "C" ASC NULLS LAST, m\.id ASC`).WillReturnRows(sqlmock.NewRows([]string{"row"}).AddRow([]byte(`{"metric_id":"a"}`)))
			a := MetricAdapter{DB: db, ArtifactPath: func(int64) string { return file }}
			v, err := a.Capture(context.Background(), 1)
			if tc.write && err == nil {
				_, err = a.Reconcile(v)
			}
			if !errors.Is(err, ErrInvalidOutput) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
