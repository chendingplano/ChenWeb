package kbhandler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestComplianceGoldInputFilter(t *testing.T) {
	where, args, err := buildWhereClause(listInputsFilters{ParseState: "all", HasGoldMetrics: true, GoldKind: "compliance", GoldSkillVersion: "2.0.0", GoldModelName: "test-model"})
	if err != nil || !strings.Contains(where, "compliance_gold_activities") || !strings.Contains(where, "skill_version = $1") || len(args) != 2 {
		t.Fatalf("where=%s args=%v err=%v", where, args, err)
	}
}

func TestGoldComplianceRequiresExactRun(t *testing.T) {
	for _, query := range []string{"", "input_record_id=0&run_id=x", "input_record_id=416", "input_record_id=416&run_id=bad"} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?"+query, nil), rec)
		if err := ListGoldCompliance(c); err != nil || rec.Code != http.StatusBadRequest {
			t.Fatalf("query=%s status=%d err=%v", query, rec.Code, err)
		}
	}
}

func TestGoldComplianceReadAndMissingRun(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved run", true: "wrong document or missing run"}[missing], func(t *testing.T) {
			mock := withMockProjectDB(t)
			expect := mock.ExpectQuery(regexp.QuoteMeta(goldComplianceQuery)).WithArgs(int64(416), "20261114_123456")
			if missing {
				expect.WillReturnError(sql.ErrNoRows)
			} else {
				expect.WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(`{"run":{"run_key":"20261114_123456","snapshot_id":3},"activities":[{"id":1,"judgements":[]}]}`))
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?input_record_id=416&run_id=20261114_123456", nil), rec)
			if err := ListGoldCompliance(c); err != nil {
				t.Fatal(err)
			}
			want := http.StatusOK
			if missing {
				want = http.StatusNotFound
			}
			if rec.Code != want || (!missing && !strings.Contains(rec.Body.String(), `"judgements":[]`)) {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
