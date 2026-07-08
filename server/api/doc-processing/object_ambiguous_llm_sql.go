package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ApplyAmbiguousObjectLLMNodeChanges applies DR7 object-node field updates and
// merge/repoint operations in one transaction.
func (s ObjectNodeSQLStore) ApplyAmbiguousObjectLLMNodeChanges(ctx context.Context, obj ArtifactObject, updates []AmbiguousObjectNodeLLMUpdate, merges []AmbiguousObjectNodeLLMMerge, audit AmbiguousObjectLLMAudit) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, update := range updates {
		if err := applyAmbiguousObjectLLMNodeUpdate(ctx, tx, update, audit); err != nil {
			return err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, merge := range merges {
		for _, loserID := range merge.LoserObjectIDs {
			if err := applyAmbiguousObjectLLMNodeMerge(ctx, tx, obj, merge, loserID, now, audit); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func applyAmbiguousObjectLLMNodeUpdate(ctx context.Context, tx *sql.Tx, update AmbiguousObjectNodeLLMUpdate, audit AmbiguousObjectLLMAudit) error {
	extraNames := uniqueStrings([]string{
		normalizeObjectName(update.CanonicalNameEn),
		normalizeObjectName(update.CanonicalNameZh),
	})
	namesJSON, _ := json.Marshal(extraNames)
	searchText := joinUniqueSearchParts(update.CanonicalNameEn, update.CanonicalNameZh, update.ObjectType, update.Description)
	res, err := tx.ExecContext(ctx, `
UPDATE kb.object_nodes SET
	canonical_name_en = COALESCE(NULLIF($1, ''), canonical_name_en),
	canonical_name_zh = COALESCE(NULLIF($2, ''), canonical_name_zh),
	object_type = COALESCE(NULLIF($3, ''), object_type),
	description = COALESCE(NULLIF($4, ''), description),
	normalized_names = COALESCE((
		SELECT jsonb_agg(DISTINCT value)
		FROM jsonb_array_elements_text(normalized_names || $5::jsonb) AS t(value)
	), '[]'::jsonb),
	search_document = trim(concat_ws(' ', search_document, NULLIF($6, '')))
WHERE object_id = $7`,
		strings.TrimSpace(update.CanonicalNameEn),
		strings.TrimSpace(update.CanonicalNameZh),
		normalizeObjectToken(update.ObjectType),
		strings.TrimSpace(update.Description),
		string(namesJSON),
		searchText,
		update.ObjectID,
	)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("object node %q not found for LLM update", update.ObjectID)
	}
	payload := map[string]any{
		"source":            ObjectReconcileMethodLLMAmbiguous,
		"model":             strings.TrimSpace(audit.ModelName),
		"rationale":         strings.TrimSpace(audit.Rationale),
		"canonical_name_en": strings.TrimSpace(update.CanonicalNameEn),
		"canonical_name_zh": strings.TrimSpace(update.CanonicalNameZh),
		"object_type":       normalizeObjectToken(update.ObjectType),
		"description":       strings.TrimSpace(update.Description),
	}
	return insertObjectAuditLog(ctx, tx, "kb.object_nodes", update.ObjectID, "edit_fields", "llm", payload)
}

func applyAmbiguousObjectLLMNodeMerge(ctx context.Context, tx *sql.Tx, obj ArtifactObject, merge AmbiguousObjectNodeLLMMerge, loserID, now string, audit AmbiguousObjectLLMAudit) error {
	repointRes, err := tx.ExecContext(ctx,
		`UPDATE kb.artifact_objects SET object_id = $1 WHERE object_id = $2`,
		merge.SurvivorObjectID, loserID)
	if err != nil {
		return err
	}
	repointed, _ := repointRes.RowsAffected()

	extInfo := map[string]any{
		"merged_to":  merge.SurvivorObjectID,
		"merge_time": now,
	}
	extJSON, _ := json.Marshal(extInfo)
	res, err := tx.ExecContext(ctx, `
UPDATE kb.object_nodes SET canonical_object_id = $1,
	reconcile_status = 'merged',
	ext_info = COALESCE(ext_info, '{}'::jsonb) || $2::jsonb
WHERE object_id = $3`,
		merge.SurvivorObjectID, string(extJSON), loserID)
	if err != nil {
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return fmt.Errorf("object node %q not found for LLM merge", loserID)
	}
	payload := map[string]any{
		"source":             ObjectReconcileMethodLLMAmbiguous,
		"model":              strings.TrimSpace(audit.ModelName),
		"rationale":          strings.TrimSpace(audit.Rationale),
		"artifact_id":        obj.ArtifactID,
		"survivor_object_id": merge.SurvivorObjectID,
		"repointed_mentions": repointed,
		"confidence":         merge.Confidence,
	}
	return insertObjectAuditLog(ctx, tx, "kb.object_nodes", loserID, "merge_nodes", "llm", payload)
}

func insertObjectAuditLog(ctx context.Context, tx *sql.Tx, tableName, rowKey, action, actor string, payload map[string]any) error {
	changes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)`,
		tableName, rowKey, action, string(changes), actor)
	return err
}
