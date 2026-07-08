package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newStatsGetContext(target string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestClampConnectivityTopN(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, 50}, {-5, 50}, {7, 50}, {999, 50},
		{20, 20}, {50, 50}, {100, 100}, {200, 200}, {300, 300},
	}
	for _, tc := range cases {
		if got := clampConnectivityTopN(tc.in); got != tc.want {
			t.Errorf("clampConnectivityTopN(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestGetArtifactObjectStatsComputesOther(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WillReturnRows(sqlmock.NewRows([]string{"total", "provisions", "metrics", "inventory_items", "unresolved"}).
			AddRow(int64(10), int64(3), int64(4), int64(2), int64(1)))

	c, rec := newStatsGetContext("/api/v1/kb/objects/stats/artifact-objects")
	if err := GetArtifactObjectStats(c); err != nil {
		t.Fatalf("GetArtifactObjectStats: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"total":10`, `"provisions":3`, `"metrics":4`, `"inventory_items":2`, `"other":1`, `"unresolved":1`} {
		if !regexp.MustCompile(want).MatchString(body) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetObjectNodeStatsReturnsCounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WillReturnRows(sqlmock.NewRows([]string{"total", "provisions", "metrics", "inventory_items"}).
			AddRow(int64(5), int64(2), int64(3), int64(1)))

	c, rec := newStatsGetContext("/api/v1/kb/objects/stats/object-nodes")
	if err := GetObjectNodeStats(c); err != nil {
		t.Fatalf("GetObjectNodeStats: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"total":5`, `"provisions":2`, `"metrics":3`, `"inventory_items":1`} {
		if !regexp.MustCompile(want).MatchString(body) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetObjectConnectivityDefaultsAndClampsTopN(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	// top_n=999 is invalid and must clamp to the default 50 passed as LIMIT arg.
	mock.ExpectQuery("FROM kb.object_nodes").
		WithArgs(50).
		WillReturnRows(sqlmock.NewRows([]string{"object_id", "canonical_name", "connections"}).
			AddRow("O1", "Pump", int64(7)).
			AddRow("O2", "Valve", int64(3)))

	c, rec := newStatsGetContext("/api/v1/kb/objects/connectivity?top_n=999")
	if err := GetObjectConnectivity(c); err != nil {
		t.Fatalf("GetObjectConnectivity: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"object_id":"O1"`, `"canonical_name":"Pump"`, `"connections":7`, `"object_id":"O2"`} {
		if !regexp.MustCompile(want).MatchString(body) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
