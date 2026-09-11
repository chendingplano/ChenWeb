package chadsessionshandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestListSessionsReturnsNewestFirstSummaries(t *testing.T) {
	root := t.TempDir()
	writeSessionFixture(t, root, "older", 100, 2, "Older session", "older request")
	writeSessionFixture(t, root, "newer", 200, 1, "Newer session", "newer request")

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chad/sessions", nil)
	c := e.NewContext(req, rec)

	if err := New(root).ListSessions(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Sessions) != 2 {
		t.Fatalf("session count = %d, want 2", len(response.Sessions))
	}
	if response.Sessions[0].ID != "newer" || response.Sessions[1].ID != "older" {
		t.Fatalf("ids = %#v, want newer then older", response.Sessions)
	}
	if response.Sessions[0].MessageCount != 2 {
		t.Fatalf("newer message count = %d, want 2", response.Sessions[0].MessageCount)
	}
}

func TestGetSessionReturnsMessagesAndNotFoundForUnknownID(t *testing.T) {
	root := t.TempDir()
	writeSessionFixture(t, root, "session-a", 300, 2, "A session", "a request")
	h := New(root)

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chad/sessions/session-a", nil)
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/chad/sessions/:id")
	c.SetParamNames("id")
	c.SetParamValues("session-a")

	if err := h.GetSession(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var response SessionDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "session-a" || len(response.Messages) != 2 {
		t.Fatalf("detail = %#v, want session-a with 2 messages", response)
	}

	missingRec := httptest.NewRecorder()
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/chad/sessions/missing", nil)
	missing := e.NewContext(missingReq, missingRec)
	missing.SetPath("/api/v1/chad/sessions/:id")
	missing.SetParamNames("id")
	missing.SetParamValues("missing")
	if err := h.GetSession(missing); err != nil {
		t.Fatal(err)
	}
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want %d", missingRec.Code, http.StatusNotFound)
	}
}

func TestSessionIDRejectsTraversal(t *testing.T) {
	h := New(t.TempDir())
	for _, id := range []string{"", ".", "..", "a/b", `a\\b`} {
		e := echo.New()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/chad/sessions/"+id, nil)
		c := e.NewContext(req, rec)
		c.SetPath("/api/v1/chad/sessions/:id")
		c.SetParamNames("id")
		c.SetParamValues(id)
		if err := h.GetSession(c); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("id %q status = %d, want %d", id, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestListSessionsSkipsMalformedEntries(t *testing.T) {
	root := t.TempDir()
	writeSessionFixture(t, root, "valid", 400, 1, "Valid", "request")
	if err := os.Mkdir(filepath.Join(root, "broken"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken", "index.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := New(root).ListSessions(c); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].ID != "valid" {
		t.Fatalf("sessions = %#v, want only valid", response.Sessions)
	}
}

func TestGetSessionRejectsOversizedSessionFile(t *testing.T) {
	root := t.TempDir()
	writeSessionFixture(t, root, "large", 500, 1, "Large", "request")
	contentPath := filepath.Join(root, "large", "20260911-000000-large.json")
	if err := os.WriteFile(contentPath, make([]byte, maxSessionFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	c.SetPath("/api/v1/chad/sessions/:id")
	c.SetParamNames("id")
	c.SetParamValues("large")
	if err := New(root).GetSession(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func writeSessionFixture(t *testing.T, root, id string, updated float64, turns int, title, request string) {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	contentID := "20260911-000000-" + id
	index := map[string]any{"sessions": map[string]any{contentID: map[string]any{
		"title": title, "updated": updated, "turns": turns,
	}}}
	writeJSON(t, filepath.Join(dir, "index.json"), index)
	session := map[string]any{
		"cwd":        "/tmp/example",
		"session_id": contentID,
		"updated":    updated,
		"messages": []any{
			map[string]any{"role": "user", "content": request},
			map[string]any{"role": "assistant", "content": map[string]any{"answer": "response"}},
		},
	}
	writeJSON(t, filepath.Join(dir, contentID+".json"), session)
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
