package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// AmbiguousObjectRow is one kb.artifact_objects row stuck at
// reconcile_status='ambiguous' (object_id NULL), reconstructed enough to
// re-run reconciliation.
type AmbiguousObjectRow struct {
	RowID  int64
	Object ArtifactObject
}

// AmbiguousObjectStore is the DB seam for loading and resolving ambiguous
// kb.artifact_objects rows.
type AmbiguousObjectStore interface {
	LoadAmbiguous(ctx context.Context, limit int) ([]AmbiguousObjectRow, error)
	UpdateResolution(ctx context.Context, rowID int64, objectID, status string, confidence float64, extInfo map[string]any) error
}

// ResolveAmbiguousResult summarizes one call to ResolveAmbiguousArtifactObjects.
type ResolveAmbiguousResult struct {
	Scanned   int
	Matched   int
	TieBroken int
	Created   int
	Failed    int
}

// ResolveAmbiguousArtifactObjects re-attempts reconciliation for rows left at
// reconcile_status='ambiguous' by ReconcileOne's tied-candidate branch (see
// artifact_objects.go). Unlike the original per-record pass, this always
// reaches a resolution: a natural match if the tie broke on its own since the
// row was written, a deterministic tie-break (pickTieBreakCandidate) if it is
// still tied, or a new object node if no candidates remain. Call repeatedly
// with a bounded limit to drain the backlog.
func ResolveAmbiguousArtifactObjects(ctx context.Context, store AmbiguousObjectStore, reconciler ObjectReconciler, logger ApiTypes.JimoLogger, limit int) (ResolveAmbiguousResult, error) {
	var res ResolveAmbiguousResult
	rows, err := store.LoadAmbiguous(ctx, limit)
	if err != nil {
		return res, err
	}
	res.Scanned = len(rows)

	for _, row := range rows {
		candidates, err := reconciler.Store.FindCandidates(ctx, row.Object, reconciler.Options)
		if err != nil {
			res.Failed++
			if logger != nil {
				logger.Warn("resolve ambiguous object: find candidates failed",
					"row_id", row.RowID, "artifact_id", row.Object.ArtifactID, "error", err.Error())
			}
			continue
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })

		var (
			objectID   string
			status     string
			confidence float64
			method     string
		)
		switch {
		case len(candidates) == 0:
			node, err := reconciler.Store.CreateNode(ctx, row.Object)
			if err != nil {
				res.Failed++
				if logger != nil {
					logger.Warn("resolve ambiguous object: create node failed",
						"row_id", row.RowID, "artifact_id", row.Object.ArtifactID, "error", err.Error())
				}
				continue
			}
			objectID, status, confidence, method = node.ObjectID, ObjectReconcileNew, 1, "new_node"
			res.Created++
		case len(candidates) == 1 || candidates[0].Score > candidates[1].Score:
			objectID, status, confidence, method = candidates[0].Node.ObjectID, ObjectReconcileMatched, candidates[0].Score, candidates[0].Method
			res.Matched++
		default:
			winner := pickTieBreakCandidate(row.Object, candidates)
			objectID, status, confidence, method = winner.Node.ObjectID, ObjectReconcileAmbiguousResolved, winner.Score, "tie_break_deterministic"
			res.TieBroken++
		}

		extInfo := map[string]any{"reconcile_method": method}
		if err := store.UpdateResolution(ctx, row.RowID, objectID, status, confidence, extInfo); err != nil {
			res.Failed++
			if logger != nil {
				logger.Warn("resolve ambiguous object: update failed",
					"row_id", row.RowID, "artifact_id", row.Object.ArtifactID, "error", err.Error())
			}
			continue
		}
		if logger != nil {
			logger.Info("resolved ambiguous object",
				"row_id", row.RowID, "artifact_id", row.Object.ArtifactID, "object_id", objectID, "status", status)
		}
	}
	return res, nil
}

// pickTieBreakCandidate deterministically resolves a tie between object node
// candidates that FindCandidates scored equally. It prefers the candidate that
// shares the most normalized names with the artifact object, and falls back to
// the lexicographically smallest object_id for stability across runs.
func pickTieBreakCandidate(obj ArtifactObject, candidates []ObjectNodeCandidate) ObjectNodeCandidate {
	best := candidates[0]
	bestOverlap := normalizedNameOverlapCount(obj.NormalizedNames, best.Node.NormalizedNames)
	for _, c := range candidates[1:] {
		overlap := normalizedNameOverlapCount(obj.NormalizedNames, c.Node.NormalizedNames)
		if overlap > bestOverlap || (overlap == bestOverlap && c.Node.ObjectID < best.Node.ObjectID) {
			best = c
			bestOverlap = overlap
		}
	}
	return best
}

func normalizedNameOverlapCount(a, b []string) int {
	set := make(map[string]struct{}, len(a))
	for _, x := range a {
		if x != "" {
			set[x] = struct{}{}
		}
	}
	count := 0
	for _, y := range uniqueStrings(b) {
		if _, ok := set[y]; ok {
			count++
		}
	}
	return count
}

// LoadAmbiguous implements AmbiguousObjectStore for ArtifactObjectSQLStore.
func (s ArtifactObjectSQLStore) LoadAmbiguous(ctx context.Context, limit int) ([]AmbiguousObjectRow, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, source_record_id, input_record_id, artifact_type, artifact_id,
       object_name, COALESCE(object_name_en, ''), COALESCE(object_name_zh, ''),
       COALESCE(language, ''), object_type, object_role,
       COALESCE(aliases, '[]'::jsonb), COALESCE(acronyms, '[]'::jsonb), COALESCE(normalized_names, '[]'::jsonb),
       COALESCE(description, ''), COALESCE(evidence_quote, ''), COALESCE(source_line_spans, '[]'::jsonb), confidence
FROM kb.artifact_objects
WHERE reconcile_status = $1 AND object_id IS NULL
ORDER BY id
LIMIT $2`, ObjectReconcileAmbiguous, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []AmbiguousObjectRow
	for rows.Next() {
		var (
			row                                    AmbiguousObjectRow
			aliasesRaw, acronymsRaw, normalizedRaw []byte
			spansRaw                               []byte
		)
		if err := rows.Scan(
			&row.RowID, &row.Object.SourceRecordID, &row.Object.InputRecordID,
			&row.Object.ArtifactType, &row.Object.ArtifactID,
			&row.Object.ObjectName, &row.Object.ObjectNameEn, &row.Object.ObjectNameZh,
			&row.Object.Language, &row.Object.ObjectType, &row.Object.ObjectRole,
			&aliasesRaw, &acronymsRaw, &normalizedRaw,
			&row.Object.Description, &row.Object.EvidenceQuote, &spansRaw, &row.Object.Confidence,
		); err != nil {
			return nil, err
		}
		row.Object.Aliases = jsonStringArray(aliasesRaw)
		row.Object.Acronyms = jsonStringArray(acronymsRaw)
		row.Object.NormalizedNames = jsonStringArray(normalizedRaw)
		_ = json.Unmarshal(spansRaw, &row.Object.SourceLineSpans)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateResolution implements AmbiguousObjectStore for ArtifactObjectSQLStore.
func (s ArtifactObjectSQLStore) UpdateResolution(ctx context.Context, rowID int64, objectID, status string, confidence float64, extInfo map[string]any) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	ext, err := json.Marshal(extInfo)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
UPDATE kb.artifact_objects
SET object_id = $1, reconcile_status = $2, reconcile_confidence = $3, ext_info = $4::jsonb
WHERE id = $5`, nullEmpty(objectID), status, confidence, string(ext), rowID)
	return err
}
