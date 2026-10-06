package kbhandler

import (
	"net/http"
	"regexp"
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestSortSkillVersionsDescIsNumeric(t *testing.T) {
	got := []string{"1.2.0", "1.10.0", "1.0.0", "2.0", "1.1.0"}
	sortSkillVersionsDesc(got)
	want := []string{"2.0", "1.10.0", "1.2.0", "1.1.0", "1.0.0"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListMetricsTestbedFiltersBySkillVersionAndModel(t *testing.T) {
	cases := []struct {
		name        string
		query       map[string]string
		mockNewest  bool
		wantVersion string
		wantModel   string
	}{
		{"explicit", map[string]string{"skill_version": "1.1.0", "model_name": "gpt-6.1-sol"}, false, "1.1.0", "gpt-6.1-sol"},
		{"defaults to newest version, any model", nil, true, "1.10.0", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			defer db.Close()
			oldDB := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = oldDB }()

			if tc.mockNewest {
				mock.ExpectQuery(regexp.QuoteMeta(`SELECT DISTINCT skill_version FROM testbed.metrics`)).
					WillReturnRows(sqlmock.NewRows([]string{"skill_version"}).AddRow("1.2.0").AddRow("1.10.0").AddRow("1.0.0"))
			}
			mock.ExpectQuery(regexp.QuoteMeta(testbedMetricsQuery)).
				WithArgs(int64(244), tc.wantVersion, tc.wantModel).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))

			c, rec := newListMetricsContext(t, "244")
			c.QueryParams().Set("source", "testbed")
			for k, v := range tc.query {
				c.QueryParams().Set(k, v)
			}
			if err := ListMetrics(c); err != nil {
				t.Fatalf("ListMetrics returned error: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet db expectations: %v", err)
			}
		})
	}
}
