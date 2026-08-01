package assertions

import (
	"context"
	"fmt"
	"sync"
)

// AssociationResolver implements the resolve/validate/adjudicate/persist
// step (spec §10.3-§10.7) for one source_artifact_type's decision
// candidates. Registering a resolver for a new family is what lets
// AssociateSemantics.Run pick up its candidates without editing this
// package's Run/processOne dispatch -- DR11 seam 5 extended from candidate
// generation (normalize_assertions) through to adjudication
// (associate_semantics), closing the gap where the registry was open but
// its only consumer's SQL filter and dispatch switch were hardcoded to
// 'metric'/'provision'.
type AssociationResolver func(ctx context.Context, a AssociateSemantics, dcStore DecisionCandidateStore, asStore AssertionStore, evStore EvidenceStore, dc DecisionCandidate, inputRecordID int64) (string, error)

type associationResolverRegistry struct {
	mu        sync.RWMutex
	resolvers map[string]AssociationResolver
}

var defaultAssociationResolvers = &associationResolverRegistry{resolvers: map[string]AssociationResolver{}}

// RegisterAssociationResolver adds a resolver for one source_artifact_type
// to the default registry. Registering two resolvers for the same type is a
// programming error.
func RegisterAssociationResolver(sourceArtifactType string, resolver AssociationResolver) error {
	if sourceArtifactType == "" {
		return fmt.Errorf("source artifact type is required")
	}
	if resolver == nil {
		return fmt.Errorf("resolver is required for source artifact type %q", sourceArtifactType)
	}
	defaultAssociationResolvers.mu.Lock()
	defer defaultAssociationResolvers.mu.Unlock()
	if _, exists := defaultAssociationResolvers.resolvers[sourceArtifactType]; exists {
		return fmt.Errorf("resolver already registered for source artifact type %q", sourceArtifactType)
	}
	defaultAssociationResolvers.resolvers[sourceArtifactType] = resolver
	return nil
}

// LookupAssociationResolver returns the default registry's resolver for a
// source_artifact_type, if any.
func LookupAssociationResolver(sourceArtifactType string) (AssociationResolver, bool) {
	defaultAssociationResolvers.mu.RLock()
	defer defaultAssociationResolvers.mu.RUnlock()
	r, ok := defaultAssociationResolvers.resolvers[sourceArtifactType]
	return r, ok
}

// RegisteredAssociationResolverTypes returns every source_artifact_type with
// a registered resolver, in no particular order.
func RegisteredAssociationResolverTypes() []string {
	defaultAssociationResolvers.mu.RLock()
	defer defaultAssociationResolvers.mu.RUnlock()
	out := make([]string, 0, len(defaultAssociationResolvers.resolvers))
	for t := range defaultAssociationResolvers.resolvers {
		out = append(out, t)
	}
	return out
}
