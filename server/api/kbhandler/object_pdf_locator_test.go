package kbhandler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newLocatorGetContext(target string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestObjectPDFLocatorByArtifactObjectID(t *testing.T) {
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "input_record_id", "source_line_spans"}).
			AddRow(int64(42), int64(7), `["12", "13:15"]`))
	mock.ExpectQuery("FROM kb.inputs").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"file_name"}).AddRow("manual.pdf"))

	c, rec := newLocatorGetContext("/api/v1/kb/objects/pdf-locator?artifact_object_id=42")
	if err := GetObjectPDFLocator(c); err != nil {
		t.Fatalf("GetObjectPDFLocator: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"artifact_object_id":42`, `"input_record_id":7`, `"document":"manual.pdf"`, `"13:15"`} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(body) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestObjectPDFLocatorByObjectIDPicksRepresentative(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	// Representative mention chosen by the query (ORDER BY confidence DESC ...).
	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs("O1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "input_record_id", "source_line_spans"}).
			AddRow(int64(99), int64(3), `["5"]`))
	mock.ExpectQuery("FROM kb.inputs").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"file_name"}).AddRow("spec.pdf"))

	c, rec := newLocatorGetContext("/api/v1/kb/objects/pdf-locator?object_id=O1")
	if err := GetObjectPDFLocator(c); err != nil {
		t.Fatalf("GetObjectPDFLocator: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"artifact_object_id":99`, `"document":"spec.pdf"`} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(body) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestObjectPDFLocatorRequiresParam(t *testing.T) {
	c, rec := newLocatorGetContext("/api/v1/kb/objects/pdf-locator")
	if err := GetObjectPDFLocator(c); err != nil {
		t.Fatalf("GetObjectPDFLocator: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestObjectPDFLocatorNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("FROM kb.artifact_objects").
		WithArgs(int64(1000)).
		WillReturnError(sql.ErrNoRows)

	c, rec := newLocatorGetContext("/api/v1/kb/objects/pdf-locator?artifact_object_id=1000")
	if err := GetObjectPDFLocator(c); err != nil {
		t.Fatalf("GetObjectPDFLocator: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
