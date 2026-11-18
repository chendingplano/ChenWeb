package useradminhandler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

func TestListUsersReloadsPhoneOnlySelfIdentity(t *testing.T) {
	const identityID = "a46719fa-636c-455c-9464-a572bc890018"
	original := EchoFactory.DefaultAuthenticator
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = original })
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{UserId: identityID, Verified: true, FirstName: "Cached"}, nil
	}

	kratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/admin/identities/"+identityID || r.URL.RawQuery != "" {
			t.Errorf("unexpected Kratos request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"`+identityID+`","traits":{"phone":"+8613800000000","name":{"first":"Fresh"}}}`)
	}))
	t.Cleanup(kratos.Close)
	t.Setenv("KRATOS_ADMIN_URL", kratos.URL)

	const path = "/api/v1/system-admin/users"
	e := echo.New()
	e.GET(path, ListUsers)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response listUsersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Scope != "self" || response.CanManageAll || len(response.Users) != 1 {
		t.Fatalf("unexpected user scope: %s", rec.Body.String())
	}
	user := response.Users[0]
	if user == nil || user.UserId != identityID || user.Email != "" || user.UserMobile != "+8613800000000" || user.FirstName != "Fresh" {
		t.Fatalf("expected freshly loaded phone-only identity, got %s", rec.Body.String())
	}
}
