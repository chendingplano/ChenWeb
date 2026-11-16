package datasync

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchChangesPageFromSourceAllowsSlowBody(t *testing.T) {
	// Shorten the discovery client's timeout to reproduce a transfer exceeding
	// its budget without making the test wait for the production 30 seconds.
	original := httpClient
	httpClient = &http.Client{Timeout: 100 * time.Millisecond}
	t.Cleanup(func() { httpClient = original })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"item_id":"slow","rows":[`)
		w.(http.Flusher).Flush()
		select {
		case <-time.After(300 * time.Millisecond):
			_, _ = io.WriteString(w, `{"name":"complete"}],"next_cursor":"2026-11-16T00:00:00Z","has_more":false}`)
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	page, err := fetchChangesPageFromSource(ctx, srv.URL, "test-secret", TableSyncItem{ID: "slow"}, "")
	if err != nil {
		t.Fatalf("fetch slow changes page: %v", err)
	}
	if len(page.Rows) != 1 || string(page.Rows[0]["name"]) != `"complete"` || page.NextCursor != "2026-11-16T00:00:00Z" {
		t.Fatalf("incomplete changes page: %+v", page)
	}
}

func TestFetchChangesPageFromSourceHonorsContextDuringBodyRead(t *testing.T) {
	bodyStarted := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"rows":[`)
		w.(http.Flusher).Flush()
		close(bodyStarted)
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		select {
		case <-bodyStarted:
			cancel()
		case <-ctx.Done():
		}
	}()
	_, err := fetchChangesPageFromSource(ctx, srv.URL, "test-secret", TableSyncItem{ID: "cancel"}, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fetch canceled page error = %v, want context.Canceled", err)
	}
}
