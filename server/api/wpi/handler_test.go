package wpi

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

var (
	plainUser = &ApiTypes.UserInfo{Email: "user@example.com"}
	adminUser = &ApiTypes.UserInfo{Email: "admin@example.com", Roles: []string{"admin"}}
)

func testHandler(db *sql.DB, user *ApiTypes.UserInfo) *Handler {
	return &Handler{DB: db, User: func(echo.Context) *ApiTypes.UserInfo { return user }}
}

// call runs a handler method with path params and returns the recorder.
func call(t *testing.T, fn echo.HandlerFunc, method, body string, params ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	var names, values []string
	for i := 0; i+1 < len(params); i += 2 {
		names, values = append(names, params[i]), append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	if err := fn(c); err != nil {
		t.Fatal(err)
	}
	return rec
}

func descriptorRows(rows ...[2]string) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"descriptor_id", "version", "body"})
	for _, row := range rows {
		r.AddRow(row[0], 1, []byte(row[1]))
	}
	return r
}

const singlePaneApp = `{"descriptor_id": "hello", "schema_version": "0.2", "layout_template": "single-pane", "panes": {"main": {"page": "greeting"}}}`
const greetingPage = `{"descriptor_id": "greeting", "schema_version": "0.2", "layouter": "flow-layouter", "elements": [{"element": "text", "text": "Hello"}]}`

// expectClasses expects the bundle's load of all element and action classes.
func expectClasses(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(latestAllQuery).WithArgs("element").WillReturnRows(descriptorRows([2]string{"text", testClasses()[KindElement]["text"]}))
	mock.ExpectQuery(latestAllQuery).WithArgs("action").WillReturnRows(descriptorRows())
}

func TestGetApp(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(latestQuery).WithArgs("app", sqlmock.AnyArg()).WillReturnRows(descriptorRows([2]string{"hello", singlePaneApp}))
		expectClasses(mock)
		mock.ExpectQuery(latestPagesQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(descriptorRows([2]string{"greeting", greetingPage}))
		rec := call(t, testHandler(db, plainUser).GetApp, "GET", "", "appId", "hello")
		if rec.Code != 200 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		var b Bundle
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		if _, ok := b.Pages["greeting"]; !ok || b.Warnings == nil {
			t.Fatalf("bundle %s", rec.Body)
		}
	})
	t.Run("unknown app", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(latestQuery).WillReturnRows(descriptorRows())
		if rec := call(t, testHandler(db, plainUser).GetApp, "GET", "", "appId", "nope"); rec.Code != 404 {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("invalid app", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(latestQuery).WithArgs("app", sqlmock.AnyArg()).WillReturnRows(descriptorRows([2]string{"hello", singlePaneApp}))
		expectClasses(mock)
		mock.ExpectQuery(latestPagesQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(descriptorRows())
		rec := call(t, testHandler(db, plainUser).GetApp, "GET", "", "appId", "hello")
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), `page \"greeting\" does not exist`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})
	t.Run("not logged in", func(t *testing.T) {
		if rec := call(t, testHandler(nil, nil).GetApp, "GET", "", "appId", "hello"); rec.Code != 401 {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("database error", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(latestQuery).WillReturnError(sql.ErrConnDone)
		rec := call(t, testHandler(db, plainUser).GetApp, "GET", "", "appId", "hello")
		if rec.Code != 500 || strings.Contains(rec.Body.String(), "connection") {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})
}

func TestPutDescriptor(t *testing.T) {
	t.Run("stored", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(`INSERT INTO kb\.frontend_pages`).WithArgs("greeting", SchemaVersion, greetingPage, "admin@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		rec := call(t, testHandler(db, adminUser).PutDescriptor, "PUT", greetingPage, "kind", "page", "descriptorId", "greeting")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"version":1`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	rejected := []struct {
		name, kind, id, body string
		user                 *ApiTypes.UserInfo
		status               int
		want                 string
	}{
		{"element class without code", "element", "slider", `{"descriptor_id": "slider", "schema_version": "0.3"}`, adminUser, 400, `element class \"slider\" has no code in this interpreter`},
		{"ID mismatch", "page", "other", greetingPage, adminUser, 400, `does not match`},
		{"unknown kind", "widget", "greeting", greetingPage, adminUser, 400, "Unknown descriptor kind"},
		{"not JSON", "page", "greeting", `[1, 2]`, adminUser, 400, "JSON object"},
		{"not an admin", "page", "greeting", greetingPage, plainUser, 403, "Admin access required"},
		{"not logged in", "page", "greeting", greetingPage, nil, 401, "Authentication required"},
	}
	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			rec := call(t, testHandler(db, c.user).PutDescriptor, "PUT", c.body, "kind", c.kind, "descriptorId", c.id)
			if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.want) {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal("nothing may be stored: ", err)
			}
		})
	}
}

const sliderDraft = `{"descriptor_id": "slider", "schema_version": "0.3", "description": {"en": "A slider"}, "implement_notes": "A range input", "label": {"help_tip": "Caption", "default_value": null}, "min": {"default_value": 0}}`

func TestCreateElementClass(t *testing.T) {
	t.Run("stored as a draft", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectExec(insertDraftQuery).WithArgs("slider", ElementSchemaVersion, sliderDraft, "admin@example.com").WillReturnResult(sqlmock.NewResult(1, 1))
		rec := call(t, testHandler(db, adminUser).CreateElementClass, "POST", sliderDraft)
		if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"status":"draft"`) || !strings.Contains(rec.Body.String(), `"version":1`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("already exists", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectExec(insertDraftQuery).WillReturnResult(sqlmock.NewResult(0, 0))
		rec := call(t, testHandler(db, adminUser).CreateElementClass, "POST", strings.Replace(sliderDraft, "slider", "input", 1))
		if rec.Code != 409 || !strings.Contains(rec.Body.String(), `element class \"input\" already exists`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})
	rejected := []struct {
		name, body string
		user       *ApiTypes.UserInfo
		status     int
		want       string
	}{
		{"no description", `{"descriptor_id": "slider", "schema_version": "0.3"}`, adminUser, 400, `a draft class needs a description`},
		{"action default", `{"descriptor_id": "slider", "schema_version": "0.3", "description": "x", "on_click": {"default_value": {"action": "close-page"}}}`, adminUser, 400, `holds actions`},
		{"not JSON", `[1]`, adminUser, 400, "JSON object"},
		{"not an admin", sliderDraft, plainUser, 403, "Admin access required"},
		{"not logged in", sliderDraft, nil, 401, "Authentication required"},
	}
	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			rec := call(t, testHandler(db, c.user).CreateElementClass, "POST", c.body)
			if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.want) {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal("nothing may be stored: ", err)
			}
		})
	}
}

func TestListElementClasses(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(elementClassesQuery).WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "status", "body"}).
		AddRow("input", 1, "active", []byte(`{"descriptor_id": "input", "description": "A field", "input_type": {"help_tip": "Kind", "default_value": "text"}}`)).
		AddRow("slider", 1, "draft", []byte(`{"descriptor_id": "slider", "description": {"en": "A slider"}}`)))
	rec := call(t, testHandler(db, plainUser).ListElementClasses, "GET", "")
	want := `{"classes":[{"descriptor_id":"input","version":1,"status":"active","description":"A field","attributes":{"input_type":{"default_value":"text","help_tip":"Kind"}}},{"descriptor_id":"slider","version":1,"status":"draft","description":{"en":"A slider"},"attributes":{}}]}`
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec := call(t, testHandler(db, nil).ListElementClasses, "GET", ""); rec.Code != 401 {
		t.Fatalf("not logged in: status %d", rec.Code)
	}
}
