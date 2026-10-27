package wpi

import (
	"database/sql/driver"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// directoryPage reads its input from a stored file, lists customers through a
// stored statement and opens one through another, with a parameter.
const directoryPage = `{"descriptor_id": "directory", "schema_version": "0.2", "layouter": "flow-layouter",
  "input": {"action": "load-file", "file_id": "about"},
  "elements": [{"element": "list",
    "source": {"action": "query", "sql_stmt_id": "directory", "result": "list"},
    "columns": [{"field": "email", "label": "Email"}],
    "on_row_click": {"action": "open-page", "page": "customer-form", "pane": "right", "args": {"id": "$record.id"},
      "input": {"action": "query", "sql_stmt_id": "customer-by-id", "result": "record", "params": ["$args.id", "x"]}}}]}`

const (
	directoryRef = `"ref": {"kind": "page", "id": "directory", "pointer": "/elements/0/source"}`
	byIDRef      = `"ref": {"kind": "page", "id": "directory", "pointer": "/elements/0/on_row_click/input"}`
	aboutRef     = `"ref": {"kind": "page", "id": "directory", "pointer": "/input"}`
	stmtRowQuery = `SELECT sql_stmt, access_controls FROM kb\.stored_stmts WHERE stmt_name = \$1`
	fileRowQuery = `SELECT file_content::text, access_controls FROM kb\.stored_files WHERE file_name = \$1`
)

// storedRow expects the lookup of one stored statement or file; access is a
// PostgreSQL array literal, or nil for NULL.
func storedRow(mock sqlmock.Sqlmock, query, name, content string, access any) {
	mock.ExpectQuery(query).WithArgs(name).
		WillReturnRows(sqlmock.NewRows([]string{"content", "access_controls"}).AddRow(content, access))
}

// expectReadOnlyQuery expects the statement to run in a read-only transaction.
func expectReadOnlyQuery(mock sqlmock.Sqlmock, stmt string, limit string, rows *sqlmock.Rows, args ...driver.Value) {
	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL statement_timeout = '10s'`).WillReturnResult(sqlmock.NewResult(0, 0))
	q := mock.ExpectQuery(`SELECT to_jsonb\(t\)::text FROM \(\s+` + stmt + `\s+\) t LIMIT ` + limit)
	if len(args) > 0 {
		q.WithArgs(args...)
	}
	q.WillReturnRows(rows)
	mock.ExpectRollback()
}

func jsonRows(objs ...string) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"to_jsonb"})
	for _, o := range objs {
		r.AddRow(o)
	}
	return r
}

func TestRunQueryList(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "directory", directoryPage)
	storedClass(mock, "query")
	storedRow(mock, stmtRowQuery, "directory", "SELECT id, email FROM kb.c ORDER BY email;\n", "{all}")
	expectReadOnlyQuery(mock, `SELECT id, email FROM kb\.c ORDER BY email`, "1000", jsonRows(`{"id": 1, "email": "a@x.org"}`, `{"id": 2, "email": "b@x.org"}`))
	runAction(t, mock, testHandler(db, plainUser), `{`+directoryRef+`}`, 200, `{"result":[{"id":1,"email":"a@x.org"},{"id":2,"email":"b@x.org"}]}`)
}

func TestRunQueryRecordWithParams(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "directory", directoryPage)
	storedClass(mock, "query")
	storedRow(mock, stmtRowQuery, "customer-by-id", "SELECT id, email FROM kb.c WHERE id = $1 AND kind = $2", nil)
	expectReadOnlyQuery(mock, `SELECT id, email FROM kb\.c WHERE id = \$1 AND kind = \$2`, "1", jsonRows(`{"id": 7, "email": "g@x.org"}`), "7", "x")
	runAction(t, mock, testHandler(db, plainUser), `{`+byIDRef+`, "args": {"id": 7}}`, 200, `{"result":{"id":7,"email":"g@x.org"}}`)
}

func TestRunQueryRecordNotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "directory", directoryPage)
	storedClass(mock, "query")
	storedRow(mock, stmtRowQuery, "customer-by-id", "SELECT id FROM kb.c WHERE id = $1 AND kind = $2", nil)
	expectReadOnlyQuery(mock, `SELECT id FROM kb\.c WHERE id = \$1 AND kind = \$2`, "1", jsonRows(), "99", "x")
	runAction(t, mock, testHandler(db, plainUser), `{`+byIDRef+`, "args": {"id": 99}}`, 404, "Record not found")
}

func TestRunLoadFile(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	stored(mock, KindPage, "directory", directoryPage)
	storedClass(mock, "load-file")
	storedRow(mock, fileRowQuery, "about", `{"title": "About", "version": 3}`, "{}")
	runAction(t, mock, testHandler(db, plainUser), `{`+aboutRef+`}`, 200, `{"result":{"title":"About","version":3}}`)
}

func TestRunStoredRefused(t *testing.T) {
	cases := []struct {
		name   string
		user   *ApiTypes.UserInfo
		setup  func(sqlmock.Sqlmock)
		body   string
		status int
		want   string
	}{
		{"statement not registered", plainUser, func(m sqlmock.Sqlmock) {
			stored(m, KindPage, "directory", directoryPage)
			storedClass(m, "query")
			m.ExpectQuery(stmtRowQuery).WithArgs("directory").WillReturnRows(sqlmock.NewRows([]string{"content", "access_controls"}))
		}, `{` + directoryRef + `}`, 400, `SQL statement \"directory\" is not registered`},
		{"role required", plainUser, func(m sqlmock.Sqlmock) {
			stored(m, KindPage, "directory", directoryPage)
			storedClass(m, "query")
			storedRow(m, stmtRowQuery, "directory", "SELECT 1", "{finance}")
		}, `{` + directoryRef + `}`, 403, `You may not use SQL statement \"directory\"`},
		{"file not registered", plainUser, func(m sqlmock.Sqlmock) {
			stored(m, KindPage, "directory", directoryPage)
			storedClass(m, "load-file")
			m.ExpectQuery(fileRowQuery).WithArgs("about").WillReturnRows(sqlmock.NewRows([]string{"content", "access_controls"}))
		}, `{` + aboutRef + `}`, 400, `file \"about\" is not registered`},
		{"file for admins only", plainUser, func(m sqlmock.Sqlmock) {
			stored(m, KindPage, "directory", directoryPage)
			storedClass(m, "load-file")
			storedRow(m, fileRowQuery, "about", `{}`, "{admin}")
		}, `{` + aboutRef + `}`, 403, `You may not use file \"about\"`},
		{"missing argument", plainUser, func(m sqlmock.Sqlmock) {
			stored(m, KindPage, "directory", directoryPage)
			storedClass(m, "query")
			storedRow(m, stmtRowQuery, "customer-by-id", "SELECT 1", nil)
		}, `{` + byIDRef + `, "args": {}}`, 400, `missing argument \"id\"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			c.setup(mock)
			runAction(t, mock, testHandler(db, c.user), c.body, c.status, c.want)
		})
	}
}

func TestMayUse(t *testing.T) {
	sales := &ApiTypes.UserInfo{Roles: []string{"sales"}}
	owner := &ApiTypes.UserInfo{IsOwner: true}
	cases := []struct {
		user   *ApiTypes.UserInfo
		access []string
		want   bool
	}{
		{sales, nil, true},
		{sales, []string{}, true},
		{sales, []string{"finance", "all"}, true},
		{sales, []string{"finance"}, false},
		{sales, []string{"finance", "sales"}, true},
		{owner, []string{"admin"}, true},
		{owner, []string{"finance"}, false},
		{adminUser, []string{"admin"}, true},
	}
	for i, c := range cases {
		if got := mayUse(c.user, c.access); got != c.want {
			t.Errorf("case %d: mayUse(%v, %v) = %v", i, c.user.Roles, c.access, got)
		}
	}
}

func TestWrapStatement(t *testing.T) {
	got := wrapStatement("  SELECT 1 -- one\n;; \n", 5)
	want := "SELECT to_jsonb(t)::text FROM (\nSELECT 1 -- one\n) t LIMIT 5"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

const summariesQuery = `SELECT DISTINCT ON \(descriptor_id\) descriptor_id, version, body->'page_name', body->'description', updated_at\s+FROM kb\.frontend_pages`

func summaryRows() *sqlmock.Rows {
	at := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{"descriptor_id", "version", "page_name", "description", "updated_at"}).
		AddRow("app-header", 3, nil, nil, at).
		AddRow("customer-form", 3, []byte(`"Customer"`), []byte(`{"en": "Edit one customer", "zh-cn": "编辑客户"}`), at).
		AddRow("customers", 3, []byte(`{"en": "Customers", "zh-cn": "客户列表"}`), nil, at)
}

// callQuery runs a handler with a query string.
func callQuery(t *testing.T, fn echo.HandlerFunc, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/?"+query, nil)
	rec := httptest.NewRecorder()
	if err := fn(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestSearchPages(t *testing.T) {
	cases := []struct {
		name, query string
		status      int
		want        []string
	}{
		{"everything", "", 200, []string{"app-header", "customer-form", "customers"}},
		{"by ID", "q=HEADER", 200, []string{"app-header"}},
		{"by Chinese name", "q=%E5%AE%A2%E6%88%B7%E5%88%97%E8%A1%A8", 200, []string{"customers"}},
		{"by description in Chinese", "q=%E7%BC%96%E8%BE%91", 200, []string{"customer-form"}},
		{"limit", "q=customer&limit=1", 200, []string{"customer-form"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			mock.ExpectQuery(summariesQuery).WillReturnRows(summaryRows())
			rec := callQuery(t, testHandler(db, plainUser).SearchPages, c.query)
			body := rec.Body.String()
			if rec.Code != c.status || strings.Count(body, `"descriptor_id"`) != len(c.want) {
				t.Fatalf("status %d: %s", rec.Code, body)
			}
			for _, id := range c.want {
				if !strings.Contains(body, `"descriptor_id":"`+id+`"`) {
					t.Fatalf("%s missing: %s", id, body)
				}
			}
		})
	}
	t.Run("summary fields", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(summariesQuery).WillReturnRows(summaryRows())
		body := callQuery(t, testHandler(db, plainUser).SearchPages, "q=customer-form").Body.String()
		want := `{"pages":[{"descriptor_id":"customer-form","version":3,"page_name":"Customer","description":{"en":"Edit one customer","zh-cn":"编辑客户"},"updated_at":"2026-10-27T09:00:00Z"}]}`
		if strings.TrimSpace(body) != want {
			t.Fatalf("got  %s\nwant %s", body, want)
		}
	})
	t.Run("bad limit", func(t *testing.T) {
		db, _, _ := sqlmock.New()
		defer db.Close()
		if rec := callQuery(t, testHandler(db, plainUser).SearchPages, "limit=0"); rec.Code != 400 {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("not logged in", func(t *testing.T) {
		db, _, _ := sqlmock.New()
		defer db.Close()
		if rec := callQuery(t, testHandler(db, nil).SearchPages, ""); rec.Code != 401 {
			t.Fatalf("status %d", rec.Code)
		}
	})
}

func TestGetPage(t *testing.T) {
	t.Run("latest version", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(latestPagesQuery).WithArgs(sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "body"}).AddRow("greeting", 4, []byte(greetingPage)))
		rec := call(t, testHandler(db, plainUser).GetPage, "GET", "", "pageId", "greeting")
		if rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), `{"page":{"descriptor_id":"greeting"`) || !strings.Contains(rec.Body.String(), `"version":4`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})
	t.Run("unknown page", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectQuery(latestPagesQuery).WillReturnRows(descriptorRows())
		if rec := call(t, testHandler(db, plainUser).GetPage, "GET", "", "pageId", "nope"); rec.Code != 404 {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("invalid ID", func(t *testing.T) {
		db, _, _ := sqlmock.New()
		defer db.Close()
		if rec := call(t, testHandler(db, plainUser).GetPage, "GET", "", "pageId", "Bad_ID"); rec.Code != 404 {
			t.Fatalf("status %d", rec.Code)
		}
	})
	t.Run("not logged in", func(t *testing.T) {
		db, _, _ := sqlmock.New()
		defer db.Close()
		if rec := call(t, testHandler(db, nil).GetPage, "GET", "", "pageId", "greeting"); rec.Code != 401 {
			t.Fatalf("status %d", rec.Code)
		}
	})
}
