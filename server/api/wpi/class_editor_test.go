package wpi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestListActionClassesAllScope(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{"", `{"actions":["close-page","save"]}`},
		{"?scope=all", `{"actions":["close-page","load","query","save"]}`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			mock.ExpectQuery(latestAllQuery).WithArgs("action").WillReturnRows(descriptorRows(
				[2]string{"load", `{}`}, [2]string{"close-page", `{}`}, [2]string{"save", `{}`}, [2]string{"query", `{}`},
			))
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest("GET", "/api/v1/wpi/classes/action"+tc.query, nil), rec)
			if err := testHandler(db, plainUser).ListTriggerActions(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != tc.want {
				t.Fatalf("%d: %s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClassEditorMetadata(t *testing.T) {
	for _, kind := range []Kind{KindElement, KindAction, KindPageLayouter, KindElementLayouter} {
		t.Run(string(kind), func(t *testing.T) {
			var raw string
			switch kind {
			case KindElement:
				raw = `{"descriptor_id":"example","schema_version":"0.4","description":"Example","label":{"default_value":"Hello"}}`
			case KindAction:
				raw = `{"descriptor_id":"close-page","schema_version":"0.2"}`
			case KindPageLayouter:
				raw = `{"descriptor_id":"example","schema_version":"0.5","code_module_id":"page-layouter","root":{"pane_id":"main"}}`
			case KindElementLayouter:
				raw = `{"descriptor_id":"example","schema_version":"0.5","code_module_id":"flow-layouter","config":{}}`
			}
			body, _ := decodeBody([]byte(raw))
			body["attribute_definitions"] = map[string]any{"/label": map[string]any{
				"description": map[string]any{"en": "A label", "zh-cn": "标签"}, "scope": "instance",
				"value_editor": map[string]any{"id": "number", "options": map[string]any{"min": json.Number("1"), "integer": true}},
			}}
			var issues []Issue
			if kind == KindElement {
				issues = ValidateDraftClass(body)
			} else {
				issues = ValidateDescriptor(kind, str(body["descriptor_id"]), body)
			}
			if len(issues) > 0 {
				t.Fatalf("valid editor metadata rejected: %+v", issues)
			}
			if kind == KindAction || kind == KindElement {
				defaults, _ := classDefaults(kind, body)
				if _, ok := defaults["attribute_definitions"]; ok {
					t.Fatal("editor metadata leaked into runtime defaults")
				}
			}
		})
	}
}

func TestClassEditorInvalidMetadata(t *testing.T) {
	for _, metadata := range []string{
		`[]`, `{"label":{}}`, `{"/a~2b":{}}`, `{"/label":{"scope":"page"}}`,
		`{"/label":{"description":12}}`, `{"/label":{"value_editor":{"id":"missing"}}}`,
		`{"/label":{"value_editor":{"id":"number","options":{"integer":"yes"}}}}`,
		`{"/label":{"value_editor":{"id":"length","options":{"units":[1]}}}}`,
		`{"/label":{"value_editor":{"id":"date-picker","options":{"format":12}}}}`,
	} {
		t.Run(metadata, func(t *testing.T) {
			body, _ := decodeBody([]byte(fmt.Sprintf(`{"descriptor_id":"example","schema_version":"0.4","description":"Example","attribute_definitions":%s}`, metadata)))
			if issues := ValidateDraftClass(body); len(issues) == 0 {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
}

func TestElementScopeMatchesStoredAttribute(t *testing.T) {
	for _, tc := range []struct {
		name, attribute, pointer, scope string
		valid                           bool
	}{
		{"identity is class", ``, "/descriptor_id", "class", true},
		{"identity cannot be instance", ``, "/descriptor_id", "instance", false},
		{"code module cannot be instance", ``, "/code_module_id", "instance", false},
		{"ordinary instance", `,"label":{"default_value":"Hi"}`, "/label", "instance", true},
		{"fixed class", `,"label":{"default_value":"Hi","instance_modifiable":false}`, "/label", "class", true},
		{"class must be fixed", `,"label":{"default_value":"Hi"}`, "/label", "class", false},
		{"instance cannot be fixed", `,"label":{"default_value":"Hi","instance_modifiable":false}`, "/label", "instance", false},
		{"class needs a non-null default", `,"label":{"default_value":null,"instance_modifiable":false}`, "/label", "class", false},
		{"nested classification", `,"label":{"default_value":{"en":"Hi"}}`, "/label/en", "class", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := decode(t, fmt.Sprintf(`{"descriptor_id":"example","schema_version":"0.4","description":"Example"%s,"attribute_definitions":{%q:{"scope":%q}}}`, tc.attribute, tc.pointer, tc.scope))
			issues := ValidateDraftClass(body)
			if (len(issues) == 0) != tc.valid {
				t.Fatalf("valid=%v, issues=%+v", tc.valid, issues)
			}
		})
	}
}

func TestGenericClassMissingAndInvalidUpdates(t *testing.T) {
	for _, tc := range []struct {
		name, current, body string
		disappear           bool
		status              int
	}{
		{"missing", "", `{"descriptor_id":"close-page","schema_version":"0.2"}`, false, 404},
		{"invalid", `{}`, `{"descriptor_id":"close-page","schema_version":"0.2","unexpected":true}`, false, 400},
		{"disappeared", `{}`, `{"descriptor_id":"close-page","schema_version":"0.2"}`, true, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			rows := sqlmock.NewRows([]string{"version", "status", "body"})
			if tc.current != "" {
				rows.AddRow(2, "active", []byte(tc.current))
			}
			mock.ExpectQuery(`SELECT version, status, body FROM kb\.wpi_descriptors`).WithArgs("action", "close-page").WillReturnRows(rows)
			if tc.disappear {
				mock.ExpectQuery(`UPDATE kb\.wpi_descriptors`).WithArgs("action", "close-page", ActionSchemaVersion, tc.body, adminUser.Email, 2, "active").WillReturnRows(sqlmock.NewRows([]string{"version"}))
			}
			rec := call(t, testHandler(db, adminUser).UpdateClass, "PUT", tc.body, "kind", "action", "classId", "close-page")
			if rec.Code != tc.status {
				t.Fatalf("%d: %s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClassEditorMetadataCopies(t *testing.T) {
	page := withCopies(t, testClasses(), "p", decode(t, pageWith(`{"element":"text","text":"Hi"}`)))
	metadata := decode(t, `{"/description":{"scope":"class","value_editor":{"id":"localized-text"}}}`)
	for _, site := range copySites(page) {
		row, ok := site.holder[copyKey].(map[string]any)
		if ok {
			row["attribute_definitions"] = metadata
		}
	}
	if issues := ValidateDescriptor(KindPage, "p", page); len(issues) > 0 {
		t.Fatalf("metadata in frozen class copies rejected: %+v", issues)
	}
}

func TestClassSummaryKeepsMetadataSeparate(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(elementClassesQuery).WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "status", "body"}).AddRow("text", 1, "active", []byte(`{"descriptor_id":"text","schema_version":"0.4","code_module_id":"text","text":{"default_value":"Hi"},"attribute_definitions":{"/text":{"scope":"instance"}}}`)))
	classes, err := (store{db}).elementClasses(context.Background())
	if err != nil || len(classes) != 1 {
		t.Fatalf("%+v %v", classes, err)
	}
	if _, exists := classes[0].Attributes["attribute_definitions"]; exists {
		t.Fatal("editor metadata appeared as an instance attribute")
	}
	if classes[0].AttributeDefinitions["/text"] == nil {
		t.Fatal("summary dropped editor metadata")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetGenericClass(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(`SELECT version, status, body FROM kb\.wpi_descriptors`).WithArgs("action", "close-page").
		WillReturnRows(sqlmock.NewRows([]string{"version", "status", "body"}).AddRow(3, "active", []byte(`{"descriptor_id":"close-page","schema_version":"0.2"}`)))
	rec := call(t, testHandler(db, plainUser).GetClass, "GET", "", "kind", "action", "classId", "close-page")
	if rec.Code != 200 {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	var result ClassRow
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result.Version != 3 || result.Status != "active" {
		t.Fatalf("%+v: %v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateGenericClassPreservesRecord(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	raw := `{"descriptor_id":"close-page","schema_version":"0.2","description":"Close"}`
	mock.ExpectQuery(`SELECT version, status, body FROM kb\.wpi_descriptors`).WithArgs("action", "close-page").
		WillReturnRows(sqlmock.NewRows([]string{"version", "status", "body"}).AddRow(3, "active", []byte(raw)))
	mock.ExpectQuery(`UPDATE kb\.wpi_descriptors`).WithArgs("action", "close-page", ActionSchemaVersion, raw, adminUser.Email, 3, "active").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))
	rec := call(t, testHandler(db, adminUser).UpdateClass, "PUT", raw, "kind", "action", "classId", "close-page")
	if rec.Code != 200 {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGenericClassAuthorizationAndKinds(t *testing.T) {
	for _, tc := range []struct {
		kind   string
		method string
		user   bool
		status int
	}{
		{"action", "GET", false, 401}, {"action", "PUT", false, 401}, {"action", "PUT", true, 403},
		{"page", "GET", true, 404}, {"app", "GET", true, 404},
	} {
		h := testHandler(nil, nil)
		if tc.user {
			h = testHandler(nil, plainUser)
		}
		fn := h.GetClass
		if tc.method == "PUT" {
			fn = h.UpdateClass
		}
		rec := call(t, fn, tc.method, `{}`, "kind", tc.kind, "classId", "example")
		if rec.Code != tc.status {
			t.Fatalf("%+v: %d %s", tc, rec.Code, rec.Body.String())
		}
	}
}
