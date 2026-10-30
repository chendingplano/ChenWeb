package wpi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

// panelPage is a single-pane page of format 0.5 whose panel holds elements,
// with extra page attributes before the page layouter.
func panelPage(id, attrs, elements string) string {
	return `{"descriptor_id": "` + id + `", "schema_version": "0.5", ` + attrs + `"page_layouter": {"layouter": "single-pane"},
	  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [` + elements + `]}}}`
}

// filledPage is a page as it is stored: with copies of the test classes in its
// instances and layouter copies.
func filledPage(page string) string {
	body, err := decodeBody([]byte(page))
	if err != nil {
		panic(err)
	}
	load := func(_ context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
		out := map[string]Descriptor{}
		for _, id := range ids {
			if s, ok := testClasses()[kind][id]; ok {
				out[id] = Descriptor{Kind: kind, ID: id, Version: 1, Raw: []byte(s)}
			}
		}
		return out, nil
	}
	if issues, err := fillClassCopies(context.Background(), str(body["descriptor_id"]), body, load); err != nil || len(issues) > 0 {
		panic(fmt.Sprint(err, issues))
	}
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(b)
}

var (
	// customersPage holds a list whose source loads customers and whose row
	// click opens a customer through a nested load instance.
	customersPage = filledPage(panelPage("customers", ``, `{"element": "list",
	   "source": {"action": "load", "table": "kb.wpi_demo_customers", "result": "list", "limit": 5000, "order_by": [{"column": "email"}],
	     "fields": [{"column": "id", "attribute": "id"}, {"column": "email", "attribute": "email"}]},
	   "columns": [{"field": "email", "label": "Email"}],
	   "on_row_click": {"action": "open-page", "page": "customer-form", "pane": "right", "args": {"id": "$record.id"},
	     "input": {"action": "load", "table": "kb.wpi_demo_customers", "result": "record", "where": {"id": "$args.id"},
	       "fields": [{"column": "id", "attribute": "id"}, {"column": "email", "attribute": "email"}]}}}`))
	// formPageRaw relies on its save class copy's defaults for mode and key.
	formPageRaw = panelPage("customer-form", `"input": {},
	  "save_action": {"action": "save", "table": "kb.wpi_demo_customers",
	    "fields": [{"attribute": "user_name", "column": "user_name", "required": true},
	               {"attribute": "email", "column": "email", "required": true, "rules": {"format": "email"}}]}, `,
		`{"element": "button", "label": "Save", "on_click": {"action": "save-page"}}`)
	formPage = filledPage(formPageRaw)
)

const catalogQuery = `SELECT column_name FROM information_schema.columns`

func stored(mock sqlmock.Sqlmock, kind Kind, id, body string) {
	if kind == KindPage {
		mock.ExpectQuery(latestPagesQuery).WithArgs(sqlmock.AnyArg()).WillReturnRows(descriptorRows([2]string{id, body}))
		return
	}
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

const (
	loadOneRef  = `"ref": {"kind": "page", "id": "customers", "pointer": "/panels/main/elem_list/0/on_row_click/input"}`
	loadListRef = `"ref": {"kind": "page", "id": "customers", "pointer": "/panels/main/elem_list/0/source"}`
	saveRef     = `"ref": {"kind": "page", "id": "customer-form", "pointer": "/save_action"}`
)

func TestRunLoadRecord(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "customers", customersPage)
	customerColumns(mock)
	mock.ExpectQuery(`SELECT "id", "email" FROM "kb"\."wpi_demo_customers" WHERE "id" = \$1 LIMIT 1`).WithArgs("7").
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(int64(7), []byte("g@example.com")))
	// "table" outside ref is ignored: only the stored instance picks the table.
	runAction(t, mock, testHandler(db, plainUser), `{`+loadOneRef+`, "args": {"id": 7}, "table": "kb.users"}`,
		200, `{"result":{"email":"g@example.com","id":7}}`)
}

func TestRunLoadRecordNotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "customers", customersPage)
	customerColumns(mock)
	mock.ExpectQuery(`SELECT`).WithArgs("99").WillReturnRows(sqlmock.NewRows([]string{"id", "email"}))
	runAction(t, mock, testHandler(db, plainUser), `{`+loadOneRef+`, "args": {"id": 99}}`, 404, "Record not found")
}

func TestRunLoadListCapsLimit(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "customers", customersPage)
	customerColumns(mock)
	mock.ExpectQuery(`ORDER BY "email" ASC LIMIT \$1`).WithArgs(maxListLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(int64(1), "a@example.com").AddRow(int64(2), "b@example.com"))
	runAction(t, mock, testHandler(db, plainUser), `{`+loadListRef+`}`, 200, `"id":2`)
}

func TestRunSaveUpsert(t *testing.T) {
	setup := func(mock sqlmock.Sqlmock) {
		stored(mock, KindPage, "customer-form", formPage)
		customerColumns(mock)
	}
	t.Run("inserts without key", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		setup(mock)
		mock.ExpectQuery(`INSERT INTO "kb"\."wpi_demo_customers" \("user_name", "email"\) VALUES \(\$1, \$2\) RETURNING "id"`).
			WithArgs("dana", "d@example.com").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(4)))
		runAction(t, mock, testHandler(db, plainUser), `{`+saveRef+`, "input_doc": {"user_name": "dana", "email": "d@example.com"}}`,
			200, `{"key_attribute":"id","key_value":4}`)
	})
	t.Run("updates with key, no version check", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		setup(mock)
		mock.ExpectQuery(`UPDATE "kb"\."wpi_demo_customers" SET "user_name" = \$1, "email" = \$2 WHERE "id" = \$3 RETURNING "id"`).
			WithArgs("dana", "d@example.com", "7").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
		runAction(t, mock, testHandler(db, plainUser), `{`+saveRef+`, "input_doc": {"id": 7, "user_name": "dana", "email": "d@example.com"}}`,
			200, `"key_value":7`)
	})
	t.Run("update of a missing row", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		setup(mock)
		mock.ExpectQuery(`UPDATE`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		runAction(t, mock, testHandler(db, plainUser), `{`+saveRef+`, "input_doc": {"id": 99, "user_name": "dana", "email": "d@example.com"}}`,
			404, "Record not found")
	})
}

func TestRunSaveFieldErrorsWriteNothing(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "customer-form", formPage)
	customerColumns(mock)
	runAction(t, mock, testHandler(db, plainUser), `{`+saveRef+`, "input_doc": {"email": "nope"}}`,
		422, `{"field_errors":{"email":["format"],"user_name":["required"]}}`)
}

func TestRunDatabaseRejectsValue(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "customer-form", formPage)
	customerColumns(mock)
	mock.ExpectQuery(`INSERT`).WillReturnError(&pq.Error{Code: "23502", Message: "null value in column"})
	runAction(t, mock, testHandler(db, plainUser), `{`+saveRef+`, "input_doc": {"user_name": "dana", "email": "d@example.com"}}`,
		400, "The database rejected the values")
}

func TestRunActionRefused(t *testing.T) {
	cases := []struct {
		name, body string
		setup      func(sqlmock.Sqlmock)
		status     int
		want       string
	}{
		{"not a server action", `{"ref": {"kind": "page", "id": "customers", "pointer": "/panels/main/elem_list/0/on_row_click"}}`,
			func(m sqlmock.Sqlmock) { stored(m, KindPage, "customers", customersPage) }, 400, `only load, save, query, load-file actions run on the server`},
		{"missing argument", `{` + loadOneRef + `, "args": {}}`,
			func(m sqlmock.Sqlmock) {
				stored(m, KindPage, "customers", customersPage)
				customerColumns(m)
			}, 400, `missing argument \"id\"`},
		{"table no longer exists", `{` + loadOneRef + `, "args": {"id": 1}}`,
			func(m sqlmock.Sqlmock) {
				stored(m, KindPage, "customers", customersPage)
				m.ExpectQuery(catalogQuery).WillReturnRows(sqlmock.NewRows([]string{"column_name"}))
			}, 400, `table \"kb.wpi_demo_customers\" does not exist`},
		{"unknown page", `{"ref": {"kind": "page", "id": "nope", "pointer": "/save_action"}}`,
			func(m sqlmock.Sqlmock) { m.ExpectQuery(latestPagesQuery).WillReturnRows(descriptorRows()) }, 404, `page \"nope\" does not exist`},
		{"instance without a class copy", `{` + saveRef + `, "input_doc": {}}`,
			func(m sqlmock.Sqlmock) { stored(m, KindPage, "customer-form", formPageRaw) }, 400, `missing attribute \"class\"`},
		{"pointer at a non-action", `{"ref": {"kind": "page", "id": "customers", "pointer": "/panels/main/elem_list/0"}}`,
			func(m sqlmock.Sqlmock) { stored(m, KindPage, "customers", customersPage) }, 400, "does not point at an action instance"},
		{"pointer at an empty input", `{"ref": {"kind": "page", "id": "customer-form", "pointer": "/input"}}`,
			func(m sqlmock.Sqlmock) { stored(m, KindPage, "customer-form", formPage) }, 400, "points at static data"},
		{"pointer missing", `{"ref": {"kind": "page", "id": "customer-form"}}`, nil, 400, "ref.pointer is required"},
		{"stored action by ID", `{"ref": {"kind": "action", "id": "load"}}`, nil, 400, "ref.kind must be page"},
		{"browser-supplied descriptor kind", `{"ref": {"kind": "app", "id": "x"}}`, nil, 400, "ref.kind must be page"},
		{"argument is an object", `{` + loadOneRef + `, "args": {"id": {"x": 1}}}`,
			func(m sqlmock.Sqlmock) {
				stored(m, KindPage, "customers", customersPage)
				customerColumns(m)
			}, 400, "must be a string, number, boolean or null"},
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
		runAction(t, mock, testHandler(db, nil), `{`+saveRef+`}`, 401, "Authentication required")
	})
}
