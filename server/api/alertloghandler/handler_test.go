package alertloghandler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

func TestAssignAndResolve(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldDB, oldAuth := ApiTypes.SharedDBHandle, EchoFactory.DefaultAuthenticator
	ApiTypes.SharedDBHandle = db
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{Email: "alice@example.com"}, nil
	}
	defer func() { ApiTypes.SharedDBHandle, EchoFactory.DefaultAuthenticator = oldDB, oldAuth }()

	cases := []struct {
		name, path, body, query string
		args                    []any
		handler                 echo.HandlerFunc
	}{
		{"assign self", "/assign", `{"self":true}`, `UPDATE public\.jimo_alert_log SET owner = \$1 WHERE id = \$2`, []any{"alice@example.com", int64(12)}, Assign},
		{"assign named owner", "/assign", `{"owner":"bob@example.com"}`, `UPDATE public\.jimo_alert_log SET owner = \$1 WHERE id = \$2`, []any{"bob@example.com", int64(12)}, Assign},
		{"resolve and assign only if unassigned", "/resolve", `{"resolution":"fixed"}`, `UPDATE public\.jimo_alert_log SET status = 'resolved', resolution = \$1, resolver = \$2, resolve_time = NOW\(\), owner = CASE WHEN .* THEN \$2 ELSE owner END WHERE id = \$3`, []any{"fixed", "alice@example.com", int64(12)}, Resolve},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectation := mock.ExpectExec(tc.query)
			if tc.name == "resolve and assign only if unassigned" {
				expectation.WithArgs(tc.args[0], tc.args[1], tc.args[2])
			} else {
				expectation.WithArgs(tc.args[0], tc.args[1])
			}
			expectation.WillReturnResult(sqlmock.NewResult(0, 1))
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, tc.path, strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("12")
			if err := tc.handler(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestListFiltersAcrossPages(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldDB, oldAuth := ApiTypes.SharedDBHandle, EchoFactory.DefaultAuthenticator
	ApiTypes.SharedDBHandle = db
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{Email: "alice@example.com"}, nil
	}
	defer func() { ApiTypes.SharedDBHandle, EchoFactory.DefaultAuthenticator = oldDB, oldAuth }()
	mock.ExpectQuery(`SELECT COUNT\(\*\).*l\.message ILIKE.*l\.resolver =`).WithArgs("%failure%", "alice@example.com").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT id, level, message.*l\.message ILIKE.*l\.resolver =`).WithArgs("%failure%", "alice@example.com", 20, 20).WillReturnRows(sqlmock.NewRows([]string{"id", "level", "message", "req_id", "loc", "call_flow", "attrs", "created_at", "status", "owner", "resolution", "resolver", "resolve_time", "comments"}))
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/?page=2&page_size=20&filter=message%7Ccontains%7Cfailure&filter=resolver%7Ceq%7Calice%40example.com", nil)
	rec := httptest.NewRecorder()
	if err := List(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFilterOptionsIncludeValuesOutsideCurrentPage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldDB, oldAuth := ApiTypes.SharedDBHandle, EchoFactory.DefaultAuthenticator
	ApiTypes.SharedDBHandle = db
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{Email: "alice@example.com"}, nil
	}
	defer func() { ApiTypes.SharedDBHandle, EchoFactory.DefaultAuthenticator = oldDB, oldAuth }()
	mock.ExpectQuery(`SELECT 'level', level FROM public\.jimo_alert_log`).WillReturnRows(sqlmock.NewRows([]string{"kind", "value"}).AddRow("loc", "CWB_001").AddRow("owner", "alice@example.com").AddRow("resolver", "bob@example.com"))
	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/filter-options", nil)
	if err := FilterOptions(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"loc":["CWB_001"]`) || !strings.Contains(rec.Body.String(), `"resolver":["bob@example.com"]`) {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
