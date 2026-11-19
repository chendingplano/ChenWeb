package releaseshandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

func setup(t *testing.T, user *ApiTypes.UserInfo, machineRole string) sqlmock.Sqlmock {
	t.Helper()
	oldAuth, oldRole, oldDB := EchoFactory.DefaultAuthenticator, role, ApiTypes.ProjectDBHandle
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator, role, ApiTypes.ProjectDBHandle = oldAuth, oldRole, oldDB })
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	role = func() string { return machineRole }
	db, mock := newMock(t)
	ApiTypes.ProjectDBHandle = db
	return mock
}

func admin() *ApiTypes.UserInfo { return &ApiTypes.UserInfo{Admin: true, Email: "admin@example.com"} }

func call(t *testing.T, h echo.HandlerFunc, method, target, body string, params ...string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if len(params) == 2 {
		c.SetParamNames(params[0])
		c.SetParamValues(params[1])
	}
	_ = h(c)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestWritesRefusedOffTheBuildMachine(t *testing.T) {
	for name, h := range map[string]echo.HandlerFunc{
		"major": CreateMajor, "minor": CreateMinor, "item": CreateItem,
		"update": UpdateItemHandler, "delete": DeleteItemHandler,
	} {
		setup(t, admin(), "deploy")
		rec, out := call(t, h, http.MethodPost, "/", `{}`, "id", "1")
		if rec.Code != http.StatusForbidden || out["code"] != codeReadOnly {
			t.Errorf("%s: %d %v, want 403 read_only", name, rec.Code, out)
		}
	}
}

func TestNonAdminRefused(t *testing.T) {
	setup(t, &ApiTypes.UserInfo{Email: "user@example.com"}, RoleBuild)
	if rec, _ := call(t, ListReleases, http.MethodGet, "/releases", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
	setup(t, nil, RoleBuild)
	if rec, _ := call(t, GetNotes, http.MethodGet, "/releases/notes", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestUnknownProjectRejected(t *testing.T) {
	setup(t, admin(), RoleBuild)
	if rec, _ := call(t, ListReleases, http.MethodGet, "/releases?project=tax", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestEditingBoundItemConflicts(t *testing.T) {
	mock := setup(t, admin(), RoleBuild)
	mock.ExpectQuery(`UPDATE kb.release_items AS i SET`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	rec, _ := call(t, UpdateItemHandler, http.MethodPut, "/releases/items/5",
		`{"item_type":"bug fix","component":"ChenWeb","description":"x"}`, "id", "5")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestInvalidIssueRejected(t *testing.T) {
	setup(t, admin(), RoleBuild)
	rec, out := call(t, CreateItem, http.MethodPost, "/releases/items",
		`{"item_type":"bug fix","component":"ChenWeb","description":"x","issue":"ChenWeb-142"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(out["error_msg"].(string), "issue") {
		t.Fatalf("%d %v", rec.Code, out)
	}
}

func expectHistory(mock sqlmock.Sqlmock) {
	now := time.Now()
	mock.ExpectQuery(`FROM kb.releases r WHERE r.project = \$1`).WillReturnRows(sqlmock.NewRows(releaseCols).
		AddRow(2, ProjectChenWeb, 1, 0, 1, KindBuild, StatusReleased, "", "20261119063000", "2026-11-19", "", false, "", false, "", now, "", 0).
		AddRow(1, ProjectChenWeb, 1, 0, 0, KindMajor, StatusReleased, "First", "", "2026-11-19", "", false, "", false, "", now, "", 0))
	mock.ExpectQuery(`FROM kb.release_items i`).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "release_id", "item_type", "component", "description", "issue", "pull_request", "notes", "created_by", "created_at"}).
		AddRow(7, 2, "bug fix", "ChenWeb", "Fix one", "142", "", "", "", now))
}

func TestNotesText(t *testing.T) {
	mock := setup(t, admin(), "deploy")
	expectHistory(mock)
	rec, _ := call(t, GetNotes, http.MethodGet, "/releases/notes?from=01.00.00&to=01.00.01", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if body := rec.Body.String(); !strings.Contains(body, "Release 01.00.01") || !strings.Contains(body, "Fix one (#142)") {
		t.Fatalf("body = %s", body)
	}
}

func TestNotesPDF(t *testing.T) {
	mock := setup(t, admin(), "deploy")
	expectHistory(mock)
	old := renderer
	t.Cleanup(func() { renderer = old })
	dir := t.TempDir()
	renderer = func() *PDFRenderer {
		return &PDFRenderer{CacheDir: dir, Convert: func(_ context.Context, _, dst string) error {
			return os.WriteFile(dst, []byte("%PDF-1.7 test"), 0o644)
		}}
	}
	rec, _ := call(t, GetNotes, http.MethodGet, "/releases/notes?format=pdf&lang=zh-cn", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("%d %s %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
}

func TestListReleases(t *testing.T) {
	mock := setup(t, admin(), RoleBuild)
	mock.ExpectQuery(`SELECT major, minor, revision, build_number FROM kb.release_current`).
		WillReturnRows(sqlmock.NewRows([]string{"major", "minor", "revision", "build_number"}).AddRow(1, 0, 1, "20261119063000"))
	expectHistory(mock)
	mock.ExpectQuery(`release_id IS NULL ORDER BY i.id`).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "release_id", "item_type", "component", "description", "issue", "pull_request", "notes", "created_by", "created_at"}))
	rec, out := call(t, ListReleases, http.MethodGet, "/releases", "")
	if rec.Code != http.StatusOK || out["current"] != "01.00.01" || out["next_build"] != "01.00.02" || out["editable"] != true {
		t.Fatalf("%d %v", rec.Code, out)
	}
	if n := len(out["releases"].([]any)); n != 2 {
		t.Fatalf("releases = %d", n)
	}
}
