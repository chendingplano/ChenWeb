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

func newRebindPostContext(body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/objects/rebind", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestRebindArtifactObjectsToMasterUpdatesArtifactObjects(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.object_nodes WHERE object_id = \\$1").
		WithArgs("obj_master").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.artifact_objects
SET object_id = $1,
    ext_info = COALESCE(ext_info, '{}'::jsonb) || $2::jsonb
WHERE id = $3`)).
		WithArgs("obj_master", `{"reconcile_method":"manual_admin"}`, int64(201)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.artifact_objects
SET object_id = $1,
    ext_info = COALESCE(ext_info, '{}'::jsonb) || $2::jsonb
WHERE id = $3`)).
		WithArgs("obj_master", `{"reconcile_method":"manual_admin"}`, int64(202)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("INSERT INTO kb.object_audit_log").
		WithArgs("kb.artifact_objects", "201", "resolve_object_id", `{"object_id":"obj_master"}`, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.object_audit_log").
		WithArgs("kb.artifact_objects", "202", "resolve_object_id", `{"object_id":"obj_master"}`, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	c, rec := newRebindPostContext(`{"artifact_object_ids":[201,202],"survivor_object_id":"obj_master"}`)
	if err := RebindArtifactObjectsToMaster(c); err != nil {
		t.Fatalf("RebindArtifactObjectsToMaster: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"survivor_object_id":"obj_master"`, `"updated":2`} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(body) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRebindArtifactObjectsToMasterRequiresMaster(t *testing.T) {
	c, rec := newRebindPostContext(`{"artifact_object_ids":[201,202],"survivor_object_id":""}`)
	if err := RebindArtifactObjectsToMaster(c); err != nil {
		t.Fatalf("RebindArtifactObjectsToMaster: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}
