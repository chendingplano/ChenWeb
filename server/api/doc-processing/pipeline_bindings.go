package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

const (
	PipelineBindingKindConditional  = "conditional"
	PipelineBindingKindStoreDefault = "store_default"

	PipelineBindingScopeSystem         = "system"
	PipelineBindingScopeTenant         = "tenant"
	PipelineBindingScopeKnowledgeStore = "knowledge_store"
	PipelineBindingScopeUser           = "user"
	PipelineBindingScopeDocument       = "document"

	PipelineBindingOnConflictBlock    = "block"
	PipelineBindingOnConflictFallback = "fallback"
)

type PipelineBinding struct {
	ID                int64
	Name              string
	Priority          int
	Scope             string
	BindingKind       string
	PipelineName      string
	Predicate         semrules.Document
	PredicateChecksum string
	Active            bool
	LegacyRule        *ProductionPipelineRule `json:"-"`
}

type PipelineBindingSelection struct {
	SelectedPipeline string
	Source           string
	BindingName      string
	// BindingID/PredicateChecksum identify the winning kb.pipeline_bindings
	// row when Source == "conditional_binding" -- E3's FinalizeRoutingPlan
	// needs both to build the D2 clearance subject
	// (ConditionalBindingSubjectChecksum) for a suppressive selection. Zero
	// value for every other source (store_default/system_default), which
	// never need a P5 clearance.
	BindingID         int64
	PredicateChecksum string
	Trace             []PipelineBindingTrace
}

type PipelineBindingTrace struct {
	BindingName  string
	PipelineName string
	BindingKind  string
	Priority     int
	Scope        string
	Truth        semrules.Truth
	Reason       string
}

// PipelineBindingStore reads the active policy's canonical routing bindings.
type PipelineBindingStore interface {
	ListPipelineBindings(ctx context.Context) ([]PipelineBinding, error)
}

type PipelineBindingSQLStore struct {
	DB *sql.DB
}

var (
	productionPipelineBindingsMu sync.RWMutex
	productionPipelineBindings   []PipelineBinding
)

// SetProductionPipelineBindings installs the in-process canonical binding set
// consulted by ResolveProductionPipelineBinding. Nil/empty means no canonical
// policy bindings have been loaded yet.
func SetProductionPipelineBindings(bindings []PipelineBinding) {
	productionPipelineBindingsMu.Lock()
	defer productionPipelineBindingsMu.Unlock()
	productionPipelineBindings = bindings
}

func currentProductionPipelineBindings() []PipelineBinding {
	productionPipelineBindingsMu.RLock()
	defer productionPipelineBindingsMu.RUnlock()
	out := make([]PipelineBinding, len(productionPipelineBindings))
	copy(out, productionPipelineBindings)
	return out
}

// ListPipelineBindings returns active canonical bindings for the active policy.
func (s PipelineBindingSQLStore) ListPipelineBindings(ctx context.Context) ([]PipelineBinding, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	const stmt = `
SELECT b.id, COALESCE(b.name, ''), b.priority, b.binding_kind,
       COALESCE(p.name, ''), COALESCE(b.predicate, '{}'::jsonb)::text,
       COALESCE(b.predicate_checksum, ''), b.active,
       CASE
         WHEN b.input_record_id IS NOT NULL THEN 'document'
         WHEN NULLIF(b.user_id, '') IS NOT NULL THEN 'user'
         WHEN b.ks_store_id IS NOT NULL THEN 'knowledge_store'
         WHEN NULLIF(b.tenant_id, '') IS NOT NULL AND b.tenant_id <> '-' THEN 'tenant'
         ELSE 'system'
       END AS binding_scope
FROM kb.pipeline_bindings b
LEFT JOIN kb.pipelines p ON p.id = b.pipeline_id
WHERE b.active AND b.policy_id = (SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1)
ORDER BY b.priority DESC,
         CASE
           WHEN b.input_record_id IS NOT NULL THEN 4
           WHEN NULLIF(b.user_id, '') IS NOT NULL THEN 3
           WHEN b.ks_store_id IS NOT NULL THEN 2
           WHEN NULLIF(b.tenant_id, '') IS NOT NULL AND b.tenant_id <> '-' THEN 1
           ELSE 0
         END DESC,
         b.id`
	rows, err := s.DB.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []PipelineBinding
	for rows.Next() {
		var binding PipelineBinding
		var predicateRaw string
		if err := rows.Scan(
			&binding.ID, &binding.Name, &binding.Priority, &binding.BindingKind,
			&binding.PipelineName, &predicateRaw, &binding.PredicateChecksum,
			&binding.Active, &binding.Scope,
		); err != nil {
			return nil, err
		}
		if strings.TrimSpace(predicateRaw) != "" && strings.TrimSpace(predicateRaw) != "{}" {
			if err := json.Unmarshal([]byte(predicateRaw), &binding.Predicate); err != nil {
				return nil, fmt.Errorf("decode pipeline binding %q predicate: %w", binding.Name, err)
			}
		}
		if binding.BindingKind == "" {
			binding.BindingKind = PipelineBindingKindStoreDefault
		}
		if binding.Scope == "" {
			binding.Scope = PipelineBindingScopeSystem
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bindings, nil
}

// LoadProductionPipelineBindings loads canonical policy bindings into the
// process. Empty results are valid and clear stale in-memory bindings.
func LoadProductionPipelineBindings(ctx context.Context, store PipelineBindingStore) error {
	if store == nil {
		return errors.New("pipeline binding store is nil")
	}
	bindings, err := store.ListPipelineBindings(ctx)
	if err != nil {
		return fmt.Errorf("list pipeline bindings: %w", err)
	}
	SetProductionPipelineBindings(bindings)
	return nil
}

// LegacyRulePredicateDocument adapts the old flat rule fields to the canonical
// version-1 predicate shape used by P5 conditional bindings.
func LegacyRulePredicateDocument(rule ProductionPipelineRule) (semrules.Document, string, error) {
	items := make([]semrules.Predicate, 0, 3)
	for _, leaf := range []struct {
		path  string
		value string
	}{
		{"document.input_doc_type", normalizeRuleMatchValue(rule.MatchInputDocType)},
		{"document.source_language", normalizeRuleMatchValue(rule.MatchSourceLanguage)},
		{"document.knowledge_store_binding_state", normalizeRuleMatchValue(rule.MatchKnowledgeStoreBinding)},
	} {
		if leaf.value == "" {
			continue
		}
		items = append(items, semrules.Predicate{Kind: "fact", Path: leaf.path, Op: "eq", Value: leaf.value})
	}
	doc := semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "all", Items: items}}
	_, checksum, err := semrules.Canonicalize(doc)
	return doc, checksum, err
}

func BuildPipelineBindingFactSet(facts ProductionPlanFacts) semrules.FactSet {
	facets := facts.RoutingFacets
	if facets.InputDocType == "" {
		facets.InputDocType = normalizeRuleMatchValue(facts.InputDocType)
	}
	if facets.SourceLanguage == "" {
		facets.SourceLanguage = normalizeRuleMatchValue(facts.SourceLanguage)
	}
	if facets.KnowledgeStoreBinding == "" {
		if facts.KnowledgeStoreID > 0 {
			facets.KnowledgeStoreBinding = "bound"
		} else {
			facets.KnowledgeStoreBinding = "absent"
		}
	}
	facets.HasDocumentNumber = facets.HasDocumentNumber || strings.TrimSpace(facts.DocumentNumber) != ""

	builder := semrules.NewFactSetBuilder()
	addKnownPipelineBindingFact(builder, "document.input_doc_type", facets.InputDocType)
	addKnownPipelineBindingFact(builder, "document.source_language", facets.SourceLanguage)
	addKnownPipelineBindingFact(builder, "document.knowledge_store_binding_state", facets.KnowledgeStoreBinding)
	_ = builder.Add(semrules.Fact{Path: "document.has_document_number", State: semrules.FactKnown, Value: facets.HasDocumentNumber})
	if facts.KnowledgeStoreID > 0 {
		addKnownPipelineBindingFact(builder, "deployment.knowledge_store", fmt.Sprint(facts.KnowledgeStoreID))
	}
	base := builder.Build()

	// P5 two-pass resolver overlay: enriched facts (base + classifier
	// observations) replace the base set when present. The resolver's
	// enrichFactSet already preserves known-over-classifier precedence,
	// so we only need to ensure deployment-level facts survive.
	if facts.EnrichedFacts != nil {
		merged := make(semrules.FactSet, len(facts.EnrichedFacts)+len(base))
		for path, fact := range facts.EnrichedFacts {
			merged[path] = fact
		}
		for path, fact := range base {
			if _, exists := merged[path]; !exists {
				merged[path] = fact
			}
		}
		return merged
	}
	return base
}

func addKnownPipelineBindingFact(builder *semrules.FactSetBuilder, path string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	_ = builder.Add(semrules.Fact{Path: path, State: semrules.FactKnown, Value: strings.ToLower(strings.TrimSpace(value))})
}

func ResolvePipelineBindings(bindings []PipelineBinding, facts semrules.FactSet, onConflict string) (PipelineBindingSelection, error) {
	if onConflict == "" {
		onConflict = PipelineBindingOnConflictBlock
	}
	conditional := make([]PipelineBinding, 0, len(bindings))
	var storeDefaults []PipelineBinding
	for _, binding := range bindings {
		if !binding.Active {
			continue
		}
		switch binding.BindingKind {
		case PipelineBindingKindConditional:
			conditional = append(conditional, binding)
		case PipelineBindingKindStoreDefault:
			storeDefaults = append(storeDefaults, binding)
		}
	}
	sort.SliceStable(conditional, func(i, j int) bool {
		if conditional[i].Priority != conditional[j].Priority {
			return conditional[i].Priority > conditional[j].Priority
		}
		return pipelineBindingScopeSpecificity(conditional[i].Scope) > pipelineBindingScopeSpecificity(conditional[j].Scope)
	})

	var trace []PipelineBindingTrace
	for start := 0; start < len(conditional); {
		end := start + 1
		for end < len(conditional) && sameBindingRank(conditional[start], conditional[end]) {
			end++
		}
		selection, rankTrace, err := resolveBindingRank(conditional[start:end], facts)
		trace = append(trace, rankTrace...)
		if err == nil && selection.SelectedPipeline != "" {
			selection.Trace = trace
			return selection, nil
		}
		if err != nil {
			if onConflict == PipelineBindingOnConflictFallback {
				start = end
				continue
			}
			return PipelineBindingSelection{Trace: trace}, err
		}
		start = end
	}

	for _, binding := range storeDefaults {
		if binding.PipelineName == "" {
			continue
		}
		trace = append(trace, PipelineBindingTrace{
			BindingName:  binding.Name,
			PipelineName: binding.PipelineName,
			BindingKind:  binding.BindingKind,
			Priority:     binding.Priority,
			Scope:        binding.Scope,
			Truth:        semrules.TruthTrue,
			Reason:       "store_default",
		})
		return PipelineBindingSelection{
			SelectedPipeline: binding.PipelineName,
			Source:           "store_default",
			BindingName:      binding.Name,
			Trace:            trace,
		}, nil
	}
	return PipelineBindingSelection{
		SelectedPipeline: DefaultProductionPipelineName,
		Source:           "system_default",
		Trace:            trace,
	}, nil
}

// PipelineBindingConflictError marks a decision-relevant DR7 conditional-
// binding conflict/indeterminacy under PipelineBindingOnConflictBlock, as
// distinct from a structural/configuration error (unknown pipeline, unknown
// processor). control.go uses this type -- via IsDecisionRelevantPlanConflict
// -- to decide whether a planErr must block processing (spec 2026080102
// section 11) or is merely logged/alarmed as today.
type PipelineBindingConflictError struct {
	Priority int
	Reason   string // "indeterminate" | "conflicting"
}

func (e *PipelineBindingConflictError) Error() string {
	if e.Reason == "conflicting" {
		return fmt.Sprintf("conflicting conditional pipeline bindings at priority %d", e.Priority)
	}
	return fmt.Sprintf("indeterminate conditional pipeline bindings at priority %d", e.Priority)
}

func resolveBindingRank(bindings []PipelineBinding, facts semrules.FactSet) (PipelineBindingSelection, []PipelineBindingTrace, error) {
	truePipelines := map[string]PipelineBinding{}
	indeterminatePipelines := map[string]PipelineBinding{}
	trace := make([]PipelineBindingTrace, 0, len(bindings))
	for _, binding := range bindings {
		result, err := semrules.EvaluateDocumentValidated(binding.Predicate, facts)
		if err != nil {
			return PipelineBindingSelection{}, trace, fmt.Errorf("pipeline binding %d predicate: %w", binding.ID, err)
		}
		trace = append(trace, PipelineBindingTrace{
			BindingName:  binding.Name,
			PipelineName: binding.PipelineName,
			BindingKind:  binding.BindingKind,
			Priority:     binding.Priority,
			Scope:        binding.Scope,
			Truth:        result.Truth,
			Reason:       result.TraceTree.ReasonCode,
		})
		switch result.Truth {
		case semrules.TruthTrue:
			truePipelines[binding.PipelineName] = binding
		case semrules.TruthIndeterminate:
			indeterminatePipelines[binding.PipelineName] = binding
		}
	}
	if len(truePipelines) == 0 && len(indeterminatePipelines) == 0 {
		return PipelineBindingSelection{}, trace, nil
	}
	if len(truePipelines) == 0 {
		return PipelineBindingSelection{}, trace, &PipelineBindingConflictError{Priority: bindings[0].Priority, Reason: "indeterminate"}
	}
	if len(truePipelines) > 1 {
		return PipelineBindingSelection{}, trace, &PipelineBindingConflictError{Priority: bindings[0].Priority, Reason: "conflicting"}
	}
	var selected PipelineBinding
	for _, binding := range truePipelines {
		selected = binding
	}
	for pipeline := range indeterminatePipelines {
		if pipeline != selected.PipelineName {
			return PipelineBindingSelection{}, trace, &PipelineBindingConflictError{Priority: bindings[0].Priority, Reason: "indeterminate"}
		}
	}
	return PipelineBindingSelection{
		SelectedPipeline:  selected.PipelineName,
		Source:            "conditional_binding",
		BindingName:       selected.Name,
		BindingID:         selected.ID,
		PredicateChecksum: selected.PredicateChecksum,
	}, trace, nil
}

func sameBindingRank(left, right PipelineBinding) bool {
	return left.Priority == right.Priority && pipelineBindingScopeSpecificity(left.Scope) == pipelineBindingScopeSpecificity(right.Scope)
}

func pipelineBindingScopeSpecificity(scope string) int {
	switch strings.TrimSpace(scope) {
	case PipelineBindingScopeDocument:
		return 4
	case PipelineBindingScopeUser:
		return 3
	case PipelineBindingScopeKnowledgeStore:
		return 2
	case PipelineBindingScopeTenant:
		return 1
	default:
		return 0
	}
}
