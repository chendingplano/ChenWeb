package useradminhandler

import (
	"encoding/json"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateUserValidationAndAuthorization(t *testing.T) {
	original := EchoFactory.DefaultAuthenticator
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = original })
	for _, tc := range []struct {
		name, body string
		user       *ApiTypes.UserInfo
		want       int
	}{
		{"unauthenticated", `{}`, nil, 401},
		{"nonadmin", `{}`, &ApiTypes.UserInfo{UserId: "viewer"}, 403},
		{"invalid_email", `{"email":"bad","password":"strong-pass-123"}`, &ApiTypes.UserInfo{Admin: true}, 400},
		{"missing_password", `{"email":"new@example.com"}`, &ApiTypes.UserInfo{Admin: true}, 400},
		{"invalid_status", `{"email":"new@example.com","password":"strong-pass-123","status":"unknown"}`, &ApiTypes.UserInfo{Admin: true}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return tc.user, nil }
			e := echo.New()
			e.POST("/users", CreateUser)
			req := httptest.NewRequest("POST", "/users", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateUserKratosLifecycle(t *testing.T) {
	original := EchoFactory.DefaultAuthenticator
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = original })
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{UserId: "admin", Admin: true}, nil
	}
	for _, tc := range []struct {
		name                             string
		createStatus, updateStatus, want int
		rollback                         bool
	}{
		{"success", 201, 200, 201, false},
		{"duplicate", 409, 200, 409, false},
		{"configuration_failure", 201, 500, 502, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var created, updated, deleted bool
			identity := map[string]any{"id": "new-user", "schema_id": "default", "state": "active", "traits": map[string]any{"email": "new@example.com"}}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case "POST":
					if r.URL.Path != "/admin/identities" {
						t.Errorf("unexpected path %s", r.URL.Path)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body["traits"].(map[string]any)["email"] != "new@example.com" {
						t.Error("email not normalized")
					}
					if body["credentials"].(map[string]any)["password"].(map[string]any)["config"].(map[string]any)["password"] != "strong-pass-123" {
						t.Error("password missing")
					}
					created = true
					w.WriteHeader(tc.createStatus)
				case "PUT":
					if err := json.NewDecoder(r.Body).Decode(&identity); err != nil {
						t.Fatal(err)
					}
					identity["id"] = "new-user"
					updated = true
					w.WriteHeader(tc.updateStatus)
				case "DELETE":
					deleted = true
					w.WriteHeader(204)
					return
				}
				json.NewEncoder(w).Encode(identity)
			}))
			defer upstream.Close()
			t.Setenv("KRATOS_ADMIN_URL", upstream.URL)
			e := echo.New()
			e.POST("/users", CreateUser)
			req := httptest.NewRequest("POST", "/users", strings.NewReader(`{"email":" New@Example.com ","password":"strong-pass-123","status":"trial","roles":["dev"]}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.want || !created || deleted != tc.rollback {
				t.Fatalf("status=%d deleted=%v body=%s", rec.Code, deleted, rec.Body.String())
			}
			if tc.createStatus == 201 && !updated {
				t.Fatal("metadata not updated")
			}
			if tc.want == 201 && !strings.Contains(rec.Body.String(), `"trial"`) {
				t.Fatalf("trial role missing: %s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "strong-pass") {
				t.Fatal("password returned")
			}
		})
	}
}
