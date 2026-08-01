package docprocessing

import (
	"context"
	"testing"
)

func TestProcessorRegistryIsTheSeam(t *testing.T) {
	// Seam 1: adding a processor with a spec never requires editing the
	// mechanism.
	spec := ProcessorSpec{Name: "extract_test_methods", Phase: "B", Class: "routed", Cost: "cheap_llm", OnUndetermined: "skip"}
	if err := RegisterProcessor(spec); err != nil {
		t.Fatalf("RegisterProcessor: %v", err)
	}
	got, ok := LookupProcessor("extract_test_methods")
	if !ok || got.Cost != "cheap_llm" || got.OnUndetermined != "skip" {
		t.Fatalf("unexpected registered spec: %#v ok=%v", got, ok)
	}
}

func TestProcessorRegistrySeededFromProductionRoster(t *testing.T) {
	if _, ok := LookupProcessor("extract_metrics"); !ok {
		t.Fatal("expected production roster seeded into the registry")
	}
	spec, _ := LookupProcessor("extract_metrics")
	if spec.Phase != "B" {
		t.Fatalf("expected extract_metrics phase B, got %q", spec.Phase)
	}
}

func TestP4OntologyHarvestersAreRoutedProductionProcessors(t *testing.T) {
	for _, name := range []string{"extract_metric_definitions", "extract_product_structure", "extract_test_methods"} {
		spec, ok := LookupProcessor(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if spec.Phase != "B" || spec.Class != "routed" || spec.Cost != "cheap_llm" || spec.OnUndetermined != "skip" {
			t.Fatalf("%s spec=%#v, want a routed cheap Phase-B processor that skips undetermined routing", name, spec)
		}
	}
}

func TestFacetProducerRegistryIsTheSeam(t *testing.T) {
	producer := facetProducerAdapter{name: "tier1_probe", fn: func(ctx context.Context, rec InputRecord) ([]Facet, error) {
		return []Facet{{Key: "probe", Value: "ok", Confidence: 1, Method: "tier1"}}, nil
	}}
	if err := RegisterFacetProducer(producer); err != nil {
		t.Fatalf("RegisterFacetProducer: %v", err)
	}
	fs, err := RunFacetProducers(context.Background(), InputRecord{})
	if err != nil {
		t.Fatalf("RunFacetProducers: %v", err)
	}
	found := false
	for _, f := range fs {
		if f.Key == "probe" && f.Value == "ok" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected produced facet, got %#v", fs)
	}
}

type facetProducerAdapter struct {
	name string
	fn   func(ctx context.Context, rec InputRecord) ([]Facet, error)
}

func (a facetProducerAdapter) Name() string { return a.name }
func (a facetProducerAdapter) Produce(ctx context.Context, rec InputRecord) ([]Facet, error) {
	return a.fn(ctx, rec)
}
