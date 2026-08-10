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
	// PipelineVersions is the version each named pipeline ended up at after
	// this run (ADR 2026081001 DR1/DR3 replaced the retired system-wide
	// PolicyID/PolicyVersion with per-pipeline versioning).
	PipelineVersions map[string]int
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
// name), authoring a new version of each (ADR 2026081001 DR1/DR2: atomic,
// one transaction, superseding whatever version was previously current for
// that name) carrying one unconditional kb.pipeline_rules gate row per
// processor named in that policy's processors list (require effect,
// always-true predicate -- makes kb.pipeline_rules a real Tier-2 mirror of
// the Tier-1 processors list, since nothing else authors Tier-2 gates
// today), validates each version (DR8), then upserts the system-wide
// store_default binding (cfg.DefaultPolicyName()) plus one store_default
// binding per cfg.Bindings entry to point at the freshly authored pipeline
// versions.
//
// ADR 2026081001 DR3 retired kb.pipeline_policies (the single system-wide
// "activation envelope" this used to mint/archive/activate as one unit).
// There is no longer a single switch that atomically replaces every
// binding/rule system-wide: each named pipeline gets its own new version
// (independent of every other pipeline this run touches), and each binding
// is upserted by name rather than wholesale-replaced. This is a real,
// narrower semantic than before -- a REST-API-authored binding unrelated to
// this config is untouched by a reseed, where the old model's "activation is
// a full replacement" would have deactivated it. Safe to call repeatedly:
// each call authors a new pipeline version per configured policy and
// upserts (not duplicates) the two configured binding kinds.
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

	result := DocProcessingPolicySeedResult{PipelineVersions: map[string]int{}}
	pipelineIDs := map[string]int64{}
	policyNames := make([]string, 0, len(cfg.Policies))
	for name := range cfg.Policies {
		policyNames = append(policyNames, name)
	}
	sort.Strings(policyNames)

	predicateJSON, predicateChecksum, err := semrules.Canonicalize(alwaysTruePredicate)
	if err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("canonicalize always-true predicate: %w", err)
	}

	for _, name := range policyNames {
		pipelineID, version, created, err := authorDocProcessingPipelineVersion(ctx, tx, name, cfg.Policies[name], predicateJSON, predicateChecksum)
		if err != nil {
			return DocProcessingPolicySeedResult{}, fmt.Errorf("pipeline %q: %w", name, err)
		}
		pipelineIDs[name] = pipelineID
		result.PipelineVersions[name] = version
		if created {
			result.PipelinesCreated = append(result.PipelinesCreated, name)
		} else {
			result.PipelinesUpdated = append(result.PipelinesUpdated, name)
		}
		result.RulesWritten += len(cfg.Policies[name].Processors)
	}

	defaultPipelineID := pipelineIDs[cfg.DefaultPolicyName()]
	if err := upsertDocProcessingBinding(ctx, tx, "system-default", nil, defaultPipelineID); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("upsert system-default binding: %w", err)
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
		if err := upsertDocProcessingBinding(ctx, tx, fmt.Sprintf("store:%s", store), &ksStoreID, pipelineID); err != nil {
			return DocProcessingPolicySeedResult{}, fmt.Errorf("upsert binding for %q: %w", store, err)
		}
		result.BindingsWritten++
	}

	if err := tx.Commit(); err != nil {
		return DocProcessingPolicySeedResult{}, fmt.Errorf("commit: %w", err)
	}
	return result, nil
}

// authorDocProcessingPipelineVersion authors one new kb.pipelines version
// for name (ADR 2026081001 DR1/DR2: locks the current version row,
// validates the proposed version per DR8, inserts the new row, supersedes
// the prior current version if one existed, and inserts one unconditional
// require-effect gate row per processor) -- the same atomic pattern
// kbhandler.CreatePipeline uses, duplicated here rather than shared across
// the kbhandler/docprocessing package boundary.
func authorDocProcessingPipelineVersion(ctx context.Context, tx *sql.Tx, name string, policy DocProcessingPolicySeedPolicy, predicateJSON []byte, predicateChecksum string) (pipelineID int64, version int, created bool, err error) {
	processors := append([]string(nil), policy.Processors...)
	sort.Strings(processors)

	rules := make([]PipelineGate, 0, len(processors))
	for _, processor := range processors {
		rules = append(rules, PipelineGate{
			Name: fmt.Sprintf("%s: %s", name, processor), TargetProcessor: processor,
			Effect: GateEffectRequire, Predicate: alwaysTruePredicate, PredicateChecksum: predicateChecksum, Active: true,
		})
	}
	if err := ValidatePipelineVersion(PipelineVersionDraft{Processors: processors, Rules: rules}); err != nil {
		return 0, 0, false, err
	}

	var priorID sql.NullInt64
	var priorVersion int
	err = tx.QueryRowContext(ctx, `SELECT id, version FROM kb.pipelines WHERE name = $1 ORDER BY version DESC LIMIT 1 FOR UPDATE`, name).Scan(&priorID, &priorVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, fmt.Errorf("lock prior version: %w", err)
	}
	nextVersion := priorVersion + 1

	displayName := titleFromPipelineName(name)
	var description any
	if strings.TrimSpace(policy.Description) != "" {
		description = policy.Description
	}
	if err := tx.QueryRowContext(ctx, `
INSERT INTO kb.pipelines (name, display_name, description, processors, legacy_equivalent, is_system_default, version, status)
VALUES ($1, $2, $3, $4, false, $5, $6, 'active')
RETURNING id`, name, displayName, description, pq.Array(processors), policy.IsDefault, nextVersion).Scan(&pipelineID); err != nil {
		return 0, 0, false, fmt.Errorf("create version: %w", err)
	}

	if priorID.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE kb.pipelines SET status = 'superseded', modify_time = NOW() WHERE id = $1`, priorID.Int64); err != nil {
			return 0, 0, false, fmt.Errorf("supersede prior version: %w", err)
		}
	}

	for _, processor := range processors {
		ruleName := fmt.Sprintf("%s: %s", name, processor)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO kb.pipeline_rules
    (name, priority, predicate, predicate_checksum, target_processor, effect, active, pipeline_id, approval_status)
VALUES ($1, 0, $2::jsonb, $3, $4, 'require', true, $5, 'approved')`,
			ruleName, string(predicateJSON), predicateChecksum, processor, pipelineID); err != nil {
			return 0, 0, false, fmt.Errorf("insert rule %q: %w", ruleName, err)
		}
	}

	return pipelineID, nextVersion, !priorID.Valid, nil
}

// upsertDocProcessingBinding materializes one store_default binding by name:
// updates an existing row's target pipeline (reactivating it) or inserts a
// new one. ADR 2026081001 DR3 retired the "one policy_id groups every
// binding this run wrote" replacement model; upsert-by-name is the
// equivalent for a single binding under the per-row active model.
func upsertDocProcessingBinding(ctx context.Context, tx *sql.Tx, name string, ksStoreID *int64, pipelineID int64) error {
	var existingID int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM kb.pipeline_bindings WHERE name = $1`, name).Scan(&existingID)
	switch {
	case err == nil:
		_, execErr := tx.ExecContext(ctx, `UPDATE kb.pipeline_bindings SET pipeline_id = $1, active = true, modify_time = NOW() WHERE id = $2`, pipelineID, existingID)
		return execErr
	case errors.Is(err, sql.ErrNoRows):
		var ksArg any
		if ksStoreID != nil {
			ksArg = *ksStoreID
		}
		_, execErr := tx.ExecContext(ctx, `
INSERT INTO kb.pipeline_bindings (ks_store_id, pipeline_id, name, priority, active, binding_kind)
VALUES ($1, $2, $3, 0, true, 'store_default')`, ksArg, pipelineID, name)
		return execErr
	default:
		return err
	}
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
