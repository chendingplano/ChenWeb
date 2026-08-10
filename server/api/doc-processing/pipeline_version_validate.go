package docprocessing

import (
	"fmt"
	"sort"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

// baselineProcessors always run first, ahead of anything else in a pipeline
// version's DAG (ResolveProcessorGate's "mandatory" source, isMandatoryProcessor).
var baselineProcessors = []string{"static_analyzer", "chunking"}

// baselineRoutingFactPaths are populated straight from the input record
// before any processor runs (BuildPipelineBindingFactSet), so a gate
// predicate referencing only these paths never needs a DAG producer.
var baselineRoutingFactPaths = map[string]bool{
	"document.input_doc_type":                true,
	"document.source_language":               true,
	"document.knowledge_store_binding_state": true,
	"document.has_document_number":           true,
}

// PipelineVersionDraft is the proposed content of a new kb.pipelines version:
// its processor selection and the per-processor kb.pipeline_rules rows
// (gates + DR6 DAG edges) that will be inserted with it in the same
// creation transaction (ADR 2026081001 DR2).
type PipelineVersionDraft struct {
	Processors []string
	Rules      []PipelineGate
}

// ValidatePipelineVersion runs the three ADR 2026081001 DR8 checks against a
// proposed pipeline version, once, at creation time. Any failure rejects the
// whole version; nothing here is re-checked at runtime once a version exists.
func ValidatePipelineVersion(draft PipelineVersionDraft) error {
	processors := stringSet(draft.Processors)
	if err := validateProcessorClosure(draft.Processors, processors); err != nil {
		return err
	}
	dag, err := validateDAGWellFormedness(draft.Rules, processors)
	if err != nil {
		return err
	}
	if err := validateGateFactAvailability(draft.Rules, dag); err != nil {
		return err
	}
	return nil
}

// validateProcessorClosure is DR8 check 1: every Requires artifact kind of
// every selected processor must be Produced by another selected processor,
// or guaranteed by a baseline processor.
func validateProcessorClosure(names []string, selected map[string]bool) error {
	producers := map[string]bool{}
	for _, name := range append(append([]string(nil), names...), baselineProcessors...) {
		spec, ok := LookupProcessor(normalizeRuntimeName(name))
		if !ok {
			continue
		}
		for _, produced := range spec.Produces {
			producers[produced] = true
		}
	}
	for _, name := range names {
		spec, ok := LookupProcessor(normalizeRuntimeName(name))
		if !ok {
			continue
		}
		for _, required := range spec.Requires {
			if !producers[required] {
				return fmt.Errorf("pipeline version rejected: processor %q requires artifact kind %q, which no selected or baseline processor produces", spec.Name, required)
			}
		}
	}
	return nil
}

// processorDAG is the resolved depends_on_processors edge set for a
// pipeline version, keyed by target processor -> its direct dependencies.
type processorDAG map[string][]string

// validateDAGWellFormedness is DR8 check 2: depends_on_processors edges must
// reference only processors in the version's own selected set, and must not
// contain a cycle.
func validateDAGWellFormedness(rules []PipelineGate, selected map[string]bool) (processorDAG, error) {
	dag := make(processorDAG, len(rules))
	for _, rule := range rules {
		target := normalizeRuntimeName(rule.TargetProcessor)
		if target == "" {
			continue
		}
		for _, dep := range rule.DependsOnProcessors {
			dep = normalizeRuntimeName(dep)
			if !selected[dep] {
				return nil, fmt.Errorf("pipeline version rejected: processor %q depends on %q, which is not part of this pipeline version's processor set", target, dep)
			}
			dag[target] = append(dag[target], dep)
		}
	}

	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := map[string]int{}
	var names []string
	for target := range dag {
		names = append(names, target)
	}
	sort.Strings(names) // deterministic error message across runs
	var cyclePath []string
	var visit func(name string) bool
	visit = func(name string) bool {
		switch state[name] {
		case done:
			return false
		case visiting:
			cyclePath = append(cyclePath, name)
			return true
		}
		state[name] = visiting
		cyclePath = append(cyclePath, name)
		for _, dep := range dag[name] {
			if visit(dep) {
				return true
			}
		}
		cyclePath = cyclePath[:len(cyclePath)-1]
		state[name] = done
		return false
	}
	for _, name := range names {
		if state[name] == unvisited && visit(name) {
			return nil, fmt.Errorf("pipeline version rejected: depends_on_processors cycle detected involving %q", cyclePath)
		}
	}
	return dag, nil
}

// upstreamOf returns every processor guaranteed to have already run before
// target, per the DAG's depends_on_processors edges (transitive) plus the
// always-first baseline processors.
func upstreamOf(target string, dag processorDAG) map[string]bool {
	upstream := map[string]bool{}
	for _, b := range baselineProcessors {
		upstream[b] = true
	}
	var walk func(name string)
	walk = func(name string) {
		for _, dep := range dag[name] {
			if upstream[dep] {
				continue
			}
			upstream[dep] = true
			walk(dep)
		}
	}
	walk(target)
	return upstream
}

// validateGateFactAvailability is DR8 check 3 (folding in DR9): every fact a
// gate's predicate references must be guaranteed available by the time that
// gate is evaluated. baselineRoutingFactPaths are available before any
// processor runs. Every other fact path is only available once some
// upstream processor Produces the "facets" artifact kind (facet_tier1/
// facet_tier2/classify_document today; ResolveProcessorSpec's union for any
// future facet producer) -- the same coarse Requires/Produces vocabulary
// check 1 already uses, applied to document facts instead of processor
// artifact kinds.
func validateGateFactAvailability(rules []PipelineGate, dag processorDAG) error {
	for _, rule := range rules {
		target := normalizeRuntimeName(rule.TargetProcessor)
		if target == "" || rule.Effect == GateEffectDefer {
			// GateEffectDefer is handled by its own creation-time rejection
			// (ADR DR9); this check only concerns gates that could otherwise
			// be accepted.
			if rule.Effect == GateEffectDefer {
				return fmt.Errorf("pipeline version rejected: gate %q for processor %q uses effect=defer, which is not a storable gate effect", rule.Name, rule.TargetProcessor)
			}
			continue
		}
		analysis := semrules.Analyze(rule.Predicate)
		if len(analysis.RequiredPaths) == 0 {
			continue
		}
		upstream := upstreamOf(target, dag)
		facetProducerAvailable := false
		for name := range upstream {
			spec, ok := LookupProcessor(name)
			if !ok {
				continue
			}
			for _, produced := range spec.Produces {
				if produced == "facets" {
					facetProducerAvailable = true
				}
			}
		}
		for _, path := range analysis.RequiredPaths {
			if baselineRoutingFactPaths[path] {
				continue
			}
			if !facetProducerAvailable {
				return fmt.Errorf("pipeline version rejected: gate %q for processor %q requires fact %q, which no processor guaranteed to run upstream produces", rule.Name, rule.TargetProcessor, path)
			}
		}
	}
	return nil
}
