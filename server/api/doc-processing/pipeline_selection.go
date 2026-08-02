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
	// PolicyID/PolicyVersion identify the kb.pipeline_policies row that was
	// active when this binding was resolved -- DR6's "every run writes...
	// the policy version, the selected pipeline and why" explainability
	// requirement. Zero value means no policy store was consulted.
	PolicyID      int64
	PolicyVersion int
	BindingTrace  []PipelineBindingTrace `json:",omitempty"`
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
		if _, ok := LookupProductionPipeline(requested); !ok {
			return ProductionPipelineBindingResolution{}, fmt.Errorf("unknown requested pipeline %q", requested)
		}
		return ProductionPipelineBindingResolution{
			RequestedPipeline:  requested,
			StoreBoundPipeline: storeBound,
			Source:             "explicit_request",
			SelectedPipeline:   requested,
			PolicyID:           facts.ActivePolicyID,
			PolicyVersion:      facts.ActivePolicyVersion,
		}, nil
	}
	if bindings := currentProductionPipelineBindings(); len(bindings) > 0 {
		selection, err := ResolvePipelineBindings(bindings, BuildPipelineBindingFactSet(facts), PipelineBindingOnConflictBlock)
		if err != nil {
			return ProductionPipelineBindingResolution{}, err
		}
		if selection.SelectedPipeline != DefaultProductionPipelineName || selection.Source != "system_default" {
			if _, ok := LookupProductionPipeline(selection.SelectedPipeline); !ok {
				return ProductionPipelineBindingResolution{}, fmt.Errorf("pipeline binding %q selected unknown pipeline %q", selection.BindingName, selection.SelectedPipeline)
			}
			return ProductionPipelineBindingResolution{
				RequestedPipeline:  requested,
				StoreBoundPipeline: storeBound,
				RuleName:           selection.BindingName,
				Source:             selection.Source,
				SelectedPipeline:   selection.SelectedPipeline,
				PolicyID:           facts.ActivePolicyID,
				PolicyVersion:      facts.ActivePolicyVersion,
				BindingTrace:       selection.Trace,
			}, nil
		}
	}
	if storeBound != "" {
		if _, ok := LookupProductionPipeline(storeBound); !ok {
			return ProductionPipelineBindingResolution{}, fmt.Errorf("unknown store-bound pipeline %q", storeBound)
		}
		return ProductionPipelineBindingResolution{
			RequestedPipeline:  requested,
			StoreBoundPipeline: storeBound,
			Source:             "knowledge_store_binding",
			SelectedPipeline:   storeBound,
			PolicyID:           facts.ActivePolicyID,
			PolicyVersion:      facts.ActivePolicyVersion,
		}, nil
	}
	return ProductionPipelineBindingResolution{
		RequestedPipeline:  requested,
		StoreBoundPipeline: storeBound,
		Source:             "system_default",
		SelectedPipeline:   DefaultProductionPipelineName,
		PolicyID:           facts.ActivePolicyID,
		PolicyVersion:      facts.ActivePolicyVersion,
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
