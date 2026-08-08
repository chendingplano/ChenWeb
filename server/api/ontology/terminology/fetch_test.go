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
	"sync"
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
		if got := r.URL.Query().Get("ids"); got != "Q221656|Q355386" {
			t.Errorf("ids = %q", got)
		}
		if got := r.URL.Query().Get("props"); got != "info|labels|aliases|claims" {
			t.Errorf("props = %q, want info|labels|aliases|claims", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entities":{
			"Q355386":{"id":"Q355386","lastrevid":2490316763,"labels":{"en":{"language":"en","value":"luminance"},"zh":{"language":"zh","value":"亮度"}},"aliases":{"en":[{"language":"en","value":"luminous density"}]},"claims":{"P646":[{"mainsnak":{"snaktype":"value","datatype":"external-id","datavalue":{"value":"Luminance"}}}],"P1889":[{"mainsnak":{"snaktype":"value","datatype":"wikibase-item","datavalue":{"value":{"id":"Q358951"}}}}],"P279":[{"mainsnak":{"snaktype":"value","datatype":"wikibase-item","datavalue":{"value":{"id":"Q11425"}}}}]}},
			"Q221656":{"id":"Q221656","lastrevid":2490000001}
		},"success":1}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	st, err := Fetch(context.Background(), ResourceWikidata, dir,
		WithURLOverride(ResourceWikidata, srv.URL),
		WithWikidataIDs([]string{"Q355386", "Q221656"}),
		WithNow(now))
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
	got := map[string]WikidataLine{}
	for _, ln := range lines {
		var e WikidataLine
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatalf("decode line %q: %v", ln, err)
		}
		got[e.ID] = e
	}
	// Lines must be the normalized adapter schema: revision-pinned, flat
	// labels/aliases, external ids and statements projected from claims.
	main, ok := got["Q355386"]
	if !ok || main.Revision != 2490316763 {
		t.Fatalf("Q355386 line = %+v, want revision 2490316763", main)
	}
	if main.Labels["en"] != "luminance" || main.Labels["zh"] != "亮度" {
		t.Fatalf("Q355386 labels = %+v", main.Labels)
	}
	if len(main.Aliases["en"]) != 1 || main.Aliases["en"][0] != "luminous density" {
		t.Fatalf("Q355386 aliases = %+v", main.Aliases)
	}
	if len(main.ExternalIDs) != 1 || main.ExternalIDs[0] != (ExternalIDClaim{Property: "P646", Value: "Luminance"}) {
		t.Fatalf("Q355386 external_ids = %+v", main.ExternalIDs)
	}
	wantStatements := []WikidataStatement{{Type: "broader", Object: "Q11425"}, {Type: "different_from", Object: "Q358951"}}
	if len(main.Statements) != len(wantStatements) {
		t.Fatalf("Q355386 statements = %+v, want %+v", main.Statements, wantStatements)
	}
	for i := range wantStatements {
		if main.Statements[i] != wantStatements[i] {
			t.Fatalf("Q355386 statements = %+v, want %+v", main.Statements, wantStatements)
		}
	}
	if !main.RetrievedAt.Equal(now) {
		t.Fatalf("Q355386 retrieved_at = %v, want %v", main.RetrievedAt, now)
	}
	other, ok := got["Q221656"]
	if !ok || other.Revision != 2490000001 || len(other.Labels) != 0 {
		t.Fatalf("Q221656 line = %+v", other)
	}
}

func TestFetchWikidataAPIErrorFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"info":"no such entity id: Q999999"}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	_, err := Fetch(context.Background(), ResourceWikidata, dir,
		WithURLOverride(ResourceWikidata, srv.URL), WithWikidataIDs([]string{"Q999999"}))
	if err == nil || !strings.Contains(err.Error(), "wikidata API error") {
		t.Fatalf("err = %v, want wikidata API error", err)
	}
	// Failed fetch must persist a not-downloaded status with the error.
	st, rerr := ReadStatus(dir, ResourceWikidata)
	if rerr != nil || st.Downloaded || !strings.Contains(st.Error, "wikidata API error") {
		t.Fatalf("status = %+v err=%v", st, rerr)
	}
}

func TestFetchWikidataBatchesAndSkipsMissing(t *testing.T) {
	var (
		mu      sync.Mutex
		batches [][]string
	)
	ids := make([]string, 0, 120)
	for i := 1; i <= 120; i++ {
		ids = append(ids, fmt.Sprintf("Q%d", i))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested := strings.Split(r.URL.Query().Get("ids"), "|")
		mu.Lock()
		batches = append(batches, requested)
		mu.Unlock()
		if len(requested) > wikidataBatchSize {
			t.Errorf("batch has %d ids, want <= %d", len(requested), wikidataBatchSize)
		}
		entities := map[string]any{}
		for _, id := range requested {
			if id == "Q50" {
				entities[id] = map[string]any{"id": id, "missing": ""}
				continue
			}
			entities[id] = map[string]any{"id": id, "lastrevid": 1234500}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"entities": entities, "success": 1})
	}))
	defer srv.Close()

	dir := t.TempDir()
	st, err := Fetch(context.Background(), ResourceWikidata, dir,
		WithURLOverride(ResourceWikidata, srv.URL), WithWikidataIDs(ids),
		WithNow(time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("Fetch wikidata: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 3 {
		t.Fatalf("requests = %d, want 3 (50+50+20)", len(batches))
	}
	b, err := os.ReadFile(filepath.Join(dir, string(ResourceWikidata), "snapshot.jsonl"))
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 119 {
		t.Fatalf("snapshot lines = %d, want 119 (Q50 missing)", len(lines))
	}
	previous := ""
	for _, ln := range lines {
		var e WikidataLine
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatalf("decode line %q: %v", ln, err)
		}
		if e.ID <= previous {
			t.Fatalf("lines not sorted by id: %q after %q", e.ID, previous)
		}
		previous = e.ID
	}
	if strings.Contains(string(b), "Q50") {
		t.Fatalf("snapshot must skip the missing entity, got %s", string(b))
	}
	t.Logf("last line: %s", lines[len(lines)-1])
	if st.SHA256 == "" || st.SizeBytes != int64(len(b)) {
		t.Fatalf("status = %+v", st)
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

func TestFetchSIRPRequestsTurtleAndWritesTTLArtifact(t *testing.T) {
	payload := "@prefix si: <https://si-digital-framework.org/SI#> .\n" +
		"<https://si-digital-framework.org/quantities/LUMA> a si:QuantityKind ;" +
		" skos:prefLabel \"luminance\"@en .\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "text/turtle" {
			t.Errorf("Accept = %q, want text/turtle", got)
		}
		w.Header().Set("Content-Type", "text/turtle")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	dir := t.TempDir()
	st, err := Fetch(context.Background(), ResourceSIRP, dir, WithURLOverride(ResourceSIRP, srv.URL))
	if err != nil {
		t.Fatalf("Fetch sirp: %v", err)
	}
	if st.Artifact != "quantities.ttl" || st.SHA256 != sha256Hex(payload) {
		t.Fatalf("status = %+v, want quantities.ttl artifact", st)
	}
	if _, err := os.Stat(filepath.Join(dir, string(ResourceSIRP), "quantities.ttl")); err != nil {
		t.Fatalf("quantities.ttl artifact missing: %v", err)
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
