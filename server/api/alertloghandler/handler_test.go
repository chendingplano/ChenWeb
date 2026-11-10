package alertloghandler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

func TestListRequestCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, stage string
		canceled    bool
		deadline    bool
		queryErr    error
	}{
		{name: "canceled before count", stage: "count", canceled: true},
		{name: "canceled during list", stage: "list", canceled: true},
		{name: "active count cancellation error", stage: "count", queryErr: context.Canceled},
		{name: "active list database error", stage: "list", queryErr: sql.ErrConnDone},
		{name: "request deadline exceeded", deadline: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.deadline {
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer stop()
			}
			matcher := sqlmock.QueryMatcherFunc(func(expected, actual string) error {
				if err := sqlmock.QueryMatcherRegexp.Match(expected, actual); err != nil {
					return err
				}
				if tc.canceled && tc.stage == "list" && strings.HasPrefix(actual, "SELECT id,") {
					cancel()
				}
				return nil
			})
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(matcher))
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

			switch {
			case tc.canceled && tc.stage == "count":
				cancel()
			case tc.deadline:
				// database/sql rejects an expired context before querying the driver.
			case tc.stage == "list":
				mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				query := mock.ExpectQuery(`SELECT id, level, message`)
				if tc.canceled {
					query.WillDelayFor(time.Second).WillReturnError(sqlmock.ErrCancelled)
				} else {
					query.WillReturnError(tc.queryErr)
				}
			default:
				mock.ExpectQuery(`SELECT COUNT\(\*\)`).WillReturnError(tc.queryErr)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
			c := echo.New().NewContext(req, rec)
			if err := List(c); err != nil {
				t.Fatal(err)
			}
			if tc.canceled {
				if c.Response().Committed || rec.Body.Len() != 0 {
					t.Fatalf("canceled request wrote status %d: %s", rec.Code, rec.Body.String())
				}
			} else if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

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
		{"resolve and assign only if unassigned", "/resolve", `{"resolution":"fixed"}`, `WITH updated AS .*UPDATE public\.jimo_alert_log.*INSERT INTO public\.jimo_alert_comments`, []any{"fixed", "alice@example.com"}, Resolve},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "resolve and assign only if unassigned" {
				mock.ExpectBegin()
			}
			expectation := mock.ExpectExec(tc.query)
			if tc.name == "resolve and assign only if unassigned" {
				expectation.WithArgs(tc.args[0], tc.args[1], sqlmock.AnyArg())
			} else {
				expectation.WithArgs(tc.args[0], tc.args[1])
			}
			expectation.WillReturnResult(sqlmock.NewResult(0, 1))
			if tc.name == "resolve and assign only if unassigned" {
				mock.ExpectCommit()
			}
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

func TestBulkResolveAtomic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rows   int64
		status int
	}{{"all selected", 2, http.StatusOK}, {"one already resolved", 1, http.StatusConflict}} {
		t.Run(tc.name, func(t *testing.T) {
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
			mock.ExpectBegin()
			mock.ExpectExec(`WITH updated AS .*UPDATE public\.jimo_alert_log.*INSERT INTO public\.jimo_alert_comments`).WithArgs("fixed", "alice@example.com", "{12,13}").WillReturnResult(sqlmock.NewResult(0, tc.rows))
			if tc.status == http.StatusOK {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/resolve-bulk", strings.NewReader(`{"ids":[12,13],"resolution":" fixed "}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			if err := ResolveBulk(e.NewContext(req, rec)); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
