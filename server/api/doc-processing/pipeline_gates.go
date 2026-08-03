package docprocessing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

const (
	GateEffectRequire = "require"
	GateEffectEnable  = "enable"
	GateEffectSkip    = "skip"
	GateEffectDefer   = "defer"
)

type PipelineGate struct {
	ID                int64             `json:"id"`
	Name              string            `json:"name"`
	Priority          int               `json:"priority"`
	TargetProcessor   string            `json:"target_processor"`
	Effect            string            `json:"effect"`
	Predicate         semrules.Document `json:"predicate"`
	PredicateChecksum string            `json:"predicate_checksum"`
	RequiredFacets    []string          `json:"required_facets,omitempty"`
	Active            bool              `json:"active"`
}

type ProcessorGateTrace struct {
	RuleID            int64              `json:"rule_id"`
	RuleName          string             `json:"rule_name"`
	PredicateChecksum string             `json:"predicate_checksum"`
	Priority          int                `json:"priority"`
	Specificity       int                `json:"specificity"`
	Effect            string             `json:"effect"`
	Truth             semrules.Truth     `json:"truth"`
	Trace             semrules.TraceNode `json:"trace"`
	MissingPaths      []string           `json:"missing_paths,omitempty"`
}

type ProcessorGateResolution struct {
	Processor        string               `json:"processor"`
	Effect           string               `json:"effect"`
	Truth            semrules.Truth       `json:"truth"`
	Source           string               `json:"source"`
	WinningRuleID    int64                `json:"winning_rule_id,omitempty"`
	WinningChecksum  string               `json:"winning_checksum,omitempty"`
	Trace            []ProcessorGateTrace `json:"trace,omitempty"`
	RuleChecksums    []string             `json:"rule_checksums,omitempty"`
	DeferredPaths    []string             `json:"deferred_paths,omitempty"`
	DeferFingerprint string               `json:"defer_fingerprint,omitempty"`
}

type GateResolutionOptions struct {
	OnConflict  string
	Explicit    bool
	RunOverride string
}

type PipelineGateResolutionError struct {
	Processor string
	Reason    string
}

func (e *PipelineGateResolutionError) Error() string {
	return fmt.Sprintf("processor gate %q is indeterminate: %s", e.Processor, e.Reason)
}

// ResolveProcessorGate applies priority, predicate specificity, then the
// governed require > defer > skip > enable order for one processor.
func ResolveProcessorGate(spec ProcessorSpec, gates []PipelineGate, facts semrules.FactSet, options GateResolutionOptions) (ProcessorGateResolution, error) {
	processor := normalizeRuntimeName(spec.Name)
	if isMandatoryProcessor(spec) {
		return ProcessorGateResolution{Processor: processor, Effect: GateEffectRequire, Truth: semrules.TruthTrue, Source: "mandatory"}, nil
	}
	if override := strings.TrimSpace(options.RunOverride); override != "" {
		if !validGateEffect(override) {
			return ProcessorGateResolution{}, fmt.Errorf("invalid run override effect %q", override)
		}
		return ProcessorGateResolution{Processor: processor, Effect: override, Truth: semrules.TruthTrue, Source: "run_override"}, nil
	}
	if options.Explicit {
		return ProcessorGateResolution{Processor: processor, Effect: GateEffectRequire, Truth: semrules.TruthTrue, Source: "explicit_request"}, nil
	}

	candidates := make([]PipelineGate, 0, len(gates))
	for _, gate := range gates {
		if gate.Active && normalizeRuntimeName(gate.TargetProcessor) == processor {
			candidates = append(candidates, gate)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		return gateSpecificity(candidates[i]) > gateSpecificity(candidates[j])
	})

	resolution := ProcessorGateResolution{Processor: processor, Effect: GateEffectEnable, Truth: semrules.TruthFalse, Source: "processor_default"}
	for start := 0; start < len(candidates); {
		end := start + 1
		for end < len(candidates) && candidates[end].Priority == candidates[start].Priority && gateSpecificity(candidates[end]) == gateSpecificity(candidates[start]) {
			end++
		}
		var trueGates, indeterminateGates []PipelineGate
		results := make(map[int64]semrules.Result, end-start)
		for _, gate := range candidates[start:end] {
			result, err := semrules.EvaluateDocumentValidated(gate.Predicate, facts)
			if err != nil {
				return ProcessorGateResolution{}, fmt.Errorf("processor gate %d predicate: %w", gate.ID, err)
			}
			results[gate.ID] = result
			resolution.Trace = append(resolution.Trace, ProcessorGateTrace{
				RuleID: gate.ID, RuleName: gate.Name, PredicateChecksum: gate.PredicateChecksum,
				Priority: gate.Priority, Specificity: gateSpecificity(gate), Effect: gate.Effect,
				Truth: result.Truth, Trace: result.TraceTree,
				MissingPaths: append([]string(nil), result.DecisionRelevantMissingPaths...),
			})
			if gate.PredicateChecksum != "" {
				resolution.RuleChecksums = append(resolution.RuleChecksums, gate.PredicateChecksum)
			}
			switch result.Truth {
			case semrules.TruthTrue:
				trueGates = append(trueGates, gate)
			case semrules.TruthIndeterminate:
				indeterminateGates = append(indeterminateGates, gate)
			}
		}
		if len(trueGates) == 0 && len(indeterminateGates) == 0 {
			start = end
			continue
		}
		if len(trueGates) == 0 {
			return resolveIndeterminateGate(spec, resolution, indeterminateGates, results, options)
		}
		winner := strongestGate(trueGates)
		for _, uncertain := range indeterminateGates {
			if gateEffectRank(uncertain.Effect) > gateEffectRank(winner.Effect) {
				return resolveIndeterminateGate(spec, resolution, indeterminateGates, results, options)
			}
		}
		resolution.Effect = winner.Effect
		resolution.Truth = semrules.TruthTrue
		resolution.Source = "policy_gate"
		resolution.WinningRuleID = winner.ID
		resolution.WinningChecksum = winner.PredicateChecksum
		if winner.Effect == GateEffectDefer {
			paths := append([]string(nil), winner.RequiredFacets...)
			paths = append(paths, semrules.Analyze(winner.Predicate).RequiredPaths...)
			paths = append(paths, results[winner.ID].DecisionRelevantMissingPaths...)
			resolution.DeferredPaths = sortedUniqueNonEmpty(paths)
			if len(resolution.DeferredPaths) == 0 {
				return ProcessorGateResolution{}, fmt.Errorf("defer gate %d has no dependency facts", winner.ID)
			}
			resolution.DeferFingerprint = deferFingerprint(processor, winner.PredicateChecksum, resolution.DeferredPaths)
		}
		resolution.RuleChecksums = sortedUniqueNonEmpty(resolution.RuleChecksums)
		return resolution, nil
	}
	resolution.RuleChecksums = sortedUniqueNonEmpty(resolution.RuleChecksums)
	return resolution, nil
}

func resolveIndeterminateGate(spec ProcessorSpec, resolution ProcessorGateResolution, gates []PipelineGate, results map[int64]semrules.Result, options GateResolutionOptions) (ProcessorGateResolution, error) {
	if options.OnConflict != PipelineBindingOnConflictFallback {
		return resolution, &PipelineGateResolutionError{Processor: spec.Name, Reason: "decision_relevant_indeterminate"}
	}
	resolution.Truth = semrules.TruthIndeterminate
	resolution.Source = "indeterminate_fallback"
	resolution.Effect = GateEffectEnable
	if strings.EqualFold(strings.TrimSpace(spec.OnUndetermined), "skip") {
		resolution.Effect = GateEffectSkip
	}
	var paths []string
	for _, gate := range gates {
		paths = append(paths, results[gate.ID].DecisionRelevantMissingPaths...)
		paths = append(paths, gate.RequiredFacets...)
	}
	resolution.DeferredPaths = sortedUniqueNonEmpty(paths)
	resolution.RuleChecksums = sortedUniqueNonEmpty(resolution.RuleChecksums)
	return resolution, nil
}

type GateShadowOptions struct {
	ExplicitProcessors []string
	RunOverrides       map[string]string
	OnConflict         string
}

type ProcessorGateShadowPlan struct {
	EffectiveProcessors []string                  `json:"effective_processors"`
	WouldRun            []string                  `json:"would_run,omitempty"`
	WouldSkip           []string                  `json:"would_skip,omitempty"`
	WouldDefer          []string                  `json:"would_defer,omitempty"`
	Decisions           []ProcessorGateResolution `json:"decisions,omitempty"`
}

// BuildProcessorGateShadowPlan computes complete would-run/skip/defer
// decisions while preserving the baseline effective processor list.
func BuildProcessorGateShadowPlan(baseline []string, specs []ProcessorSpec, gates []PipelineGate, facts semrules.FactSet, options GateShadowOptions) (ProcessorGateShadowPlan, error) {
	shadow := ProcessorGateShadowPlan{EffectiveProcessors: append([]string(nil), baseline...)}
	explicit := stringSet(options.ExplicitProcessors)
	specByName := make(map[string]ProcessorSpec, len(specs))
	for _, spec := range specs {
		specByName[normalizeRuntimeName(spec.Name)] = spec
	}
	for _, raw := range baseline {
		processor := normalizeRuntimeName(raw)
		spec := specByName[processor]
		if spec.Name == "" {
			spec.Name = processor
		}
		decision, err := ResolveProcessorGate(spec, gates, facts, GateResolutionOptions{
			OnConflict: options.OnConflict, Explicit: explicit[processor], RunOverride: options.RunOverrides[processor],
		})
		if err != nil {
			return ProcessorGateShadowPlan{}, err
		}
		shadow.Decisions = append(shadow.Decisions, decision)
		switch decision.Effect {
		case GateEffectRequire, GateEffectEnable:
			shadow.WouldRun = append(shadow.WouldRun, processor)
		case GateEffectSkip:
			shadow.WouldSkip = append(shadow.WouldSkip, processor)
		case GateEffectDefer:
			shadow.WouldDefer = append(shadow.WouldDefer, processor)
		}
	}
	return shadow, nil
}

type PipelineGateStore interface {
	ListPipelineGates(context.Context) ([]PipelineGate, error)
}

type PipelineGateSQLStore struct{ DB *sql.DB }

func (s PipelineGateSQLStore) ListPipelineGates(ctx context.Context) ([]PipelineGate, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,r.name,r.priority,r.target_processor,r.effect,r.predicate::text,r.predicate_checksum,r.required_facets::text,r.active
FROM kb.pipeline_rules r
WHERE r.active AND r.predicate IS NOT NULL AND r.target_processor IS NOT NULL
  AND r.policy_id=(SELECT id FROM kb.pipeline_policies WHERE status='active' LIMIT 1)
ORDER BY r.target_processor,r.priority DESC,r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var gates []PipelineGate
	for rows.Next() {
		var gate PipelineGate
		var predicateRaw, facetsRaw string
		if err := rows.Scan(&gate.ID, &gate.Name, &gate.Priority, &gate.TargetProcessor, &gate.Effect, &predicateRaw, &gate.PredicateChecksum, &facetsRaw, &gate.Active); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(predicateRaw), &gate.Predicate); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(facetsRaw), &gate.RequiredFacets); err != nil {
			return nil, err
		}
		gates = append(gates, gate)
	}
	return gates, rows.Err()
}

var (
	productionPipelineGatesMu sync.RWMutex
	productionPipelineGates   []PipelineGate
)

func SetProductionPipelineGates(gates []PipelineGate) {
	productionPipelineGatesMu.Lock()
	defer productionPipelineGatesMu.Unlock()
	productionPipelineGates = append([]PipelineGate(nil), gates...)
}

func currentProductionPipelineGates() []PipelineGate {
	productionPipelineGatesMu.RLock()
	defer productionPipelineGatesMu.RUnlock()
	return append([]PipelineGate(nil), productionPipelineGates...)
}

func LoadProductionPipelineGates(ctx context.Context, store PipelineGateStore) error {
	if store == nil {
		return errors.New("pipeline gate store is nil")
	}
	gates, err := store.ListPipelineGates(ctx)
	if err != nil {
		return fmt.Errorf("list pipeline gates: %w", err)
	}
	SetProductionPipelineGates(gates)
	return nil
}

func strongestGate(gates []PipelineGate) PipelineGate {
	winner := gates[0]
	for _, gate := range gates[1:] {
		if gateEffectRank(gate.Effect) > gateEffectRank(winner.Effect) || gateEffectRank(gate.Effect) == gateEffectRank(winner.Effect) && gate.ID < winner.ID {
			winner = gate
		}
	}
	return winner
}

func gateSpecificity(gate PipelineGate) int { return semrules.Analyze(gate.Predicate).Specificity }

func gateEffectRank(effect string) int {
	switch effect {
	case GateEffectRequire:
		return 4
	case GateEffectDefer:
		return 3
	case GateEffectSkip:
		return 2
	case GateEffectEnable:
		return 1
	default:
		return 0
	}
}

func validGateEffect(effect string) bool { return gateEffectRank(effect) > 0 }

func isMandatoryProcessor(spec ProcessorSpec) bool {
	class := strings.ToLower(strings.TrimSpace(spec.Class))
	return class == "mandatory" || class == "mandatory_gated" || normalizeRuntimeName(spec.Name) == "static_analyzer" || normalizeRuntimeName(spec.Name) == "chunking"
}

func deferFingerprint(processor, checksum string, paths []string) string {
	raw, _ := json.Marshal(struct {
		Processor string   `json:"processor"`
		Checksum  string   `json:"checksum"`
		Paths     []string `json:"paths"`
	}{processor, checksum, paths})
	digest := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func sortedUniqueNonEmpty(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[normalizeRuntimeName(value)] = true
	}
	return out
}
