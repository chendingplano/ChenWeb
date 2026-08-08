package terminology

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

func TestResourcesCatalog(t *testing.T) {
	got := Resources()
	if len(got) != 5 {
		t.Fatalf("Resources() = %d entries, want 5", len(got))
	}
	ids := map[ResourceID]bool{}
	for _, r := range got {
		if r.ID == "" || r.Name == "" || r.URL == "" || r.License == "" {
			t.Fatalf("resource %+v has empty required field", r)
		}
		if r.Downloadable && (r.Artifact == "" || r.MediaType == "" || r.MaxBytes <= 0 || r.Adapter == "") {
			t.Fatalf("downloadable resource %s missing artifact/media/maxbytes/adapter: %+v", r.ID, r)
		}
		ids[r.ID] = true
	}
	for _, id := range []ResourceID{ResourceQUDT, ResourceUCUM, ResourceSIRP, ResourceWikidata, ResourceIEC} {
		if !ids[id] {
			t.Fatalf("catalog missing %s", id)
		}
	}
	iec, ok := ResourceByID(ResourceIEC)
	if !ok || iec.Downloadable || !iec.PermissionRequired {
		t.Fatalf("IEC resource must be permission-gated, got %+v", iec)
	}
}

func TestFetchWritesArtifactStatusAndDraftManifest(t *testing.T) {
	payload := "@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		w.Header().Set("Content-Type", "text/turtle")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	dir := t.TempDir()
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	st, err := Fetch(context.Background(), ResourceQUDT, dir,
		WithURLOverride(ResourceQUDT, srv.URL), WithNow(now))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !st.Downloaded || st.Source != string(ResourceQUDT) || st.Release != "3.5.0" {
		t.Fatalf("status = %+v", st)
	}
	if st.SHA256 != sha256Hex(payload) || st.SizeBytes != int64(len(payload)) {
		t.Fatalf("sha/size mismatch: %+v", st)
	}
	if st.SourceURL != srv.URL || st.ManifestDraft != "manifest.draft.json" || st.Artifact != "qudt-all.ttl" {
		t.Fatalf("status fields = %+v", st)
	}

	artifact, err := os.ReadFile(filepath.Join(dir, string(ResourceQUDT), "qudt-all.ttl"))
	if err != nil || string(artifact) != payload {
		t.Fatalf("artifact = %q err=%v", artifact, err)
	}

	var draft Manifest
	b, err := os.ReadFile(filepath.Join(dir, string(ResourceQUDT), "manifest.draft.json"))
	if err != nil {
		t.Fatalf("read draft manifest: %v", err)
	}
	if err := json.Unmarshal(b, &draft); err != nil {
		t.Fatalf("draft manifest decode: %v", err)
	}
	if draft.Policy.LicenseReviewStatus != LicenseReviewPending {
		t.Fatalf("draft license_review_status = %q, want pending_review", draft.Policy.LicenseReviewStatus)
	}
	if draft.Policy.ApprovedBy != "" || draft.Policy.ApprovedAt != nil {
		t.Fatalf("draft must not be pre-approved: %+v", draft.Policy)
	}
	if draft.Policy.ContentChecksum != st.SHA256 || draft.Artifacts[0].SHA256 != st.SHA256 {
		t.Fatalf("draft checksum mismatch: %+v", draft)
	}
	// The draft must be rejected by import validation until approved.
	if err := draft.Policy.SourcePolicy().Validate(); err == nil {
		t.Fatal("pending draft must fail source policy validation")
	}

	got, err := ReadStatus(dir, ResourceQUDT)
	if err != nil || got.SHA256 != st.SHA256 {
		t.Fatalf("ReadStatus = %+v err=%v", got, err)
	}
}

func TestFetchWikidataWritesJSONLSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("action"); got != "wbgetentities" {
			t.Errorf("action = %q", got)
		}
		if got := r.URL.Query().Get("titles"); got != "Luminance|Brightness" {
			t.Errorf("titles = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entities":{"Q355386":{"id":"Q355386","lastrevid":2490316763,"labels":{"en":{"language":"en","value":"luminance"}}},"Q221656":{"id":"Q221656","lastrevid":2490000001}},"success":1}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	st, err := Fetch(context.Background(), ResourceWikidata, dir,
		WithURLOverride(ResourceWikidata, srv.URL),
		WithWikidataTitles([]string{"Luminance", "Brightness"}),
		WithNow(time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Fetch wikidata: %v", err)
	}
	if st.Release != "dump-2026-08-07" {
		t.Fatalf("wikidata release = %q, want dump-2026-08-07", st.Release)
	}
	b, err := os.ReadFile(filepath.Join(dir, string(ResourceWikidata), "snapshot.jsonl"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d: %s", len(lines), b)
	}
	type entityLine struct {
		ID   string `json:"id"`
		Last int64  `json:"lastrevid"`
	}
	got := map[string]int64{}
	for _, ln := range lines {
		var e entityLine
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatalf("decode line %q: %v", ln, err)
		}
		got[e.ID] = e.Last
	}
	if got["Q355386"] != 2490316763 || got["Q221656"] != 2490000001 {
		t.Fatalf("entities = %+v", got)
	}
}

func TestFetchWikidataAPIErrorFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"info":"no such title: LuminanceXYZ"}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	_, err := Fetch(context.Background(), ResourceWikidata, dir,
		WithURLOverride(ResourceWikidata, srv.URL), WithWikidataTitles([]string{"LuminanceXYZ"}))
	if err == nil || !strings.Contains(err.Error(), "wikidata API error") {
		t.Fatalf("err = %v, want wikidata API error", err)
	}
	// Failed fetch must persist a not-downloaded status with the error.
	st, rerr := ReadStatus(dir, ResourceWikidata)
	if rerr != nil || st.Downloaded || !strings.Contains(st.Error, "wikidata API error") {
		t.Fatalf("status = %+v err=%v", st, rerr)
	}
}

func TestFetchIECFailsClosed(t *testing.T) {
	dir := t.TempDir()
	_, err := Fetch(context.Background(), ResourceIEC, dir)
	if err == nil || !strings.Contains(err.Error(), "requires permission") {
		t.Fatalf("err = %v, want permission error", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, string(ResourceIEC))); !os.IsNotExist(statErr) {
		t.Fatal("IEC fetch must not create a source directory")
	}
}

func TestFetchHTTPErrorPersistsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	dir := t.TempDir()
	_, err := Fetch(context.Background(), ResourceUCUM, dir, WithURLOverride(ResourceUCUM, srv.URL))
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	st, rerr := ReadStatus(dir, ResourceUCUM)
	if rerr != nil || st.Downloaded || st.Error == "" {
		t.Fatalf("status = %+v err=%v", st, rerr)
	}
}

func TestFetchReportsProgressWhileDownloading(t *testing.T) {
	chunks := [][]byte{
		bytes.Repeat([]byte("a"), 4096),
		bytes.Repeat([]byte("b"), 4096),
		bytes.Repeat([]byte("c"), 4096),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/turtle")
		w.Header().Set("Content-Length", fmt.Sprint(len(chunks[0])+len(chunks[1])+len(chunks[2])))
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		for _, c := range chunks {
			_, _ = w.Write(c)
			fl.Flush()
			time.Sleep(150 * time.Millisecond)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	type result struct {
		st  FetchStatus
		err error
	}
	done := make(chan result, 1)
	go func() {
		st, err := Fetch(context.Background(), ResourceQUDT, dir, WithURLOverride(ResourceQUDT, srv.URL))
		done <- result{st: st, err: err}
	}()

	// The server persists a "downloading" status with bytes so far while the
	// artifact still streams; poll until we observe one mid-download.
	var sawProgress bool
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, err := ReadStatus(dir, ResourceQUDT)
		if err == nil && st.Downloading && st.DownloadedBytes > 0 && st.TotalBytes > 0 {
			sawProgress = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sawProgress {
		t.Fatal("did not observe a downloading progress status during fetch")
	}

	got := <-done
	if got.err != nil {
		t.Fatalf("Fetch: %v", got.err)
	}
	wantSize := int64(len(chunks[0]) + len(chunks[1]) + len(chunks[2]))
	if !got.st.Downloaded || got.st.SizeBytes != wantSize || got.st.Downloading {
		t.Fatalf("final status = %+v, want downloaded size %d with downloading=false", got.st, wantSize)
	}
}

func TestReadStatusAbsentReturnsNotDownloaded(t *testing.T) {
	st, err := ReadStatus(t.TempDir(), ResourceSIRP)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if st.Downloaded || st.Source != string(ResourceSIRP) {
		t.Fatalf("st = %+v", st)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestDraftReviewStatus(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, string(ResourceUCUM))
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// No draft yet -> no review status.
	if got, err := DraftReviewStatus(dir, ResourceUCUM); err != nil || got != "" {
		t.Fatalf("no draft: got=%q err=%v", got, err)
	}

	// Fresh fetch writes a pending draft.
	pending := `{"adapter":"ucum","policy":{"license_review_status":"pending_review"},"artifacts":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(pending), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := DraftReviewStatus(dir, ResourceUCUM); err != nil || got != LicenseReviewPending {
		t.Fatalf("pending draft: got=%q err=%v", got, err)
	}

	// Operator approval edits the draft in place.
	approved := `{"adapter":"ucum","policy":{"license_review_status":"approved","approved_by":"ontology-board"},"artifacts":[]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(approved), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := DraftReviewStatus(dir, ResourceUCUM); err != nil || got != "approved" {
		t.Fatalf("approved draft: got=%q err=%v", got, err)
	}

	// Malformed draft surfaces an error so the handler fails visibly.
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DraftReviewStatus(dir, ResourceUCUM); err == nil {
		t.Fatal("malformed draft must error")
	}
}

func TestApproveDraftWritesApprovalInPlace(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	// Build a downloaded source with a pending draft (as Fetch would).
	sourceDir := filepath.Join(dir, string(ResourceUCUM))
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"ucum","downloaded":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pending := `{"adapter":"ucum","policy":{"provider_id":"ucum","source":"ucum","source_subset":"essence","release":"2.2","retrieved_at":"2026-08-07T12:00:00Z","content_checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","license":"UCUM-License-1.1","license_review_status":"pending_review","authority_role":"context_only","allowed_scopes":["display"],"languages":["en"],"adapter_version":"0.1.0","provenance_locator":"https://example.test/essence.xml"},"artifacts":[{"id":"ucum-essence.xml","path":"ucum-essence.xml","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","media_type":"application/xml","provenance_locator":"https://example.test/essence.xml"}]}`
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(pending), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := DraftReviewStatus(dir, ResourceUCUM); got != LicenseReviewPending {
		t.Fatalf("review status before approve = %q", got)
	}

	st, err := ApproveDraft(dir, ResourceUCUM, "alice@example.test", "license and role confirmed against fixture", now)
	if err != nil {
		t.Fatalf("ApproveDraft: %v", err)
	}
	if !st.Downloaded {
		t.Fatalf("status lost download state: %+v", st)
	}
	b, err := os.ReadFile(filepath.Join(dir, string(ResourceUCUM), "manifest.draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Policy.LicenseReviewStatus != keywords.LicenseReviewApproved {
		t.Fatalf("license_review_status = %q", m.Policy.LicenseReviewStatus)
	}
	if m.Policy.ApprovedBy != "alice@example.test" || m.Policy.ApprovedAt == nil || !m.Policy.ApprovedAt.Equal(now) {
		t.Fatalf("approval fields = %+v", m.Policy)
	}
	if m.Policy.ReviewComments != "license and role confirmed against fixture" ||
		m.Policy.ReviewedBy != "alice@example.test" || m.Policy.ReviewedAt == nil || !m.Policy.ReviewedAt.Equal(now) {
		t.Fatalf("review fields = %+v", m.Policy)
	}
	if got, _ := DraftReviewStatus(dir, ResourceUCUM); got != keywords.LicenseReviewApproved {
		t.Fatalf("review status after approve = %q", got)
	}
	if got, _ := ReadDraftReview(dir, ResourceUCUM); got.Status != keywords.LicenseReviewApproved || got.Comments != m.Policy.ReviewComments {
		t.Fatalf("ReadDraftReview = %+v", got)
	}
}

func TestApproveDraftFailsClosed(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	if _, err := ApproveDraft(dir, ResourceSIRP, "alice", "", now); !errors.Is(err, ErrNotDownloaded) {
		t.Fatalf("not downloaded: err = %v", err)
	}
	if _, err := DisapproveDraft(dir, ResourceSIRP, "alice", "bad license", now); !errors.Is(err, ErrNotDownloaded) {
		t.Fatalf("disapprove not downloaded: err = %v", err)
	}

	// Downloaded but no draft (e.g. operator already moved it).
	sourceDir := filepath.Join(dir, string(ResourceSIRP))
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "status.json"), []byte(`{"source":"sirp","downloaded":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveDraft(dir, ResourceSIRP, "alice", "", now); !errors.Is(err, ErrNoDraftManifest) {
		t.Fatalf("no draft: err = %v", err)
	}

	// Already approved.
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(`{"adapter":"bipm-sirp-quantity","policy":{"license_review_status":"approved","approved_by":"bob","approved_at":"2026-08-07T12:00:00Z"},"artifacts":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveDraft(dir, ResourceSIRP, "alice", "", now); !errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("already approved: err = %v", err)
	}
	// Disapproving an approved draft fails closed too.
	if _, err := DisapproveDraft(dir, ResourceSIRP, "alice", "retract", now); !errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("disapprove already approved: err = %v", err)
	}

	// Reset to pending, then disapprove: comments and reviewer are saved.
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(`{"adapter":"bipm-sirp-quantity","policy":{"license_review_status":"pending_review"},"artifacts":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DisapproveDraft(dir, ResourceSIRP, "carol@example.test", "scope out of pilot; license unclear", now); err != nil {
		t.Fatalf("DisapproveDraft: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(sourceDir, "manifest.draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Policy.LicenseReviewStatus != LicenseReviewDisapproved {
		t.Fatalf("license_review_status = %q", m.Policy.LicenseReviewStatus)
	}
	if m.Policy.ReviewComments != "scope out of pilot; license unclear" || m.Policy.ReviewedBy != "carol@example.test" || m.Policy.ReviewedAt == nil {
		t.Fatalf("review fields = %+v", m.Policy)
	}
	if got, _ := DraftReviewStatus(dir, ResourceSIRP); got != LicenseReviewDisapproved {
		t.Fatalf("review status after disapprove = %q", got)
	}
	// Re-disapproving (or approving) a decided draft fails closed.
	if _, err := DisapproveDraft(dir, ResourceSIRP, "carol", "again", now); !errors.Is(err, ErrAlreadyDisapproved) {
		t.Fatalf("already disapproved: err = %v", err)
	}
	if _, err := ApproveDraft(dir, ResourceSIRP, "carol", "", now); !errors.Is(err, ErrAlreadyDisapproved) {
		t.Fatalf("approve already disapproved: err = %v", err)
	}

	// Missing reviewer/approver.
	if err := os.WriteFile(filepath.Join(sourceDir, "manifest.draft.json"), []byte(`{"adapter":"bipm-sirp-quantity","policy":{"license_review_status":"pending_review"},"artifacts":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveDraft(dir, ResourceSIRP, " ", "", now); err == nil {
		t.Fatal("empty approved_by must error")
	}
	if _, err := DisapproveDraft(dir, ResourceSIRP, " ", "no reviewer", now); err == nil {
		t.Fatal("empty reviewer must error")
	}
}
