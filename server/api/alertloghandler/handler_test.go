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
