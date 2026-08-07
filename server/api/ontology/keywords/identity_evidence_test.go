package keywords

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

type identityEvidenceProviderStub struct {
	id string
}

func (p identityEvidenceProviderStub) ProviderID() string { return p.id }
func (identityEvidenceProviderStub) LoadClaims(context.Context, *sql.Tx, CandidateIdentityContext) ([]IdentityClaim, error) {
	return nil, nil
}

func TestValidateIdentityEvidenceProviders(t *testing.T) {
	tests := []struct {
		name      string
		providers []IdentityEvidenceProvider
		wantErr   string
	}{
		{name: "required provider", providers: []IdentityEvidenceProvider{identityEvidenceProviderStub{id: TripleEvidenceProviderID}}},
		{name: "additional unique provider", providers: []IdentityEvidenceProvider{identityEvidenceProviderStub{id: "catalog"}, identityEvidenceProviderStub{id: TripleEvidenceProviderID}}},
		{name: "missing required", providers: []IdentityEvidenceProvider{identityEvidenceProviderStub{id: "catalog"}}, wantErr: TripleEvidenceProviderID},
		{name: "blank", providers: []IdentityEvidenceProvider{identityEvidenceProviderStub{id: TripleEvidenceProviderID}, identityEvidenceProviderStub{id: " \t"}}, wantErr: "blank"},
		{name: "duplicate", providers: []IdentityEvidenceProvider{identityEvidenceProviderStub{id: TripleEvidenceProviderID}, identityEvidenceProviderStub{id: TripleEvidenceProviderID}}, wantErr: "duplicate"},
		{name: "nil", providers: []IdentityEvidenceProvider{identityEvidenceProviderStub{id: TripleEvidenceProviderID}, nil}, wantErr: "nil"},
		{name: "typed nil", providers: []IdentityEvidenceProvider{identityEvidenceProviderStub{id: TripleEvidenceProviderID}, (*identityEvidenceProviderStub)(nil)}, wantErr: "nil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdentityEvidenceProviders(tt.providers)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ValidateIdentityEvidenceProviders() error = %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("ValidateIdentityEvidenceProviders() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestCanonicalCandidateIdentityContextSortsAndDeduplicatesSurfaces(t *testing.T) {
	got := CanonicalCandidateIdentityContext(CandidateIdentityContext{
		CandidateConceptID: "candidate", Scope: "scope", SurfaceIDs: []string{"z", "a", "z", "b"},
	})
	want := []string{"a", "b", "z"}
	if strings.Join(got.SurfaceIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("SurfaceIDs = %v, want %v", got.SurfaceIDs, want)
	}
}

func TestNormalizeIdentityClaimsRejectsMalformedClaims(t *testing.T) {
	validRef := EvidenceRef{Key: "authority", Kind: "authority_configuration", Locator: "postgres:authority"}
	valid := IdentityClaim{
		ProviderID: "provider", CandidateConceptID: "candidate", TargetConceptID: "target",
		Relation: IdentityRelationExactEquivalent, Authority: IdentityAuthorityAuthoritative,
		AuthorityRef: "authority", EvidenceRefs: []EvidenceRef{validRef},
	}
	tests := []struct {
		name    string
		mutate  func(*IdentityClaim)
		wantErr string
	}{
		{name: "wrong provider", mutate: func(c *IdentityClaim) { c.ProviderID = "other" }, wantErr: "provider"},
		{name: "wrong candidate", mutate: func(c *IdentityClaim) { c.CandidateConceptID = "other" }, wantErr: "candidate"},
		{name: "unknown relation", mutate: func(c *IdentityClaim) { c.Relation = "synonym" }, wantErr: "relation"},
		{name: "unknown authority", mutate: func(c *IdentityClaim) { c.Authority = "trusted" }, wantErr: "authority"},
		{name: "authoritative non-exact", mutate: func(c *IdentityClaim) { c.Relation = IdentityRelationRelated }, wantErr: "exact_equivalent"},
		{name: "authoritative targetless", mutate: func(c *IdentityClaim) { c.TargetConceptID = "" }, wantErr: "target"},
		{name: "authoritative without refs", mutate: func(c *IdentityClaim) { c.EvidenceRefs = nil }, wantErr: "evidence"},
		{name: "missing authority ref", mutate: func(c *IdentityClaim) { c.AuthorityRef = "" }, wantErr: "authority_ref"},
		{name: "unknown authority ref", mutate: func(c *IdentityClaim) { c.AuthorityRef = "missing" }, wantErr: "authority_ref"},
		{name: "wrong authority ref kind", mutate: func(c *IdentityClaim) { c.EvidenceRefs[0].Kind = "source" }, wantErr: "authority_configuration"},
		{name: "blank ref key", mutate: func(c *IdentityClaim) { c.EvidenceRefs[0].Key = " " }, wantErr: "key"},
		{name: "blank ref kind", mutate: func(c *IdentityClaim) { c.EvidenceRefs[0].Kind = "" }, wantErr: "kind"},
		{name: "blank ref locator", mutate: func(c *IdentityClaim) { c.EvidenceRefs[0].Locator = "\t" }, wantErr: "locator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claim := valid
			claim.EvidenceRefs = append([]EvidenceRef(nil), valid.EvidenceRefs...)
			tt.mutate(&claim)
			_, err := NormalizeIdentityClaims("provider", "candidate", []IdentityClaim{claim})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NormalizeIdentityClaims() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeIdentityClaimsAcceptsClosedEnums(t *testing.T) {
	relations := []IdentityRelation{
		IdentityRelationExactEquivalent, IdentityRelationRelated, IdentityRelationBroader,
		IdentityRelationNarrower, IdentityRelationTranslation, IdentityRelationProbabilistic,
		IdentityRelationOther,
	}
	for _, relation := range relations {
		claim := IdentityClaim{
			ProviderID: "provider", CandidateConceptID: "candidate", Relation: relation,
			Authority:    IdentityAuthorityNonAuthoritative,
			EvidenceRefs: []EvidenceRef{{Key: string(relation), Kind: "test", Locator: "test:" + string(relation)}},
		}
		if _, err := NormalizeIdentityClaims("provider", "candidate", []IdentityClaim{claim}); err != nil {
			t.Errorf("relation %q rejected: %v", relation, err)
		}
	}
}

func TestNormalizeIdentityClaimsRejectsGlobalEvidenceKeyCollision(t *testing.T) {
	claims := []IdentityClaim{
		{ProviderID: "provider", CandidateConceptID: "candidate", Relation: IdentityRelationOther, Authority: IdentityAuthorityNonAuthoritative, EvidenceRefs: []EvidenceRef{{Key: "same", Kind: "one", Locator: "one"}}},
		{ProviderID: "provider", CandidateConceptID: "candidate", TargetConceptID: "target", Relation: IdentityRelationRelated, Authority: IdentityAuthorityNonAuthoritative, EvidenceRefs: []EvidenceRef{{Key: "same", Kind: "two", Locator: "two"}}},
	}
	if _, err := NormalizeIdentityClaims("provider", "candidate", claims); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("NormalizeIdentityClaims() error = %v, want collision", err)
	}
}

func TestNormalizeIdentityClaimsDeduplicatesAndCanonicalizesJSON(t *testing.T) {
	claims := []IdentityClaim{
		{ProviderID: "provider", CandidateConceptID: "candidate", TargetConceptID: "z", Relation: IdentityRelationRelated, Authority: IdentityAuthorityNonAuthoritative, EvidenceRefs: []EvidenceRef{{Key: "b", Kind: "kind", Locator: "two"}, {Key: "a", Kind: "kind", Locator: "one"}}},
		{ProviderID: "provider", CandidateConceptID: "candidate", TargetConceptID: "a", Relation: IdentityRelationOther, Authority: IdentityAuthorityNonAuthoritative, EvidenceRefs: []EvidenceRef{{Key: "c", Kind: "kind", Locator: "three"}}},
		{ProviderID: "provider", CandidateConceptID: "candidate", TargetConceptID: "z", Relation: IdentityRelationRelated, Authority: IdentityAuthorityNonAuthoritative, EvidenceRefs: []EvidenceRef{{Key: "a", Kind: "kind", Locator: "one"}, {Key: "c", Kind: "kind", Locator: "three"}}},
	}
	normalized, err := NormalizeIdentityClaims("provider", "candidate", claims)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(normalized), normalized)
	}
	if normalized[0].TargetConceptID != "a" || normalized[1].TargetConceptID != "z" {
		t.Fatalf("claim order = %#v", normalized)
	}
	if got := []string{normalized[1].EvidenceRefs[0].Key, normalized[1].EvidenceRefs[1].Key, normalized[1].EvidenceRefs[2].Key}; strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("ref order = %v", got)
	}

	jsonOne, err := CanonicalIdentityClaimsJSON("provider", "candidate", claims)
	if err != nil {
		t.Fatal(err)
	}
	shuffled := []IdentityClaim{claims[2], claims[0], claims[1]}
	jsonTwo, err := CanonicalIdentityClaimsJSON("provider", "candidate", shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonOne) != string(jsonTwo) {
		t.Fatalf("canonical JSON differs:\n%s\n%s", jsonOne, jsonTwo)
	}
	want := `[{"provider_id":"provider","candidate_concept_id":"candidate","target_concept_id":"a","relation":"other","authority":"non_authoritative","authority_ref":"","evidence_refs":[{"key":"c","kind":"kind","locator":"three"}]},{"provider_id":"provider","candidate_concept_id":"candidate","target_concept_id":"z","relation":"related","authority":"non_authoritative","authority_ref":"","evidence_refs":[{"key":"a","kind":"kind","locator":"one"},{"key":"b","kind":"kind","locator":"two"},{"key":"c","kind":"kind","locator":"three"}]}]`
	if string(jsonOne) != want {
		t.Fatalf("JSON = %s\nwant = %s", jsonOne, want)
	}
}
