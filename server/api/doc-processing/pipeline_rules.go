package docprocessing

import (
	"fmt"
	"strings"
	"sync"
)

// ProductionPipelineRule is an authored facet-match rule: if a document's
// routing facets match every non-empty Match* field (empty = wildcard), the
// rule is a candidate to select PipelineName. See resolveProductionPipelineRuleMatch
// for how ties across candidates are handled.
type ProductionPipelineRule struct {
	ID                         int64
	Name                       string
	Priority                   int
	MatchInputDocType          string
	MatchSourceLanguage        string
	MatchKnowledgeStoreBinding string
	PipelineName               string
}

func (r ProductionPipelineRule) matches(facets ProductionRoutingFacets) bool {
	if r.MatchInputDocType != "" && r.MatchInputDocType != facets.InputDocType {
		return false
	}
	if r.MatchSourceLanguage != "" && r.MatchSourceLanguage != facets.SourceLanguage {
		return false
	}
	if r.MatchKnowledgeStoreBinding != "" && r.MatchKnowledgeStoreBinding != facets.KnowledgeStoreBinding {
		return false
	}
	return true
}

var (
	productionPipelineRulesMu sync.RWMutex
	productionPipelineRules   []ProductionPipelineRule
)

// SetProductionPipelineRules installs the in-process rule set consulted by
// resolveProductionPipelineRuleMatch. Nil/empty (the default) means no
// rules are authored, so rule matching is a no-op and binding falls through
// to store-binding/system-default exactly as it did before this existed —
// unlike the pipeline registry, there is no fallback set to install here,
// because "no rules" is itself the correct, safe default state.
func SetProductionPipelineRules(rules []ProductionPipelineRule) {
	productionPipelineRulesMu.Lock()
	defer productionPipelineRulesMu.Unlock()
	productionPipelineRules = rules
}

func currentProductionPipelineRules() []ProductionPipelineRule {
	productionPipelineRulesMu.RLock()
	defer productionPipelineRulesMu.RUnlock()
	return productionPipelineRules
}

// resolveProductionPipelineRuleMatchName finds the winning rule, if any, for
// the given facets, returning the winning rule's own name (for
// explainability) and the pipeline it selects. ("", "", false, nil) means no
// rule matched — the caller should fall through to the next binding-
// precedence tier. An error means multiple rules tied at the highest
// matching priority and named different pipelines: per the ADR invariant
// "rule conflicts are never silently resolved," that is reported rather
// than picked arbitrarily. A tie where every candidate names the same
// pipeline is not a conflict.
func resolveProductionPipelineRuleMatchName(facets ProductionRoutingFacets) (ruleName string, pipelineName string, matched bool, err error) {
	rules := currentProductionPipelineRules()
	var (
		candidates   []ProductionPipelineRule
		bestPriority int
		haveMatch    bool
	)
	for _, rule := range rules {
		if !rule.matches(facets) {
			continue
		}
		switch {
		case !haveMatch || rule.Priority > bestPriority:
			bestPriority = rule.Priority
			candidates = []ProductionPipelineRule{rule}
			haveMatch = true
		case rule.Priority == bestPriority:
			candidates = append(candidates, rule)
		}
	}
	if !haveMatch {
		return "", "", false, nil
	}
	winner := candidates[0]
	for _, other := range candidates[1:] {
		if other.PipelineName != winner.PipelineName {
			return "", "", false, fmt.Errorf(
				"conflicting pipeline rules at priority %d: %q selects %q, %q selects %q",
				bestPriority, winner.Name, winner.PipelineName, other.Name, other.PipelineName,
			)
		}
	}
	return winner.Name, winner.PipelineName, true, nil
}

func normalizeRuleMatchValue(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
