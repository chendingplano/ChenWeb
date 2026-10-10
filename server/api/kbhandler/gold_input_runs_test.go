package kbhandler

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAttachGoldMetricRuns(t *testing.T) {
	for _, model := range []string{"", "model-a"} {
		t.Run("model="+model, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			records := []inputRecord{{ID: 753}, {ID: 800}}
			query := "SELECT DISTINCT input_record_id, skill_version, model_name FROM testbed.metrics WHERE input_record_id IN ($1, $2) AND skill_version = $3"
			if model != "" {
				query += " AND model_name = $4"
			}
			query += " ORDER BY input_record_id, skill_version, model_name"
			rows := sqlmock.NewRows([]string{"input_record_id", "skill_version", "model_name"}).AddRow(753, "6.0.0", "model-a")
			if model == "" {
				rows.AddRow(753, "6.0.0", "model-b").AddRow(800, "6.0.0", "model-b")
			}
			expectation := mock.ExpectQuery(regexp.QuoteMeta(query))
			if model == "" {
				expectation.WithArgs(int64(753), int64(800), "6.0.0")
			} else {
				expectation.WithArgs(int64(753), int64(800), "6.0.0", model)
			}
			expectation.WillReturnRows(rows)
			if err := attachGoldMetricRuns(db, records, listInputsFilters{GoldSkillVersion: "6.0.0", GoldModelName: model}); err != nil {
				t.Fatal(err)
			}
			if len(records) != 2 || records[0].ID != 753 {
				t.Fatalf("documents duplicated or reordered: %+v", records)
			}
			wantRuns := 1
			if model == "" {
				wantRuns = 2
			}
			if len(records[0].GoldMetricRuns) != wantRuns || records[0].GoldMetricRuns[0].ModelName != "model-a" || records[0].GoldMetricRuns[0].SkillVersion != "6.0.0" {
				t.Fatalf("incorrect matching runs: %+v", records)
			}
			if model == "" && (len(records[1].GoldMetricRuns) != 1 || records[1].GoldMetricRuns[0].ModelName != "model-b") {
				t.Fatalf("runs attached to wrong document: %+v", records)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAttachGoldMetricRunsEmptyAndFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := attachGoldMetricRuns(db, nil, listInputsFilters{}); err != nil {
		t.Fatal(err)
	}
	want := errors.New("database unavailable")
	mock.ExpectQuery("SELECT DISTINCT input_record_id").WithArgs(int64(753)).WillReturnError(want)
	if err := attachGoldMetricRuns(db, []inputRecord{{ID: 753}}, listInputsFilters{}); !errors.Is(err, want) {
		t.Fatalf("expected database failure, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
