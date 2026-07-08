package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/lib/pq"
)

type ObjectNodeSQLStore struct {
	DB *sql.DB
}

func (s ObjectNodeSQLStore) FindCandidates(ctx context.Context, obj ArtifactObject, opts ObjectReconcileOptions) ([]ObjectNodeCandidate, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	names := uniqueStrings(obj.NormalizedNames)
	if len(names) == 0 {
		return nil, nil
	}
	maxCandidates := opts.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 10
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT object_id,
       COALESCE(canonical_object_id, ''),
       COALESCE(canonical_name, ''),
       COALESCE(canonical_name_en, ''),
       COALESCE(canonical_name_zh, ''),
       COALESCE(primary_language, ''),
       COALESCE(object_type, ''),
       COALESCE(aliases, '[]'::jsonb),
       COALESCE(acronyms, '[]'::jsonb),
       COALESCE(normalized_names, '[]'::jsonb),
       COALESCE(description, ''),
       COALESCE(search_document, ''),
       COALESCE(reconcile_status, ''),
       COALESCE(ext_info, '{}'::jsonb)
FROM kb.object_nodes
WHERE reconcile_status NOT IN ('rejected', 'merged')
  AND (normalized_names ?| $1 OR canonical_name = $2 OR canonical_name_en = $2 OR canonical_name_zh = $2)
ORDER BY CASE WHEN object_type = $3 THEN 0 ELSE 1 END, id
LIMIT $4`, pq.Array(names), obj.ObjectName, obj.ObjectType, maxCandidates)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	nodes, err := scanObjectNodes(rows)
	if err != nil {
		return nil, err
	}

	out := make([]ObjectNodeCandidate, 0, len(nodes))
	for _, node := range nodes {
		score := 0.85
		method := "lexical_name"
		if objectTypesCompatible(obj.ObjectType, node.ObjectType) && objectNameBundlesOverlap(obj.NormalizedNames, node.NormalizedNames) {
			score = 1
			method = "exact_name"
		}
		out = append(out, ObjectNodeCandidate{Node: node, Score: score, Method: method})
	}
	return out, nil
}

// FindByCanonicalName returns every kb.object_nodes row whose canonical_name
// exactly matches name. Used by the "Create New" action on the Resolve
// Ambiguous Objects admin page to avoid creating a duplicate node for a name
// that already exists.
func (s ObjectNodeSQLStore) FindByCanonicalName(ctx context.Context, name string) ([]ObjectNode, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT object_id,
       COALESCE(canonical_object_id, ''),
       COALESCE(canonical_name, ''),
       COALESCE(canonical_name_en, ''),
       COALESCE(canonical_name_zh, ''),
       COALESCE(primary_language, ''),
       COALESCE(object_type, ''),
       COALESCE(aliases, '[]'::jsonb),
       COALESCE(acronyms, '[]'::jsonb),
       COALESCE(normalized_names, '[]'::jsonb),
       COALESCE(description, ''),
       COALESCE(search_document, ''),
       COALESCE(reconcile_status, ''),
       COALESCE(ext_info, '{}'::jsonb)
FROM kb.object_nodes
WHERE canonical_name = $1
ORDER BY object_id`, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanObjectNodes(rows)
}

// scanObjectNodes scans the shared column set used by FindCandidates and
// FindByCanonicalName into ObjectNode rows.
func scanObjectNodes(rows *sql.Rows) ([]ObjectNode, error) {
	var out []ObjectNode
	for rows.Next() {
		var (
			node                    ObjectNode
			aliasesRaw, acronymsRaw []byte
			namesRaw, extRaw        []byte
		)
		if err := rows.Scan(
			&node.ObjectID,
			&node.CanonicalObjectID,
			&node.CanonicalName,
			&node.CanonicalNameEn,
			&node.CanonicalNameZh,
			&node.PrimaryLanguage,
			&node.ObjectType,
			&aliasesRaw,
			&acronymsRaw,
			&namesRaw,
			&node.Description,
			&node.SearchDocument,
			&node.ReconcileStatus,
			&extRaw,
		); err != nil {
			return nil, err
		}
		node.Aliases = jsonStringArray(aliasesRaw)
		node.Acronyms = jsonStringArray(acronymsRaw)
		node.NormalizedNames = jsonStringArray(namesRaw)
		_ = json.Unmarshal(extRaw, &node.ExtInfo)
		out = append(out, node)
	}
	return out, rows.Err()
}

func (s ObjectNodeSQLStore) CreateNode(ctx context.Context, obj ArtifactObject) (ObjectNode, error) {
	if s.DB == nil {
		return ObjectNode{}, fmt.Errorf("db is nil")
	}
	node := ObjectNode{
		ObjectID:          buildObjectID(obj.InputRecordID, firstNonEmptyTrimmed(obj.ObjectName, obj.ObjectNameEn, obj.ObjectNameZh), 1),
		CanonicalName:     firstNonEmptyTrimmed(obj.ObjectName, obj.ObjectNameEn, obj.ObjectNameZh),
		CanonicalNameEn:   obj.ObjectNameEn,
		CanonicalNameZh:   obj.ObjectNameZh,
		PrimaryLanguage:   obj.Language,
		ObjectType:        firstNonEmptyTrimmed(obj.ObjectType, "other"),
		Aliases:           obj.Aliases,
		Acronyms:          obj.Acronyms,
		NormalizedNames:   obj.NormalizedNames,
		Description:       obj.Description,
		SearchDocument:    buildObjectNodeSearchDocument(obj),
		ReconcileStatus:   "active",
		ExtInfo:           map[string]any{"source": "object_reconciliation"},
		CanonicalObjectID: "",
	}
	aliases, _ := json.Marshal(orEmptySlice(node.Aliases))
	acronyms, _ := json.Marshal(orEmptySlice(node.Acronyms))
	names, _ := json.Marshal(orEmptySlice(node.NormalizedNames))
	ext, _ := json.Marshal(node.ExtInfo)
	err := s.DB.QueryRowContext(ctx, `
INSERT INTO kb.object_nodes (
	object_id, canonical_name, canonical_name_en, canonical_name_zh,
	primary_language, object_type, aliases, acronyms, normalized_names,
	description, search_document, reconcile_status, ext_info
) VALUES (
	$1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10,$11,$12,$13::jsonb
)
ON CONFLICT (object_id) DO UPDATE SET
	normalized_names = (
		COALESCE(
			(
				SELECT jsonb_agg(DISTINCT value)
				FROM jsonb_array_elements_text(kb.object_nodes.normalized_names || EXCLUDED.normalized_names) AS t(value)
			),
			'[]'::jsonb
		)
	),
	aliases = (
		COALESCE(
			(
				SELECT jsonb_agg(DISTINCT value)
				FROM jsonb_array_elements_text(kb.object_nodes.aliases || EXCLUDED.aliases) AS t(value)
			),
			'[]'::jsonb
		)
	),
	acronyms = (
		COALESCE(
			(
				SELECT jsonb_agg(DISTINCT value)
				FROM jsonb_array_elements_text(kb.object_nodes.acronyms || EXCLUDED.acronyms) AS t(value)
			),
			'[]'::jsonb
		)
	),
	search_document = EXCLUDED.search_document
RETURNING object_id`,
		node.ObjectID,
		node.CanonicalName,
		nullEmpty(node.CanonicalNameEn),
		nullEmpty(node.CanonicalNameZh),
		nullEmpty(node.PrimaryLanguage),
		node.ObjectType,
		string(aliases),
		string(acronyms),
		string(names),
		nullEmpty(node.Description),
		node.SearchDocument,
		node.ReconcileStatus,
		string(ext),
	).Scan(&node.ObjectID)
	if err != nil {
		return ObjectNode{}, err
	}
	return node, nil
}

func reconcileArtifactObjects(ctx context.Context, objects []ArtifactObject, reconciler ObjectReconciler, logger ApiTypes.JimoLogger) ([]ArtifactObject, error) {
	out := make([]ArtifactObject, 0, len(objects))
	for _, obj := range objects {
		result, err := reconciler.ReconcileOne(ctx, obj)
		if err != nil {
			return nil, err
		}
		if result.Status == ObjectReconcileAmbiguous && logger != nil {
			candidates, candErr := reconciler.Store.FindCandidates(ctx, obj, reconciler.Options)
			var names []string
			if candErr == nil {
				for _, c := range candidates {
					names = append(names, c.Node.ObjectID+":"+c.Node.CanonicalName)
				}
			}
			logger.Info("***** object reconciliation ambiguous",
				"input_record_id", obj.InputRecordID,
				"artifact_type", obj.ArtifactType,
				"artifact_id", obj.ArtifactID,
				"object_name", obj.ObjectName,
				"top_score", result.Confidence,
				"candidates", names,
			)
		}
		obj.ObjectID = result.ObjectID
		obj.ReconcileStatus = result.Status
		obj.ReconcileConfidence = result.Confidence
		if obj.ExtInfo == nil {
			obj.ExtInfo = map[string]any{}
		}
		obj.ExtInfo["reconcile_method"] = result.Method
		out = append(out, obj)
	}
	return out, nil
}

func buildObjectNodeSearchDocument(obj ArtifactObject) string {
	return joinUniqueSearchParts(
		obj.ObjectName,
		obj.ObjectNameEn,
		obj.ObjectNameZh,
		searchDocumentArrayText(obj.Aliases),
		searchDocumentArrayText(obj.Acronyms),
		obj.ObjectType,
		obj.ObjectRole,
		obj.Description,
		obj.EvidenceQuote,
	)
}

func jsonStringArray(raw []byte) []string {
	var out []string
	_ = json.Unmarshal(raw, &out)
	return uniqueStrings(out)
}

func ObjectReconcileOptionsFromEnv() ObjectReconcileOptions {
	return ObjectReconcileOptions{
		EmbeddingEnabled: strings.EqualFold(strings.TrimSpace(envString("OBJECT_RECONCILE_EMBEDDING_ENABLED", "false")), "true"),
		MaxCandidates:    envInt("OBJECT_RECONCILE_MAX_CANDIDATES", 10, 1),
	}
}

func envString(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}
