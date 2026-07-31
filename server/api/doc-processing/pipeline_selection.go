package docprocessing

import (
	"fmt"
	"strings"
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
	Source             string
	SelectedPipeline   string
}

type ProductionPipelineResolution struct {
	Binding   ProductionPipelineBindingResolution
	Selection ProductionPipelineSelection
	Spec      ProductionPipelineSpec
}

var seededProductionPipelines = []ProductionPipelineSpec{
	{Name: DefaultProductionPipelineName, DisplayName: "Legacy Default", LegacyEquivalent: true},
	{Name: "store_default", DisplayName: "Store Default"},
	{Name: "request_override", DisplayName: "Request Override"},
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

func ResolveProductionPipelineBinding(facts ProductionPlanFacts) (ProductionPipelineBindingResolution, error) {
	requested := strings.TrimSpace(facts.RequestedPipeline)
	storeBound := strings.TrimSpace(facts.StoreBoundPipeline)
	if requested != "" {
		if _, ok := LookupSeededProductionPipeline(requested); !ok {
			return ProductionPipelineBindingResolution{}, fmt.Errorf("unknown requested pipeline %q", requested)
		}
		return ProductionPipelineBindingResolution{
			RequestedPipeline:  requested,
			StoreBoundPipeline: storeBound,
			Source:             "explicit_request",
			SelectedPipeline:   requested,
		}, nil
	}
	if storeBound != "" {
		if _, ok := LookupSeededProductionPipeline(storeBound); !ok {
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
	spec, ok := LookupSeededProductionPipeline(selection.PipelineName)
	if !ok {
		return ProductionPipelineResolution{}, fmt.Errorf("unknown resolved pipeline %q", selection.PipelineName)
	}
	return ProductionPipelineResolution{
		Binding:   binding,
		Selection: selection,
		Spec:      spec,
	}, nil
}

func LookupSeededProductionPipeline(name string) (ProductionPipelineSpec, bool) {
	name = strings.TrimSpace(name)
	for _, spec := range seededProductionPipelines {
		if spec.Name == name {
			return spec, true
		}
	}
	return ProductionPipelineSpec{}, false
}
