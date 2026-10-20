package harnessfrontend

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const session = "11111111-2222-3333-4444-555555555555"

func withUser(t *testing.T, user *ApiTypes.UserInfo) {
	t.Helper()
	old := EchoFactory.DefaultAuthenticator
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	t.Cleanup(func() { EchoFactory.DefaultAuthenticator = old })
}

type gatewayCall struct {
	method, path, auth string
	body               map[string]any
}

// fakeGateway speaks protocol v1 well enough to check what ChenWeb sends.
func fakeGateway(t *testing.T) (*echo.Echo, *[]gatewayCall) {
	t.Helper()
	calls := &[]gatewayCall{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := gatewayCall{method: r.Method, path: r.URL.RequestURI(), auth: r.Header.Get("Authorization")}
		_ = json.NewDecoder(r.Body).Decode(&call.body)
		*calls = append(*calls, call)
		switch {
		case r.URL.Path == "/v1/turns" && call.body["sessionId"] == "99999999-2222-3333-4444-555555555555":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"session not found"}`)
		case r.URL.Path == "/v1/turns":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(w, `{"type":"session","sessionId":"`+session+`","model":"claude-opus-5-5","cwd":"/work","permissionMode":"default"}`+"\n")
			_, _ = io.WriteString(w, `{"type":"text","text":"Hello"}`+"\n")
			_, _ = io.WriteString(w, `{"type":"future_event","x":1}`+"\n")
			_, _ = io.WriteString(w, `{"type":"result","status":"completed","sessionId":"`+session+`"}`+"\n")
		case r.URL.Path == "/v1/sessions":
			_, _ = io.WriteString(w, `{"sessions":[{"sessionId":"`+session+`","title":"Fix the build"}]}`)
		default:
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		}
	}))
	t.Cleanup(server.Close)
	registry := NewRegistry()
	if err := registry.Add(NewHost("claude-code", server.URL, "gateway-secret", server.Client())); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	RegisterRoutes(e.Group("/api/v1/harness-frontend"), NewHandler(registry))
	return e, calls
}

func request(e *echo.Echo, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestRoutesAreAdminOnly(t *testing.T) {
	e, calls := fakeGateway(t)
	withUser(t, &ApiTypes.UserInfo{UserId: "u", Roles: []string{"user"}})
	for _, path := range []string{"/api/v1/harness-frontend", "/api/v1/harness-frontend/claude-code/sessions"} {
		if rec := request(e, http.MethodGet, path, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
	}
	if rec := request(e, http.MethodPost, "/api/v1/harness-frontend/claude-code/turns", `{"message":"hi"}`); rec.Code != http.StatusForbidden || len(*calls) != 0 {
		t.Fatalf("turn status=%d gateway calls=%d", rec.Code, len(*calls))
	}
}

func TestTurnRelaysHarnessEventsAsSSE(t *testing.T) {
	e, calls := fakeGateway(t)
	withUser(t, &ApiTypes.UserInfo{UserId: "admin", Roles: []string{"admin"}})
	rec := request(e, http.MethodPost, "/api/v1/harness-frontend/claude-code/turns", `{"message":"hi","session_id":"`+session+`","permission_mode":"acceptEdits"}`)
	if rec.Code != http.StatusOK || rec.Header().Get(echo.HeaderContentType) != "text/event-stream; charset=utf-8" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"event: session\n", "event: text\ndata: {\"type\":\"text\",\"text\":\"Hello\"}\n\n", "event: result\n"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, "future_event") {
		t.Fatal("unknown event type relayed")
	}
	call := (*calls)[0]
	if call.path != "/v1/turns" || call.auth != "Bearer gateway-secret" || call.body["message"] != "hi" || call.body["sessionId"] != session || call.body["permissionMode"] != "acceptEdits" {
		t.Fatalf("gateway call %+v", call)
	}
}

func TestTurnValidatesInputAndRelaysGatewayRefusal(t *testing.T) {
	e, calls := fakeGateway(t)
	withUser(t, &ApiTypes.UserInfo{UserId: "admin", IsOwner: true})
	for _, body := range []string{`{"message":""}`, `{"message":"hi","session_id":"../x"}`, `{"message":"hi","permission_mode":"bypassPermissions"}`, `{"message":"hi","extra":1}`} {
		if rec := request(e, http.MethodPost, "/api/v1/harness-frontend/claude-code/turns", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d", body, rec.Code)
		}
	}
	if len(*calls) != 0 {
		t.Fatal("invalid turn reached the gateway")
	}
	rec := request(e, http.MethodPost, "/api/v1/harness-frontend/claude-code/turns", `{"message":"hi","session_id":"99999999-2222-3333-4444-555555555555"}`)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "session not found") {
		t.Fatalf("refusal status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := request(e, http.MethodGet, "/api/v1/harness-frontend/codex/sessions", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown harness status=%d", rec.Code)
	}
}

func TestSessionsInterruptAndDecisionsReachTheGateway(t *testing.T) {
	e, calls := fakeGateway(t)
	withUser(t, &ApiTypes.UserInfo{UserId: "admin", Admin: true})
	if rec := request(e, http.MethodGet, "/api/v1/harness-frontend/claude-code/sessions?limit=5", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Fix the build") {
		t.Fatalf("sessions status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := request(e, http.MethodGet, "/api/v1/harness-frontend/claude-code/sessions/"+session+"/messages", ""); rec.Code != http.StatusOK {
		t.Fatalf("messages status=%d", rec.Code)
	}
	if rec := request(e, http.MethodPost, "/api/v1/harness-frontend/claude-code/sessions/"+session+"/interrupt", ""); rec.Code != http.StatusOK {
		t.Fatalf("interrupt status=%d", rec.Code)
	}
	decision := `{"behavior":"allow","updated_input":{"answers":{"Which DB?":"Postgres"}}}`
	if rec := request(e, http.MethodPost, "/api/v1/harness-frontend/claude-code/sessions/"+session+"/permissions/toolu_q1", decision); rec.Code != http.StatusOK {
		t.Fatalf("decision status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := request(e, http.MethodPost, "/api/v1/harness-frontend/claude-code/sessions/"+session+"/permissions/toolu_q1", `{"behavior":"maybe"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad decision status=%d", rec.Code)
	}
	paths := []string{}
	for _, call := range *calls {
		paths = append(paths, call.method+" "+call.path)
	}
	want := "GET /v1/sessions?limit=5&offset=0,GET /v1/sessions/" + session + "/messages?limit=200&offset=0,POST /v1/sessions/" + session + "/interrupt,POST /v1/sessions/" + session + "/permissions/toolu_q1"
	if strings.Join(paths, ",") != want {
		t.Fatalf("gateway paths\n got %s\nwant %s", strings.Join(paths, ","), want)
	}
	last := (*calls)[len(*calls)-1].body
	if last["behavior"] != "allow" || last["updatedInput"].(map[string]any)["answers"] == nil {
		t.Fatalf("decision body %v", last)
	}
}
