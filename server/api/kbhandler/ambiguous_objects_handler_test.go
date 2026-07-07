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

func newUpdateArtifactObjectContext(t *testing.T, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/kb/objects/artifact-objects/"+id, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/objects/artifact-objects/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestUpdateArtifactObjectSuccessStampsExtInfoWhenObjectIDSet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta(
		"UPDATE kb.artifact_objects SET object_id = $1, object_name = $2, reconcile_status = $3, ext_info = COALESCE(ext_info, '{}'::jsonb) || $4::jsonb WHERE id = $5",
	)
	mock.ExpectExec(updateQuery).
		WithArgs("obj_b", "Pressure Regulator", "ambiguous_resolved", `{"reconcile_method":"manual_admin"}`, int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	c, rec := newUpdateArtifactObjectContext(t, "42", `{
		"object_id":"obj_b",
		"object_name":"Pressure Regulator",
		"reconcile_status":"ambiguous_resolved"
	}`)
	if err := UpdateArtifactObject(c); err != nil {
		t.Fatalf("UpdateArtifactObject returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateArtifactObjectRejectsInvalidReconcileStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newUpdateArtifactObjectContext(t, "42", `{"reconcile_status":"bogus"}`)
	if err := UpdateArtifactObject(c); err != nil {
		t.Fatalf("UpdateArtifactObject returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateArtifactObjectRejectsNullObjectName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newUpdateArtifactObjectContext(t, "42", `{"object_name":null}`)
	if err := UpdateArtifactObject(c); err != nil {
		t.Fatalf("UpdateArtifactObject returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateArtifactObjectNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.artifact_objects SET description = $1 WHERE id = $2")
	mock.ExpectExec(updateQuery).
		WithArgs("no such row", int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newUpdateArtifactObjectContext(t, "999", `{"description":"no such row"}`)
	if err := UpdateArtifactObject(c); err != nil {
		t.Fatalf("UpdateArtifactObject returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func newUpdateObjectNodeContext(t *testing.T, objectID string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/kb/object-nodes/"+objectID, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/object-nodes/:object_id")
	c.SetParamNames("object_id")
	c.SetParamValues(objectID)
	return c, rec
}

func TestUpdateObjectNodeSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.object_nodes SET aliases = $1, canonical_name = $2 WHERE object_id = $3")
	mock.ExpectExec(updateQuery).
		WithArgs(`["reg","regulator"]`, "Pressure Regulator", "obj_a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	c, rec := newUpdateObjectNodeContext(t, "obj_a", `{
		"aliases":["reg","regulator"],
		"canonical_name":"Pressure Regulator"
	}`)
	if err := UpdateObjectNode(c); err != nil {
		t.Fatalf("UpdateObjectNode returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateObjectNodeRejectsNullCanonicalName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newUpdateObjectNodeContext(t, "obj_a", `{"canonical_name":null}`)
	if err := UpdateObjectNode(c); err != nil {
		t.Fatalf("UpdateObjectNode returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateObjectNodeNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.object_nodes SET description = $1 WHERE object_id = $2")
	mock.ExpectExec(updateQuery).
		WithArgs("no such node", "obj_missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newUpdateObjectNodeContext(t, "obj_missing", `{"description":"no such node"}`)
	if err := UpdateObjectNode(c); err != nil {
		t.Fatalf("UpdateObjectNode returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
