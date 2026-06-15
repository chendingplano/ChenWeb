package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newMetricWikiContext(t *testing.T, metricID, lang string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/metrics/" + metricID + "/wiki"
	if lang != "" {
		target += "?lang=" + lang
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("metric_id")
	c.SetParamValues(metricID)
	return c, rec
}

func TestGetMetricWikiCacheHit(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	recordDir := filepath.Join(artifactDir, "0", "5")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pageJSON := `{"metric_id":"5_mtc_3","title":"Switching Frequency"}`
	if err := os.WriteFile(filepath.Join(recordDir, "wikipage_metric_5_mtc_3.en.json"), []byte(pageJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	c, rec := newMetricWikiContext(t, "5_mtc_3", "")
	if err := GetMetricWiki(c); err != nil {
		t.Fatalf("GetMetricWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp metricWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Status || resp.Generated {
		t.Errorf("resp status=%v generated=%v, want true/false", resp.Status, resp.Generated)
	}
	if string(resp.Page) != pageJSON {
		t.Errorf("page = %s, want %s", resp.Page, pageJSON)
	}
}

func TestGetMetricWikiBadID(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	c, rec := newMetricWikiContext(t, "not-a-metric", "")
	if err := GetMetricWiki(c); err != nil {
		t.Fatalf("GetMetricWiki err = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetMetricWikiCacheMissGenerates(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	if err := os.MkdirAll(filepath.Join(artifactDir, "0", "5"), 0o755); err != nil {
		t.Fatal(err)
	}

	generated := []byte(`{"metric_id":"5_mtc_3","title":"Generated"}`)
	orig := buildMetricWikiPageFn
	buildMetricWikiPageFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, recordID int64, metricID, lang string) (json.RawMessage, error) {
		return json.RawMessage(generated), nil
	}
	defer func() { buildMetricWikiPageFn = orig }()

	c, rec := newMetricWikiContext(t, "5_mtc_3", "")
	if err := GetMetricWiki(c); err != nil {
		t.Fatalf("GetMetricWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp metricWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Status || !resp.Generated {
		t.Errorf("resp status=%v generated=%v, want true/true", resp.Status, resp.Generated)
	}
	// The page must have been persisted to the cache path.
	saved, err := os.ReadFile(filepath.Join(artifactDir, "0", "5", "wikipage_metric_5_mtc_3.en.json"))
	if err != nil {
		t.Fatalf("page not saved: %v", err)
	}
	if string(saved) != string(generated) {
		t.Errorf("saved = %s, want %s", saved, generated)
	}
}

func TestGetMetricWikiLegacyIDNormalizesToCanonicalMetricID(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	if err := os.MkdirAll(filepath.Join(artifactDir, "0", "5"), 0o755); err != nil {
		t.Fatal(err)
	}

	generated := []byte(`{"metric_id":"5_mtc_3","title":"Generated"}`)
	orig := buildMetricWikiPageFn
	buildMetricWikiPageFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, recordID int64, metricID, lang string) (json.RawMessage, error) {
		if recordID != 5 {
			t.Fatalf("recordID=%d, want 5", recordID)
		}
		if metricID != "5_mtc_3" {
			t.Fatalf("metricID=%q, want canonical 5_mtc_3", metricID)
		}
		return json.RawMessage(generated), nil
	}
	defer func() { buildMetricWikiPageFn = orig }()

	c, rec := newMetricWikiContext(t, "5_3", "")
	if err := GetMetricWiki(c); err != nil {
		t.Fatalf("GetMetricWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	saved, err := os.ReadFile(filepath.Join(artifactDir, "0", "5", "wikipage_metric_5_mtc_3.en.json"))
	if err != nil {
		t.Fatalf("canonical page not saved: %v", err)
	}
	if string(saved) != string(generated) {
		t.Errorf("saved = %s, want %s", saved, generated)
	}
}

func TestGetMetricWikiSingleFlight(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	if err := os.MkdirAll(filepath.Join(artifactDir, "0", "5"), 0o755); err != nil {
		t.Fatal(err)
	}

	var calls int32
	orig := buildMetricWikiPageFn
	buildMetricWikiPageFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, recordID int64, metricID, lang string) (json.RawMessage, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond) // widen the race window
		return json.RawMessage(`{"metric_id":"5_mtc_3"}`), nil
	}
	defer func() { buildMetricWikiPageFn = orig }()

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			c, _ := newMetricWikiContext(t, "5_mtc_3", "")
			_ = GetMetricWiki(c)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("generator called %d times, want 1 (single-flight)", got)
	}
}

func TestParseMetricID(t *testing.T) {
	rid, seq, err := parseMetricID("5_3")
	if err != nil || rid != 5 || seq != 3 {
		t.Fatalf("parseMetricID(\"5_3\") = (%d, %d, %v), want (5, 3, nil)", rid, seq, err)
	}

	rid, seq, err = parseMetricID("5_mtc_3")
	if err != nil || rid != 5 || seq != 3 {
		t.Fatalf("parseMetricID(\"5_mtc_3\") = (%d, %d, %v), want (5, 3, nil)", rid, seq, err)
	}

	// record ids and seqnos can be multi-digit.
	rid, seq, err = parseMetricID("1234_57")
	if err != nil || rid != 1234 || seq != 57 {
		t.Fatalf("parseMetricID(\"1234_57\") = (%d, %d, %v), want (1234, 57, nil)", rid, seq, err)
	}

	rid, seq, err = parseMetricID("1234_mtc_57")
	if err != nil || rid != 1234 || seq != 57 {
		t.Fatalf("parseMetricID(\"1234_mtc_57\") = (%d, %d, %v), want (1234, 57, nil)", rid, seq, err)
	}

	for _, bad := range []string{
		"",      // empty
		"5",     // no seqno
		"5_",    // empty seqno
		"_3",    // empty record id
		"5_0",   // seqno must be >= 1
		"0_3",   // record id must be >= 1
		"x_3",   // non-numeric record id
		"5_x",   // non-numeric seqno
		"5_3_1", // too many parts
		"5__3",  // empty middle
		" 5_3",  // surrounding space is not trimmed by the parser
		"-5_3",  // negative
	} {
		if rid, seq, err := parseMetricID(bad); err == nil {
			t.Errorf("parseMetricID(%q) = (%d, %d, nil), want error", bad, rid, seq)
		}
	}
}

func TestMetricWikiPath(t *testing.T) {
	artifactDir := t.TempDir()
	// Create the canonical record dir so resolveRecordArtifactDir finds it.
	recordDir := filepath.Join(artifactDir, "0", "5")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Default language is English when lang is empty.
	got, err := metricWikiPath(artifactDir, 5, "5_3", "")
	if err != nil {
		t.Fatalf("metricWikiPath default lang err = %v", err)
	}
	want := filepath.Join(recordDir, "wikipage_metric_5_3.en.json")
	if got != want {
		t.Errorf("metricWikiPath default lang = %q, want %q", got, want)
	}

	// Explicit zh-cn.
	got, err = metricWikiPath(artifactDir, 5, "5_3", "zh-cn")
	if err != nil {
		t.Fatalf("metricWikiPath zh-cn err = %v", err)
	}
	want = filepath.Join(recordDir, "wikipage_metric_5_3.zh-cn.json")
	if got != want {
		t.Errorf("metricWikiPath zh-cn = %q, want %q", got, want)
	}

	// group_id is floor(record_id/1000): record 1234 lives under "1".
	if err := os.MkdirAll(filepath.Join(artifactDir, "1", "1234"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = metricWikiPath(artifactDir, 1234, "1234_2", "en")
	if err != nil {
		t.Fatalf("metricWikiPath group err = %v", err)
	}
	want = filepath.Join(artifactDir, "1", "1234", "wikipage_metric_1234_2.en.json")
	if got != want {
		t.Errorf("metricWikiPath group = %q, want %q", got, want)
	}

	// Unknown language is rejected.
	if _, err := metricWikiPath(artifactDir, 5, "5_3", "fr"); err == nil {
		t.Error("metricWikiPath(lang=fr) = nil error, want error")
	}
	// Empty ARTIFACT_DIR is rejected.
	if _, err := metricWikiPath("", 5, "5_3", "en"); err == nil {
		t.Error("metricWikiPath(artifactDir=\"\") = nil error, want error")
	}
}
