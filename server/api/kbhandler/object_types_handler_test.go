package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type objectTypeReviewStub struct {
	rows  []docprocessing.ObjectType
	row   docprocessing.ObjectType
	err   error
	calls int
	id    int64
	actor string
}

func (s *objectTypeReviewStub) List(context.Context) ([]docprocessing.ObjectType, error) {
	s.calls++
	return s.rows, s.err
}

func (s *objectTypeReviewStub) Approve(_ context.Context, id int64, actor string) (docprocessing.ObjectType, error) {
	s.calls++
	s.id, s.actor = id, actor
	return s.row, s.err
}

func objectTypeTestContext(t *testing.T, method, id string, user *ApiTypes.UserInfo) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	old := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = old })
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(method, "/api/v1/kb/object-types", nil), rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestObjectTypeReviewAuthorization(t *testing.T) {
	for _, endpoint := range []string{"list", "approve"} {
		for _, tc := range []struct {
			name string
			user *ApiTypes.UserInfo
			want int
		}{
			{"anonymous", nil, http.StatusUnauthorized},
			{"ordinary user", &ApiTypes.UserInfo{UserId: "u1"}, http.StatusForbidden},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				c, rec := objectTypeTestContext(t, http.MethodPost, "12", tc.user)
				store := &objectTypeReviewStub{}
				var err error
				if endpoint == "list" {
					err = listObjectTypes(c, store)
				} else {
					err = approveObjectType(c, store)
				}
				if err != nil {
					t.Fatal(err)
				}
				if rec.Code != tc.want {
					t.Fatalf("status %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
				}
				if store.calls != 0 {
					t.Fatal("unauthorized request reached the registry")
				}
				var body errorResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Status || body.ErrorMsg == "" {
					t.Fatalf("missing structured error: %s", rec.Body.String())
				}
			})
		}
	}
}

func TestListObjectTypesAdminAndEmptyList(t *testing.T) {
	for _, user := range []*ApiTypes.UserInfo{
		{UserId: "admin", Admin: true}, {UserId: "owner", IsOwner: true},
		{UserId: "role", Roles: []string{"admin"}}, {UserId: "root", Roles: []string{"root"}},
	} {
		t.Run(user.UserId, func(t *testing.T) {
			c, rec := objectTypeTestContext(t, http.MethodGet, "", user)
			store := &objectTypeReviewStub{}
			if err := listObjectTypes(c, store); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || store.calls != 1 {
				t.Fatalf("status %d, calls %d: %s", rec.Code, store.calls, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"rows":[]`) {
				t.Fatalf("empty rows must be array: %s", rec.Body.String())
			}
		})
	}
}

func TestApproveObjectTypeValidatesID(t *testing.T) {
	for _, id := range []string{"", "0", "-1", "bad", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			c, rec := objectTypeTestContext(t, http.MethodPost, id, &ApiTypes.UserInfo{UserId: "admin", Admin: true})
			store := &objectTypeReviewStub{}
			if err := approveObjectType(c, store); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusBadRequest || store.calls != 0 {
				t.Fatalf("status %d, calls %d", rec.Code, store.calls)
			}
		})
	}
}

func TestApproveObjectTypeUsesAuthenticatedActor(t *testing.T) {
	c, rec := objectTypeTestContext(t, http.MethodPost, "12", &ApiTypes.UserInfo{UserId: "reviewer", Admin: true})
	store := &objectTypeReviewStub{row: docprocessing.ObjectType{ID: 12, Name: "new_type", Status: "approved"}}
	if err := approveObjectType(c, store); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || store.id != 12 || store.actor != "reviewer" {
		t.Fatalf("status %d, id %d actor %q: %s", rec.Code, store.id, store.actor, rec.Body.String())
	}
	var body struct {
		Status bool                     `json:"status"`
		Row    docprocessing.ObjectType `json:"row"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Status || body.Row.ID != 12 || body.Row.Status != "approved" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestListObjectTypesReturnsRegistryEntries(t *testing.T) {
	c, rec := objectTypeTestContext(t, http.MethodGet, "", &ApiTypes.UserInfo{UserId: "admin", Admin: true})
	store := &objectTypeReviewStub{rows: []docprocessing.ObjectType{{ID: 12, Name: "new_type", Status: "proposed", RawValue: "New type", SourceRecordID: 224}}}
	if err := listObjectTypes(c, store); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Status bool                       `json:"status"`
		Rows   []docprocessing.ObjectType `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !body.Status || len(body.Rows) != 1 || body.Rows[0].SourceRecordID != 224 || body.Rows[0].RawValue != "New type" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestObjectTypeReviewAuthDoesNotWriteTwiceThroughEcho(t *testing.T) {
	_, _ = objectTypeTestContext(t, http.MethodGet, "", nil)
	e := echo.New()
	e.GET("/api/v1/kb/object-types", ListObjectTypes)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/kb/object-types", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response written more than once: %v: %s", err, rec.Body.String())
	}
}

func TestListObjectTypesDatabaseFailure(t *testing.T) {
	c, rec := objectTypeTestContext(t, http.MethodGet, "", &ApiTypes.UserInfo{UserId: "admin", Admin: true})
	if err := listObjectTypes(c, &objectTypeReviewStub{err: errors.New("private detail")}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "private detail") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestObjectTypeReviewErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"missing", sql.ErrNoRows, http.StatusNotFound},
		{"database", errors.New("private database detail"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := objectTypeTestContext(t, http.MethodPost, "12", &ApiTypes.UserInfo{UserId: "admin", Admin: true})
			if err := approveObjectType(c, &objectTypeReviewStub{err: tc.err}); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d", rec.Code, tc.want)
			}
			if strings.Contains(rec.Body.String(), "private database detail") {
				t.Fatal("database details exposed to client")
			}
		})
	}
}
