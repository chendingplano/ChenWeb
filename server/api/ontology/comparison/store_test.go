package comparison

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestComparisonStoreCreateScopeAndPersistDirectionalCell(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	releasedMetricTermRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "term_id", "version", "term_kind", "module_id", "status", "definition", "scope", "source_candidate_id", "create_time", "create_by", "modify_time", "modify_by"}).
			AddRow(1, "time_to_alarm", 1, "metric_definition", "measurement", "included_in_release", nil, nil, nil, now, nil, now, nil)
	}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms")).WithArgs("time_to_alarm").WillReturnRows(releasedMetricTermRows())
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_comparison_scopes")).
		WithArgs("comparison-scope-1", `["display-1"]`, `["time_to_alarm"]`, "2026-08-01", `[{"module_id":"ventilator-display","release_id":42}]`, `[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`, `{"authority":"GB 9706.212-2020"}`, `{"time_to_alarm":true}`, "reviewer", "fixture").
		WillReturnRows(sqlmock.NewRows([]string{"comparison_scope_id", "target_object_ids", "metric_keys", "as_of_date", "module_releases", "profile_releases", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time"}).
			AddRow("comparison-scope-1", []byte(`["display-1"]`), []byte(`["time_to_alarm"]`), "2026-08-01", []byte(`[{"module_id":"ventilator-display","release_id":42}]`), []byte(`[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`), []byte(`{"authority":"GB 9706.212-2020"}`), []byte(`{"time_to_alarm":true}`), "reviewer", "fixture", now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_comparison_runs")).
		WithArgs("comparison-scope-1", int64(99), "assertion:1234", "comparison-v1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comparison_scope_id", "input_record_id", "assertion_watermark", "comparator_version", "create_time"}).
			AddRow(7, "comparison-scope-1", 99, "assertion:1234", "comparison-v1", now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms")).WithArgs("time_to_alarm").WillReturnRows(releasedMetricTermRows())
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_comparison_cells")).
		WithArgs(int64(7), "display-1", "time_to_alarm", "enterprise", int64(101), `[{"assertion_id":101,"citation":"enterprise.pdf#p12"}]`, int64(0), "GB", int64(202), `[{"assertion_id":202,"citation":"GB-9706.212-2020#5.3"}]`, int64(1), string(Stronger), "subject_to_authority", "subject's satisfying set is a strict subset").
		WillReturnResult(sqlmock.NewResult(1, 1))

	store := ComparisonStore{DB: db}
	scope, err := store.CreateScope(context.Background(), ComparisonScope{
		ComparisonScopeID: "comparison-scope-1", TargetObjectIDs: json.RawMessage(`["display-1"]`), MetricKeys: json.RawMessage(`["time_to_alarm"]`), AsOfDate: "2026-08-01", ModuleReleases: json.RawMessage(`[{"module_id":"ventilator-display","release_id":42}]`), ProfileReleases: json.RawMessage(`[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`), PrecedencePolicy: json.RawMessage(`{"authority":"GB 9706.212-2020"}`), ClosedDimensions: json.RawMessage(`{"time_to_alarm":true}`), SelectedBy: "reviewer", SelectionReason: "fixture",
	})
	if err != nil || scope.ComparisonScopeID != "comparison-scope-1" {
		t.Fatalf("CreateScope = %#v, %v", scope, err)
	}
	run, err := store.CreateRun(context.Background(), ComparisonRun{ComparisonScopeID: scope.ComparisonScopeID, InputRecordID: 99, AssertionWatermark: "assertion:1234", ComparatorVersion: "comparison-v1"})
	if err != nil || run.ID != 7 {
		t.Fatalf("CreateRun = %#v, %v", run, err)
	}
	err = store.PersistCell(context.Background(), ComparisonCell{RunID: run.ID, TargetObjectID: "display-1", MetricKey: "time_to_alarm", SubjectFamily: "enterprise", SubjectRepresentativeAssertionID: 101, SubjectAssertions: json.RawMessage(`[{"assertion_id":101,"citation":"enterprise.pdf#p12"}]`), AuthorityFamily: "GB", AuthorityRepresentativeAssertionID: 202, AuthorityAssertions: json.RawMessage(`[{"assertion_id":202,"citation":"GB-9706.212-2020#5.3"}]`), AuthorityRemainderCount: 1, Verdict: Stronger, Direction: "subject_to_authority", Rationale: "subject's satisfying set is a strict subset"})
	if err != nil {
		t.Fatalf("PersistCell: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestComparisonStoreCreateScopeRejectsMetricKeyThatIsNotAReleasedMetricDefinitionTerm(t *testing.T) {
	cases := []struct {
		name string
		rows func() *sqlmock.Rows
	}{
		{"unknown term id", func() *sqlmock.Rows {
			return sqlmock.NewRows([]string{"id", "term_id", "version", "term_kind", "module_id", "status", "definition", "scope", "source_candidate_id", "create_time", "create_by", "modify_time", "modify_by"})
		}},
		{"wrong term_kind", func() *sqlmock.Rows {
			return sqlmock.NewRows([]string{"id", "term_id", "version", "term_kind", "module_id", "status", "definition", "scope", "source_candidate_id", "create_time", "create_by", "modify_time", "modify_by"}).
				AddRow(1, "time_to_alarm", 1, "unit", "measurement", "included_in_release", nil, nil, nil, time.Now(), nil, time.Now(), nil)
		}},
		{"not yet released", func() *sqlmock.Rows {
			return sqlmock.NewRows([]string{"id", "term_id", "version", "term_kind", "module_id", "status", "definition", "scope", "source_candidate_id", "create_time", "create_by", "modify_time", "modify_by"}).
				AddRow(1, "time_to_alarm", 1, "metric_definition", "measurement", "draft", nil, nil, nil, time.Now(), nil, time.Now(), nil)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer db.Close()
			mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms")).WithArgs("time_to_alarm").WillReturnRows(tc.rows())
			_, err = (ComparisonStore{DB: db}).CreateScope(context.Background(), ComparisonScope{
				ComparisonScopeID: "comparison-scope-1", TargetObjectIDs: json.RawMessage(`["display-1"]`), MetricKeys: json.RawMessage(`["time_to_alarm"]`), AsOfDate: "2026-08-01", ModuleReleases: json.RawMessage(`[{"module_id":"ventilator-display","release_id":42}]`), ProfileReleases: json.RawMessage(`[{"profile_id":"ventilator-display:display_metrics","release_id":42}]`),
			})
			if err == nil {
				t.Fatal("CreateScope: want error, got nil")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEvaluateDirectionalCellUsesFamilyEvidence(t *testing.T) {
	subject := upperBound("Time", "ms", "120")
	authority := upperBound("Time", "ms", "150")
	cell, err := EvaluateDirectionalCell(DirectionalCellInput{
		RunID: 7, TargetObjectID: "display-1", MetricKey: "time_to_alarm",
		SubjectFamily: "enterprise", SubjectConstraint: subject,
		SubjectRepresentativeAssertionID: 101,
		SubjectAssertions:                json.RawMessage(`[{"assertion_id":101,"citation":"enterprise.pdf#p12"}]`),
		AuthorityFamily:                  "GB", AuthorityEvidence: FamilyEvidence{Representative: &authority, Applicable: true},
		AuthorityRepresentativeAssertionID: 202,
		AuthorityAssertions:                json.RawMessage(`[{"assertion_id":202,"citation":"GB-9706.212-2020#5.3"}]`),
	})
	if err != nil {
		t.Fatalf("EvaluateDirectionalCell: %v", err)
	}
	if cell.Verdict != Stronger || cell.Direction != "subject_to_authority" {
		t.Fatalf("cell = %#v, want stronger subject-to-authority", cell)
	}
}

func TestComparisonStoreGetScopeLoadsFrozenReleaseSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT comparison_scope_id, target_object_ids")).WithArgs("comparison-scope-1").
		WillReturnRows(sqlmock.NewRows([]string{"comparison_scope_id", "target_object_ids", "metric_keys", "as_of_date", "module_releases", "profile_releases", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time"}).
			AddRow("comparison-scope-1", []byte(`[]`), []byte(`[]`), "2026-08-01", []byte(`[{"release_id":42}]`), []byte(`[]`), []byte(`{}`), []byte(`{}`), "reviewer", "historical", now))
	got, err := (ComparisonStore{DB: db}).GetScope(context.Background(), "comparison-scope-1")
	if err != nil || string(got.ModuleReleases) != `[{"release_id":42}]` {
		t.Fatalf("GetScope = %#v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestComparisonStoreListCellsForRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_comparison_cells WHERE comparison_run_id = $1")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"comparison_run_id", "target_object_id", "metric_key", "subject_family", "subject_representative_assertion_id", "subject_assertions", "subject_remainder_count", "authority_family", "authority_representative_assertion_id", "authority_assertions", "authority_remainder_count", "verdict", "direction", "rationale"}).AddRow(7, "obj", "metric", "enterprise", 1, []byte(`[]`), 0, "authority", 2, []byte(`[]`), 0, "stronger", "subject_to_authority", "tighter"))
	got, err := (ComparisonStore{DB: db}).ListCells(context.Background(), 7)
	if err != nil || len(got) != 1 || got[0].Verdict != Stronger {
		t.Fatalf("ListCells=%#v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
