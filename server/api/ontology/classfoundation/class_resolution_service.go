package classfoundation

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	ResolutionResolvedExisting          = "resolved_existing"
	ResolutionProvisionalNew            = "provisional_new"
	ResolutionAmbiguousCandidates       = "ambiguous_candidates"
	ResolutionCandidateEvidenceConflict = "candidate_evidence_conflict"
	ResolutionRejected                  = "rejected"
)

// IdentityClassCreator is satisfied by ContractStore and lets resolution
// create an identity-only class without granting it unsupported capability.
type IdentityClassCreator interface {
	CreateIdentityOnlyClass(context.Context, ClassIdentity) (ContractRevision, error)
}

type ClassResolutionCandidate struct {
	TermID             string
	Label              string
	QuantityKindTermID string
	Safe               bool
}

type ClassResolutionRequest struct {
	QuantityKindTermID string
	Candidates         []ClassResolutionCandidate
	ProvisionalClass   ClassIdentity
}

type ClassResolutionResult struct {
	SelectedClassTermID string
	IdentityState       string
	Alternatives        []ClassResolutionCandidate
}

// ClassResolutionService makes the narrow deterministic decision permitted in
// shadow mode: reuse exactly one safe compatible class, or create/select a
// provisional identity-only class. It never returns a classless outcome.
type ClassResolutionService struct {
	ProvisionalClasses IdentityClassCreator
}

func (s ClassResolutionService) Resolve(ctx context.Context, request ClassResolutionRequest) (ClassResolutionResult, error) {
	compatible := make([]ClassResolutionCandidate, 0, len(request.Candidates))
	quantity := strings.TrimSpace(request.QuantityKindTermID)
	for _, candidate := range request.Candidates {
		if !candidate.Safe || strings.TrimSpace(candidate.TermID) == "" {
			continue
		}
		if strings.TrimSpace(candidate.QuantityKindTermID) != quantity {
			continue
		}
		compatible = append(compatible, candidate)
	}
	if len(compatible) == 1 {
		return ClassResolutionResult{
			SelectedClassTermID: compatible[0].TermID,
			IdentityState:       ResolutionResolvedExisting,
			Alternatives:        append([]ClassResolutionCandidate(nil), request.Candidates...),
		}, nil
	}
	if s.ProvisionalClasses == nil {
		return ClassResolutionResult{}, errors.New("provisional class creator is required when no unique safe class exists")
	}
	identity := request.ProvisionalClass
	if strings.TrimSpace(identity.TermID) == "" || strings.TrimSpace(identity.ModuleID) == "" {
		return ClassResolutionResult{}, errors.New("provisional class term ID and module ID are required")
	}
	created, err := s.ProvisionalClasses.CreateIdentityOnlyClass(ctx, identity)
	if err != nil {
		return ClassResolutionResult{}, fmt.Errorf("create provisional class: %w", err)
	}
	state := ResolutionProvisionalNew
	if len(compatible) > 1 {
		state = ResolutionAmbiguousCandidates
	}
	return ClassResolutionResult{
		SelectedClassTermID: created.TermID,
		IdentityState:       state,
		Alternatives:        append([]ClassResolutionCandidate(nil), request.Candidates...),
	}, nil
}
