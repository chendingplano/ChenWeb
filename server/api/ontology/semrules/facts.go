package semrules

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// FactRegistry stores the code-owned metadata for canonical fact paths.
type FactRegistry struct {
	mu    sync.RWMutex
	paths map[string]PathSpec
}

// NewFactRegistry returns an empty fact-path registry.
func NewFactRegistry() *FactRegistry {
	return &FactRegistry{paths: make(map[string]PathSpec)}
}

// Register adds a path specification. Existing paths cannot be replaced.
func (r *FactRegistry) Register(spec PathSpec) error {
	if r == nil {
		return errors.New("fact registry is nil")
	}
	if strings.TrimSpace(spec.Path) == "" {
		return errors.New("fact path is required")
	}
	if strings.TrimSpace(spec.Namespace) == "" {
		return errors.New("fact namespace is required")
	}
	if spec.Type == "" {
		return errors.New("fact type is required")
	}
	if len(spec.Operators) == 0 {
		return errors.New("at least one fact operator is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.paths[spec.Path]; exists {
		return fmt.Errorf("fact path %q is already registered", spec.Path)
	}
	r.paths[spec.Path] = clonePathSpec(spec)
	return nil
}

// Lookup returns a copy of the specification for path.
func (r *FactRegistry) Lookup(path string) (PathSpec, bool) {
	if r == nil {
		return PathSpec{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.paths[path]
	return clonePathSpec(spec), ok
}

// Snapshot returns a deep copy suitable for inspection by callers.
func (r *FactRegistry) Snapshot() map[string]PathSpec {
	if r == nil {
		return map[string]PathSpec{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot := make(map[string]PathSpec, len(r.paths))
	for path, spec := range r.paths {
		snapshot[path] = clonePathSpec(spec)
	}
	return snapshot
}

func clonePathSpec(spec PathSpec) PathSpec {
	spec.Operators = append([]string(nil), spec.Operators...)
	return spec
}

var defaultFactRegistry = newInitialFactRegistry()

// RegisteredFactPaths returns an immutable snapshot of the initial registry.
func RegisteredFactPaths() map[string]PathSpec {
	return defaultFactRegistry.Snapshot()
}

// LookupFactPath returns the registered metadata for an exact, case-sensitive
// canonical path.
func LookupFactPath(path string) (PathSpec, bool) {
	return defaultFactRegistry.Lookup(path)
}

func newInitialFactRegistry() *FactRegistry {
	registry := NewFactRegistry()
	for _, spec := range initialPathSpecs() {
		if err := registry.Register(spec); err != nil {
			panic(fmt.Sprintf("initialize fact registry: %v", err))
		}
	}
	return registry
}

func initialPathSpecs() []PathSpec {
	stringOps := []string{"eq", "neq", "in", "not_in", "exists"}
	numberOps := []string{"eq", "neq", "in", "not_in", "gt", "gte", "lt", "lte", "exists"}
	booleanOps := []string{"eq", "neq", "exists"}
	dateOps := []string{"eq", "neq", "in", "not_in", "gt", "gte", "lt", "lte", "exists"}
	stringSetOps := []string{"contains", "exists"}

	return []PathSpec{
		{Path: "document.input_doc_type", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		{Path: "document.source_language", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		{Path: "document.knowledge_store_binding_state", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		{Path: "document.has_document_number", Namespace: "document", Type: FactTypeBoolean, Operators: booleanOps},
		{Path: "document.numeric_unit_density", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		{Path: "document.doc_kind", Namespace: "document", Type: FactTypeString, Operators: stringOps, Tier3Producible: true, GovernedValueScheme: "document.doc_kind"},
		{Path: "document.domain", Namespace: "document", Type: FactTypeString, Operators: stringOps, Tier3Producible: true, GovernedValueScheme: "document.domain"},
		{Path: "document.normative_status", Namespace: "document", Type: FactTypeString, Operators: stringOps, Tier3Producible: true, GovernedValueScheme: "document.normative_status"},
		{Path: "document.jurisdiction", Namespace: "document", Type: FactTypeString, Operators: stringOps, Tier3Producible: true, GovernedValueScheme: "jurisdiction"},
		{Path: "object.class", Namespace: "object", Type: FactTypeStringSet, Operators: stringSetOps},
		{Path: "review.as_of", Namespace: "review", Type: FactTypeDate, Operators: dateOps},
		{Path: "review.jurisdiction", Namespace: "review", Type: FactTypeString, Operators: stringOps, GovernedValueScheme: "jurisdiction"},
		{Path: "review.operating_context", Namespace: "review", Type: FactTypeString, Operators: stringOps},
		{Path: "review.purpose", Namespace: "review", Type: FactTypeString, Operators: stringOps, GovernedValueScheme: "review.purpose"},
		{Path: "deployment.workspace", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
		{Path: "deployment.tenant", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
		{Path: "deployment.knowledge_store", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
		{Path: "deployment.user", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
		{Path: "deployment.corpus", Namespace: "deployment", Type: FactTypeString, Operators: stringOps},
	}
}
