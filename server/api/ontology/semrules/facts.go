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

func (r *FactRegistry) setOperatorPaths(paths []string, operator string) error {
	if r == nil {
		return errors.New("fact registry is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, path := range paths {
		if _, ok := r.paths[path]; !ok {
			return fmt.Errorf("unknown fact path %q", path)
		}
	}
	for path, spec := range r.paths {
		spec.Operators = removeString(spec.Operators, operator)
		r.paths[path] = spec
	}
	for _, path := range paths {
		spec := r.paths[path]
		if !containsString(spec.Operators, operator) {
			spec.Operators = append(spec.Operators, operator)
			r.paths[path] = spec
		}
	}
	return nil
}

func removeString(values []string, unwanted string) []string {
	result := values[:0]
	for _, value := range values {
		if value != unwanted {
			result = append(result, value)
		}
	}
	return result
}

func clonePathSpec(spec PathSpec) PathSpec {
	spec.Operators = append([]string(nil), spec.Operators...)
	return spec
}

var defaultFactRegistry = newInitialFactRegistry()

// RegisteredFactPaths returns an immutable snapshot of current path metadata.
func RegisteredFactPaths() map[string]PathSpec {
	return defaultFactRegistry.Snapshot()
}

// LookupFactPath returns the registered metadata for an exact, case-sensitive
// canonical path.
func LookupFactPath(path string) (PathSpec, bool) {
	return defaultFactRegistry.Lookup(path)
}

// FactSetBuilder merges independently produced facts without silently
// overwriting two known producers for the same path.
type FactSetBuilder struct {
	facts FactSet
}

func NewFactSetBuilder() *FactSetBuilder {
	return &FactSetBuilder{facts: FactSet{}}
}

func (b *FactSetBuilder) Add(fact Fact) error {
	if b == nil {
		return errors.New("fact set builder is nil")
	}
	if fact.Path == "" {
		return errors.New("fact path is required")
	}
	if fact.State == "" {
		fact.State = FactKnown
	}
	if existing, ok := b.facts[fact.Path]; ok {
		if existing.State == FactKnown && fact.State == FactKnown {
			return fmt.Errorf("duplicate known fact producer for %q", fact.Path)
		}
		if existing.State == FactKnown && fact.State == FactMissing {
			return nil
		}
	}
	b.facts[fact.Path] = fact
	return nil
}

func (b *FactSetBuilder) AddSet(facts FactSet) error {
	for _, fact := range facts {
		if err := b.Add(fact); err != nil {
			return err
		}
	}
	return nil
}

func (b *FactSetBuilder) Build() FactSet {
	if b == nil {
		return FactSet{}
	}
	out := make(FactSet, len(b.facts))
	for path, fact := range b.facts {
		out[path] = fact
	}
	return out
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
		// Tier-1 deterministic facets (spec 2026072901 S3.5 DR4, S16.1 "Facet
		// tiers 1-2"): computed once from the static analyzer's line file by
		// ComputeTier1Facets (server/api/doc-processing/facet_tier1.go). No
		// GovernedValueScheme -- these are measurements, not a classifier
		// vocabulary, and Tier3Producible is deliberately false: the
		// tier-3 LLM classifier has no prompt support for producing a page
		// count or a density ratio, so there is no fallback path for these
		// if tier 1 can't determine one.
		{Path: "document.page_count", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		{Path: "document.toc_presence", Namespace: "document", Type: FactTypeBoolean, Operators: booleanOps},
		{Path: "document.heading_count", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		{Path: "document.table_line_ratio", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		{Path: "document.modal_verb_density", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		{Path: "document.figure_density", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		{Path: "document.language_mix", Namespace: "document", Type: FactTypeNumber, Operators: numberOps},
		// Tier-2 facets: derived from extract_doc_metadata's already-
		// extracted output (doc_no, publish_date) by ComputeTier2Facets.
		// publish_date is FactTypeString, not FactTypeDate: the extractor's
		// raw output isn't guaranteed ISO-normalized (see
		// normalizePublishDateForColumn), and a string-typed fact degrades
		// safely to "unusable for gt/lt" rather than failing to parse.
		{Path: "document.publish_date", Namespace: "document", Type: FactTypeString, Operators: stringOps},
		{Path: "document.authority_hint", Namespace: "document", Type: FactTypeString, Operators: stringOps},
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
