// policy_seed.go
package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

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
}

// SeedDocProcessingPolicies upserts cfg's policies into kb.pipelines (by
// name), authors a new draft kb.pipeline_policies version carrying one
// system-wide store_default binding (cfg.DefaultPolicyName()) plus one
// store_default binding per cfg.Bindings entry, compiles that version, and
// activates it -- archiving whatever policy was previously active. Every
// write happens in one transaction; any failure rolls everything back and
// leaves the previously active policy untouched. Safe to call repeatedly:
// each call creates and activates a new policy version rather than mutating
// a previous one.
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

func upsertDocProcessingPipeline(ctx context.Context, tx *sql.Tx, name string, policy DocProcessingPolicySeedPolicy) (id int64, created bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id FROM kb.pipelines WHERE name = $1`, name).Scan(&id)
	switch {
	case err == nil:
		if _, execErr := tx.ExecContext(ctx, `
UPDATE kb.pipelines SET display_name = $1, processors = $2, modify_time = NOW() WHERE id = $3`,
			policy.Description, pq.Array(policy.Processors), id); execErr != nil {
			return 0, false, execErr
		}
		return id, false, nil
	case errors.Is(err, sql.ErrNoRows):
		if scanErr := tx.QueryRowContext(ctx, `
INSERT INTO kb.pipelines (name, display_name, processors, legacy_equivalent)
VALUES ($1, $2, $3, false)
RETURNING id`, name, policy.Description, pq.Array(policy.Processors)).Scan(&id); scanErr != nil {
			return 0, false, scanErr
		}
		return id, true, nil
	default:
		return 0, false, err
	}
}
