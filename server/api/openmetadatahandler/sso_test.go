package openmetadatahandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureOpenMetadataUserSkipsCreateWhenUserExists(t *testing.T) {
	createCalled := false
	var gotAuthHeader string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/users" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"om-user-1","email":"alice@example.com"}],"paging":{"total":1}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/users" {
			createCalled = true
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	user := &CurrentUser{
		UserID:      "cw-1",
		Email:       "alice@example.com",
		DisplayName: "Alice",
	}
	err := EnsureOpenMetadataUser(t.Context(), "admin-token", upstream.URL, user)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if createCalled {
		t.Fatal("expected user create not to be called when user already exists")
	}
	if gotAuthHeader != "Bearer admin-token" {
		t.Fatalf("expected admin bearer token, got %q", gotAuthHeader)
	}
}

func TestEnsureOpenMetadataUserCreatesUserWhenNotFound(t *testing.T) {
	var createBody map[string]any
	var createCalled bool

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/users" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[],"paging":{"total":0}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/users" {
			createCalled = true
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	user := &CurrentUser{
		UserID:      "cw-2",
		Email:       "bob@example.com",
		DisplayName: "Bob Smith",
	}
	err := EnsureOpenMetadataUser(t.Context(), "admin-token", upstream.URL, user)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !createCalled {
		t.Fatal("expected create user to be called")
	}
	if createBody["email"] != "bob@example.com" {
		t.Fatalf("expected email bob@example.com in create body, got %v", createBody["email"])
	}
	if createBody["name"] != "bob" {
		t.Fatalf("expected name bob (local part of email), got %v", createBody["name"])
	}
	if createBody["displayName"] != "Bob Smith" {
		t.Fatalf("expected displayName Bob Smith, got %v", createBody["displayName"])
	}
	if createBody["isAdmin"] != false {
		t.Fatalf("expected isAdmin false, got %v", createBody["isAdmin"])
	}
}

func TestEnsureOpenMetadataUserReturnsErrorOnLookupFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	user := &CurrentUser{UserID: "cw-3", Email: "carol@example.com", DisplayName: "Carol"}
	err := EnsureOpenMetadataUser(t.Context(), "admin-token", upstream.URL, user)
	if err == nil {
		t.Fatal("expected error on upstream failure, got nil")
	}
}

func TestEnsureOpenMetadataUserReturnsErrorOnCreateFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[],"paging":{"total":0}}`))
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer upstream.Close()

	user := &CurrentUser{UserID: "cw-4", Email: "dave@example.com", DisplayName: "Dave"}
	err := EnsureOpenMetadataUser(t.Context(), "admin-token", upstream.URL, user)
	if err == nil {
		t.Fatal("expected error on create failure, got nil")
	}
}

func TestEmailLocalPart(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"alice@example.com", "alice"},
		{"bob.smith@corp.io", "bob.smith"},
		{"noatsign", "noatsign"},
		{"@empty-local", ""},
	}
	for _, c := range cases {
		got := emailLocalPart(c.input)
		if got != c.want {
			t.Errorf("emailLocalPart(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestProvisionCacheMarksAndDetectsProvisioned(t *testing.T) {
	c := newProvisionCache()
	if c.isProvisioned("user-1") {
		t.Fatal("expected user-1 to not be provisioned initially")
	}
	c.markProvisioned("user-1")
	if !c.isProvisioned("user-1") {
		t.Fatal("expected user-1 to be provisioned after marking")
	}
	if c.isProvisioned("user-2") {
		t.Fatal("expected user-2 to not be provisioned")
	}
}
