package keywords

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type tripleEvidenceRow struct {
	id         int64
	source     string
	externalID string
	release    string
	authority  bool
	enabled    []bool
	target     string
	status     string
	scope      string
}

func beginTripleProviderTest(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *sql.Tx) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback()
		_ = db.Close()
	})
	return db, mock, tx
}

func expectTripleProviderRows(mock sqlmock.Sqlmock, rows []tripleEvidenceRow) {
	evidenceRows := sqlmock.NewRows([]string{"id", "source", "external_id", "release"})
	for _, row := range rows {
		evidenceRows.AddRow(row.id, row.source, row.externalID, row.release)
	}
	mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(evidenceRows)

	seenSources := make(map[string]bool)
	for _, row := range rows {
		sourceKey := row.source + "\x00" + row.release
		if !seenSources[sourceKey] {
			seenSources[sourceKey] = true
			mock.ExpectQuery(regexp.QuoteMeta(tripleSourcePolicySQL)).
				WithArgs(row.source, row.release).
				WillReturnRows(sqlmock.NewRows([]string{"identity_authority"}).AddRow(row.authority))
			deploymentRows := sqlmock.NewRows([]string{"enabled"})
			for _, enabled := range row.enabled {
				deploymentRows.AddRow(enabled)
			}
			mock.ExpectQuery(regexp.QuoteMeta(tripleDeploymentsSQL)).
				WithArgs(row.source, row.release).
				WillReturnRows(deploymentRows)
		}
	}
	seenMappings := make(map[string]bool)
	for _, row := range rows {
		sourceKey := row.source + "\x00" + row.release
		if strings.TrimSpace(row.externalID) == "" {
			continue
		}
		mappingKey := sourceKey + "\x00" + row.externalID
		if seenMappings[mappingKey] {
			continue
		}
		seenMappings[mappingKey] = true
		mappingRows := sqlmock.NewRows([]string{"concept_id", "status", "scope"})
		if row.target != "" {
			mappingRows.AddRow(row.target, row.status, row.scope)
		}
		mock.ExpectQuery(regexp.QuoteMeta(tripleExternalMappingSQL)).
			WithArgs(row.source, row.externalID, row.release).
			WillReturnRows(mappingRows)
	}
}

func TestTripleEvidenceProviderAuthorizesOnlyGovernedEnabledActiveSameScopeMapping(t *testing.T) {
	tests := []struct {
		name          string
		row           tripleEvidenceRow
		wantTarget    string
		wantAuthority IdentityAuthority
	}{
		{name: "authoritative", row: tripleEvidenceRow{id: 7, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, enabled: []bool{true}, target: "established", status: "active", scope: "quantity"}, wantTarget: "established", wantAuthority: IdentityAuthorityAuthoritative},
		{name: "release mismatch is unmapped", row: tripleEvidenceRow{id: 8, source: "qudt", externalID: "QK-1", release: "3.2", authority: true, enabled: []bool{true}}, wantAuthority: IdentityAuthorityNonAuthoritative},
		{name: "non-authoritative source", row: tripleEvidenceRow{id: 9, source: "qudt", externalID: "QK-1", release: "3.1", enabled: []bool{true}, target: "established", status: "active", scope: "quantity"}, wantTarget: "established", wantAuthority: IdentityAuthorityNonAuthoritative},
		{name: "no enabled deployment", row: tripleEvidenceRow{id: 10, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, enabled: []bool{false}, target: "established", status: "active", scope: "quantity"}, wantTarget: "established", wantAuthority: IdentityAuthorityNonAuthoritative},
		{name: "missing mapping", row: tripleEvidenceRow{id: 11, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, enabled: []bool{true}}, wantAuthority: IdentityAuthorityNonAuthoritative},
		{name: "inactive target retained", row: tripleEvidenceRow{id: 12, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, enabled: []bool{true}, target: "inactive", status: "deprecated", scope: "quantity"}, wantTarget: "inactive", wantAuthority: IdentityAuthorityNonAuthoritative},
		{name: "provisional target retained", row: tripleEvidenceRow{id: 13, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, enabled: []bool{true}, target: "provisional", status: "provisional", scope: "quantity"}, wantTarget: "provisional", wantAuthority: IdentityAuthorityNonAuthoritative},
		{name: "scope mismatch target retained", row: tripleEvidenceRow{id: 14, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, enabled: []bool{true}, target: "other-scope", status: "active", scope: "unit"}, wantTarget: "other-scope", wantAuthority: IdentityAuthorityNonAuthoritative},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, mock, tx := beginTripleProviderTest(t)
			expectTripleProviderRows(mock, []tripleEvidenceRow{tt.row})
			claims, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{
				CandidateConceptID: "candidate", Scope: "quantity", SurfaceIDs: []string{"surface"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(claims) != 1 {
				t.Fatalf("len(claims) = %d, want 1", len(claims))
			}
			claim := claims[0]
			if claim.TargetConceptID != tt.wantTarget || claim.Authority != tt.wantAuthority || claim.Relation != IdentityRelationExactEquivalent {
				t.Fatalf("claim = %#v, want target %q authority %q", claim, tt.wantTarget, tt.wantAuthority)
			}
			if tt.wantAuthority == IdentityAuthorityAuthoritative && claim.AuthorityRef != claim.EvidenceRefs[0].Key {
				t.Fatalf("AuthorityRef = %q, refs = %#v", claim.AuthorityRef, claim.EvidenceRefs)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTripleEvidenceProviderBlankLegacyExternalIDRemainsTargetlessEvidence(t *testing.T) {
	_, mock, tx := beginTripleProviderTest(t)
	row := tripleEvidenceRow{id: 15, source: "legacy", externalID: "", release: "", authority: true, enabled: []bool{true}}
	expectTripleProviderRows(mock, []tripleEvidenceRow{row})
	claims, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"surface"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].TargetConceptID != "" || claims[0].Authority != IdentityAuthorityNonAuthoritative {
		t.Fatalf("claims = %#v", claims)
	}
	if len(claims[0].EvidenceRefs) != 2 {
		t.Fatalf("refs = %#v, want source and evidence only", claims[0].EvidenceRefs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTripleEvidenceProviderCanonicalReferenceEncoding(t *testing.T) {
	_, mock, tx := beginTripleProviderTest(t)
	row := tripleEvidenceRow{id: 42, source: "源:a/b", externalID: "ID:α/β", release: "", authority: true, enabled: []bool{true}, target: "provider-only-target", status: "active", scope: "scope"}
	expectTripleProviderRows(mock, []tripleEvidenceRow{row})
	claims, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"only-candidate-surface"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []EvidenceRef{
		{Key: "keyword_source:5rqQOmEvYg:", Kind: "authority_configuration", Locator: "postgres:kb.keyword_sources/5rqQOmEvYg/"},
		{Key: "keyword_surface_evidence:42", Kind: "candidate_surface_evidence", Locator: "postgres:kb.keyword_surface_evidence/42"},
		{Key: "keyword_external_id:5rqQOmEvYg:SUQ6zrEvzrI:", Kind: "target_external_mapping", Locator: "postgres:kb.keyword_external_ids/5rqQOmEvYg/SUQ6zrEvzrI/"},
	}
	if len(claims) != 1 || len(claims[0].EvidenceRefs) != len(want) {
		t.Fatalf("claims = %#v", claims)
	}
	for i := range want {
		if claims[0].EvidenceRefs[i] != want[i] {
			t.Errorf("ref[%d] = %#v, want %#v", i, claims[0].EvidenceRefs[i], want[i])
		}
	}
	if claims[0].AuthorityRef != want[0].Key {
		t.Errorf("AuthorityRef = %q, want %q", claims[0].AuthorityRef, want[0].Key)
	}
}

func TestTripleEvidenceReferenceEncoding(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		externalID string
		release    string
		wantSource EvidenceRef
		wantMap    EvidenceRef
	}{
		{
			name: "ASCII", source: "qudt", externalID: "QK-1", release: "3.1",
			wantSource: EvidenceRef{Key: "keyword_source:cXVkdA:My4x", Kind: "authority_configuration", Locator: "postgres:kb.keyword_sources/cXVkdA/My4x"},
			wantMap:    EvidenceRef{Key: "keyword_external_id:cXVkdA:UUstMQ:My4x", Kind: "target_external_mapping", Locator: "postgres:kb.keyword_external_ids/cXVkdA/UUstMQ/My4x"},
		},
		{
			name: "Unicode delimiters and empty release", source: "源:a/b", externalID: "ID:α/β", release: "",
			wantSource: EvidenceRef{Key: "keyword_source:5rqQOmEvYg:", Kind: "authority_configuration", Locator: "postgres:kb.keyword_sources/5rqQOmEvYg/"},
			wantMap:    EvidenceRef{Key: "keyword_external_id:5rqQOmEvYg:SUQ6zrEvzrI:", Kind: "target_external_mapping", Locator: "postgres:kb.keyword_external_ids/5rqQOmEvYg/SUQ6zrEvzrI/"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tripleSourceEvidenceRef(tt.source, tt.release); got != tt.wantSource {
				t.Errorf("source ref = %#v, want %#v", got, tt.wantSource)
			}
			if got := tripleMappingEvidenceRef(tt.source, tt.externalID, tt.release); got != tt.wantMap {
				t.Errorf("mapping ref = %#v, want %#v", got, tt.wantMap)
			}
		})
	}
	if got := tripleSurfaceEvidenceRef(42); got.Key != "keyword_surface_evidence:42" || got.Locator != "postgres:kb.keyword_surface_evidence/42" {
		t.Errorf("surface ref = %#v", got)
	}
}

func TestTripleEvidenceProviderMultipleTriplesAreRetainedBeforeCanonicalDedup(t *testing.T) {
	rows := []tripleEvidenceRow{
		{id: 3, source: "a", externalID: "one", release: "", authority: true, enabled: []bool{true}, target: "same", status: "active", scope: "scope"},
		{id: 1, source: "b", externalID: "two", release: "2026", authority: true, enabled: []bool{true}, target: "same", status: "active", scope: "scope"},
		{id: 2, source: "c", externalID: "three", release: "2026", authority: true, enabled: []bool{true}, target: "different", status: "active", scope: "scope"},
	}
	_, mock, tx := beginTripleProviderTest(t)
	expectTripleProviderRows(mock, rows)
	claims, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 3 {
		t.Fatalf("len = %d, want 3", len(claims))
	}
	normalized, err := NormalizeIdentityClaims(TripleEvidenceProviderID, "candidate", claims)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 3 {
		t.Fatalf("different authority refs preserve all triples, len = %d", len(normalized))
	}
}

func TestTripleEvidenceProviderSameTripleClaimsCanonicalizeByUnioningEvidence(t *testing.T) {
	rows := []tripleEvidenceRow{
		{id: 21, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, enabled: []bool{true}, target: "same", status: "active", scope: "scope"},
		{id: 22, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, enabled: []bool{true}, target: "same", status: "active", scope: "scope"},
	}
	_, mock, tx := beginTripleProviderTest(t)
	expectTripleProviderRows(mock, rows)
	claims, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s1", "s2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 2 {
		t.Fatalf("raw claims len = %d, want 2", len(claims))
	}
	normalized, err := NormalizeIdentityClaims(TripleEvidenceProviderID, "candidate", claims)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 1 || len(normalized[0].EvidenceRefs) != 4 {
		t.Fatalf("normalized claims = %#v, want one claim with source, mapping, and two evidence refs", normalized)
	}
}

func TestTripleEvidenceProviderFailsClosed(t *testing.T) {
	t.Run("nil transaction", func(t *testing.T) {
		_, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), nil, CandidateIdentityContext{})
		if err == nil || !strings.Contains(err.Error(), "transaction") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("unregistered source", func(t *testing.T) {
		_, mock, tx := beginTripleProviderTest(t)
		mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnRows(
			sqlmock.NewRows([]string{"id", "source", "external_id", "release"}).AddRow(int64(1), "missing", "id", "r"),
		)
		mock.ExpectQuery(regexp.QuoteMeta(tripleSourcePolicySQL)).WithArgs("missing", "r").WillReturnError(sql.ErrNoRows)
		_, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s"}})
		if err == nil || !strings.Contains(err.Error(), "unregistered") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("query failure", func(t *testing.T) {
		_, mock, tx := beginTripleProviderTest(t)
		mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("database unavailable"))
		_, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{SurfaceIDs: []string{"s"}})
		if err == nil || !strings.Contains(err.Error(), "database unavailable") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestTripleEvidenceProviderID(t *testing.T) {
	if got := (TripleEvidenceProvider{}).ProviderID(); got != "triple_external_identity" {
		t.Fatalf("ProviderID() = %q", got)
	}
}
