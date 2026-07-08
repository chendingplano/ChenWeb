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

func TestValidateMergeObjectNodes(t *testing.T) {
	if err := validateMergeObjectNodes("", "b"); err == nil {
		t.Error("empty loser should be rejected")
	}
	if err := validateMergeObjectNodes("a", ""); err == nil {
		t.Error("empty survivor should be rejected")
	}
	if err := validateMergeObjectNodes("a", "a"); err == nil {
		t.Error("identical ids should be rejected")
	}
	if err := validateMergeObjectNodes("a", "b"); err != nil {
		t.Errorf("valid distinct ids should pass, got %v", err)
	}
}

func newMergePostContext(body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/objects/merge", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestMergeObjectNodesRepointsAndMarksMerged(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	// Both nodes exist.
	mock.ExpectQuery("FROM kb.object_nodes").
		WithArgs("O_lose", "O_keep").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))

	mock.ExpectBegin()
	// Evidence mention rows are repointed to the survivor, not deleted.
	mock.ExpectExec("UPDATE kb.artifact_objects SET object_id").
		WithArgs("O_keep", "O_lose").
		WillReturnResult(sqlmock.NewResult(0, 3))
	// Loser node is marked merged and redirected to the survivor.
	mock.ExpectExec("UPDATE kb.object_nodes SET canonical_object_id").
		WithArgs("O_keep", "O_lose").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	// Audit entry on the loser row (best-effort, after commit).
	mock.ExpectExec("INSERT INTO kb.object_audit_log").
		WithArgs("kb.object_nodes", "O_lose", "merge_nodes", sqlmock.AnyArg(), nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	c, rec := newMergePostContext(`{"loser_object_id":"O_lose","survivor_object_id":"O_keep"}`)
	if err := MergeObjectNodes(c); err != nil {
		t.Fatalf("MergeObjectNodes: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !regexp.MustCompile(`"repointed_mentions":3`).MatchString(rec.Body.String()) {
		t.Errorf("response should report repointed mention count: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMergeObjectNodesRejectsMissingNode(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	// Only one of the two ids exists.
	mock.ExpectQuery("FROM kb.object_nodes").
		WithArgs("O_lose", "O_missing").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	c, rec := newMergePostContext(`{"loser_object_id":"O_lose","survivor_object_id":"O_missing"}`)
	if err := MergeObjectNodes(c); err != nil {
		t.Fatalf("MergeObjectNodes: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
