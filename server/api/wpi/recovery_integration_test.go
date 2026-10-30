package wpi

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
)

// Runs only against an explicitly prepared disposable PostgreSQL database.
func TestPageArchiveDatabase(t *testing.T) {
	if os.Getenv("WPI_ARCHIVE_TEST_DB") != "1" {
		t.Skip("requires a disposable archive test database")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	s := store{db}
	for i, body := range []string{`{"descriptor_id":"test-page","value":1}`, `{"descriptor_id":"test-page","value":2}`} {
		version, err := s.savePage(ctx, "test-page", []byte(body), "editor", "", false)
		if err != nil || version != i+1 {
			t.Fatalf("save: %d %v", version, err)
		}
	}
	var version, value int
	if err := db.QueryRow(`SELECT version, (body->>'value')::int FROM archive.frontend_pages WHERE descriptor_id = 'test-page' AND NOT recovery`).Scan(&version, &value); err != nil || version != 1 || value != 1 {
		t.Fatalf("archive: %d %d %v", version, value, err)
	}
	h := testHandler(db, adminUser)
	rec := call(t, h.PageRecovery, "PUT", `{"version":2,"page":{"descriptor_id":"test-page","value":3}}`, "pageId", "test-page")
	if rec.Code != 200 {
		t.Fatalf("recovery save: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, h.PageRecovery, "GET", "", "pageId", "test-page")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"value":3`) {
		t.Fatalf("recovery load: %d %s", rec.Code, rec.Body)
	}
	if _, err := s.savePage(ctx, "test-page", []byte(`{"descriptor_id":"test-page","value":4}`), "admin@example.com", "", false); err != nil {
		t.Fatal(err)
	}
	rec = call(t, h.PageRecovery, "GET", "", "pageId", "test-page")
	if rec.Body.String() != "{\"page\":null}\n" {
		t.Fatalf("recovery not removed: %s", rec.Body)
	}
	rec = call(t, h.PageRecovery, "PUT", `{"version":2,"page":{"descriptor_id":"test-page"}}`, "pageId", "test-page")
	if rec.Code != 409 {
		t.Fatalf("stale recovery: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, h.PageRecovery, "PUT", `{"version":0,"page":{"descriptor_id":"unsaved-123","value":5}}`, "pageId", "unsaved-123")
	if rec.Code != 200 {
		t.Fatalf("new page recovery: %d %s", rec.Code, rec.Body)
	}
	if _, err := s.savePage(ctx, "new-page", []byte(`{"descriptor_id":"new-page"}`), "admin@example.com", "unsaved-123", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.savePage(ctx, "new-page", []byte(`{"descriptor_id":"new-page"}`), "admin@example.com", "", true); err != sql.ErrNoRows {
		t.Fatalf("duplicate create: %v", err)
	}
	rec = call(t, h.PageRecovery, "GET", "", "pageId", "unsaved-123")
	if rec.Body.String() != "{\"page\":null}\n" {
		t.Fatalf("new page recovery not removed: %s", rec.Body)
	}
}
