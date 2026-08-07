package terminology

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testContentChecksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validTestPolicy() ManifestPolicy {
	approvedAt := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	return ManifestPolicy{
		ProviderID: "synthetic-provider", Source: "iec-seed", SourceSubset: "pilot",
		Release: "v1", RetrievedAt: approvedAt, ContentChecksum: testContentChecksum,
		License: "CC0-1.0", LicenseReviewStatus: "approved",
		AuthorityRole: "exact_identity_authority", AuthoritativeRelations: []string{"exact_equivalent"},
		AllowedScopes: []string{"display"}, Languages: []string{"en", "zh"},
		AdapterVersion: "0.1.0", ProvenanceLocator: "https://example.test/iec-seed/v1",
		ApprovedBy: "ontology-board", ApprovedAt: &approvedAt, IdentityAuthority: true,
	}
}

func writeTestArtifact(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func writeTestManifest(t *testing.T, dir string, mutate func(*Manifest)) (Manifest, string) {
	t.Helper()
	m := Manifest{
		Adapter: "synthetic",
		Policy:  validTestPolicy(),
		Artifacts: []ManifestArtifact{
			{ID: "seed.json", Path: "seed.json", SHA256: "", MediaType: "application/json", ProvenanceLocator: "https://example.test/seed.json"},
		},
	}
	content := []byte(`{"entries":[{"external_id":"845-21-050"}]}`)
	checksum := writeTestArtifact(t, dir, "seed.json", content)
	m.Artifacts[0].SHA256 = checksum
	if mutate != nil {
		mutate(&m)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return m, path
}

func TestParseAndVerifyManifestAcceptsValidManifest(t *testing.T) {
	dir := t.TempDir()
	manifest, path := writeTestManifest(t, dir, nil)

	got, artifacts, err := ParseAndVerifyManifest(path)
	if err != nil {
		t.Fatalf("ParseAndVerifyManifest: %v", err)
	}
	if !reflect.DeepEqual(got.Adapter, manifest.Adapter) || got.Policy.Release != "v1" {
		t.Fatalf("manifest=%+v", got)
	}
	if len(artifacts) != 1 || artifacts[0].ID != "seed.json" {
		t.Fatalf("artifacts=%+v", artifacts)
	}
	if string(artifacts[0].Content) != `{"entries":[{"external_id":"845-21-050"}]}` {
		t.Fatalf("artifact content=%q", artifacts[0].Content)
	}
}

func TestParseAndVerifyManifestRejectsEscapingArtifactPath(t *testing.T) {
	for _, path := range []string{"../escape.json", "/absolute/path.json"} {
		dir := t.TempDir()
		_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
			m.Artifacts[0].Path = path
		})
		if _, _, err := ParseAndVerifyManifest(manifestPath); err == nil {
			t.Fatalf("path %q: expected error", path)
		}
	}
}

func TestParseAndVerifyManifestRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
		m.Artifacts[0].Path = "link.json"
	})
	if _, _, err := ParseAndVerifyManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("symlink escape error = %v, want escapes manifest directory", err)
	}
}

func TestParseAndVerifyManifestChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
		m.Artifacts[0].SHA256 = strings.Repeat("ab", 32)
	})
	if _, _, err := ParseAndVerifyManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
}

func TestParseAndVerifyManifestRequiresRelease(t *testing.T) {
	dir := t.TempDir()
	_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
		m.Policy.Release = ""
	})
	if _, _, err := ParseAndVerifyManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "release") {
		t.Fatalf("error = %v, want missing release", err)
	}
}

func TestParseAndVerifyManifestRejectsInvalidAuthorityCombinations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{
			name: "exact authority without exact relation",
			mutate: func(m *Manifest) {
				m.Policy.AuthorityRole = "exact_identity_authority"
				m.Policy.AuthoritativeRelations = []string{"related"}
			},
			want: "exact_equivalent",
		},
		{
			name: "identity authority inconsistent",
			mutate: func(m *Manifest) {
				m.Policy.IdentityAuthority = false
			},
			want: "identity_authority",
		},
		{
			name: "unapproved license review",
			mutate: func(m *Manifest) {
				m.Policy.LicenseReviewStatus = "unreviewed"
			},
			want: "license review",
		},
		{
			name: "unknown authority role",
			mutate: func(m *Manifest) {
				m.Policy.AuthorityRole = "super_authority"
			},
			want: "unknown source authority role",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			_, manifestPath := writeTestManifest(t, dir, tc.mutate)
			if _, _, err := ParseAndVerifyManifest(manifestPath); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestParseAndVerifyManifestRejectsDuplicateArtifacts(t *testing.T) {
	dir := t.TempDir()
	_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
		m.Artifacts = append(m.Artifacts, m.Artifacts[0])
	})
	if _, _, err := ParseAndVerifyManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "duplicate artifact id") {
		t.Fatalf("error = %v, want duplicate artifact id", err)
	}

	dir = t.TempDir()
	_, manifestPath = writeTestManifest(t, dir, func(m *Manifest) {
		second := m.Artifacts[0]
		second.ID = "second.json"
		m.Artifacts = append(m.Artifacts, second)
	})
	if _, _, err := ParseAndVerifyManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "duplicate artifact path") {
		t.Fatalf("error = %v, want duplicate artifact path", err)
	}
}

func TestParseAndVerifyManifestRejectsDuplicateJSONKeys(t *testing.T) {
	dir := t.TempDir()
	writeTestArtifact(t, dir, "seed.json", []byte(`{}`))
	body := `{
  "adapter": "synthetic",
  "policy": {
    "source": "a",
    "source": "b"
  },
  "artifacts": [{"id":"seed.json","path":"seed.json","sha256":"` + strings.Repeat("ab", 32) + `","media_type":"application/json","provenance_locator":"https://example.test/seed.json"}]
}`
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseAndVerifyManifest(path); err == nil || !strings.Contains(err.Error(), `duplicate key "source"`) {
		t.Fatalf("error = %v, want duplicate key", err)
	}
}

func TestParseAndVerifyManifestRejectsTrailingData(t *testing.T) {
	dir := t.TempDir()
	writeTestArtifact(t, dir, "seed.json", []byte(`{}`))
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(`{"adapter":"synthetic","policy":{"provider_id":"p","source":"s","source_subset":"x","release":"v1","retrieved_at":"2026-08-07T00:00:00Z","content_checksum":"`+testContentChecksum+`","license":"CC0-1.0","license_review_status":"approved","authority_role":"exact_identity_authority","authoritative_relations":["exact_equivalent"],"allowed_scopes":["display"],"languages":["en"],"adapter_version":"0.1.0","provenance_locator":"https://example.test","approved_by":"board","approved_at":"2026-08-07T00:00:00Z","identity_authority":true},"artifacts":[{"id":"seed.json","path":"seed.json","sha256":"`+strings.Repeat("ab", 32)+`","media_type":"application/json","provenance_locator":"https://example.test/seed.json"}]} {}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseAndVerifyManifest(path); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("error = %v, want multiple JSON values", err)
	}
}

func TestParseAndVerifyManifestRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	writeTestArtifact(t, dir, "seed.json", []byte(`{}`))
	body := `{"adapter":"synthetic","policy":{"source":"s","source_subset":"x","release":"v1","retrieved_at":"2026-08-07T00:00:00Z","content_checksum":"` + testContentChecksum + `","license":"CC0-1.0","license_review_status":"approved","authority_role":"exact_identity_authority","authoritative_relations":["exact_equivalent"],"allowed_scopes":["display"],"languages":["en"],"adapter_version":"0.1.0","provenance_locator":"https://example.test","approved_by":"board","approved_at":"2026-08-07T00:00:00Z","identity_authority":true,"provider_id":"p","mystery":1},"artifacts":[{"id":"seed.json","path":"seed.json","sha256":"` + strings.Repeat("ab", 32) + `","media_type":"application/json","provenance_locator":"https://example.test/seed.json"}]}`
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseAndVerifyManifest(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}

func TestParseAndVerifyManifestRejectsMissingArtifactGovernance(t *testing.T) {
	dir := t.TempDir()
	_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
		m.Artifacts[0].ProvenanceLocator = ""
	})
	if _, _, err := ParseAndVerifyManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "provenance_locator") {
		t.Fatalf("error = %v, want missing provenance_locator", err)
	}
}

func TestParseAndVerifyManifestCanonicalizesMetadataAndSortsArtifacts(t *testing.T) {
	dir := t.TempDir()
	first := []byte(`{"id":"z"}`)
	second := []byte(`{"id":"a"}`)
	checksumA := writeTestArtifact(t, dir, "a.json", first)
	checksumB := writeTestArtifact(t, dir, "z.json", second)
	m := Manifest{
		Adapter: "synthetic",
		Policy:  validTestPolicy(),
		Artifacts: []ManifestArtifact{
			{ID: "z.json", Path: "z.json", SHA256: checksumB, MediaType: "application/json", ProvenanceLocator: "https://example.test/z", Metadata: json.RawMessage(`{"b":2,"a":1}`)},
			{ID: "a.json", Path: "a.json", SHA256: checksumA, MediaType: "application/json", ProvenanceLocator: "https://example.test/a"},
		},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	_, artifacts, err := ParseAndVerifyManifest(path)
	if err != nil {
		t.Fatalf("ParseAndVerifyManifest: %v", err)
	}
	if artifacts[0].ID != "a.json" || artifacts[1].ID != "z.json" {
		t.Fatalf("artifact order=%+v, want sorted by id", []string{artifacts[0].ID, artifacts[1].ID})
	}
	if got := string(artifacts[1].Metadata); got != `{"a":1,"b":2}` {
		t.Fatalf("metadata canonicalization=%q, want sorted keys", got)
	}
}

func TestCanonicalManifestJSONIsDeterministic(t *testing.T) {
	approvedAt := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	base := Manifest{
		Adapter: "synthetic",
		Policy: ManifestPolicy{
			ProviderID: "synthetic-provider", Source: "iec-seed", SourceSubset: "pilot", Release: "v1",
			RetrievedAt: approvedAt, ContentChecksum: testContentChecksum, License: "CC0-1.0",
			LicenseReviewStatus: "approved", AuthorityRole: "exact_identity_authority",
			AuthoritativeRelations: []string{"exact_equivalent", "related"},
			AllowedScopes:          []string{"display"}, Languages: []string{"zh", "en"},
			AdapterVersion: "0.1.0", ProvenanceLocator: "https://example.test/v1",
			ApprovedBy: "board", ApprovedAt: &approvedAt, IdentityAuthority: true,
		},
		Artifacts: []ManifestArtifact{
			{ID: "b.json", Path: "b.json", SHA256: testContentChecksum, MediaType: "application/json", ProvenanceLocator: "https://example.test/b"},
			{ID: "a.json", Path: "a.json", SHA256: testContentChecksum, MediaType: "application/json", ProvenanceLocator: "https://example.test/a"},
		},
	}
	reversed := Manifest{
		Adapter: base.Adapter,
		Policy:  base.Policy,
		Artifacts: []ManifestArtifact{
			{ID: "a.json", Path: "a.json", SHA256: testContentChecksum, MediaType: "application/json", ProvenanceLocator: "https://example.test/a"},
			{ID: "b.json", Path: "b.json", SHA256: testContentChecksum, MediaType: "application/json", ProvenanceLocator: "https://example.test/b"},
		},
	}
	reversed.Policy.AuthoritativeRelations = []string{"related", "exact_equivalent"}
	reversed.Policy.Languages = []string{"en", "zh", "zh"}

	first, err := CanonicalManifestJSON(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalManifestJSON(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("canonical JSON differs:\n%s\n%s", first, second)
	}
	var got Manifest
	if err := json.Unmarshal(first, &got); err != nil {
		t.Fatal(err)
	}
	if got.Policy.AuthoritativeRelations[0] != "exact_equivalent" || got.Policy.AuthoritativeRelations[1] != "related" {
		t.Fatalf("relations=%v, want sorted unique", got.Policy.AuthoritativeRelations)
	}
	if len(got.Policy.Languages) != 2 || got.Policy.Languages[0] != "en" || got.Policy.Languages[1] != "zh" {
		t.Fatalf("languages=%v, want sorted unique", got.Policy.Languages)
	}
	if got.Artifacts[0].ID != "a.json" || got.Artifacts[1].ID != "b.json" {
		t.Fatalf("artifacts=%v, want sorted by id", got.Artifacts)
	}
}
