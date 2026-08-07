package keywords

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func validExactSourcePolicy() SourcePolicy {
	approvedAt := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	return SourcePolicy{
		ProviderID:             "triple_external_identity",
		Source:                 "qudt-quantity-kind",
		SourceSubset:           "quantitykind",
		Release:                "3.5.0",
		RetrievedAt:            approvedAt.Add(-time.Hour),
		ContentChecksum:        "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		License:                "https://www.qudt.org/pages/QUDToverviewPage.html",
		LicenseReviewStatus:    LicenseReviewApproved,
		AuthorityRole:          ExactIdentityAuthority,
		AuthoritativeRelations: []string{"exact_equivalent"},
		AllowedScopes:          []string{"display-metric"},
		Languages:              []string{"en", "zh-CN"},
		AdapterVersion:         "qudt-adapter/v1",
		ProvenanceLocator:      "file:///imports/qudt-3.5.0.ttl",
		ApprovedBy:             "reviewer@example.test",
		ApprovedAt:             &approvedAt,
		IdentityAuthority:      true,
		Notes:                  "reviewed Stage-1 quantity-kind subset",
	}
}

func TestSourcePolicyValidateRejectsIncompleteAndUnsafePolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*SourcePolicy)
	}{
		{"blank provider", func(p *SourcePolicy) { p.ProviderID = " " }},
		{"blank source", func(p *SourcePolicy) { p.Source = " " }},
		{"blank subset", func(p *SourcePolicy) { p.SourceSubset = "" }},
		{"missing retrieval", func(p *SourcePolicy) { p.RetrievedAt = time.Time{} }},
		{"bad checksum", func(p *SourcePolicy) { p.ContentChecksum = "sha256:not-a-digest" }},
		{"missing license", func(p *SourcePolicy) { p.License = "" }},
		{"unknown license review", func(p *SourcePolicy) { p.LicenseReviewStatus = "pending" }},
		{"unapproved license", func(p *SourcePolicy) { p.LicenseReviewStatus = LicenseReviewUnreviewed }},
		{"unknown role", func(p *SourcePolicy) { p.AuthorityRole = "trusted" }},
		{"unknown relation", func(p *SourcePolicy) { p.AuthoritativeRelations = []string{"same_as"} }},
		{"missing scope", func(p *SourcePolicy) { p.AllowedScopes = nil }},
		{"blank scope member", func(p *SourcePolicy) { p.AllowedScopes = []string{"display-metric", " \t"} }},
		{"missing language", func(p *SourcePolicy) { p.Languages = nil }},
		{"blank language member", func(p *SourcePolicy) { p.Languages = []string{"en", "  "} }},
		{"missing adapter", func(p *SourcePolicy) { p.AdapterVersion = "" }},
		{"missing provenance", func(p *SourcePolicy) { p.ProvenanceLocator = "" }},
		{"missing approver", func(p *SourcePolicy) { p.ApprovedBy = "" }},
		{"missing approval time", func(p *SourcePolicy) { p.ApprovedAt = nil }},
		{"exact role missing exact relation", func(p *SourcePolicy) { p.AuthoritativeRelations = []string{"related"}; p.IdentityAuthority = false }},
		{"proposal shortcut", func(p *SourcePolicy) { p.AuthorityRole = ProposalOnly; p.IdentityAuthority = true }},
		{"context shortcut", func(p *SourcePolicy) { p.AuthorityRole = ContextOnly; p.IdentityAuthority = true }},
		{"conditional shortcut", func(p *SourcePolicy) { p.AuthorityRole = ConditionalIdentityAuthority; p.IdentityAuthority = true }},
		{"exact shortcut disabled", func(p *SourcePolicy) { p.IdentityAuthority = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validExactSourcePolicy()
			tt.mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSourcePolicyStoreCanonicalizesCollectionsAndPostgresTimes(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	p := validExactSourcePolicy()
	nowWithMonotonic := time.Now()
	p.RetrievedAt = nowWithMonotonic.Add(789 * time.Nanosecond)
	approved := nowWithMonotonic.In(time.FixedZone("review-zone", -7*60*60)).Add(321 * time.Nanosecond)
	p.ApprovedAt = &approved
	p.AuthoritativeRelations = []string{" exact_equivalent ", "exact_equivalent"}
	p.AllowedScopes = []string{" display-metric ", "display-metric"}
	p.Languages = []string{" zh-CN ", "en"}

	dbPolicy := p
	dbPolicy.RetrievedAt = time.UnixMicro(p.RetrievedAt.UnixMicro()).UTC()
	dbApproved := time.UnixMicro(p.ApprovedAt.UnixMicro()).UTC()
	dbPolicy.ApprovedAt = &dbApproved
	dbPolicy.AuthoritativeRelations = []string{"exact_equivalent"}
	dbPolicy.AllowedScopes = []string{"display-metric"}
	dbPolicy.Languages = []string{"en", "zh-CN"}
	mock.ExpectQuery(regexp.QuoteMeta("WITH attempted AS ( INSERT INTO kb.keyword_sources")).
		WillReturnRows(sourcePolicyRows(dbPolicy))

	if err := (SourcePolicyStore{DB: db}).Register(context.Background(), p); err != nil {
		t.Fatalf("Register canonical replay: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSourcePolicyValidateAllowsNonAuthorizingRolesOnlyWithoutShortcut(t *testing.T) {
	t.Parallel()
	for _, role := range []AuthorityRole{ConditionalIdentityAuthority, ProposalOnly, ContextOnly} {
		p := validExactSourcePolicy()
		p.AuthorityRole = role
		p.IdentityAuthority = false
		p.AuthoritativeRelations = nil
		if err := p.Validate(); err != nil {
			t.Errorf("role %s: %v", role, err)
		}
	}
}

func TestSourcePolicyStoreRegisterExactReplayAndMutation(t *testing.T) {
	t.Parallel()
	p := validExactSourcePolicy()

	tests := []struct {
		name    string
		mutate  func(*SourcePolicy)
		wantErr error
	}{
		{"exact replay", func(*SourcePolicy) {}, nil},
		{"changed checksum", func(p *SourcePolicy) {
			p.ContentChecksum = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
		}, ErrImmutableSourceRelease},
		{"changed scope", func(p *SourcePolicy) { p.AllowedScopes = []string{"other"} }, ErrImmutableSourceRelease},
		{"changed rights", func(p *SourcePolicy) { p.License = "different-license" }, ErrImmutableSourceRelease},
		{"changed approval", func(p *SourcePolicy) { p.ApprovedBy = "other-reviewer" }, ErrImmutableSourceRelease},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			got := p
			tt.mutate(&got)
			mock.ExpectQuery(regexp.QuoteMeta("WITH attempted AS ( INSERT INTO kb.keyword_sources")).
				WillReturnRows(sourcePolicyRows(p))

			err = (SourcePolicyStore{DB: db}).Register(context.Background(), got)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Register error = %v, want %v", err, tt.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourcePolicyStoreRegisterArtifactExactReplayAndPayloadMutation(t *testing.T) {
	t.Parallel()
	p := validExactSourcePolicy()
	a := SourceArtifact{
		Source:            p.Source,
		Release:           p.Release,
		ArtifactID:        "qudt.ttl",
		ContentChecksum:   p.ContentChecksum,
		MediaType:         "text/turtle",
		ProvenanceLocator: p.ProvenanceLocator,
		Payload:           []byte(`{"triples":42}`),
	}

	for _, tc := range []struct {
		name    string
		payload []byte
		wantErr error
	}{
		{"exact replay", []byte(`{"triples":42}`), nil},
		{"changed payload", []byte(`{"triples":43}`), ErrImmutableSourceArtifact},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			got := a
			got.Payload = tc.payload
			mock.ExpectQuery(regexp.QuoteMeta("WITH attempted AS ( INSERT INTO kb.keyword_source_artifacts")).
				WillReturnRows(sqlmock.NewRows([]string{
					"source", "release", "artifact_id", "content_checksum", "media_type", "provenance_locator", "payload",
				}).AddRow(a.Source, a.Release, a.ArtifactID, a.ContentChecksum, a.MediaType, a.ProvenanceLocator, a.Payload))

			err = (SourcePolicyStore{DB: db}).RegisterArtifact(context.Background(), got)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("RegisterArtifact error = %v, want %v", err, tc.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourcePolicyStoreSetDeploymentUsesLockHistoryAndPointer(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1 FOR UPDATE")).
		WithArgs("tier6-primary").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_identity_deployments")).
		WithArgs("tier6-primary", "qudt-quantity-kind", "3.5.0", true, "operator@example.test").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_identity_deployment_history")).
		WithArgs("tier6-primary", "qudt-quantity-kind", "3.5.0", true, "activate", "operator@example.test").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	change := DeploymentChange{
		DeploymentKey: "tier6-primary", Source: "qudt-quantity-kind", Release: "3.5.0",
		Enabled: true, Action: DeploymentActivate, ChangedBy: "operator@example.test",
	}
	if err := (SourcePolicyStore{DB: db}).SetDeployment(context.Background(), change); err != nil {
		t.Fatalf("SetDeployment: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSourcePolicyStoreSetDeploymentExactReplayIsIdempotent(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT source, release, enabled FROM kb.keyword_identity_deployments WHERE deployment_key = $1 FOR UPDATE")).
		WithArgs("tier6-primary").
		WillReturnRows(sqlmock.NewRows([]string{"source", "release", "enabled"}).AddRow("qudt-quantity-kind", "3.5.0", true))
	mock.ExpectCommit()

	change := DeploymentChange{
		DeploymentKey: "tier6-primary", Source: "qudt-quantity-kind", Release: "3.5.0",
		Enabled: true, Action: DeploymentActivate, ChangedBy: "operator@example.test",
	}
	if err := (SourcePolicyStore{DB: db}).SetDeployment(context.Background(), change); err != nil {
		t.Fatalf("SetDeployment: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func sourcePolicyRows(p SourcePolicy) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"provider_id", "source", "source_subset", "release", "retrieved_at", "content_checksum", "license",
		"license_review_status", "authority_role", "authoritative_relations", "allowed_scopes", "languages",
		"adapter_version", "provenance_locator", "approved_by", "approved_at", "identity_authority", "notes",
	}).AddRow(
		p.ProviderID, p.Source, p.SourceSubset, p.Release, p.RetrievedAt, p.ContentChecksum, p.License,
		p.LicenseReviewStatus, p.AuthorityRole, "{exact_equivalent}", "{display-metric}", `{"en","zh-CN"}`,
		p.AdapterVersion, p.ProvenanceLocator, p.ApprovedBy, *p.ApprovedAt, p.IdentityAuthority, p.Notes,
	)
}
