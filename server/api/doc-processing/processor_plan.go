package docprocessing

import (
	"fmt"
	"strings"
)

type ProcessorSpec struct {
	Name      string
	Phase     string
	DependsOn []string
}

type ProductionRoutingFacets struct {
	KnowledgeStoreBinding string
	InputDocType          string
	SourceLanguage        string
	HasDocumentNumber     bool
}

type ProductionPlanFacts struct {
	RequestedProcessors []string
	RequestedPipeline   string
	StoreBoundPipeline  string
	KnowledgeStoreID    int64
	KnowledgeStoreType  string
	InputDocType        string
	SourceLanguage      string
	DocumentNumber      string
	ParserName          string
	DocumentTitle       string
	RoutingFacets       ProductionRoutingFacets
	// Mode is the resolved DocPipelineMode for this plan (DocPipelineModePlanOnly
	// or DocPipelineModeEnforced). Zero value ("") behaves as plan-only, so
	// callers that don't set it (tests, the constructor-time BuildProductionProcessorPlan
	// seam) get legacy-equivalent behavior by default.
	Mode string
}

type ProductionProcessorPlan struct {
	specs             []ProcessorSpec
	steps             []ProcessorPlanStep
	facts             ProductionPlanFacts
	pipelineBinding   ProductionPipelineBindingResolution
	pipelineSelection ProductionPipelineSelection
	pipelineSpec      ProductionPipelineSpec
	excludedByPolicy  []string
}

type ProcessorPlanStep struct {
	Name      string
	Phase     string
	DependsOn []string
	Reason    string
}

// applyPolicyFilter implements DocPipelineModeEnforced's intersect/filter
// semantics: when enforced and the resolved pipeline declares a non-empty
// Processors list, a requested processor only survives if the pipeline
// allows it. static_analyzer/chunking are exempt — they are the mandatory
// baseline resolveRequiredProcessors always adds regardless of policy, not
// something a pipeline could meaningfully exclude. A pipeline with an empty
// Processors list has no effect (it hasn't opted into governing selection),
// and plan-only mode never filters, so both return the request unchanged.
// Returns the filtered request and, separately, what got excluded — the
// exclusion is recorded on the plan rather than silently dropped, per the
// ADR's "plan computation is explainable" invariant.
func applyPolicyFilter(requested []string, mode string, spec ProductionPipelineSpec) (effective []string, excluded []string) {
	if mode != DocPipelineModeEnforced || len(spec.Processors) == 0 {
		return requested, nil
	}
	allowed := map[string]bool{}
	for _, name := range spec.Processors {
		allowed[normalizeRuntimeName(name)] = true
	}
	effective = make([]string, 0, len(requested))
	for _, name := range requested {
		norm := normalizeRuntimeName(name)
		if norm == "static_analyzer" || norm == "chunking" || allowed[norm] {
			effective = append(effective, name)
			continue
		}
		excluded = append(excluded, name)
	}
	return effective, excluded
}

func BuildProductionProcessorPlan(requested []string) (ProductionProcessorPlan, error) {
	return BuildProductionProcessorPlanFromFacts(ProductionPlanFacts{RequestedProcessors: append([]string(nil), requested...)})
}

func BuildProductionPlanFactsFromInputRecord(requested []string, rec DocMetadataInputRecord) ProductionPlanFacts {
	routingFacets := BuildProductionRoutingFacetsFromInputRecord(rec)
	return ProductionPlanFacts{
		RequestedProcessors: append([]string(nil), requested...),
		RequestedPipeline:   strings.TrimSpace(rec.RequestedPipeline),
		StoreBoundPipeline:  strings.TrimSpace(rec.StoreBoundPipeline),
		KnowledgeStoreID:    rec.KSStoreID,
		InputDocType:        rec.InputDocType,
		SourceLanguage:      rec.SourceLanguage,
		DocumentNumber:      rec.DocumentNumber,
		ParserName:          rec.ParserName,
		DocumentTitle:       rec.Title,
		RoutingFacets:       routingFacets,
	}
}

func BuildProductionRoutingFacetsFromInputRecord(rec DocMetadataInputRecord) ProductionRoutingFacets {
	binding := "absent"
	if rec.KSStoreID > 0 {
		binding = "bound"
	}
	return ProductionRoutingFacets{
		KnowledgeStoreBinding: binding,
		InputDocType:          strings.ToLower(strings.TrimSpace(rec.InputDocType)),
		SourceLanguage:        strings.ToLower(strings.TrimSpace(rec.SourceLanguage)),
		HasDocumentNumber:     strings.TrimSpace(rec.DocumentNumber) != "",
	}
}

func BuildProductionProcessorPlanFromFacts(facts ProductionPlanFacts) (ProductionProcessorPlan, error) {
	requested := append([]string(nil), facts.RequestedProcessors...)
	if err := validateRequiredProcessors(requested); err != nil {
		return ProductionProcessorPlan{}, err
	}
	resolution, err := ResolveProductionPipelineResolution(facts)
	if err != nil {
		return ProductionProcessorPlan{}, err
	}
	effectiveRequested, excludedByPolicy := applyPolicyFilter(requested, facts.Mode, resolution.Spec)
	enabled := map[string]bool{}
	requestedSet := map[string]bool{}
	for _, name := range resolveRequiredProcessors(effectiveRequested) {
		enabled[normalizeRuntimeName(name)] = true
	}
	for _, name := range effectiveRequested {
		requestedSet[normalizeRuntimeName(name)] = true
	}
	specs := make([]ProcessorSpec, 0, len(productionProcessorSpecs))
	for _, spec := range productionProcessorSpecs {
		if !enabled[spec.Name] {
			continue
		}
		if spec.Phase != "A" && spec.Phase != "B" && spec.Phase != "C" {
			return ProductionProcessorPlan{}, fmt.Errorf("processor %q has unsupported phase %q", spec.Name, spec.Phase)
		}
		specs = append(specs, spec)
	}
	plan := ProductionProcessorPlan{specs: specs, pipelineBinding: resolution.Binding, pipelineSelection: resolution.Selection, pipelineSpec: resolution.Spec, excludedByPolicy: excludedByPolicy, facts: ProductionPlanFacts{
		RequestedProcessors: append([]string(nil), facts.RequestedProcessors...),
		RequestedPipeline:   strings.TrimSpace(facts.RequestedPipeline),
		StoreBoundPipeline:  strings.TrimSpace(facts.StoreBoundPipeline),
		KnowledgeStoreID:    facts.KnowledgeStoreID,
		KnowledgeStoreType:  facts.KnowledgeStoreType,
		InputDocType:        facts.InputDocType,
		SourceLanguage:      facts.SourceLanguage,
		DocumentNumber:      facts.DocumentNumber,
		ParserName:          facts.ParserName,
		DocumentTitle:       facts.DocumentTitle,
		RoutingFacets:       facts.RoutingFacets,
		Mode:                facts.Mode,
	}}
	order := plan.ExecutionOrder()
	steps := make([]ProcessorPlanStep, 0, len(order))
	for _, name := range order {
		phase := ""
		deps := []string{}
		for _, spec := range specs {
			if spec.Name != name {
				continue
			}
			phase = spec.Phase
			if len(spec.DependsOn) == 0 {
				deps = []string{}
			} else {
				deps = append([]string(nil), spec.DependsOn...)
			}
			break
		}
		reason := "mandatory_baseline"
		if requestedSet[name] {
			reason = "explicit_request"
		} else if len(deps) > 0 {
			reason = "implicit_dependency"
		}
		steps = append(steps, ProcessorPlanStep{
			Name:      name,
			Phase:     phase,
			DependsOn: deps,
			Reason:    reason,
		})
	}
	plan.steps = steps
	return plan, nil
}

func (p ProductionProcessorPlan) AllNames() []string {
	out := make([]string, 0, len(p.specs))
	for _, spec := range p.specs {
		out = append(out, spec.Name)
	}
	return out
}

func (p ProductionProcessorPlan) ExecutionOrder() []string {
	out := make([]string, 0, len(p.specs))
	for _, phase := range []string{"A", "B", "C"} {
		out = append(out, p.PhaseNames(phase)...)
	}
	return out
}

func (p ProductionProcessorPlan) PhaseNames(phase string) []string {
	out := make([]string, 0, len(p.specs))
	for _, spec := range p.specs {
		if spec.Phase == phase {
			out = append(out, spec.Name)
		}
	}
	return out
}

func (p ProductionProcessorPlan) Dependencies(name string) []string {
	name = normalizeRuntimeName(name)
	for _, spec := range p.specs {
		if spec.Name != name {
			continue
		}
		if len(spec.DependsOn) == 0 {
			return []string{}
		}
		return append([]string(nil), spec.DependsOn...)
	}
	return []string{}
}

func (p ProductionProcessorPlan) Steps() []ProcessorPlanStep {
	out := make([]ProcessorPlanStep, len(p.steps))
	copy(out, p.steps)
	return out
}

func (p ProductionProcessorPlan) Facts() ProductionPlanFacts {
	out := p.facts
	out.RequestedProcessors = append([]string(nil), p.facts.RequestedProcessors...)
	return out
}

func (p ProductionProcessorPlan) PipelineSelection() ProductionPipelineSelection {
	return p.pipelineSelection
}

func (p ProductionProcessorPlan) PipelineBinding() ProductionPipelineBindingResolution {
	return p.pipelineBinding
}

func (p ProductionProcessorPlan) PipelineSpec() ProductionPipelineSpec {
	return p.pipelineSpec
}

// ExcludedByPolicy lists requested processors that DocPipelineModeEnforced
// filtered out because they weren't in the resolved pipeline's Processors
// list. Empty in plan-only mode, and empty in enforced mode when the
// resolved pipeline declares no explicit Processors.
func (p ProductionProcessorPlan) ExcludedByPolicy() []string {
	if len(p.excludedByPolicy) == 0 {
		return nil
	}
	return append([]string(nil), p.excludedByPolicy...)
}

var productionProcessorSpecs = []ProcessorSpec{
	{Name: "static_analyzer", Phase: "A"},
	{Name: "chunking", Phase: "A", DependsOn: []string{"static_analyzer"}},
	{Name: "generate_summaries", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "generate_topics", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "extract_doc_metadata", Phase: "A", DependsOn: []string{"chunking"}},
	{Name: "extract_semantic_projections", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "extract_structured_knowledge", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "extract_entity", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "extract_relation", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "extract_inventory_items", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "extract_metrics", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "extract_provisions", Phase: "B", DependsOn: []string{"chunking"}},
	{Name: "generate_scene_blocks", Phase: "B", DependsOn: []string{"chunking"}},
}

func productionProcessorPhase(name string) string {
	name = normalizeRuntimeName(name)
	for _, spec := range productionProcessorSpecs {
		if spec.Name == name {
			return spec.Phase
		}
	}
	return ""
}

func selectableProductionProcessorOrder() []string {
	out := make([]string, 0, len(productionProcessorSpecs))
	for _, spec := range productionProcessorSpecs {
		if spec.Name == "static_analyzer" || spec.Name == "chunking" {
			continue
		}
		out = append(out, spec.Name)
	}
	return out
}
