package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/lib/pq"
)

const objectAnchorComparableLimit = 20

type objectAnchorPeerIDs map[int][]string

type artifactObjectAnchor struct {
	docIdx   int
	objectID string
}

// loadObjectAnchoredPeerIDs returns cross-document candidate artifact ids keyed by the
// document artifact index. It is a deterministic pre-LLM retrieval step: artifact under
// review -> kb.artifact_objects -> kb.object_nodes/comparable objects ->
// kb.artifact_connections object_id/belong_to edges -> peer artifact ids.
func loadObjectAnchoredPeerIDs(
	ctx context.Context,
	db *sql.DB,
	recordID int64,
	artifactType string,
	artifactIDs []string,
) (objectAnchorPeerIDs, error) {
	out := make(objectAnchorPeerIDs)
	if db == nil || len(artifactIDs) == 0 {
		return out, nil
	}

	idxByArtifactID := make(map[string]int, len(artifactIDs))
	for i, id := range artifactIDs {
		if id != "" {
			idxByArtifactID[id] = i
		}
	}
	if len(idxByArtifactID) == 0 {
		return out, nil
	}

	anchors, objectIDs, err := loadArtifactObjectAnchors(ctx, db, recordID, artifactType, artifactIDs, idxByArtifactID)
	if err != nil {
		return nil, err
	}
	if len(anchors) == 0 {
		return out, nil
	}

	comparableByObject, err := loadComparableObjectIDs(ctx, db, objectIDs)
	if err != nil {
		return nil, err
	}

	edgeObjectSet := make(map[string]struct{}, len(objectIDs))
	for _, objectID := range objectIDs {
		edgeObjectSet[objectID] = struct{}{}
		for _, compID := range comparableByObject[objectID] {
			edgeObjectSet[compID] = struct{}{}
		}
	}
	edgeObjectIDs := sortedStringKeys(edgeObjectSet)
	peerIDsByObject, err := loadObjectEdgeArtifactIDs(ctx, db, artifactType, edgeObjectIDs)
	if err != nil {
		return nil, err
	}

	for _, anchor := range anchors {
		seen := make(map[string]struct{})
		appendIDs := func(objectID string) {
			for _, peerID := range peerIDsByObject[objectID] {
				if peerID == "" {
					continue
				}
				if _, ok := seen[peerID]; ok {
					continue
				}
				seen[peerID] = struct{}{}
				out[anchor.docIdx] = append(out[anchor.docIdx], peerID)
			}
		}
		appendIDs(anchor.objectID)
		for _, compID := range comparableByObject[anchor.objectID] {
			appendIDs(compID)
		}
	}

	return out, nil
}

func loadArtifactObjectAnchors(
	ctx context.Context,
	db *sql.DB,
	recordID int64,
	artifactType string,
	artifactIDs []string,
	idxByArtifactID map[string]int,
) ([]artifactObjectAnchor, []string, error) {
	const q = `
SELECT COALESCE(artifact_id, ''), COALESCE(object_id, '')
FROM kb.artifact_objects
WHERE source_record_id = $1
  AND artifact_type = $2
  AND artifact_id = ANY($3)
  AND COALESCE(object_id, '') <> ''`
	rows, err := db.QueryContext(ctx, q, recordID, artifactType, pq.Array(artifactIDs))
	if err != nil {
		return nil, nil, fmt.Errorf("(MID_26070501) load artifact object anchors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var anchors []artifactObjectAnchor
	objectSet := make(map[string]struct{})
	anchorSeen := make(map[string]struct{})
	for rows.Next() {
		var artifactID, objectID string
		if err := rows.Scan(&artifactID, &objectID); err != nil {
			return nil, nil, fmt.Errorf("(MID_26070502) scan artifact object anchor: %w", err)
		}
		docIdx, ok := idxByArtifactID[artifactID]
		if !ok || objectID == "" {
			continue
		}
		key := fmt.Sprintf("%d\x00%s", docIdx, objectID)
		if _, ok := anchorSeen[key]; ok {
			continue
		}
		anchorSeen[key] = struct{}{}
		anchors = append(anchors, artifactObjectAnchor{docIdx: docIdx, objectID: objectID})
		objectSet[objectID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("(MID_26070503) iterate artifact object anchors: %w", err)
	}
	return anchors, sortedStringKeys(objectSet), nil
}

func loadComparableObjectIDs(ctx context.Context, db *sql.DB, objectIDs []string) (map[string][]string, error) {
	out := make(map[string][]string)
	if len(objectIDs) == 0 {
		return out, nil
	}

	const nodeQ = `
SELECT object_id, COALESCE(object_type, 'other'), COALESCE(normalized_names, '[]'::jsonb)
FROM kb.object_nodes
WHERE object_id = ANY($1)
  AND reconcile_status <> 'rejected'`
	rows, err := db.QueryContext(ctx, nodeQ, pq.Array(objectIDs))
	if err != nil {
		return nil, fmt.Errorf("(MID_26070504) load object nodes for comparable search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type node struct {
		objectID string
		objType  string
		names    []string
	}
	var nodes []node
	for rows.Next() {
		var objectID, objType string
		var namesJSON []byte
		if err := rows.Scan(&objectID, &objType, &namesJSON); err != nil {
			return nil, fmt.Errorf("(MID_26070505) scan object node for comparable search: %w", err)
		}
		names := parseJSONStringArray(namesJSON)
		if objectID != "" && objType != "" && len(names) > 0 {
			nodes = append(nodes, node{objectID: objectID, objType: objType, names: names})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("(MID_26070506) iterate object nodes for comparable search: %w", err)
	}

	const compQ = `
SELECT object_id
FROM kb.object_nodes
WHERE reconcile_status <> 'rejected'
  AND object_type = $1
  AND object_id <> $2
  AND normalized_names ?| $3
ORDER BY object_id
LIMIT $4`
	for _, n := range nodes {
		compRows, err := db.QueryContext(ctx, compQ, n.objType, n.objectID, pq.Array(n.names), objectAnchorComparableLimit)
		if err != nil {
			return nil, fmt.Errorf("(MID_26070507) load comparable object ids: %w", err)
		}
		for compRows.Next() {
			var compID string
			if err := compRows.Scan(&compID); err != nil {
				_ = compRows.Close()
				return nil, fmt.Errorf("(MID_26070508) scan comparable object id: %w", err)
			}
			if compID != "" {
				out[n.objectID] = append(out[n.objectID], compID)
			}
		}
		if err := compRows.Err(); err != nil {
			_ = compRows.Close()
			return nil, fmt.Errorf("(MID_26070509) iterate comparable object ids: %w", err)
		}
		_ = compRows.Close()
	}
	return out, nil
}

func loadObjectEdgeArtifactIDs(
	ctx context.Context,
	db *sql.DB,
	artifactType string,
	objectIDs []string,
) (map[string][]string, error) {
	out := make(map[string][]string)
	if len(objectIDs) == 0 {
		return out, nil
	}

	const q = `
SELECT target_id, COALESCE(extra_info->'artifact_ids', '[]'::jsonb)
FROM kb.artifact_connections
WHERE relation_method = 'object_id'
  AND relation_name = 'belong_to'
  AND source_type = $1
  AND target_type = 'object_node'
  AND target_id = ANY($2)`
	rows, err := db.QueryContext(ctx, q, artifactType, pq.Array(objectIDs))
	if err != nil {
		return nil, fmt.Errorf("(MID_26070510) load object edge artifact ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var objectID string
		var idsJSON []byte
		if err := rows.Scan(&objectID, &idsJSON); err != nil {
			return nil, fmt.Errorf("(MID_26070511) scan object edge artifact ids: %w", err)
		}
		var ids []string
		if err := json.Unmarshal(idsJSON, &ids); err != nil {
			ids = parseJSONStringArray(idsJSON)
		}
		out[objectID] = append(out[objectID], ids...)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("(MID_26070512) iterate object edge artifact ids: %w", err)
	}
	return out, nil
}

func sortedStringKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
