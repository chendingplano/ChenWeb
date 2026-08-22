package kbhandler

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestCreateOntologyComparisonScopePersistsFrozenReleases(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms")).WithArgs("time_to_alarm").
		WillReturnRows(sqlmock.NewRows([]string{"id", "term_id", "version", "term_kind", "module_id", "status", "definition", "scope", "source_candidate_id", "properties", "create_time", "create_by", "modify_time", "modify_by"}).
			AddRow(1, "time_to_alarm", 1, "metric_definition", "measurement", "included_in_release", nil, nil, nil, nil, now, nil, now, nil))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_comparison_scopes")).
		WithArgs("comparison-1", `["obj-1"]`, `["time_to_alarm"]`, "2026-08-01", `[{"module_id":"core","release_id":42}]`, `[{"profile_id":"profile","release_id":42}]`, `{}`, `{}`, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"comparison_scope_id", "target_object_ids", "metric_keys", "as_of_date", "module_releases", "profile_releases", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time"}).
			AddRow("comparison-1", []byte(`["obj-1"]`), []byte(`["time_to_alarm"]`), "2026-08-01", []byte(`[{"module_id":"core","release_id":42}]`), []byte(`[{"profile_id":"profile","release_id":42}]`), []byte(`{}`), []byte(`{}`), "", "", now))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/comparison-scopes", `{"comparison_scope_id":"comparison-1","target_object_ids":["obj-1"],"metric_keys":["time_to_alarm"],"as_of_date":"2026-08-01","module_releases":[{"module_id":"core","release_id":42}],"profile_releases":[{"profile_id":"profile","release_id":42}]}`, nil)
	if err := CreateOntologyComparisonScope(c); err != nil {
		t.Fatalf("CreateOntologyComparisonScope: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
