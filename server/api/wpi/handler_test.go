package wpi

import (
	"context"
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

const singlePaneApp = `{"descriptor_id": "hello", "schema_version": "0.5", "initial_page": "greeting"}`

// greetingRaw is a page as an author writes it, without class copies.
var greetingRaw = panelPage("greeting", ``, `{"element": "text", "text": "Hello"}`)

// expectClassCopies expects the loads of the classes a page write copies into
// greetingRaw, by kind in sorted order.
func expectClassCopies(mock sqlmock.Sqlmock) {
	for _, kind := range []Kind{KindElement, KindElementLayouter, KindPageLayouter} {
		var rows [][2]string
		for id, body := range testClasses()[kind] {
			rows = append(rows, [2]string{id, body})
		}
		mock.ExpectQuery(latestQuery).WithArgs(string(kind), sqlmock.AnyArg()).WillReturnRows(descriptorRows(rows...))
	}
}

// expectAllModules expects the bundle's checks of code modules, by module
// type: element, element-layouter, page-layouter.
func expectAllModules(mock sqlmock.Sqlmock, elementModules ...string) {
	expectModules(mock, elementModules...)
	mock.ExpectQuery(typedModulesQuery("element-layouter")).WillReturnRows(sqlmock.NewRows([]string{"code_module_id"}).AddRow("flow-layouter"))
	mock.ExpectQuery(typedModulesQuery("page-layouter")).WillReturnRows(sqlmock.NewRows([]string{"code_module_id"}).AddRow("page-layouter"))
}

// expectModules expects the check of code modules against kb.code_modules,
// which finds the active element modules given.
func expectModules(mock sqlmock.Sqlmock, active ...string) {
	rows := sqlmock.NewRows([]string{"code_module_id"})
	for _, m := range active {
		rows.AddRow(m)
	}
	mock.ExpectQuery(activeModulesQuery).WillReturnRows(rows)
}

func TestGetApp(t *testing.T) {
	t.Run("client canceled request", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest("GET", "/api/v1/wpi/apps/hello", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("appId")
		c.SetParamValues("hello")
		if err := testHandler(db, plainUser).GetApp(c); err != nil {
			t.Fatal(err)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("canceled request must not write an error response: %s", rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("valid", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(latestQuery).WithArgs("app", sqlmock.AnyArg()).WillReturnRows(descriptorRows([2]string{"hello", singlePaneApp}))
		mock.ExpectQuery(latestPagesQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(descriptorRows([2]string{"greeting", filledPage(greetingRaw)}))
		expectAllModules(mock, "text")
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
	t.Run("cancellation error on an active request is still a failure", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(latestQuery).WillReturnError(context.Canceled)
		rec := call(t, testHandler(db, plainUser).GetApp, "GET", "", "appId", "hello")
		if rec.Code != 500 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPutDescriptor(t *testing.T) {
	t.Run("stored", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		expectClassCopies(mock)
		mock.ExpectQuery(`INSERT INTO kb\.frontend_pages`).WithArgs("greeting", SchemaVersion, filledPage(greetingRaw), "admin@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		rec := call(t, testHandler(db, adminUser).PutDescriptor, "PUT", greetingRaw, "kind", "page", "descriptorId", "greeting")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"version":1`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		// The response returns the stored page, with its class copies.
		var resp struct{ Page map[string]any }
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if _, ok := resp.Page["page_layouter"].(map[string]any)[copyKey]; !ok {
			t.Fatalf("response page without class copies: %s", rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("page instances checked against their classes", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		expectClassCopies(mock)
		page := strings.Replace(greetingRaw, `"text": "Hello"`, `"text": "Hello", "data_binding": "$input_doc.x"`, 1)
		rec := call(t, testHandler(db, adminUser).PutDescriptor, "PUT", page, "kind", "page", "descriptorId", "greeting")
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "exactly one of") {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal("nothing may be stored: ", err)
		}
	})
	t.Run("element class with an unsupported attribute", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		body := `{"descriptor_id": "fancy-button", "schema_version": "0.4", "code_module_id": "button", "label": {"default_value": "Go!", "instance_modifiable": false}, "on_click": {"default_value": null}, "glow": {}}`
		expectModules(mock, "button")
		mock.ExpectQuery(`INSERT INTO kb\.wpi_descriptors`).WithArgs("element", "fancy-button", ElementSchemaVersion, body, "admin@example.com").
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		rec := call(t, testHandler(db, adminUser).PutDescriptor, "PUT", body, "kind", "element", "descriptorId", "fancy-button")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `code module \"button\" does not support attribute \"glow\"`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("element class whose code module is suspended or missing", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		expectModules(mock)
		body := `{"descriptor_id": "fancy-button", "schema_version": "0.4", "code_module_id": "button", "label": {"default_value": null}, "on_click": {"default_value": null}}`
		rec := call(t, testHandler(db, adminUser).PutDescriptor, "PUT", body, "kind", "element", "descriptorId", "fancy-button")
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `code module \"button\" is not an active element module in kb.code_modules`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal("nothing may be stored: ", err)
		}
	})
	rejected := []struct {
		name, kind, id, body string
		user                 *ApiTypes.UserInfo
		status               int
		want                 string
	}{
		{"element class without code", "element", "slider", `{"descriptor_id": "slider", "schema_version": "0.4", "code_module_id": "slider"}`, adminUser, 400, `code module \"slider\" has no code in this interpreter`},
		{"element class without code module", "element", "button", `{"descriptor_id": "button", "schema_version": "0.4", "label": {"default_value": null}, "on_click": {"default_value": null}}`, adminUser, 400, `missing attribute \"code_module_id\"`},
		{"ID mismatch", "page", "other", greetingRaw, adminUser, 400, `does not match`},
		{"unknown kind", "widget", "greeting", greetingRaw, adminUser, 400, "Unknown descriptor kind"},
		{"not JSON", "page", "greeting", `[1, 2]`, adminUser, 400, "JSON object"},
		{"not an admin", "page", "greeting", greetingRaw, plainUser, 403, "Admin access required"},
		{"not logged in", "page", "greeting", greetingRaw, nil, 401, "Authentication required"},
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

const sliderDraft = `{"descriptor_id": "slider", "schema_version": "0.4", "description": {"en": "A slider"}, "implement_notes": "A range input", "label": {"help_tip": "Caption", "default_value": null}, "min": {"default_value": 0}}`

func TestCreateElementClass(t *testing.T) {
	t.Run("stored as a draft", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectExec(insertDraftQuery).WithArgs("slider", ElementSchemaVersion, sliderDraft, "admin@example.com").WillReturnResult(sqlmock.NewResult(1, 1))
		rec := call(t, testHandler(db, adminUser).CreateElementClass, "POST", sliderDraft)
		if rec.Code != 201 || !strings.Contains(rec.Body.String(), `"status":"draft"`) || !strings.Contains(rec.Body.String(), `"version":1`) ||
			!strings.Contains(rec.Body.String(), `"warnings":[]`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("draft of an existing code module", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		body := `{"descriptor_id": "fancy-button", "schema_version": "0.4", "code_module_id": "button", "description": "A fancy button", "glow": {}}`
		expectModules(mock, "button")
		mock.ExpectExec(insertDraftQuery).WithArgs("fancy-button", ElementSchemaVersion, body, "admin@example.com").WillReturnResult(sqlmock.NewResult(1, 1))
		rec := call(t, testHandler(db, adminUser).CreateElementClass, "POST", body)
		if rec.Code != 201 || !strings.Contains(rec.Body.String(), `code module \"button\" does not support attribute \"glow\"`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})
	t.Run("draft naming a module whose code is not written", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		body := `{"descriptor_id": "dial", "schema_version": "0.4", "code_module_id": "slider", "description": "A dial"}`
		expectModules(mock, "slider")
		mock.ExpectExec(insertDraftQuery).WithArgs("dial", ElementSchemaVersion, body, "admin@example.com").WillReturnResult(sqlmock.NewResult(1, 1))
		if rec := call(t, testHandler(db, adminUser).CreateElementClass, "POST", body); rec.Code != 201 || !strings.Contains(rec.Body.String(), `"warnings":[]`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})
	t.Run("draft naming a module not in kb.code_modules", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		expectModules(mock)
		body := `{"descriptor_id": "dial", "schema_version": "0.4", "code_module_id": "slider", "description": "A dial"}`
		rec := call(t, testHandler(db, adminUser).CreateElementClass, "POST", body)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `code module \"slider\" is not an active element module`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal("nothing may be stored: ", err)
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
		{"no description", `{"descriptor_id": "slider", "schema_version": "0.4"}`, adminUser, 400, `a draft class needs a description`},
		{"action default", `{"descriptor_id": "slider", "schema_version": "0.4", "description": "x", "on_click": {"default_value": {"action": "close-page"}}}`, adminUser, 400, `holds actions`},
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

func TestGetElementClass(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(elementClassQuery).WithArgs("slider").
		WillReturnRows(sqlmock.NewRows([]string{"version", "status", "body"}).AddRow(1, "draft", []byte(`{"descriptor_id": "slider"}`)))
	rec := call(t, testHandler(db, plainUser).GetElementClass, "GET", "", "classId", "slider")
	if want := `{"version":1,"status":"draft","class":{"descriptor_id":"slider"}}`; rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	mock.ExpectQuery(elementClassQuery).WithArgs("missing").WillReturnRows(sqlmock.NewRows([]string{"version", "status", "body"}))
	if rec := call(t, testHandler(db, plainUser).GetElementClass, "GET", "", "classId", "missing"); rec.Code != 404 {
		t.Fatalf("missing class: status %d", rec.Code)
	}
	if rec := call(t, testHandler(db, plainUser).GetElementClass, "GET", "", "classId", "Bad_ID"); rec.Code != 404 {
		t.Fatalf("bad ID: status %d", rec.Code)
	}
	if rec := call(t, testHandler(db, nil).GetElementClass, "GET", "", "classId", "slider"); rec.Code != 401 {
		t.Fatalf("not logged in: status %d", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateElementClass(t *testing.T) {
	classRow := func(version int, status string) *sqlmock.Rows {
		return sqlmock.NewRows([]string{"version", "status", "body"}).AddRow(version, status, []byte(`{}`))
	}
	t.Run("a draft stays a draft", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(elementClassQuery).WithArgs("slider").WillReturnRows(classRow(1, "draft"))
		mock.ExpectQuery(updateClassQuery).WithArgs("slider", ElementSchemaVersion, sliderDraft, "admin@example.com", 1, "draft").
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
		rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", sliderDraft, "classId", "slider")
		if want := `{"descriptor_id":"slider","status":"draft","version":1,"warnings":[]}`; rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("an active class stays active", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		body := testClasses()[KindElement]["input"]
		mock.ExpectQuery(elementClassQuery).WithArgs("input").WillReturnRows(classRow(2, "active"))
		expectModules(mock, "input")
		mock.ExpectQuery(updateClassQuery).WithArgs("input", ElementSchemaVersion, body, "admin@example.com", 2, "active").
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
		rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", body, "classId", "input")
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"active"`) || !strings.Contains(rec.Body.String(), `"version":2`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("attributes the code module lacks are warnings", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		body := `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button","label":{"default_value":null},"on_click":{"default_value":null},"label_hori_align":{"default_value":"center"}}`
		mock.ExpectQuery(elementClassQuery).WithArgs("button").WillReturnRows(classRow(3, "active"))
		expectModules(mock, "button")
		mock.ExpectQuery(updateClassQuery).WithArgs("button", ElementSchemaVersion, body, "admin@example.com", 3, "active").
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))
		rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", body, "classId", "button")
		var res struct{ Warnings []Issue }
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		if rec.Code != 200 || len(res.Warnings) != 1 || res.Warnings[0].Path != "/label_hori_align" {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})
	for _, bad := range []struct{ name, module, want string }{
		{"active class without a code module", ``, `/code_module_id`},
		{"active class with an unregistered code module", `,"code_module_id":"slider"`, `code module \"slider\" has no code`},
	} {
		t.Run(bad.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			mock.ExpectQuery(elementClassQuery).WithArgs("button").WillReturnRows(classRow(3, "active"))
			body := `{"descriptor_id":"button","schema_version":"0.4"` + bad.module + `,"label":{"default_value":null}}`
			rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", body, "classId", "button")
			if rec.Code != 400 || !strings.Contains(rec.Body.String(), bad.want) {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal("nothing may be stored: ", err)
			}
		})
	}
	for _, edit := range []struct{ name, body string }{
		{"add alignment attributes", `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button","label":{"default_value":null},"on_click":{"default_value":null},"label_horizontal_align":{"default_value":"center","help_tip":{"en":"Horizontal alignment"}},"label_vertical_align":{"default_value":"center"}}`},
		{"delete code-required attributes", `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button"}`},
		{"rename an attribute", `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button","caption":{"default_value":"Go"},"on_click":{"default_value":null}}`},
		{"modify defaults and help tips", `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button","label":{"default_value":{"en":"Run","zh-cn":"运行"},"help_tip":{"en":"Button caption","zh-cn":"按钮标题"}},"on_click":{},"width":{"default_value":"300px"}}`},
	} {
		t.Run(edit.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			mock.ExpectQuery(elementClassQuery).WithArgs("button").WillReturnRows(classRow(2, "active"))
			expectModules(mock, "button")
			mock.ExpectQuery(updateClassQuery).WithArgs("button", ElementSchemaVersion, edit.body, "admin@example.com", 2, "active").
				WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
			rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", edit.body, "classId", "button")
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"active"`) || !strings.Contains(rec.Body.String(), `"version":2`) {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, invalid := range []struct{ attr, want string }{
		{`"bad-name":{}`, `cannot be an attribute name`},
		{`"element":{}`, `cannot be an attribute name`},
		{`"custom":42`, `must be an object`},
		{`"custom":{"value":42}`, `unknown attribute`},
		{`"custom":{"help_tip":{"zh-cn":"提示"}}`, `need an`},
		{`"on_click":{"default_value":{"action":"close-page"}}`, `holds actions`},
		{`"width":{"default_value":"wide"}`, `must match`},
	} {
		t.Run("reject malformed edit "+invalid.attr, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			body := `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button",` + invalid.attr + `}`
			mock.ExpectQuery(elementClassQuery).WithArgs("button").WillReturnRows(classRow(2, "active"))
			rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", body, "classId", "button")
			if rec.Code != 400 || !strings.Contains(rec.Body.String(), invalid.want) {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal("nothing may be stored: ", err)
			}
		})
	}
	t.Run("a draft is checked as a draft", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(elementClassQuery).WithArgs("slider").WillReturnRows(classRow(1, "draft"))
		rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", `{"descriptor_id": "slider", "schema_version": "0.4"}`, "classId", "slider")
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), `a draft class needs a description`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal("nothing may be stored: ", err)
		}
	})
	t.Run("missing class", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(elementClassQuery).WithArgs("slider").WillReturnRows(sqlmock.NewRows([]string{"version", "status", "body"}))
		if rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", sliderDraft, "classId", "slider"); rec.Code != 404 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})
	t.Run("class removed before update", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(elementClassQuery).WithArgs("slider").WillReturnRows(classRow(1, "draft"))
		mock.ExpectQuery(updateClassQuery).WithArgs("slider", ElementSchemaVersion, sliderDraft, "admin@example.com", 1, "draft").
			WillReturnRows(sqlmock.NewRows([]string{"version"}))
		rec := call(t, testHandler(db, adminUser).UpdateElementClass, "PUT", sliderDraft, "classId", "slider")
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), "Element class not found") {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	rejected := []struct {
		name, id, body string
		user           *ApiTypes.UserInfo
		status         int
		want           string
	}{
		{"renamed", "dial", sliderDraft, adminUser, 400, `descriptor_id \"slider\" does not match \"dial\"`},
		{"not JSON", "slider", `[1]`, adminUser, 400, "JSON object"},
		{"not an admin", "slider", sliderDraft, plainUser, 403, "Admin access required"},
		{"not logged in", "slider", sliderDraft, nil, 401, "Authentication required"},
	}
	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			rec := call(t, testHandler(db, c.user).UpdateElementClass, "PUT", c.body, "classId", c.id)
			if rec.Code != c.status || !strings.Contains(rec.Body.String(), c.want) {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal("nothing may be read or stored: ", err)
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

func TestListCodeModules(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(listCodeModulesQuery).WillReturnRows(sqlmock.NewRows([]string{"code_module_id", "module_name", "description", "module_type", "module_doc", "notes", "status"}).
		AddRow("button", "Button", "A button", "element", "button.md", "", "active"))
	rec := call(t, testHandler(db, plainUser).ListCodeModules, "GET", "")
	want := `{"code_modules":[{"code_module_id":"button","module_name":"Button","description":"A button","module_type":"element","module_doc":"button.md","notes":"","status":"active"}]}`
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec := call(t, testHandler(db, nil).ListCodeModules, "GET", ""); rec.Code != 401 {
		t.Fatalf("not logged in: status %d", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListPageLayouters(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(latestAllQuery).WithArgs("page-layouter").WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "body"}).
		AddRow("single-pane", 2, []byte(`{"descriptor_id":"single-pane","root":{"pane_id":"main"}}`)))
	rec := call(t, testHandler(db, plainUser).ListPageLayouters, "GET", "")
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"classes":[{"descriptor_id":"single-pane","root":{"pane_id":"main"}}]}` {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec := call(t, testHandler(db, nil).ListPageLayouters, "GET", ""); rec.Code != 401 {
		t.Fatalf("not logged in: %d", rec.Code)
	}
}
