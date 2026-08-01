package docprocessing

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ProcessorRegistry is extension seam 1 (ADR DR11): adding a processor with a
// spec must not require editing the mechanism. The registry maps canonical
// processor name -> ProcessorSpec; it is seeded from the production roster and
// is swappable (the same pattern the pipeline registries use).
var (
	processorRegistryMu sync.RWMutex
	processorRegistry   = map[string]ProcessorSpec{}
)

func init() {
	for _, s := range productionProcessorSpecs {
		processorRegistry[s.Name] = s
	}
}

// RegisterProcessor adds a processor spec to the registry (seam 1). A name
// that is already registered is replaced.
func RegisterProcessor(spec ProcessorSpec) error {
	if spec.Name == "" {
		return errors.New("processor spec name is required")
	}
	processorRegistryMu.Lock()
	defer processorRegistryMu.Unlock()
	processorRegistry[spec.Name] = spec
	return nil
}

// SetProcessorRegistry replaces the registry wholesale (used by tests and the
// startup loader).
func SetProcessorRegistry(m map[string]ProcessorSpec) {
	processorRegistryMu.Lock()
	defer processorRegistryMu.Unlock()
	processorRegistry = m
}

// LookupProcessor returns the spec for a canonical processor name.
func LookupProcessor(name string) (ProcessorSpec, bool) {
	processorRegistryMu.RLock()
	defer processorRegistryMu.RUnlock()
	s, ok := processorRegistry[name]
	return s, ok
}

// RegisteredProcessors returns the registered specs in canonical order.
func RegisteredProcessors() []ProcessorSpec {
	processorRegistryMu.RLock()
	defer processorRegistryMu.RUnlock()
	out := make([]ProcessorSpec, 0, len(processorRegistry))
	for _, s := range processorRegistry {
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Facet is one produced routing/profile fact (DR4). Key and permitted values
// are governed vocabulary in the document-authority module; Method records the
// production tier (tier1 deterministic, tier2 derived, tier3 LLM classify).
type Facet struct {
	Key        string
	Value      any
	Confidence float64
	Method     string
}

// FacetProducer is extension seam 2: registering a producer plus its governed
// facet terms must not require editing the mechanism.
type FacetProducer interface {
	Name() string
	Produce(ctx context.Context, rec InputRecord) ([]Facet, error)
}

var (
	facetProducerMu sync.RWMutex
	facetProducers  = map[string]FacetProducer{}
)

// RegisterFacetProducer adds a facet producer (seam 2).
func RegisterFacetProducer(p FacetProducer) error {
	if p == nil || p.Name() == "" {
		return errors.New("facet producer name is required")
	}
	facetProducerMu.Lock()
	defer facetProducerMu.Unlock()
	facetProducers[p.Name()] = p
	return nil
}

// FacetProducers returns the registered producers, ordered by name.
func FacetProducers() []FacetProducer {
	facetProducerMu.RLock()
	defer facetProducerMu.RUnlock()
	out := make([]FacetProducer, 0, len(facetProducers))
	for _, p := range facetProducers {
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// RunFacetProducers invokes every registered producer for a record and flattens
// the produced facets. A producer error is reported with its name.
func RunFacetProducers(ctx context.Context, rec InputRecord) ([]Facet, error) {
	var out []Facet
	for _, p := range FacetProducers() {
		fs, err := p.Produce(ctx, rec)
		if err != nil {
			return nil, fmt.Errorf("facet producer %s: %w", p.Name(), err)
		}
		out = append(out, fs...)
	}
	return out, nil
}
