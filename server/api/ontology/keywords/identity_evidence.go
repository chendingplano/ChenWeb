package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const TripleEvidenceProviderID = "triple_external_identity"

type IdentityRelation string

const (
	IdentityRelationExactEquivalent IdentityRelation = "exact_equivalent"
	IdentityRelationRelated         IdentityRelation = "related"
	IdentityRelationBroader         IdentityRelation = "broader"
	IdentityRelationNarrower        IdentityRelation = "narrower"
	IdentityRelationTranslation     IdentityRelation = "translation"
	IdentityRelationProbabilistic   IdentityRelation = "probabilistic"
	IdentityRelationOther           IdentityRelation = "other"
)

type IdentityAuthority string

const (
	IdentityAuthorityNonAuthoritative IdentityAuthority = "non_authoritative"
	IdentityAuthorityAuthoritative    IdentityAuthority = "authoritative"
)

type IdentityEvidenceProvider interface {
	ProviderID() string
	LoadClaims(context.Context, *sql.Tx, CandidateIdentityContext) ([]IdentityClaim, error)
}

type CandidateIdentityContext struct {
	CandidateConceptID string   `json:"candidate_concept_id"`
	Scope              string   `json:"scope"`
	SurfaceIDs         []string `json:"surface_ids"`
}

type IdentityClaim struct {
	ProviderID         string            `json:"provider_id"`
	CandidateConceptID string            `json:"candidate_concept_id"`
	TargetConceptID    string            `json:"target_concept_id"`
	Relation           IdentityRelation  `json:"relation"`
	Authority          IdentityAuthority `json:"authority"`
	AuthorityRef       string            `json:"authority_ref"`
	EvidenceRefs       []EvidenceRef     `json:"evidence_refs"`
}

type EvidenceRef struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"`
	Locator string `json:"locator"`
}

var validIdentityRelations = map[IdentityRelation]bool{
	IdentityRelationExactEquivalent: true,
	IdentityRelationRelated:         true,
	IdentityRelationBroader:         true,
	IdentityRelationNarrower:        true,
	IdentityRelationTranslation:     true,
	IdentityRelationProbabilistic:   true,
	IdentityRelationOther:           true,
}

var validIdentityAuthorities = map[IdentityAuthority]bool{
	IdentityAuthorityNonAuthoritative: true,
	IdentityAuthorityAuthoritative:    true,
}

// ValidateIdentityEvidenceProviders validates the provider registry before a
// reconciliation scan begins. The triple-backed provider is mandatory.
func ValidateIdentityEvidenceProviders(providers []IdentityEvidenceProvider) error {
	seen := make(map[string]bool, len(providers))
	for _, provider := range providers {
		if provider == nil || isNilIdentityEvidenceProvider(provider) {
			return errors.New("identity evidence provider is nil")
		}
		id := provider.ProviderID()
		if strings.TrimSpace(id) == "" {
			return errors.New("identity evidence provider ID is blank")
		}
		if seen[id] {
			return fmt.Errorf("duplicate identity evidence provider ID %q", id)
		}
		seen[id] = true
	}
	if !seen[TripleEvidenceProviderID] {
		return fmt.Errorf("required identity evidence provider %q is missing", TripleEvidenceProviderID)
	}
	return nil
}

func isNilIdentityEvidenceProvider(provider IdentityEvidenceProvider) bool {
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// CanonicalCandidateIdentityContext supplies providers a stable surface set.
func CanonicalCandidateIdentityContext(candidate CandidateIdentityContext) CandidateIdentityContext {
	surfaceSet := make(map[string]struct{}, len(candidate.SurfaceIDs))
	for _, surfaceID := range candidate.SurfaceIDs {
		surfaceSet[surfaceID] = struct{}{}
	}
	candidate.SurfaceIDs = make([]string, 0, len(surfaceSet))
	for surfaceID := range surfaceSet {
		candidate.SurfaceIDs = append(candidate.SurfaceIDs, surfaceID)
	}
	sort.Strings(candidate.SurfaceIDs)
	return candidate
}

type identityClaimKey struct {
	providerID         string
	candidateConceptID string
	targetConceptID    string
	relation           IdentityRelation
	authority          IdentityAuthority
	authorityRef       string
}

type evidenceIdentity struct {
	kind    string
	locator string
}

// NormalizeIdentityClaims validates provider output, rejects evidence-key
// collisions, and returns claims and references in canonical audit order.
func NormalizeIdentityClaims(providerID, candidateConceptID string, claims []IdentityClaim) ([]IdentityClaim, error) {
	evidenceKeys := make(map[string]evidenceIdentity)
	grouped := make(map[identityClaimKey]map[EvidenceRef]struct{}, len(claims))
	for claimIndex, claim := range claims {
		if claim.ProviderID != providerID {
			return nil, fmt.Errorf("identity claim %d provider %q does not match invoked provider %q", claimIndex, claim.ProviderID, providerID)
		}
		if claim.CandidateConceptID != candidateConceptID {
			return nil, fmt.Errorf("identity claim %d candidate %q does not match requested candidate %q", claimIndex, claim.CandidateConceptID, candidateConceptID)
		}
		if !validIdentityRelations[claim.Relation] {
			return nil, fmt.Errorf("identity claim %d has unknown relation %q", claimIndex, claim.Relation)
		}
		if !validIdentityAuthorities[claim.Authority] {
			return nil, fmt.Errorf("identity claim %d has unknown authority %q", claimIndex, claim.Authority)
		}

		authorityRefValid := false
		refs := make(map[EvidenceRef]struct{}, len(claim.EvidenceRefs))
		for refIndex, ref := range claim.EvidenceRefs {
			if strings.TrimSpace(ref.Key) == "" {
				return nil, fmt.Errorf("identity claim %d evidence ref %d key is blank", claimIndex, refIndex)
			}
			if strings.TrimSpace(ref.Kind) == "" {
				return nil, fmt.Errorf("identity claim %d evidence ref %d kind is blank", claimIndex, refIndex)
			}
			if strings.TrimSpace(ref.Locator) == "" {
				return nil, fmt.Errorf("identity claim %d evidence ref %d locator is blank", claimIndex, refIndex)
			}
			identity := evidenceIdentity{kind: ref.Kind, locator: ref.Locator}
			if existing, ok := evidenceKeys[ref.Key]; ok && existing != identity {
				return nil, fmt.Errorf("identity evidence key collision for %q", ref.Key)
			}
			evidenceKeys[ref.Key] = identity
			refs[ref] = struct{}{}
			if ref.Key == claim.AuthorityRef && ref.Kind == "authority_configuration" {
				authorityRefValid = true
			}
		}

		if claim.Authority == IdentityAuthorityAuthoritative {
			if claim.Relation != IdentityRelationExactEquivalent {
				return nil, fmt.Errorf("identity claim %d is authoritative without exact_equivalent relation", claimIndex)
			}
			if strings.TrimSpace(claim.TargetConceptID) == "" {
				return nil, fmt.Errorf("identity claim %d authoritative target is blank", claimIndex)
			}
			if len(claim.EvidenceRefs) == 0 {
				return nil, fmt.Errorf("identity claim %d authoritative evidence is empty", claimIndex)
			}
			if strings.TrimSpace(claim.AuthorityRef) == "" {
				return nil, fmt.Errorf("identity claim %d authority_ref is blank", claimIndex)
			}
			if !authorityRefValid {
				return nil, fmt.Errorf("identity claim %d authority_ref must name an authority_configuration evidence ref", claimIndex)
			}
		}

		key := identityClaimKey{
			providerID: claim.ProviderID, candidateConceptID: claim.CandidateConceptID,
			targetConceptID: claim.TargetConceptID, relation: claim.Relation,
			authority: claim.Authority, authorityRef: claim.AuthorityRef,
		}
		if grouped[key] == nil {
			grouped[key] = make(map[EvidenceRef]struct{})
		}
		for ref := range refs {
			grouped[key][ref] = struct{}{}
		}
	}

	normalized := make([]IdentityClaim, 0, len(grouped))
	for key, refSet := range grouped {
		claim := IdentityClaim{
			ProviderID: key.providerID, CandidateConceptID: key.candidateConceptID,
			TargetConceptID: key.targetConceptID, Relation: key.relation,
			Authority: key.authority, AuthorityRef: key.authorityRef,
			EvidenceRefs: make([]EvidenceRef, 0, len(refSet)),
		}
		for ref := range refSet {
			claim.EvidenceRefs = append(claim.EvidenceRefs, ref)
		}
		sort.Slice(claim.EvidenceRefs, func(i, j int) bool {
			left, right := claim.EvidenceRefs[i], claim.EvidenceRefs[j]
			if left.Key != right.Key {
				return left.Key < right.Key
			}
			if left.Kind != right.Kind {
				return left.Kind < right.Kind
			}
			return left.Locator < right.Locator
		})
		normalized = append(normalized, claim)
	}
	sort.Slice(normalized, func(i, j int) bool { return identityClaimLess(normalized[i], normalized[j]) })
	return normalized, nil
}

func identityClaimLess(left, right IdentityClaim) bool {
	leftTuple := [...]string{
		left.ProviderID, left.CandidateConceptID, left.TargetConceptID,
		string(left.Relation), string(left.Authority), left.AuthorityRef,
	}
	rightTuple := [...]string{
		right.ProviderID, right.CandidateConceptID, right.TargetConceptID,
		string(right.Relation), string(right.Authority), right.AuthorityRef,
	}
	for i := range leftTuple {
		if leftTuple[i] != rightTuple[i] {
			return leftTuple[i] < rightTuple[i]
		}
	}
	return false
}

// CanonicalIdentityClaimsJSON returns fixed-field-order JSON suitable for an
// audit payload. It performs validation and normalization before marshaling.
func CanonicalIdentityClaimsJSON(providerID, candidateConceptID string, claims []IdentityClaim) ([]byte, error) {
	normalized, err := NormalizeIdentityClaims(providerID, candidateConceptID, claims)
	if err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}
