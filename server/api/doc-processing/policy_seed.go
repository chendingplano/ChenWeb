// policy_seed.go

package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/lib/pq"
)

// DocProcessingPolicySeedResult summarizes one SeedDocProcessingPolicies run.
type DocProcessingPolicySeedResult struct {
	PipelinesCreated []string
	PipelinesUpdated []string
	PolicyID         int64
	PolicyVersion    int
	// BindingsWritten counts the one system-wide default binding plus one
	// per cfg.Bindings entry.
	BindingsWritten int
	// RulesWritten counts the unconditional kb.pipeline_rules gate rows
	// written -- one per processor named in each policy's processors list.
	RulesWritten int
}

// alwaysTruePredicate is the trivial "all of zero conditions" predicate
// document: vacuously true for every fact set (semrules/evaluate.go
// evaluateAllOrAny). Used to give each seeded processor an unconditional
// Tier-2 gate row -- PipelineGateSQLStore.ListPipelineGates only loads rows
// with predicate IS NOT NULL, so a NULL predicate would never be consulted
// by ResolveProcessorGate at all.
var alwaysTruePredicate = semrules.Document{Version: 1, Expression: semrules.Predicate{Kind: "all"}}

// SeedDocProcessingPolicies upserts cfg's policies into kb.pipelines (by
// name), authors a new draft kb.pipeline_policies version carrying one
// system-wide store_default binding (cfg.DefaultPolicyName()) plus one
// store_default binding per cfg.Bindings entry, one unconditional
// kb.pipeline_rules gate row per processor named in each policy's processors
// list (require effect, always-true predicate -- makes kb.pipeline_rules a
// real Tier-2 mirror of the Tier-1 processors list, since nothing else
// authors Tier-2 gates today), compiles that version, and activates it --
// archiving whatever policy was previously active. Every write happens in
// one transaction; any failure rolls everything back and leaves the
// previously active policy untouched. Safe to call repeatedly: each call
// creates and activates a new policy version rather than mutating a
// previous one. Activation is a full replacement, not a merge: once this
// commits, only the bindings/rules this function wrote are active, so any
// bindings/gates/rules authored under the previously active policy via
// other paths (e.g. the REST API) stop being consulted immediately.
func SeedDocProcessingPolicies(ctx context.Context, db *sql.DB, cfg DocProcessingPolicySeedConfig) (DocProcessingPolicySeedResult, error) {
	if db == nil {
		return DocProcessingPolicySeedResult{}, errors.New("db is nil")
	}
	if err := cfg.Validate(); err != nil {
		return DocProcessingPolicySeedResult{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	result := DocProcessingPolicySeedResult{}
	pipelineIDs := map[string]int64{}
	policyNames := make([]string, 0, len(cfg.Policies))
	for name := range cfg.Policies {
		policyNames = append(policyNames, name)
	}
	sort.Strings(policyNames)
	for _, name := range policyNames {
		id, created, err := upsertDocProcessingPipeline(ctx, tx, name, cfg.Policies[name])
		if err != nil {
			return DocProcessingPolicySeedResult{}, fmt.Errorf("pipeline %q: %w", name, err)
		}
		pipelineIDs[name] = id
		if created {
			result.PipelinesCreated = append(result.PipelinesCreated, name)
		} else {
			result.PipelinesUpdated = append(result.PipelinesUpdated, name)
		}
	}

	var nextVersion int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM kb.pipeline_policies`).Scan(&nextVersion); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("next policy version: %w", err)
	}
	var policyID int64
	if err := tx.QueryRowContext(ctx, `
INSERT INTO kb.pipeline_policies (version, status, source_ref)
VALUES ($1, 'draft', 'doc-processing-policy-seed')
RETURNING id`, nextVersion).Scan(&policyID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("create draft policy: %w", err)
	}
	result.PolicyID = policyID
	result.PolicyVersion = nextVersion

	defaultPipelineID := pipelineIDs[cfg.DefaultPolicyName()]
	if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.pipeline_bindings
    (ks_store_id, pipeline_id, policy_id, name, priority, active, binding_kind)
VALUES (NULL, $1, $2, 'system-default', 0, true, 'store_default')`,
		defaultPipelineID, policyID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("insert system-default binding: %w", err)
	}
	result.BindingsWritten++

	storeNames := make([]string, 0, len(cfg.Bindings))
	for store := range cfg.Bindings {
		storeNames = append(storeNames, store)
	}
	sort.Strings(storeNames)
	for _, store := range storeNames {
		policyName := cfg.Bindings[store]
		var ksStoreID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM kb.knowledge_store WHERE ks_name = $1`, store).Scan(&ksStoreID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return DocProcessingPolicySeedResult{}, fmt.Errorf("binding %q: unknown knowledge store", store)
			}
			return DocProcessingPolicySeedResult{}, fmt.Errorf("binding %q: lookup knowledge store: %w", store, err)
		}
		pipelineID := pipelineIDs[policyName]
		if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.pipeline_bindings
    (ks_store_id, pipeline_id, policy_id, name, priority, active, binding_kind)
VALUES ($1, $2, $3, $4, 0, true, 'store_default')`,
			ksStoreID, pipelineID, policyID, fmt.Sprintf("store:%s", store)); err != nil {
			return DocProcessingPolicySeedResult{}, fmt.Errorf("insert binding for %q: %w", store, err)
		}
		result.BindingsWritten++
	}

	predicateJSON, predicateChecksum, err := semrules.Canonicalize(alwaysTruePredicate)
	if err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("canonicalize always-true predicate: %w", err)
	}
	for _, name := range policyNames {
		processors := append([]string(nil), cfg.Policies[name].Processors...)
		sort.Strings(processors)
		for _, processor := range processors {
			ruleName := fmt.Sprintf("%s: %s", name, processor)
			if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.pipeline_rules
    (name, priority, predicate, predicate_checksum, target_processor, effect, active, policy_id, approval_status)
VALUES ($1, 0, $2::jsonb, $3, $4, 'require', true, $5, 'approved')`,
				ruleName, string(predicateJSON), predicateChecksum, processor, policyID); err != nil {
				return DocProcessingPolicySeedResult{}, fmt.Errorf("insert rule %q: %w", ruleName, err)
			}
			result.RulesWritten++
		}
	}

	compiled, err := (PolicyCompilerSQLStore{DB: tx}).CompilePolicy(ctx, policyID)
	if err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("compile policy: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE kb.pipeline_policies SET status = 'archived', modify_time = NOW() WHERE status = 'active' AND id <> $1`,
		policyID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("archive previous active policy: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.pipeline_policies
SET status = 'active', activated_at = NOW(), activated_by = 'doc-processing-policy-seed', checksum = $1, modify_time = NOW()
WHERE id = $2`, compiled.Checksum, policyID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("activate policy: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

// titleFromPipelineName derives a short display name from a config section
// name (e.g. "no-entities-relations" -> "No Entities Relations"), since
// config only supplies a name and a paragraph-length description, not a
// separate short title.
func titleFromPipelineName(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func upsertDocProcessingPipeline(ctx context.Context, tx *sql.Tx, name string, policy DocProcessingPolicySeedPolicy) (id int64, created bool, err error) {
	displayName := titleFromPipelineName(name)
	err = tx.QueryRowContext(ctx, `SELECT id FROM kb.pipelines WHERE name = $1`, name).Scan(&id)
	switch {
	case err == nil:
		if _, execErr := tx.ExecContext(ctx, `
UPDATE kb.pipelines SET display_name = $1, description = $2, processors = $3, is_system_default = $4, modify_time = NOW() WHERE id = $5`,
			displayName, policy.Description, pq.Array(policy.Processors), policy.IsDefault, id); execErr != nil {
			return 0, false, execErr
		}
		return id, false, nil
	case errors.Is(err, sql.ErrNoRows):
		if scanErr := tx.QueryRowContext(ctx, `
INSERT INTO kb.pipelines (name, display_name, description, processors, legacy_equivalent, is_system_default)
VALUES ($1, $2, $3, $4, false, $5)
RETURNING id`, name, displayName, policy.Description, pq.Array(policy.Processors), policy.IsDefault).Scan(&id); scanErr != nil {
			return 0, false, scanErr
		}
		return id, true, nil
	default:
		return 0, false, err
	}
}
