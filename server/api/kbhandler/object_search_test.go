package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestNormalizeSearchTable(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		valid bool
	}{
		{"", "object_nodes", true},
		{"object_nodes", "object_nodes", true},
		{"artifact_objects", "artifact_objects", true},
		{"  object_nodes  ", "object_nodes", true},
		{"provisions", "", false},
	}
	for _, tc := range cases {
		got, ok := normalizeSearchTable(tc.in)
		if got != tc.want || ok != tc.valid {
			t.Errorf("normalizeSearchTable(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.valid)
		}
	}
}

func TestClampObjectSearchPageSize(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 50}, {-1, 50}, {10, 10}, {200, 200}, {5000, 200},
	}
	for _, tc := range cases {
		if got := clampObjectSearchPageSize(tc.in); got != tc.want {
			t.Errorf("clampObjectSearchPageSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func newSearchPostContext(body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/objects/search", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestSearchObjectsListsObjectNodesByDefault(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("FROM kb.object_nodes").
		WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "object_id", "canonical_name", "canonical_name_en", "object_type", "reconcile_status"}).
			AddRow(int64(1), "O1", "Pump", "Pump", "equipment", "active"))

	c, rec := newSearchPostContext(`{}`)
	if err := SearchObjects(c); err != nil {
		t.Fatalf("SearchObjects: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"table":"object_nodes"`, `"object_id":"O1"`, `"canonical_name":"Pump"`} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(body) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSearchObjectsFetchesArtifactObjectByRecordID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "artifact_type", "artifact_id", "object_name", "object_name_en", "object_id", "reconcile_status"}).
			AddRow(int64(42), "metric", "M1", "Pressure", "Pressure", "O1", "matched"))

	c, rec := newSearchPostContext(`{"table":"artifact_objects","record_id":42}`)
	if err := SearchObjects(c); err != nil {
		t.Fatalf("SearchObjects: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"table":"artifact_objects"`, `"artifact_id":"M1"`, `"object_name":"Pressure"`} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(body) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSearchObjectsRejectsInvalidTable(t *testing.T) {
	c, rec := newSearchPostContext(`{"table":"provisions"}`)
	if err := SearchObjects(c); err != nil {
		t.Fatalf("SearchObjects: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}
