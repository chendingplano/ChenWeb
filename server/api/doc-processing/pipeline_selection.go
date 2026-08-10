package docprocessing

import (
	"fmt"
	"strings"
	"sync"
)

const DefaultProductionPipelineName = "legacy_default"

type ProductionPipelineSpec struct {
	Name             string
	DisplayName      string
	Processors       []string
	LegacyEquivalent bool
	// Version is the kb.pipelines.version this spec was loaded from (ADR
	// 2026081001 DR1). Zero for the legacy-equivalent fallback registry
	// (defaultProductionPipelines), which has no backing DB row.
	Version int
}

type ProductionPipelineSelection struct {
	PipelineName string
	Reason       string
}

type ProductionPipelineBindingResolution struct {
	RequestedPipeline  string
	StoreBoundPipeline string
	// RuleName is the name of the policy binding/rule row that won. It keeps
	// the historical field name for persisted-plan compatibility.
	RuleName         string
	Source           string
	SelectedPipeline string
	// SelectedPipelineVersion is the resolved kb.pipelines.version for
	// SelectedPipeline (ADR 2026081001 DR3) -- replaces the retired
	// system-wide PolicyID/PolicyVersion provenance; naming the actual
	// pipeline used is strictly more precise than an opaque policy counter.
	SelectedPipelineVersion int
	BindingTrace            []PipelineBindingTrace `json:",omitempty"`
	// BindingID/PredicateChecksum mirror PipelineBindingSelection's fields
	// for the winning conditional binding (zero value for every other
	// source), so callers building a D2 clearance subject don't need to
	// re-resolve bindings.
	BindingID         int64  `json:",omitempty"`
	PredicateChecksum string `json:",omitempty"`
}

type ProductionPipelineResolution struct {
	Binding   ProductionPipelineBindingResolution
	Selection ProductionPipelineSelection
	Spec      ProductionPipelineSpec
}

// defaultProductionPipelines is the legacy-equivalent fallback registry used
// whenever no authored kb.pipelines rows have been loaded into the process
// (before LoadProductionPipelineRegistry runs, or if it fails). It preserves
// P1's "no active policy means legacy behavior" invariant without a database
// dependency.
var defaultProductionPipelines = []ProductionPipelineSpec{
	{Name: DefaultProductionPipelineName, DisplayName: "Legacy Default", LegacyEquivalent: true},
	{Name: "store_default", DisplayName: "Store Default"},
	{Name: "request_override", DisplayName: "Request Override"},
}

var (
	productionPipelineRegistryMu sync.RWMutex
	productionPipelineRegistry   []ProductionPipelineSpec // nil/empty means "use defaultProductionPipelines"
)

// SetProductionPipelineRegistry installs the in-process pipeline registry
// consulted by LookupProductionPipeline. Pass nil (or an empty slice) to
// revert to the legacy-equivalent fallback registry.
func SetProductionPipelineRegistry(specs []ProductionPipelineSpec) {
	productionPipelineRegistryMu.Lock()
	defer productionPipelineRegistryMu.Unlock()
	productionPipelineRegistry = specs
}

func currentProductionPipelineRegistry() []ProductionPipelineSpec {
	productionPipelineRegistryMu.RLock()
	defer productionPipelineRegistryMu.RUnlock()
	if len(productionPipelineRegistry) == 0 {
		return defaultProductionPipelines
	}
	return productionPipelineRegistry
}

func ResolveProductionPipelineSelection(facts ProductionPlanFacts) (ProductionPipelineSelection, error) {
	binding, err := ResolveProductionPipelineBinding(facts)
	if err != nil {
		return ProductionPipelineSelection{}, err
	}
	return ProductionPipelineSelection{
		PipelineName: binding.SelectedPipeline,
		Reason:       binding.Source,
	}, nil
}

// ResolveProductionPipelineBinding applies binding precedence:
// explicit request > canonical conditional/store-default binding >
// knowledge-store binding > system default. The legacy flat rule matcher is
// retained only for parity tests while P5 closes out.
func ResolveProductionPipelineBinding(facts ProductionPlanFacts) (ProductionPipelineBindingResolution, error) {
	requested := strings.TrimSpace(facts.RequestedPipeline)
	storeBound := strings.TrimSpace(facts.StoreBoundPipeline)
	if requested != "" {
		spec, ok := LookupProductionPipeline(requested)
		if !ok {
			return ProductionPipelineBindingResolution{}, fmt.Errorf("unknown requested pipeline %q", requested)
		}
		return ProductionPipelineBindingResolution{
			RequestedPipeline:       requested,
			StoreBoundPipeline:      storeBound,
			Source:                  "explicit_request",
			SelectedPipeline:        requested,
			SelectedPipelineVersion: spec.Version,
		}, nil
	}
	if bindings := currentProductionPipelineBindings(); len(bindings) > 0 {
		onConflict, onConflictErr := DocPipelineOnConflictFromEnv()
		if onConflictErr != nil {
			onConflict = PipelineBindingOnConflictBlock
		}
		selection, err := ResolvePipelineBindings(bindings, BuildPipelineBindingFactSet(facts), onConflict)
		if err != nil {
			return ProductionPipelineBindingResolution{}, err
		}
		if selection.SelectedPipeline != DefaultProductionPipelineName || selection.Source != "system_default" {
			spec, ok := LookupProductionPipeline(selection.SelectedPipeline)
			if !ok {
				return ProductionPipelineBindingResolution{}, fmt.Errorf("pipeline binding %q selected unknown pipeline %q", selection.BindingName, selection.SelectedPipeline)
			}
			return ProductionPipelineBindingResolution{
				RequestedPipeline:       requested,
				StoreBoundPipeline:      storeBound,
				RuleName:                selection.BindingName,
				Source:                  selection.Source,
				SelectedPipeline:        selection.SelectedPipeline,
				SelectedPipelineVersion: spec.Version,
				BindingTrace:            selection.Trace,
				BindingID:               selection.BindingID,
				PredicateChecksum:       selection.PredicateChecksum,
			}, nil
		}
	}
	if storeBound != "" {
		spec, ok := LookupProductionPipeline(storeBound)
		if !ok {
			return ProductionPipelineBindingResolution{}, fmt.Errorf("unknown store-bound pipeline %q", storeBound)
		}
		return ProductionPipelineBindingResolution{
			RequestedPipeline:       requested,
			StoreBoundPipeline:      storeBound,
			Source:                  "knowledge_store_binding",
			SelectedPipeline:        storeBound,
			SelectedPipelineVersion: spec.Version,
		}, nil
	}
	defaultSpec, _ := LookupProductionPipeline(DefaultProductionPipelineName)
	return ProductionPipelineBindingResolution{
		RequestedPipeline:       requested,
		StoreBoundPipeline:      storeBound,
		Source:                  "system_default",
		SelectedPipeline:        DefaultProductionPipelineName,
		SelectedPipelineVersion: defaultSpec.Version,
	}, nil
}

func ResolveProductionPipelineResolution(facts ProductionPlanFacts) (ProductionPipelineResolution, error) {
	binding, err := ResolveProductionPipelineBinding(facts)
	if err != nil {
		return ProductionPipelineResolution{}, err
	}
	if name := strings.TrimSpace(facts.RequestedPipeline); name != "" {
		_ = name
	}
	selection := ProductionPipelineSelection{
		PipelineName: binding.SelectedPipeline,
		Reason:       binding.Source,
	}
	spec, ok := LookupProductionPipeline(selection.PipelineName)
	if !ok {
		return ProductionPipelineResolution{}, fmt.Errorf("unknown resolved pipeline %q", selection.PipelineName)
	}
	return ProductionPipelineResolution{
		Binding:   binding,
		Selection: selection,
		Spec:      spec,
	}, nil
}

func LookupProductionPipeline(name string) (ProductionPipelineSpec, bool) {
	name = strings.TrimSpace(name)
	for _, spec := range currentProductionPipelineRegistry() {
		if spec.Name == name {
			return spec, true
		}
	}
	return ProductionPipelineSpec{}, false
}
