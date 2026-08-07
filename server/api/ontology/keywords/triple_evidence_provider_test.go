package keywords

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

type tripleEvidenceRow struct {
	id            int64
	source        string
	externalID    string
	release       string
	authority     bool
	allowedScopes any
	target        string
	status        any
	scope         any
}

type tripleDeploymentRow struct {
	key     string
	source  string
	release string
	enabled bool
}

func beginTripleProviderTest(t *testing.T) (sqlmock.Sqlmock, *sql.Tx) {
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
	return mock, tx
}

func expectTripleProviderBatch(mock sqlmock.Sqlmock, evidence []tripleEvidenceRow, deployments []tripleDeploymentRow, configuredKeys []string) {
	evidenceRows := sqlmock.NewRows([]string{"id", "source", "external_id", "release"})
	for _, row := range evidence {
		evidenceRows.AddRow(row.id, row.source, row.externalID, row.release)
	}
	mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnRows(evidenceRows)

	sourceRows := sqlmock.NewRows([]string{"source", "release", "identity_authority", "allowed_scopes"})
	seenSources := map[string]bool{}
	type sourcePair struct{ source, release string }
	var sourcePairs []sourcePair
	for _, row := range evidence {
		key := row.source + "\x00" + row.release
		if seenSources[key] {
			continue
		}
		seenSources[key] = true
		sourcePairs = append(sourcePairs, sourcePair{source: row.source, release: row.release})
		allowedScopes := row.allowedScopes
		if allowedScopes == nil {
			allowedScopes = "{quantity,scope}"
		}
		sourceRows.AddRow(row.source, row.release, row.authority, allowedScopes)
	}
	sort.Slice(sourcePairs, func(i, j int) bool {
		return sourcePairs[i].source < sourcePairs[j].source || (sourcePairs[i].source == sourcePairs[j].source && sourcePairs[i].release < sourcePairs[j].release)
	})
	sources, releases := make([]string, len(sourcePairs)), make([]string, len(sourcePairs))
	for index, pair := range sourcePairs {
		sources[index], releases[index] = pair.source, pair.release
	}
	mock.ExpectQuery(regexp.QuoteMeta(tripleSourcePoliciesSQL)).WithArgs(pq.Array(sources), pq.Array(releases)).WillReturnRows(sourceRows)

	if len(configuredKeys) > 0 {
		canonicalKeys := append([]string(nil), configuredKeys...)
		sort.Strings(canonicalKeys)
		deploymentRows := sqlmock.NewRows([]string{"deployment_key", "source", "release", "enabled"})
		for _, row := range deployments {
			deploymentRows.AddRow(row.key, row.source, row.release, row.enabled)
		}
		mock.ExpectQuery(regexp.QuoteMeta(tripleConfiguredDeploymentsSQL)).
			WithArgs(pq.Array(canonicalKeys)).WillReturnRows(deploymentRows)
	}

	mappingRows := sqlmock.NewRows([]string{"source", "external_id", "release", "concept_id", "status", "scope"})
	hasMappingRequest := false
	seenMappings := map[string]bool{}
	type externalTriple struct{ source, externalID, release string }
	var externalTriples []externalTriple
	for _, row := range evidence {
		if strings.TrimSpace(row.externalID) == "" {
			continue
		}
		hasMappingRequest = true
		key := row.source + "\x00" + row.externalID + "\x00" + row.release
		if seenMappings[key] {
			continue
		}
		seenMappings[key] = true
		externalTriples = append(externalTriples, externalTriple{source: row.source, externalID: row.externalID, release: row.release})
		if row.target != "" {
			mappingRows.AddRow(row.source, row.externalID, row.release, row.target, row.status, row.scope)
		}
	}
	if hasMappingRequest {
		sort.Slice(externalTriples, func(i, j int) bool {
			if externalTriples[i].source != externalTriples[j].source {
				return externalTriples[i].source < externalTriples[j].source
			}
			if externalTriples[i].externalID != externalTriples[j].externalID {
				return externalTriples[i].externalID < externalTriples[j].externalID
			}
			return externalTriples[i].release < externalTriples[j].release
		})
		sources, externalIDs, releases := make([]string, len(externalTriples)), make([]string, len(externalTriples)), make([]string, len(externalTriples))
		for index, triple := range externalTriples {
			sources[index], externalIDs[index], releases[index] = triple.source, triple.externalID, triple.release
		}
		mock.ExpectQuery(regexp.QuoteMeta(tripleExternalMappingsSQL)).
			WithArgs(pq.Array(sources), pq.Array(externalIDs), pq.Array(releases)).WillReturnRows(mappingRows)
	}
}

func loadTripleClaims(t *testing.T, provider TripleEvidenceProvider, candidate CandidateIdentityContext, evidence []tripleEvidenceRow, deployments []tripleDeploymentRow) []IdentityClaim {
	t.Helper()
	mock, tx := beginTripleProviderTest(t)
	expectTripleProviderBatch(mock, evidence, deployments, provider.DeploymentKeys)
	claims, err := provider.LoadClaims(context.Background(), tx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestTripleEvidenceProviderValidatesAndCanonicalizesCandidateContext(t *testing.T) {
	provider := TripleEvidenceProvider{}
	for _, tt := range []struct {
		name      string
		candidate CandidateIdentityContext
	}{
		{name: "blank candidate", candidate: CandidateIdentityContext{CandidateConceptID: " ", Scope: "scope"}},
		{name: "blank scope", candidate: CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "\t"}},
		{name: "blank surface", candidate: CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s", " "}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.LoadClaims(context.Background(), nil, tt.candidate)
			if err == nil || !strings.Contains(err.Error(), "candidate") {
				t.Fatalf("error = %v, want candidate context error", err)
			}
		})
	}

	mock, tx := beginTripleProviderTest(t)
	mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).
		WithArgs(pq.Array([]string{"a", "b"})).
		WillReturnRows(sqlmock.NewRows([]string{"id", "source", "external_id", "release"}))
	claims, err := provider.LoadClaims(context.Background(), tx, CandidateIdentityContext{
		CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"b", "a", "b"},
	})
	if err != nil || len(claims) != 0 {
		t.Fatalf("claims = %#v, error = %v", claims, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	mock, tx = beginTripleProviderTest(t)
	claims, err = provider.LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope"})
	if err != nil || len(claims) != 0 {
		t.Fatalf("empty surface claims = %#v, error = %v", claims, err)
	}
}

func TestTripleEvidenceProviderRejectsInvalidDeploymentConfiguration(t *testing.T) {
	for _, keys := range [][]string{{"production", "production"}, {"production", " "}} {
		_, err := (TripleEvidenceProvider{DeploymentKeys: keys}).LoadClaims(context.Background(), nil, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope"})
		if err == nil || !strings.Contains(err.Error(), "deployment") {
			t.Fatalf("keys %q error = %v", keys, err)
		}
	}
}

func TestTripleEvidenceProviderDeploymentAllowlistControlsAuthority(t *testing.T) {
	evidence := []tripleEvidenceRow{{id: 7, source: "qudt", externalID: "QK-1", release: "3.1", authority: true, allowedScopes: "{quantity}", target: "established", status: "active", scope: "quantity"}}
	candidate := CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "quantity", SurfaceIDs: []string{"surface"}}
	for _, tt := range []struct {
		name        string
		provider    TripleEvidenceProvider
		deployments []tripleDeploymentRow
		want        IdentityAuthority
	}{
		{name: "enabled intended", provider: TripleEvidenceProvider{DeploymentKeys: []string{"production"}}, deployments: []tripleDeploymentRow{{key: "production", source: "qudt", release: "3.1", enabled: true}}, want: IdentityAuthorityAuthoritative},
		{name: "disabled intended ignores enabled staging", provider: TripleEvidenceProvider{DeploymentKeys: []string{"production"}}, deployments: []tripleDeploymentRow{{key: "production", source: "qudt", release: "3.1", enabled: false}}, want: IdentityAuthorityNonAuthoritative},
		{name: "empty keys authorize nothing", provider: TripleEvidenceProvider{}, want: IdentityAuthorityNonAuthoritative},
		{name: "enabled intended points elsewhere", provider: TripleEvidenceProvider{DeploymentKeys: []string{"production"}}, deployments: []tripleDeploymentRow{{key: "production", source: "qudt", release: "3.0", enabled: true}}, want: IdentityAuthorityNonAuthoritative},
	} {
		t.Run(tt.name, func(t *testing.T) {
			claims := loadTripleClaims(t, tt.provider, candidate, evidence, tt.deployments)
			if len(claims) != 1 || claims[0].Authority != tt.want {
				t.Fatalf("claims = %#v, want authority %q", claims, tt.want)
			}
		})
	}
}

func TestTripleEvidenceProviderConfiguredPortfolioEnablesMultipleReleases(t *testing.T) {
	provider := TripleEvidenceProvider{DeploymentKeys: []string{"qudt-production", "ucum-production"}}
	evidence := []tripleEvidenceRow{
		{id: 1, source: "qudt", externalID: "QK", release: "3.1", authority: true, target: "quantity", status: "active", scope: "scope"},
		{id: 2, source: "ucum", externalID: "m", release: "2.2", authority: true, target: "unit", status: "active", scope: "scope"},
	}
	deployments := []tripleDeploymentRow{
		{key: "ucum-production", source: "ucum", release: "2.2", enabled: true},
		{key: "qudt-production", source: "qudt", release: "3.1", enabled: true},
	}
	claims := loadTripleClaims(t, provider, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s"}}, evidence, deployments)
	if len(claims) != 2 || claims[0].Authority != IdentityAuthorityAuthoritative || claims[1].Authority != IdentityAuthorityAuthoritative {
		t.Fatalf("claims = %#v", claims)
	}
}

func TestTripleEvidenceProviderRetainsNonAuthorizingEvidence(t *testing.T) {
	provider := TripleEvidenceProvider{DeploymentKeys: []string{"production"}}
	deploy := []tripleDeploymentRow{{key: "production", source: "qudt", release: "3.1", enabled: true}}
	tests := []struct {
		name       string
		row        tripleEvidenceRow
		wantTarget string
	}{
		{name: "blank legacy ID", row: tripleEvidenceRow{id: 10, source: "legacy", externalID: "", release: "", allowedScopes: "{}"}},
		{name: "missing mapping", row: tripleEvidenceRow{id: 11, source: "qudt", externalID: "missing", release: "3.1", authority: true}},
		{name: "inactive mapping", row: tripleEvidenceRow{id: 12, source: "qudt", externalID: "old", release: "3.1", authority: true, target: "inactive", status: "deprecated", scope: "scope"}, wantTarget: "inactive"},
		{name: "provisional mapping", row: tripleEvidenceRow{id: 13, source: "qudt", externalID: "new", release: "3.1", authority: true, target: "provisional", status: "provisional", scope: "scope"}, wantTarget: "provisional"},
		{name: "scope mismatch", row: tripleEvidenceRow{id: 14, source: "qudt", externalID: "unit", release: "3.1", authority: true, target: "unit", status: "active", scope: "unit"}, wantTarget: "unit"},
		{name: "non-authoritative source", row: tripleEvidenceRow{id: 15, source: "qudt", externalID: "proposal", release: "3.1", target: "proposal", status: "active", scope: "scope"}, wantTarget: "proposal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys, deployments := provider, deploy
			if tt.row.source == "legacy" {
				keys, deployments = TripleEvidenceProvider{}, nil
			}
			claims := loadTripleClaims(t, keys, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"surface"}}, []tripleEvidenceRow{tt.row}, deployments)
			if len(claims) != 1 || claims[0].TargetConceptID != tt.wantTarget || claims[0].Authority != IdentityAuthorityNonAuthoritative {
				t.Fatalf("claims = %#v", claims)
			}
		})
	}
}

func TestTripleEvidenceProviderSourceScopeGovernance(t *testing.T) {
	provider := TripleEvidenceProvider{DeploymentKeys: []string{"production"}}
	deployments := []tripleDeploymentRow{{key: "production", source: "catalog", release: "r", enabled: true}}
	candidate := CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "unit", SurfaceIDs: []string{"surface"}}
	evidence := []tripleEvidenceRow{{id: 20, source: "catalog", externalID: "id", release: "r", authority: true, allowedScopes: "{quantity}", target: "unit-target", status: "active", scope: "unit"}}
	claims := loadTripleClaims(t, provider, candidate, evidence, deployments)
	if len(claims) != 1 || claims[0].Authority != IdentityAuthorityNonAuthoritative || claims[0].TargetConceptID != "unit-target" {
		t.Fatalf("claims = %#v", claims)
	}
}

func TestTripleEvidenceProviderMalformedSourceScopesFailClosed(t *testing.T) {
	for _, tt := range []struct {
		name   string
		scopes any
	}{
		{name: "null", scopes: nil},
		{name: "blank member", scopes: "{\"\t\"}"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mock, tx := beginTripleProviderTest(t)
			mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnRows(
				sqlmock.NewRows([]string{"id", "source", "external_id", "release"}).AddRow(int64(1), "corrupt", "id", "r"))
			mock.ExpectQuery(regexp.QuoteMeta(tripleSourcePoliciesSQL)).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(
				sqlmock.NewRows([]string{"source", "release", "identity_authority", "allowed_scopes"}).AddRow("corrupt", "r", true, tt.scopes))
			_, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s"}})
			if err == nil || !strings.Contains(err.Error(), "allowed_scopes") {
				t.Fatalf("error = %v, want allowed_scopes corruption", err)
			}
		})
	}
}

func TestTripleEvidenceProviderFailsClosedOnMissingSourceOrCorruptMapping(t *testing.T) {
	t.Run("missing source", func(t *testing.T) {
		mock, tx := beginTripleProviderTest(t)
		mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnRows(
			sqlmock.NewRows([]string{"id", "source", "external_id", "release"}).AddRow(int64(1), "missing", "id", "r"))
		mock.ExpectQuery(regexp.QuoteMeta(tripleSourcePoliciesSQL)).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(
			sqlmock.NewRows([]string{"source", "release", "identity_authority", "allowed_scopes"}))
		_, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s"}})
		if err == nil || !strings.Contains(err.Error(), "unregistered") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestTripleEvidenceProviderCorruptMappedConceptFailsClosed(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status any
		scope  any
	}{
		{name: "missing", status: nil, scope: nil},
		{name: "blank scope", status: "active", scope: " "},
		{name: "unknown status", status: "unknown", scope: "scope"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mock, tx := beginTripleProviderTest(t)
			mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnRows(
				sqlmock.NewRows([]string{"id", "source", "external_id", "release"}).AddRow(int64(1), "qudt", "id", "r"))
			mock.ExpectQuery(regexp.QuoteMeta(tripleSourcePoliciesSQL)).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(
				sqlmock.NewRows([]string{"source", "release", "identity_authority", "allowed_scopes"}).AddRow("qudt", "r", false, "{scope}"))
			mock.ExpectQuery(regexp.QuoteMeta(tripleExternalMappingsSQL)).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(
				sqlmock.NewRows([]string{"source", "external_id", "release", "concept_id", "status", "scope"}).AddRow("qudt", "id", "r", "missing-concept", tt.status, tt.scope))
			_, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s"}})
			if err == nil || !strings.Contains(err.Error(), "concept") {
				t.Fatalf("error = %v, want corrupt concept", err)
			}
		})
	}
}

func TestTripleEvidenceProviderUsesAtMostFourQueriesAndIsOrderIndependent(t *testing.T) {
	provider := TripleEvidenceProvider{DeploymentKeys: []string{"b-production", "a-production"}}
	evidence := []tripleEvidenceRow{
		{id: 5, source: "b", externalID: "2", release: "r2", authority: true, target: "target-b", status: "active", scope: "scope"},
		{id: 1, source: "a", externalID: "1", release: "r1", authority: true, target: "target-a", status: "active", scope: "scope"},
		{id: 4, source: "b", externalID: "2", release: "r2", authority: true, target: "target-b", status: "active", scope: "scope"},
		{id: 3, source: "a", externalID: "missing", release: "r1", authority: true},
		{id: 2, source: "a", externalID: "", release: "r1", authority: true},
	}
	deployments := []tripleDeploymentRow{
		{key: "b-production", source: "b", release: "r2", enabled: true},
		{key: "a-production", source: "a", release: "r1", enabled: true},
	}
	candidate := CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"z", "a", "z"}}
	claimsOne := loadTripleClaims(t, provider, candidate, evidence, deployments)
	for left, right := 0, len(evidence)-1; left < right; left, right = left+1, right-1 {
		evidence[left], evidence[right] = evidence[right], evidence[left]
	}
	for left, right := 0, len(deployments)-1; left < right; left, right = left+1, right-1 {
		deployments[left], deployments[right] = deployments[right], deployments[left]
	}
	claimsTwo := loadTripleClaims(t, provider, candidate, evidence, deployments)
	jsonOne, err := CanonicalIdentityClaimsJSON(TripleEvidenceProviderID, "candidate", claimsOne)
	if err != nil {
		t.Fatal(err)
	}
	jsonTwo, err := CanonicalIdentityClaimsJSON(TripleEvidenceProviderID, "candidate", claimsTwo)
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonOne) != string(jsonTwo) {
		t.Fatalf("canonical output differs:\n%s\n%s", jsonOne, jsonTwo)
	}
}

func TestTripleEvidenceProviderSQLFailureFailsClosed(t *testing.T) {
	mock, tx := beginTripleProviderTest(t)
	mock.ExpectQuery(regexp.QuoteMeta(tripleEvidenceRowsSQL)).WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("database unavailable"))
	_, err := (TripleEvidenceProvider{}).LoadClaims(context.Background(), tx, CandidateIdentityContext{CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"s"}})
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("error = %v", err)
	}
}

func TestTripleEvidenceReferenceEncoding(t *testing.T) {
	for _, tt := range []struct {
		name, source, externalID, release string
		wantSource, wantMap               EvidenceRef
	}{
		{name: "ASCII", source: "qudt", externalID: "QK-1", release: "3.1",
			wantSource: EvidenceRef{Key: "keyword_source:cXVkdA:My4x", Kind: "authority_configuration", Locator: "postgres:kb.keyword_sources/cXVkdA/My4x"},
			wantMap:    EvidenceRef{Key: "keyword_external_id:cXVkdA:UUstMQ:My4x", Kind: "target_external_mapping", Locator: "postgres:kb.keyword_external_ids/cXVkdA/UUstMQ/My4x"}},
		{name: "Unicode delimiters empty release", source: "源:a/b", externalID: "ID:α/β", release: "",
			wantSource: EvidenceRef{Key: "keyword_source:5rqQOmEvYg:", Kind: "authority_configuration", Locator: "postgres:kb.keyword_sources/5rqQOmEvYg/"},
			wantMap:    EvidenceRef{Key: "keyword_external_id:5rqQOmEvYg:SUQ6zrEvzrI:", Kind: "target_external_mapping", Locator: "postgres:kb.keyword_external_ids/5rqQOmEvYg/SUQ6zrEvzrI/"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tripleSourceEvidenceRef(tt.source, tt.release); got != tt.wantSource {
				t.Errorf("source ref = %#v, want %#v", got, tt.wantSource)
			}
			if got := tripleMappingEvidenceRef(tt.source, tt.externalID, tt.release); got != tt.wantMap {
				t.Errorf("mapping ref = %#v, want %#v", got, tt.wantMap)
			}
		})
	}
	if got := tripleSurfaceEvidenceRef(42); got.Key != "keyword_surface_evidence:42" {
		t.Errorf("surface ref = %#v", got)
	}
}

func TestTripleEvidenceProviderID(t *testing.T) {
	if got := (TripleEvidenceProvider{}).ProviderID(); got != "triple_external_identity" {
		t.Fatalf("ProviderID() = %q", got)
	}
}
