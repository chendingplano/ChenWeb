package wpi

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

const (
	loadCustomerJSON = `{"descriptor_id": "load-customer", "schema_version": "0.1", "action_type": "load",
	  "table": "kb.wpi_demo_customers", "result": "record", "where": {"id": "$args.id"},
	  "fields": [{"column": "id", "attribute": "id"}, {"column": "email", "attribute": "email"}]}`
	loadCustomersJSON = `{"descriptor_id": "load-customers", "schema_version": "0.1", "action_type": "load",
	  "table": "kb.wpi_demo_customers", "result": "list", "limit": 5000, "order_by": [{"column": "email"}],
	  "fields": [{"column": "id", "attribute": "id"}, {"column": "email", "attribute": "email"}]}`
	saveCustomerJSON = `{"descriptor_id": "save-customer", "schema_version": "0.1", "action_type": "save",
	  "table": "kb.wpi_demo_customers", "mode": "upsert", "key": "id",
	  "fields": [{"attribute": "user_name", "column": "user_name", "required": true},
	             {"attribute": "email", "column": "email", "required": true, "rules": {"format": "email"}}]}`
	openCustomerJSON = `{"descriptor_id": "open-customer", "schema_version": "0.1", "action_type": "open_page", "page": "customer-form", "pane": "right"}`
	formWithInlineSave = `{"descriptor_id": "customer-form", "schema_version": "0.1", "layouter": "flow-layouter", "elements": [
	  {"descriptor_id": "t", "element_type": "text", "text": "x"},
	  {"descriptor_id": "save", "element_type": "button", "label": "Save", "on_click":
	    {"action_type": "save", "table": "kb.wpi_demo_customers", "mode": "insert", "key": "id",
	     "fields": [{"attribute": "email", "column": "email"}]}}]}`
)

const catalogQuery = `SELECT column_name FROM information_schema.columns`

func stored(mock sqlmock.Sqlmock, kind Kind, id, body string) {
	mock.ExpectQuery(latestQuery).WithArgs(string(kind), sqlmock.AnyArg()).WillReturnRows(descriptorRows([2]string{id, body}))
}

func customerColumns(mock sqlmock.Sqlmock) {
	rows := sqlmock.NewRows([]string{"column_name"})
	for _, c := range []string{"id", "user_name", "phone_number", "email"} {
		rows.AddRow(c)
	}
	mock.ExpectQuery(catalogQuery).WithArgs("kb", "wpi_demo_customers").WillReturnRows(rows)
}

func runAction(t *testing.T, mock sqlmock.Sqlmock, h *Handler, body string, status int, want string) {
	t.Helper()
	rec := call(t, h.RunAction, "POST", body)
	if rec.Code != status || !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("status %d (want %d): %s", rec.Code, status, rec.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunLoadRecord(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindAction, "load-customer", loadCustomerJSON)
	customerColumns(mock)
	mock.ExpectQuery(`SELECT "id", "email" FROM "kb"\."wpi_demo_customers" WHERE "id" = \$1 LIMIT 1`).WithArgs("7").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(int64(7), []byte("g@example.com")))
	// "table" outside ref is ignored: only the stored descriptor picks the table.
	runAction(t, mock, testHandler(db, plainUser), `{"ref": {"kind": "action", "id": "load-customer"}, "args": {"id": 7}, "table": "kb.users"}`,
		200, `{"result":{"email":"g@example.com","id":7}}`)
}

func TestRunLoadRecordNotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindAction, "load-customer", loadCustomerJSON)
	customerColumns(mock)
	mock.ExpectQuery(`SELECT`).WithArgs("99").WillReturnRows(sqlmock.NewRows([]string{"id", "email"}))
	runAction(t, mock, testHandler(db, plainUser), `{"ref": {"kind": "action", "id": "load-customer"}, "args": {"id": 99}}`, 404, "Record not found")
}

func TestRunLoadListCapsLimit(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindAction, "load-customers", loadCustomersJSON)
	customerColumns(mock)
	mock.ExpectQuery(`ORDER BY "email" ASC LIMIT \$1`).WithArgs(maxListLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(int64(1), "a@example.com").AddRow(int64(2), "b@example.com"))
	runAction(t, mock, testHandler(db, plainUser), `{"ref": {"kind": "action", "id": "load-customers"}}`, 200, `"id":2`)
}

func TestRunSaveUpsert(t *testing.T) {
	t.Run("inserts without key", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		stored(mock, KindAction, "save-customer", saveCustomerJSON)
		customerColumns(mock)
		mock.ExpectQuery(`INSERT INTO "kb"\."wpi_demo_customers" \("user_name", "email"\) VALUES \(\$1, \$2\) RETURNING "id"`).
			WithArgs("dana", "d@example.com").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(4)))
		runAction(t, mock, testHandler(db, plainUser), `{"ref": {"kind": "action", "id": "save-customer"}, "input_doc": {"user_name": "dana", "email": "d@example.com"}}`,
			200, `{"key_attribute":"id","key_value":4}`)
	})
	t.Run("updates with key, no version check", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		stored(mock, KindAction, "save-customer", saveCustomerJSON)
		customerColumns(mock)
		mock.ExpectQuery(`UPDATE "kb"\."wpi_demo_customers" SET "user_name" = \$1, "email" = \$2 WHERE "id" = \$3 RETURNING "id"`).
			WithArgs("dana", "d@example.com", "7").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
		runAction(t, mock, testHandler(db, plainUser), `{"ref": {"kind": "action", "id": "save-customer"}, "input_doc": {"id": 7, "user_name": "dana", "email": "d@example.com"}}`,
			200, `"key_value":7`)
	})
	t.Run("update of a missing row", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		stored(mock, KindAction, "save-customer", saveCustomerJSON)
		customerColumns(mock)
		mock.ExpectQuery(`UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		runAction(t, mock, testHandler(db, plainUser), `{"ref": {"kind": "action", "id": "save-customer"}, "input_doc": {"id": 99, "user_name": "dana", "email": "d@example.com"}}`,
			404, "Record not found")
	})
}

func TestRunSaveFieldErrorsWriteNothing(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindAction, "save-customer", saveCustomerJSON)
	customerColumns(mock)
	runAction(t, mock, testHandler(db, plainUser), `{"ref": {"kind": "action", "id": "save-customer"}, "input_doc": {"email": "nope"}}`,
		422, `{"field_errors":{"email":["format"],"user_name":["required"]}}`)
}

func TestRunInlineActionByPointer(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "customer-form", formWithInlineSave)
	customerColumns(mock)
	mock.ExpectQuery(`INSERT INTO "kb"\."wpi_demo_customers" \("email"\)`).WithArgs("e@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	runAction(t, mock, testHandler(db, plainUser),
		`{"ref": {"kind": "page", "id": "customer-form", "pointer": "/elements/1/on_click"}, "input_doc": {"email": "e@example.com"}}`, 200, `"key_value":5`)
}

func TestRunDatabaseRejectsValue(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindAction, "save-customer", saveCustomerJSON)
	customerColumns(mock)
	mock.ExpectQuery(`INSERT`).WillReturnError(&pq.Error{Code: "23502", Message: "null value in column"})
	runAction(t, mock, testHandler(db, plainUser), `{"ref": {"kind": "action", "id": "save-customer"}, "input_doc": {"user_name": "dana", "email": "d@example.com"}}`,
		400, "The database rejected the values")
}

func TestRunActionRefused(t *testing.T) {
	cases := []struct {
		name, body string
		setup      func(sqlmock.Sqlmock)
		status     int
		want       string
	}{
		{"not load or save", `{"ref": {"kind": "action", "id": "open-customer"}}`,
			func(m sqlmock.Sqlmock) { stored(m, KindAction, "open-customer", openCustomerJSON) }, 400, `only load and save actions run on the server`},
		{"missing argument", `{"ref": {"kind": "action", "id": "load-customer"}, "args": {}}`,
			func(m sqlmock.Sqlmock) { stored(m, KindAction, "load-customer", loadCustomerJSON); customerColumns(m) }, 400, `missing argument \"id\"`},
		{"table no longer exists", `{"ref": {"kind": "action", "id": "load-customer"}, "args": {"id": 1}}`,
			func(m sqlmock.Sqlmock) {
				stored(m, KindAction, "load-customer", loadCustomerJSON)
				m.ExpectQuery(catalogQuery).WillReturnRows(sqlmock.NewRows([]string{"column_name"}))
			}, 400, `table \"kb.wpi_demo_customers\" does not exist`},
		{"unknown action", `{"ref": {"kind": "action", "id": "nope"}}`,
			func(m sqlmock.Sqlmock) { m.ExpectQuery(latestQuery).WillReturnRows(descriptorRows()) }, 404, `action \"nope\" does not exist`},
		{"pointer at a non-action", `{"ref": {"kind": "page", "id": "customer-form", "pointer": "/elements/0"}}`,
			func(m sqlmock.Sqlmock) { stored(m, KindPage, "customer-form", formWithInlineSave) }, 400, "does not point at an inline action"},
		{"pointer missing for page", `{"ref": {"kind": "page", "id": "customer-form"}}`, nil, 400, "a pointer is required"},
		{"browser-supplied descriptor kind", `{"ref": {"kind": "app", "id": "x"}}`, nil, 400, "ref.kind must be"},
		{"argument is an object", `{"ref": {"kind": "action", "id": "load-customer"}, "args": {"id": {"x": 1}}}`,
			func(m sqlmock.Sqlmock) { stored(m, KindAction, "load-customer", loadCustomerJSON); customerColumns(m) }, 400, "must be a string, number, boolean or null"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			if c.setup != nil {
				c.setup(mock)
			}
			runAction(t, mock, testHandler(db, plainUser), c.body, c.status, c.want)
		})
	}
	t.Run("not logged in", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		runAction(t, mock, testHandler(db, nil), `{"ref": {"kind": "action", "id": "load-customer"}}`, 401, "Authentication required")
	})
}

func TestJSONPointer(t *testing.T) {
	doc := decode(t, `{"a/b": [{"c~d": 1}]}`)
	got, err := jsonPointer(doc, "/a~1b/0/c~0d")
	if err != nil || got.(interface{ String() string }).String() != "1" {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, p := range []string{"a", "/missing", "/a~1b/5", "/a~1b/x"} {
		if _, err := jsonPointer(doc, p); err == nil {
			t.Errorf("%q must fail", p)
		}
	}
}
