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
	// RuleName is the name of the kb.pipeline_rules row that won, set only
	// when Source == "rule_match". Explains *why* a rule-based selection
	// happened, the same way StoreBoundPipeline/RequestedPipeline already
	// explain the other two non-default sources.
	RuleName         string
	Source           string
	SelectedPipeline string
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

// ResolveProductionPipelineBinding applies P1's binding precedence:
// explicit request > facet-matched rule > knowledge-store binding > system
// default. Rule matching sits between request and store binding because a
// rule is a more specific, facet-conditioned policy than a store's flat
// default, but an explicit per-request override always wins outright.
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
		}, nil
	}
	if ruleName, ruleSelected, matched, err := resolveProductionPipelineRuleMatchName(facts.RoutingFacets); err != nil {
		return ProductionPipelineBindingResolution{}, err
	} else if matched {
		if _, ok := LookupProductionPipeline(ruleSelected); !ok {
			return ProductionPipelineBindingResolution{}, fmt.Errorf("pipeline rule %q selected unknown pipeline %q", ruleName, ruleSelected)
		}
		return ProductionPipelineBindingResolution{
			RequestedPipeline:  requested,
			StoreBoundPipeline: storeBound,
			RuleName:           ruleName,
			Source:             "rule_match",
			SelectedPipeline:   ruleSelected,
		}, nil
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
		}, nil
	}
	return ProductionPipelineBindingResolution{
		RequestedPipeline:  requested,
		StoreBoundPipeline: storeBound,
		Source:             "system_default",
		SelectedPipeline:   DefaultProductionPipelineName,
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
