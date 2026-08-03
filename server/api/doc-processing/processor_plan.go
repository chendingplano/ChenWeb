package docprocessing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// ProcessorSpec is the DR5 declarative processor contract (ADR 2026072901
// DR5). Name/Phase/DependsOn were the P1 core; Requires/Produces/Class/Cost/
// OnUndetermined are the DR5 fields added by P2 seam 1. The DAG planner that
// consumes Requires/Produces arrives in a later phase; the declaration and the
// registry seam are complete now.
type ProcessorSpec struct {
	Name           string
	Phase          string
	DependsOn      []string
	Requires       []string // artifact kinds this processor needs as input
	Produces       []string // artifact kinds this processor emits
	Class          string   // mandatory | routed | on_demand
	Cost           string   // free | cheap_llm | expensive_llm
	OnUndetermined string   // run | skip — the default when rules do not decide
	Idempotent     bool
}

type ProductionRoutingFacets struct {
	KnowledgeStoreBinding string
	InputDocType          string
	SourceLanguage        string
	HasDocumentNumber     bool
	// DocKind is the governed document.doc_kind tier-3 facet (spec
	// 2026080102 section 9's clearance coverage key), populated from the
	// resolver's EnrichedFacts once known. Empty means unresolved -- spec
	// section 9 requires an empty document kind to leave the subject
	// shadow-only, never falling back to InputDocType (the file format, a
	// different dimension entirely). See documentKindFromEnrichedFacts in
	// control.go (P5 review 2026080302 finding P5-6). Omitted from
	// persisted-plan JSON when empty (the common case until the tier-3
	// classifier is live) to keep existing plan_facts snapshots
	// byte-compatible.
	DocKind string `json:",omitempty"`
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
	// ActivePolicyID/ActivePolicyVersion identify the kb.pipeline_policies
	// row that was active when this plan was built. Zero value means no
	// policy store was consulted (tests, the constructor-time
	// BuildProductionProcessorPlan seam) -- resolution behaves identically
	// either way, since bindings/rules matching is unaffected; these fields
	// exist purely for the persisted plan's explainability.
	ActivePolicyID            int64
	ActivePolicyVersion       int
	ActivePolicyChecksum      string             `json:",omitempty"`
	ExplicitProcessorOverride bool               `json:",omitempty"`
	ProcessorGateOverrides    map[string]string  `json:",omitempty"`
	RoutingSnapshot           *P5RoutingSnapshot `json:",omitempty"`
	// EnrichedFacts carries the two-pass resolver output (P5 spec section 7).
	// When non-nil, BuildPipelineBindingFactSet merges these on top of the
	// base facts so tier-3 classifier observations participate in pipeline
	// binding and processor gate evaluation. Nil means base facts only.
	EnrichedFacts semrules.FactSet `json:"-"`
}

// P5RoutingSnapshot is the immutable, self-contained routing evidence stored
// with an execution plan. Runtime registries are never needed to inspect it.
type P5RoutingSnapshot struct {
	Facts                    []semrules.Fact         `json:"facts"`
	PolicyID                 int64                   `json:"policy_id"`
	PolicyVersion            int                     `json:"policy_version"`
	PolicyChecksum           string                  `json:"policy_checksum,omitempty"`
	SelectedPipelineChecksum string                  `json:"selected_pipeline_checksum"`
	BaselinePipelineChecksum string                  `json:"baseline_pipeline_checksum"`
	BindingTrace             []PipelineBindingTrace  `json:"binding_trace,omitempty"`
	GateShadow               ProcessorGateShadowPlan `json:"gate_shadow"`
	RuleChecksums            []string                `json:"rule_checksums,omitempty"`
}

type ProductionProcessorPlan struct {
	specs             []ProcessorSpec
	steps             []ProcessorPlanStep
	facts             ProductionPlanFacts
	pipelineBinding   ProductionPipelineBindingResolution
	pipelineSelection ProductionPipelineSelection
	pipelineSpec      ProductionPipelineSpec
	excludedByPolicy  []string
	routingSnapshot   *P5RoutingSnapshot
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
		RequestedProcessors:       append([]string(nil), facts.RequestedProcessors...),
		RequestedPipeline:         strings.TrimSpace(facts.RequestedPipeline),
		StoreBoundPipeline:        strings.TrimSpace(facts.StoreBoundPipeline),
		KnowledgeStoreID:          facts.KnowledgeStoreID,
		KnowledgeStoreType:        facts.KnowledgeStoreType,
		InputDocType:              facts.InputDocType,
		SourceLanguage:            facts.SourceLanguage,
		DocumentNumber:            facts.DocumentNumber,
		ParserName:                facts.ParserName,
		DocumentTitle:             facts.DocumentTitle,
		RoutingFacets:             facts.RoutingFacets,
		Mode:                      facts.Mode,
		ActivePolicyID:            facts.ActivePolicyID,
		ActivePolicyVersion:       facts.ActivePolicyVersion,
		ActivePolicyChecksum:      facts.ActivePolicyChecksum,
		ExplicitProcessorOverride: facts.ExplicitProcessorOverride,
		ProcessorGateOverrides:    cloneStringMap(facts.ProcessorGateOverrides),
	}}
	if facts.RoutingSnapshot != nil {
		plan.routingSnapshot = cloneP5RoutingSnapshot(facts.RoutingSnapshot)
	} else {
		factSet := BuildPipelineBindingFactSet(facts)
		baselineSpec, _ := LookupProductionPipeline(DefaultProductionPipelineName)
		// The gate shadow must cover every processor FinalizeRoutingPlan
		// could encounter, not only those the selected pipeline admits: an
		// uncleared suppressive-binding fallback re-admits whatever the P5
		// baseline/store-default pipeline allows, and a processor with no
		// gate decision runs ungated instead of being gate-evaluated (P5
		// review 2026080302 finding P5-13). Union the selected pipeline's
		// specs with the baseline pipeline's own effective set.
		shadowNames := make([]string, 0, len(specs))
		shadowNameSet := map[string]bool{}
		for _, spec := range specs {
			if !shadowNameSet[spec.Name] {
				shadowNameSet[spec.Name] = true
				shadowNames = append(shadowNames, spec.Name)
			}
		}
		baselineEffective, _ := applyPolicyFilter(requested, facts.Mode, baselineSpec)
		for _, name := range resolveRequiredProcessors(baselineEffective) {
			norm := normalizeRuntimeName(name)
			if !shadowNameSet[norm] {
				shadowNameSet[norm] = true
				shadowNames = append(shadowNames, norm)
			}
		}
		explicit := []string(nil)
		if facts.ExplicitProcessorOverride {
			explicit = append(explicit, facts.RequestedProcessors...)
		}
		onConflict, onConflictErr := DocPipelineOnConflictFromEnv()
		if onConflictErr != nil {
			onConflict = PipelineBindingOnConflictBlock
		}
		shadow, shadowErr := BuildProcessorGateShadowPlan(shadowNames, productionProcessorSpecs, currentProductionPipelineGates(), factSet, GateShadowOptions{
			ExplicitProcessors: explicit,
			RunOverrides:       facts.ProcessorGateOverrides,
			OnConflict:         onConflict,
		})
		if shadowErr != nil {
			return ProductionProcessorPlan{}, shadowErr
		}
		selectedChecksum, checksumErr := productionPipelineSpecChecksum(resolution.Spec)
		if checksumErr != nil {
			return ProductionProcessorPlan{}, checksumErr
		}
		baselineChecksum, checksumErr := productionPipelineSpecChecksum(baselineSpec)
		if checksumErr != nil {
			return ProductionProcessorPlan{}, checksumErr
		}
		plan.routingSnapshot = &P5RoutingSnapshot{
			Facts:                    sortedFactSnapshot(factSet),
			PolicyID:                 facts.ActivePolicyID,
			PolicyVersion:            facts.ActivePolicyVersion,
			PolicyChecksum:           facts.ActivePolicyChecksum,
			SelectedPipelineChecksum: selectedChecksum,
			BaselinePipelineChecksum: baselineChecksum,
			BindingTrace:             append([]PipelineBindingTrace(nil), resolution.Binding.BindingTrace...),
			GateShadow:               shadow,
			RuleChecksums:            shadowRuleChecksums(shadow),
		}
	}
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
	out.ProcessorGateOverrides = cloneStringMap(p.facts.ProcessorGateOverrides)
	return out
}

func (p ProductionProcessorPlan) RoutingSnapshot() *P5RoutingSnapshot {
	return cloneP5RoutingSnapshot(p.routingSnapshot)
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
	// ADR §8.2 P4 harvesters. They are routed so a governed pipeline chooses
	// the corpus types for which their candidate output is meaningful.
	{Name: "extract_metric_definitions", Phase: "B", DependsOn: []string{"chunking"}, Class: "routed", Cost: "cheap_llm", OnUndetermined: "skip", Idempotent: true},
	{Name: "extract_product_structure", Phase: "B", DependsOn: []string{"chunking"}, Class: "routed", Cost: "cheap_llm", OnUndetermined: "skip", Idempotent: true},
	{Name: "extract_test_methods", Phase: "B", DependsOn: []string{"chunking"}, Class: "routed", Cost: "cheap_llm", OnUndetermined: "skip", Idempotent: true},
	{Name: "generate_scene_blocks", Phase: "B", DependsOn: []string{"chunking"}},
	// DR8 Phase D (ADR §8.1/§8.2): declared as Phase-C processors since this
	// planner's phase model has no separate "D" tier -- they run in the same
	// post-process-indexing tier as the rest of Phase C, ordered last via
	// PostProcessDependsOn (see phase_d.go). Gated by
	// SEMANTIC_ASSOCIATION_ENABLED so registering them here does not change
	// default production behavior.
	{Name: "normalize_assertions", Phase: "C", DependsOn: []string{"extract_metrics", "extract_provisions"}, Class: "routed", Cost: "free", OnUndetermined: "skip", Idempotent: true},
	{Name: "associate_semantics", Phase: "C", DependsOn: []string{"normalize_assertions"}, Class: "routed", Cost: "free", OnUndetermined: "skip", Idempotent: true},
	{Name: "project_semantics", Phase: "C", DependsOn: []string{"associate_semantics"}, Class: "routed", Cost: "free", OnUndetermined: "skip", Idempotent: true},
	// P5 tier-3 mandatory-gated pre-decision classifier (spec 2026080102
	// section 7). Not dispatched by the ordinary Phase A/B/C wave: the G2
	// resolver invokes it between the initial applicability pass and the
	// final freeze. Registered here so isMandatoryProcessor returns true
	// and ordinary processor-gate rules cannot skip or require it.
	{Name: "classify_document", Phase: "A", Class: "mandatory_gated", Cost: "cheap_llm", OnUndetermined: "run", Idempotent: true},
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
		if spec.Name == "static_analyzer" || spec.Name == "chunking" || spec.Name == "classify_document" {
			continue
		}
		out = append(out, spec.Name)
	}
	return out
}

func sortedFactSnapshot(facts semrules.FactSet) []semrules.Fact {
	paths := make([]string, 0, len(facts))
	for path := range facts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make([]semrules.Fact, 0, len(paths))
	for _, path := range paths {
		out = append(out, facts[path])
	}
	return out
}

func productionPipelineSpecChecksum(spec ProductionPipelineSpec) (string, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func shadowRuleChecksums(shadow ProcessorGateShadowPlan) []string {
	var checksums []string
	for _, decision := range shadow.Decisions {
		checksums = append(checksums, decision.RuleChecksums...)
	}
	return sortedUniqueNonEmpty(checksums)
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneP5RoutingSnapshot(in *P5RoutingSnapshot) *P5RoutingSnapshot {
	if in == nil {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out P5RoutingSnapshot
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return &out
}
