package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PipelineRuleStore reads the authored facet-match rule set.
type PipelineRuleStore interface {
	ListPipelineRules(ctx context.Context) ([]ProductionPipelineRule, error)
}

type PipelineRuleSQLStore struct {
	DB *sql.DB
}

// ListPipelineRules returns only active rules, joined to the pipeline name
// they select — resolveProductionPipelineRuleMatch works purely on names,
// the same way ResolveProductionPipelineBinding already does for store
// binding and explicit requests.
func (s PipelineRuleSQLStore) ListPipelineRules(ctx context.Context) ([]ProductionPipelineRule, error) {
	if s.DB == nil {
		return nil, errors.New("db is nil")
	}
	const stmt = `
SELECT r.id, r.name, r.priority,
       COALESCE(r.match_input_doc_type, ''),
       COALESCE(r.match_source_language, ''),
       COALESCE(r.match_knowledge_store_binding, ''),
       p.name
FROM kb.pipeline_rules r
JOIN kb.pipelines p ON p.id = r.pipeline_id
WHERE r.active
ORDER BY r.priority DESC, r.id`
	rows, err := s.DB.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]ProductionPipelineRule, 0)
	for rows.Next() {
		var rule ProductionPipelineRule
		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Priority,
			&rule.MatchInputDocType, &rule.MatchSourceLanguage, &rule.MatchKnowledgeStoreBinding,
			&rule.PipelineName,
		); err != nil {
			return nil, err
		}
		// Defensive normalization: the CRUD API already lowercases/trims
		// match_* values on write, but matching against already-normalized
		// ProductionRoutingFacets should not depend on every writer having
		// gone through that API.
		rule.MatchInputDocType = normalizeRuleMatchValue(rule.MatchInputDocType)
		rule.MatchSourceLanguage = normalizeRuleMatchValue(rule.MatchSourceLanguage)
		rule.MatchKnowledgeStoreBinding = normalizeRuleMatchValue(rule.MatchKnowledgeStoreBinding)
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// LoadProductionPipelineRules loads the authored rule set from store and
// installs it via SetProductionPipelineRules. Unlike LoadProductionPipelineRegistry,
// an empty result is not an error — "no rules authored yet" is the normal,
// safe default state, not a failure.
func LoadProductionPipelineRules(ctx context.Context, store PipelineRuleStore) error {
	if store == nil {
		return errors.New("pipeline rule store is nil")
	}
	rules, err := store.ListPipelineRules(ctx)
	if err != nil {
		return fmt.Errorf("list pipeline rules: %w", err)
	}
	SetProductionPipelineRules(rules)
	return nil
}
