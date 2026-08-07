package terminology

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

const fixtureDir = "testdata/fixtures/iec-seed"

const importLockSQL = "SELECT pg_advisory_xact_lock(1264011588, 1);"

type syntheticSeedAdapter struct{}

func (syntheticSeedAdapter) ID() string      { return "synthetic-seed" }
func (syntheticSeedAdapter) Version() string { return "0.1.0" }
func (syntheticSeedAdapter) Convert(_ context.Context, _ keywords.SourcePolicy, artifacts []VerifiedArtifact) (CatalogSnapshot, error) {
	if len(artifacts) != 1 {
		return CatalogSnapshot{}, errors.New("synthetic seed requires exactly one artifact")
	}
	var snapshot CatalogSnapshot
	if err := json.Unmarshal(artifacts[0].Content, &snapshot); err != nil {
		return CatalogSnapshot{}, err
	}
	return snapshot, nil
}

type failingSeedAdapter struct{}

func (failingSeedAdapter) ID() string      { return "synthetic-fail" }
func (failingSeedAdapter) Version() string { return "0.1.0" }
func (failingSeedAdapter) Convert(context.Context, keywords.SourcePolicy, []VerifiedArtifact) (CatalogSnapshot, error) {
	return CatalogSnapshot{}, errors.New("synthetic conversion failure")
}

func init() {
	_ = RegisterAdapter(syntheticSeedAdapter{})
	_ = RegisterAdapter(failingSeedAdapter{})
}

func fixtureManifestPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(fixtureDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func fixturePolicy(t *testing.T) keywords.SourcePolicy {
	t.Helper()
	manifest, _, err := ParseAndVerifyManifest(fixtureManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	return manifest.Policy.SourcePolicy()
}

func expectImportLock(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(importLockSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectPolicyRegister(mock sqlmock.Sqlmock, policy keywords.SourcePolicy) {
	expectImportLock(mock)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_sources")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT provider_id, source, source_subset, release, retrieved_at, content_checksum, license, license_review_status, authority_role, authoritative_relations, allowed_scopes, languages, adapter_version, provenance_locator, approved_by, approved_at, identity_authority, notes FROM kb.keyword_sources WHERE source = $1 AND release = $2")).
		WithArgs(policy.Source, policy.Release).
		WillReturnRows(policyRows(policy))
}

func expectArtifactRegister(mock sqlmock.Sqlmock, policy keywords.SourcePolicy, artifact VerifiedArtifact) {
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_source_artifacts")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, artifact_id, content_checksum, media_type, provenance_locator, payload FROM kb.keyword_source_artifacts WHERE source = $1 AND release = $2 AND artifact_id = $3")).
		WithArgs(policy.Source, policy.Release, artifact.ID).
		WillReturnRows(artifactRows(policy, artifact))
}

func expectCatalogImports(mock sqlmock.Sqlmock, policy keywords.SourcePolicy, snapshot CatalogSnapshot, insertAffected int64) {
	for range snapshot.Entries {
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_catalog_entries")).
			WillReturnResult(sqlmock.NewResult(0, insertAffected))
	}
	for range snapshot.Labels {
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_catalog_labels")).
			WillReturnResult(sqlmock.NewResult(0, insertAffected))
	}
	for range snapshot.Relations {
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_catalog_relations")).
			WillReturnResult(sqlmock.NewResult(0, insertAffected))
	}
	for range snapshot.NegativeDecisions {
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_catalog_negative_decisions")).
			WillReturnResult(sqlmock.NewResult(0, insertAffected))
	}
	for range snapshot.UCUMCodes {
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_ucum_codes")).
			WillReturnResult(sqlmock.NewResult(0, insertAffected))
	}
}

func expectCatalogVerification(mock sqlmock.Sqlmock, policy keywords.SourcePolicy, snapshot CatalogSnapshot) {
	entryRows := sqlmock.NewRows([]string{"external_id", "entry_status", "provenance_locator", "native_payload"})
	for _, e := range snapshot.Entries {
		entryRows.AddRow(e.ExternalID, e.EntryStatus, e.ProvenanceLocator, []byte(`{}`))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT external_id, entry_status, provenance_locator, native_payload FROM kb.keyword_catalog_entries WHERE source = $1 AND release = $2")).
		WithArgs(policy.Source, policy.Release).WillReturnRows(entryRows)

	labelRows := sqlmock.NewRows([]string{"external_id", "language", "label_role", "label", "provenance_locator", "native_payload"})
	for _, l := range snapshot.Labels {
		labelRows.AddRow(l.ExternalID, l.Language, l.LabelRole, l.Label, l.ProvenanceLocator, []byte(`{}`))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT external_id, language, label_role, label, provenance_locator, native_payload FROM kb.keyword_catalog_labels WHERE source = $1 AND release = $2")).
		WithArgs(policy.Source, policy.Release).WillReturnRows(labelRows)

	relationRows := sqlmock.NewRows([]string{"subject_external_id", "relation", "object_source", "object_release", "object_external_id", "provenance_locator", "native_payload"})
	for _, rel := range snapshot.Relations {
		relationRows.AddRow(rel.SubjectExternalID, rel.Relation, rel.ObjectSource, rel.ObjectRelease, rel.ObjectExternalID, rel.ProvenanceLocator, []byte(`{}`))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT subject_external_id, relation, object_source, object_release, object_external_id, provenance_locator, native_payload FROM kb.keyword_catalog_relations WHERE source = $1 AND release = $2")).
		WithArgs(policy.Source, policy.Release).WillReturnRows(relationRows)

	negativeRows := sqlmock.NewRows([]string{"subject_external_id", "object_source", "object_release", "object_external_id", "relation", "reason", "provenance_locator", "native_payload"})
	for _, neg := range snapshot.NegativeDecisions {
		negativeRows.AddRow(neg.SubjectExternalID, neg.ObjectSource, neg.ObjectRelease, neg.ObjectExternalID, neg.Relation, neg.Reason, neg.ProvenanceLocator, []byte(`{}`))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT subject_external_id, object_source, object_release, object_external_id, relation, reason, provenance_locator, native_payload FROM kb.keyword_catalog_negative_decisions WHERE source = $1 AND release = $2")).
		WithArgs(policy.Source, policy.Release).WillReturnRows(negativeRows)

	ucumRows := sqlmock.NewRows([]string{"code", "print_symbol", "dimension", "provenance_locator", "native_payload"})
	for _, code := range snapshot.UCUMCodes {
		ucumRows.AddRow(code.Code, code.PrintSymbol, code.Dimension, code.ProvenanceLocator, []byte(`{}`))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT code, print_symbol, dimension, provenance_locator, native_payload FROM kb.keyword_ucum_codes WHERE source = $1 AND release = $2")).
		WithArgs(policy.Source, policy.Release).WillReturnRows(ucumRows)
}

func policyRows(p keywords.SourcePolicy) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"provider_id", "source", "source_subset", "release", "retrieved_at", "content_checksum", "license",
		"license_review_status", "authority_role", "authoritative_relations", "allowed_scopes", "languages",
		"adapter_version", "provenance_locator", "approved_by", "approved_at", "identity_authority", "notes",
	}).AddRow(
		p.ProviderID, p.Source, p.SourceSubset, p.Release, p.RetrievedAt, p.ContentChecksum, p.License,
		p.LicenseReviewStatus, string(p.AuthorityRole), "{exact_equivalent}", "{display}", `{"en","zh"}`,
		p.AdapterVersion, p.ProvenanceLocator, p.ApprovedBy, *p.ApprovedAt, p.IdentityAuthority, p.Notes,
	)
}

func artifactRows(p keywords.SourcePolicy, artifact VerifiedArtifact) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"source", "release", "artifact_id", "content_checksum", "media_type", "provenance_locator", "payload",
	}).AddRow(
		p.Source, p.Release, artifact.ID, artifact.SHA256, artifact.MediaType, artifact.ProvenanceLocator, []byte(`{}`),
	)
}

func TestImportFirstImportCommitsAllOrNothing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	policy := fixturePolicy(t)
	_, artifacts, err := ParseAndVerifyManifest(fixtureManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := (syntheticSeedAdapter{}).Convert(context.Background(), policy, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	normalizeSnapshot(&snapshot)

	mock.ExpectBegin()
	expectPolicyRegister(mock, policy)
	expectArtifactRegister(mock, policy, artifacts[0])
	expectCatalogImports(mock, policy, snapshot, 1)
	expectCatalogVerification(mock, policy, snapshot)
	mock.ExpectCommit()

	result, err := (Runner{DB: db}).Import(context.Background(), fixtureManifestPath(t))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if result.Source != "iec-seed" || result.Release != "v1" {
		t.Fatalf("result=%+v", result)
	}
	if result.Replayed {
		t.Fatal("first import must not report replay")
	}
	if result.Counts.Entries != 2 || result.Counts.Labels != 4 || result.Counts.NegativeDecisions != 1 || result.Counts.Artifacts != 1 {
		t.Fatalf("counts=%+v", result.Counts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestImportIdenticalReplayIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	policy := fixturePolicy(t)
	_, artifacts, err := ParseAndVerifyManifest(fixtureManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := (syntheticSeedAdapter{}).Convert(context.Background(), policy, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	normalizeSnapshot(&snapshot)

	mock.ExpectBegin()
	expectPolicyRegister(mock, policy)
	expectArtifactRegister(mock, policy, artifacts[0])
	expectCatalogImports(mock, policy, snapshot, 0)
	expectCatalogVerification(mock, policy, snapshot)
	mock.ExpectCommit()

	result, err := (Runner{DB: db}).Import(context.Background(), fixtureManifestPath(t))
	if err != nil {
		t.Fatalf("Import replay: %v", err)
	}
	if !result.Replayed {
		t.Fatal("identical replay must report replayed=true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestImportChangedPayloadUnderExistingReleaseRejects(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	policy := fixturePolicy(t)
	_, artifacts, err := ParseAndVerifyManifest(fixtureManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := (syntheticSeedAdapter{}).Convert(context.Background(), policy, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	normalizeSnapshot(&snapshot)

	mock.ExpectBegin()
	expectPolicyRegister(mock, policy)
	expectArtifactRegister(mock, policy, artifacts[0])
	expectCatalogImports(mock, policy, snapshot, 0)

	// Registered staging differs from the manifest: changed provenance.
	entryRows := sqlmock.NewRows([]string{"external_id", "entry_status", "provenance_locator", "native_payload"})
	for _, e := range snapshot.Entries {
		entryRows.AddRow(e.ExternalID, e.EntryStatus, "https://registered.example.test/different", []byte(`{}`))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT external_id, entry_status, provenance_locator, native_payload FROM kb.keyword_catalog_entries WHERE source = $1 AND release = $2")).
		WithArgs(policy.Source, policy.Release).WillReturnRows(entryRows)
	mock.ExpectRollback()

	_, err = (Runner{DB: db}).Import(context.Background(), fixtureManifestPath(t))
	if err == nil || !strings.Contains(err.Error(), "differs from registered staging") {
		t.Fatalf("error = %v, want immutable staging rejection", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestImportAdapterFailureWritesNothing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := t.TempDir()
	_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
		m.Adapter = "synthetic-fail"
	})
	_, err = (Runner{DB: db}).Import(context.Background(), manifestPath)
	if err == nil || !strings.Contains(err.Error(), "synthetic conversion failure") {
		t.Fatalf("error = %v, want adapter failure", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("adapter failure must not start a transaction: %v", err)
	}
}

func TestImportUnknownAdapterRejected(t *testing.T) {
	dir := t.TempDir()
	_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
		m.Adapter = "no-such-adapter"
	})
	if _, err := (Runner{DB: nil}).Import(context.Background(), manifestPath); err == nil || !strings.Contains(err.Error(), `unknown adapter "no-such-adapter"`) {
		t.Fatalf("error = %v, want unknown adapter", err)
	}
}

func TestImportChecksumMismatchFailsBeforeTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dir := t.TempDir()
	_, manifestPath := writeTestManifest(t, dir, func(m *Manifest) {
		m.Artifacts[0].SHA256 = strings.Repeat("ab", 32)
	})
	if _, err := (Runner{DB: db}).Import(context.Background(), manifestPath); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("checksum mismatch must fail before any transaction: %v", err)
	}
}

func TestDiffReleasesReportsEveryChangeCategory(t *testing.T) {
	approvedAt := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	policy := keywords.SourcePolicy{
		ProviderID: "synthetic-provider", Source: "iec-seed", SourceSubset: "pilot", Release: "v1",
		RetrievedAt: approvedAt, ContentChecksum: testContentChecksum, License: "CC0-1.0",
		LicenseReviewStatus: "approved", AuthorityRole: keywords.ExactIdentityAuthority,
		AuthoritativeRelations: []string{"exact_equivalent"}, AllowedScopes: []string{"display"},
		Languages: []string{"en", "zh"}, AdapterVersion: "0.1.0",
		ProvenanceLocator: "https://example.test/iec-seed/v1", ApprovedBy: "board",
		ApprovedAt: &approvedAt, IdentityAuthority: true,
	}
	base := ReleaseContent{
		Policy: policy,
		Artifacts: []VerifiedArtifact{
			{ManifestArtifact: ManifestArtifact{ID: "seed.json", SHA256: testContentChecksum, MediaType: "application/json", ProvenanceLocator: "https://example.test/seed.json"}},
		},
		Snapshot: CatalogSnapshot{
			Entries: []CatalogEntry{
				{ExternalID: "845-21-050", EntryStatus: "current", ProvenanceLocator: "https://example.test/845-21-050"},
				{ExternalID: "845-22-059", EntryStatus: "current", ProvenanceLocator: "https://example.test/845-22-059"},
			},
			Labels: []CatalogLabel{
				{ExternalID: "845-21-050", Language: "en", LabelRole: "preferred", Label: "luminance"},
				{ExternalID: "845-21-050", Language: "zh", LabelRole: "preferred", Label: "亮度"},
			},
			UCUMCodes: []UCUMCode{{Code: "cd/m2", PrintSymbol: "cd/m²"}},
		},
	}
	candidate := ReleaseContent{
		Policy: func() keywords.SourcePolicy {
			next := policy
			next.Release = "v2"
			next.License = "CC-BY-4.0"
			return next
		}(),
		Artifacts: []VerifiedArtifact{
			{ManifestArtifact: ManifestArtifact{ID: "seed.json", SHA256: strings.Repeat("cd", 32), MediaType: "application/json", ProvenanceLocator: "https://example.test/seed.json"}},
			{ManifestArtifact: ManifestArtifact{ID: "addendum.json", SHA256: testContentChecksum, MediaType: "application/json", ProvenanceLocator: "https://example.test/addendum.json"}},
		},
		Snapshot: CatalogSnapshot{
			Entries: []CatalogEntry{
				{ExternalID: "845-21-050", EntryStatus: "current", ProvenanceLocator: "https://example.test/845-21-050"},
				{ExternalID: "845-22-059", EntryStatus: "superseded", ProvenanceLocator: "https://example.test/845-22-059"},
				{ExternalID: "845-22-060", EntryStatus: "current", ProvenanceLocator: "https://example.test/845-22-060"},
			},
			Labels: []CatalogLabel{
				{ExternalID: "845-21-050", Language: "en", LabelRole: "preferred", Label: "luminance"},
				{ExternalID: "845-21-050", Language: "zh", LabelRole: "preferred", Label: "亮度"},
				{ExternalID: "845-21-050", Language: "fr", LabelRole: "preferred", Label: "luminance"},
			},
			Relations: []CatalogRelation{
				{SubjectExternalID: "845-22-060", Relation: "replaces", ObjectSource: "iec-seed", ObjectRelease: "v1", ObjectExternalID: "845-22-059"},
			},
			NegativeDecisions: []CatalogNegativeDecision{
				{SubjectExternalID: "845-21-050", ObjectSource: "iec-seed", ObjectRelease: "v2", ObjectExternalID: "845-22-059", Relation: "different_from", Reason: "kept"},
			},
			UCUMCodes: []UCUMCode{{Code: "cd/m2", PrintSymbol: "cd per m2"}, {Code: "lx", PrintSymbol: "lx"}},
		},
	}

	diff := DiffReleases(base, candidate)
	if diff.Source != "iec-seed" || diff.BaseRelease != "v1" || diff.CandidateRelease != "v2" {
		t.Fatalf("diff=%+v", diff)
	}
	if len(diff.AddedEntries) != 1 || diff.AddedEntries[0].Key != "845-22-060" {
		t.Fatalf("added entries=%+v", diff.AddedEntries)
	}
	if len(diff.RetiredEntries) != 0 {
		t.Fatalf("retired entries=%+v", diff.RetiredEntries)
	}
	if len(diff.ReplacedEntries) != 1 || diff.ReplacedEntries[0].Key != "845-22-059" {
		t.Fatalf("replaced entries=%+v", diff.ReplacedEntries)
	}
	if len(diff.AddedLabels) != 1 || diff.AddedLabels[0].Key != "845-21-050\x00fr\x00preferred\x00luminance" {
		t.Fatalf("added labels=%+v", diff.AddedLabels)
	}
	if len(diff.AddedRelations) != 1 {
		t.Fatalf("added relations=%+v", diff.AddedRelations)
	}
	if len(diff.AddedNegativeDecisions) != 1 {
		t.Fatalf("added negative decisions=%+v", diff.AddedNegativeDecisions)
	}
	if len(diff.AddedUCUMCodes) != 1 || diff.AddedUCUMCodes[0].Key != "lx" {
		t.Fatalf("added ucum codes=%+v", diff.AddedUCUMCodes)
	}
	if len(diff.ChangedUCUMCodes) != 1 || diff.ChangedUCUMCodes[0].Key != "cd/m2" {
		t.Fatalf("changed ucum codes=%+v", diff.ChangedUCUMCodes)
	}
	if len(diff.AddedArtifacts) != 1 || diff.AddedArtifacts[0].Key != "addendum.json" {
		t.Fatalf("added artifacts=%+v", diff.AddedArtifacts)
	}
	if len(diff.ChangedArtifacts) != 1 || diff.ChangedArtifacts[0].Key != "seed.json" {
		t.Fatalf("changed artifacts=%+v", diff.ChangedArtifacts)
	}
	if len(diff.PolicyChanges) != 2 {
		t.Fatalf("policy changes=%+v", diff.PolicyChanges)
	}
	found := map[string]bool{}
	for _, change := range diff.PolicyChanges {
		found[change.Field] = true
	}
	if !found["release"] || !found["license"] {
		t.Fatalf("policy changes=%+v", diff.PolicyChanges)
	}
}

func TestDiffReleasesIsDeterministic(t *testing.T) {
	approvedAt := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
	policy := keywords.SourcePolicy{
		ProviderID: "synthetic-provider", Source: "iec-seed", SourceSubset: "pilot", Release: "v1",
		RetrievedAt: approvedAt, ContentChecksum: testContentChecksum, License: "CC0-1.0",
		LicenseReviewStatus: "approved", AuthorityRole: keywords.ExactIdentityAuthority,
		AuthoritativeRelations: []string{"exact_equivalent"}, AllowedScopes: []string{"display"},
		Languages: []string{"en", "zh"}, AdapterVersion: "0.1.0",
		ProvenanceLocator: "https://example.test/iec-seed/v1", ApprovedBy: "board",
		ApprovedAt: &approvedAt, IdentityAuthority: true,
	}
	base := ReleaseContent{Policy: policy, Snapshot: CatalogSnapshot{
		Entries: []CatalogEntry{{ExternalID: "b"}, {ExternalID: "a"}},
	}}
	candidate := ReleaseContent{Policy: policy, Snapshot: CatalogSnapshot{
		Entries: []CatalogEntry{{ExternalID: "c"}, {ExternalID: "b"}},
	}}
	first := DiffReleases(base, candidate)
	second := DiffReleases(base, candidate)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("diff not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
	if len(first.AddedEntries) != 1 || first.AddedEntries[0].Key != "c" {
		t.Fatalf("added=%+v", first.AddedEntries)
	}
	if len(first.RetiredEntries) != 1 || first.RetiredEntries[0].Key != "a" {
		t.Fatalf("retired=%+v", first.RetiredEntries)
	}
}

func TestDeploymentActivateSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	approvedAt := time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	expectImportLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT approved_at, license_review_status FROM kb.keyword_sources WHERE source = $1 AND release = $2")).
		WithArgs("iec-seed", "v1").
		WillReturnRows(sqlmock.NewRows([]string{"approved_at", "license_review_status"}).AddRow(approvedAt, "approved"))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_identity_deployments")).
		WithArgs("tier6-primary", "iec-seed", "v1", true, "operator@example.test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_identity_deployment_history")).
		WithArgs("tier6-primary", "iec-seed", "v1", true, "activate", "operator@example.test").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = (DeploymentStore{DB: db}).Activate(context.Background(), "tier6-primary", "iec-seed", "v1", "operator@example.test")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentActivateRejectsAbsentRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	expectImportLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT approved_at, license_review_status FROM kb.keyword_sources WHERE source = $1 AND release = $2")).
		WithArgs("iec-seed", "v9").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err = (DeploymentStore{DB: db}).Activate(context.Background(), "tier6-primary", "iec-seed", "v9", "operator@example.test")
	if !errors.Is(err, ErrDeploymentReleaseAbsent) {
		t.Fatalf("error = %v, want ErrDeploymentReleaseAbsent", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentActivateRejectsUnapprovedRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	expectImportLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT approved_at, license_review_status FROM kb.keyword_sources WHERE source = $1 AND release = $2")).
		WithArgs("iec-seed", "v1").
		WillReturnRows(sqlmock.NewRows([]string{"approved_at", "license_review_status"}).AddRow(nil, "unreviewed"))
	mock.ExpectRollback()

	err = (DeploymentStore{DB: db}).Activate(context.Background(), "tier6-primary", "iec-seed", "v1", "operator@example.test")
	if !errors.Is(err, ErrDeploymentReleaseUnapproved) {
		t.Fatalf("error = %v, want ErrDeploymentReleaseUnapproved", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentRollbackAdvancesToPriorRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	expectImportLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1 FOR UPDATE")).
		WithArgs("tier6-primary").
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow("iec-seed", "v2", true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, enabled FROM kb.keyword_identity_deployment_history WHERE deployment_key = $1 ORDER BY history_id DESC")).
		WithArgs("tier6-primary").
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).
			AddRow("iec-seed", "v2", true).
			AddRow("iec-seed", "v1", true))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_identity_deployments")).
		WithArgs("tier6-primary", "iec-seed", "v1", true, "operator@example.test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_identity_deployment_history")).
		WithArgs("tier6-primary", "iec-seed", "v1", true, "rollback", "operator@example.test").
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	err = (DeploymentStore{DB: db}).Rollback(context.Background(), "tier6-primary", "operator@example.test")
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentRollbackRejectsWithoutPointer(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	expectImportLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1 FOR UPDATE")).
		WithArgs("tier6-primary").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	err = (DeploymentStore{DB: db}).Rollback(context.Background(), "tier6-primary", "operator@example.test")
	if !errors.Is(err, ErrDeploymentNoPriorRelease) {
		t.Fatalf("error = %v, want ErrDeploymentNoPriorRelease", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentRollbackRejectsDisabledPointer(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	expectImportLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1 FOR UPDATE")).
		WithArgs("tier6-primary").
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow("iec-seed", "v1", false))
	mock.ExpectRollback()

	err = (DeploymentStore{DB: db}).Rollback(context.Background(), "tier6-primary", "operator@example.test")
	if !errors.Is(err, ErrDeploymentNotEnabled) {
		t.Fatalf("error = %v, want ErrDeploymentNotEnabled", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentRollbackRejectsWithoutPriorRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	expectImportLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1 FOR UPDATE")).
		WithArgs("tier6-primary").
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow("iec-seed", "v1", true))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, enabled FROM kb.keyword_identity_deployment_history WHERE deployment_key = $1 ORDER BY history_id DESC")).
		WithArgs("tier6-primary").
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow("iec-seed", "v1", true))
	mock.ExpectRollback()

	err = (DeploymentStore{DB: db}).Rollback(context.Background(), "tier6-primary", "operator@example.test")
	if !errors.Is(err, ErrDeploymentNoPriorRelease) {
		t.Fatalf("error = %v, want ErrDeploymentNoPriorRelease", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
