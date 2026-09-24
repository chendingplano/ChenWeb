package peakhourshandler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

// installAuthenticator stubs EchoFactory.DefaultAuthenticator for the
// duration of the test, restoring the original on cleanup. A nil user
// simulates an unauthenticated request.
func installAuthenticator(t *testing.T, user *ApiTypes.UserInfo) {
	t.Helper()
	old := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(rc ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return user, nil
	}
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = old })
}

func newEchoContext(method, target string, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestList_UnauthenticatedIsRejected(t *testing.T) {
	installAuthenticator(t, nil)
	c, rec := newEchoContext(http.MethodGet, "/api/v1/peak-hours", "")

	// List is expected to return a non-nil error here (it has already
	// written the response; the error only tells Echo's router not to
	// write a second one) - the response itself is what we're checking.
	_ = List(c)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status {
		t.Fatalf("status field = true, want false")
	}
}

func TestList_NonAdminIsForbidden(t *testing.T) {
	installAuthenticator(t, &ApiTypes.UserInfo{UserId: "u1"})
	c, rec := newEchoContext(http.MethodGet, "/api/v1/peak-hours", "")

	_ = List(c)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestList_AdminRoleIsAllowedThrough(t *testing.T) {
	mock := installMockDB(t)
	installAuthenticator(t, &ApiTypes.UserInfo{UserId: "u1", Roles: []string{"admin"}})
	c, rec := newEchoContext(http.MethodGet, "/api/v1/peak-hours", "")

	mock.ExpectQuery(rx("SELECT id, name, hours, timezone, applicable_days, exclude_days, country, created_at, updated_at FROM public.peak_hours ORDER BY name")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "hours", "timezone", "applicable_days", "exclude_days", "country", "created_at", "updated_at"}))

	if err := List(c); err != nil {
		t.Fatalf("List: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		Status  bool        `json:"status"`
		Results []PeakHours `json:"results"`
		Total   int         `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.Status {
		t.Fatalf("status field = false, want true")
	}
}

func TestCreate_InvalidBodyReturns400(t *testing.T) {
	installAuthenticator(t, &ApiTypes.UserInfo{UserId: "u1", IsOwner: true})
	c, rec := newEchoContext(http.MethodPost, "/api/v1/peak-hours", "{not json")

	if err := Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestIsActive_UnknownNameReturns404(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(rx("FROM public.peak_hours WHERE name = $1")).
		WithArgs("does-not-exist").
		WillReturnError(sql.ErrNoRows)

	c, rec := newEchoContext(http.MethodGet, "/api/v1/peak-hours/does-not-exist/is-active", "")
	c.SetParamNames("name")
	c.SetParamValues("does-not-exist")

	if err := IsActive(c); err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestIsActive_BadAtReturns400(t *testing.T) {
	c, rec := newEchoContext(http.MethodGet, "/api/v1/peak-hours/deepseek/is-active?at=not-a-time", "")
	c.SetParamNames("name")
	c.SetParamValues("deepseek")

	if err := IsActive(c); err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestIsActive_NoAuthRequired(t *testing.T) {
	// IsActive must not require an authenticated user, unlike the CRUD
	// handlers - it's queried by other backend code, not just admins.
	installAuthenticator(t, nil)
	mock := installMockDB(t)
	mock.ExpectQuery(rx("FROM public.peak_hours WHERE name = $1")).
		WithArgs("does-not-exist").
		WillReturnError(sql.ErrNoRows)

	c, rec := newEchoContext(http.MethodGet, "/api/v1/peak-hours/does-not-exist/is-active", "")
	c.SetParamNames("name")
	c.SetParamValues("does-not-exist")

	if err := IsActive(c); err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	// 404 (not 401/403) proves the request reached GetPeakHoursByName
	// without being blocked by an admin check.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
