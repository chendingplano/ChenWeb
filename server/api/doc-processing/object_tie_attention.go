package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/attention"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// LLM outcomes recorded on an ambiguous-object-name attention item.
const (
	tieLLMNotConfigured = "not_configured"
	tieLLMUnresolved    = "unresolved"
	tieLLMFailed        = "failed"
)

// sortCandidatesByScore orders candidates by score, highest first, keeping
// FindCandidates' order among equal scores.
func sortCandidatesByScore(candidates []ObjectNodeCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
}

// topTiedCandidates returns every candidate that shares the top score.
// candidates must already be sorted by score, highest first.
func topTiedCandidates(sorted []ObjectNodeCandidate) []ObjectNodeCandidate {
	if len(sorted) == 0 {
		return nil
	}
	n := 1
	for n < len(sorted) && sorted[n].Score == sorted[0].Score {
		n++
	}
	return sorted[:n]
}

// TieOccurrence is one artifact object that hit a tie, as stored in an
// ambiguous-object-name attention item's details.occurrences. It names the
// artifact, not the kb.artifact_objects row, because rows are re-inserted on
// every re-run of a record.
type TieOccurrence struct {
	InputRecordID int64  `json:"input_record_id"`
	ArtifactType  string `json:"artifact_type"`
	ArtifactID    string `json:"artifact_id"`
	ObjectName    string `json:"object_name"`
}

// tieDedupeKey identifies a tie by its tied node set, so every name that
// ties the same nodes lands on one attention item.
func tieDedupeKey(tied []ObjectNodeCandidate) string {
	ids := make([]string, 0, len(tied))
	for _, c := range tied {
		ids = append(ids, c.Node.ObjectID)
	}
	sort.Strings(ids)
	return strings.Join(ids, "|")
}

func ambiguousObjectTieReport(obj ArtifactObject, tied []ObjectNodeCandidate, picked ObjectNodeCandidate, llmOutcome string) attention.Report {
	name := firstNonEmptyTrimmed(obj.ObjectName, obj.ObjectNameEn, obj.ObjectNameZh)
	candidates := make([]map[string]any, 0, len(tied))
	for _, c := range tied {
		candidates = append(candidates, map[string]any{
			"object_id":      c.Node.ObjectID,
			"canonical_name": c.Node.CanonicalName,
			"object_type":    c.Node.ObjectType,
			"score":          c.Score,
			"method":         c.Method,
		})
	}
	return attention.Report{
		ItemType:      attention.TypeAmbiguousObjectName,
		DedupeKey:     tieDedupeKey(tied),
		Title:         name,
		InputRecordID: obj.InputRecordID,
		Details: map[string]any{
			"object_name":      name,
			"picked_object_id": picked.Node.ObjectID,
			"tied_candidates":  candidates,
			"llm_outcome":      llmOutcome,
		},
		Occurrence: map[string]any{
			"input_record_id": obj.InputRecordID,
			"artifact_type":   obj.ArtifactType,
			"artifact_id":     obj.ArtifactID,
			"object_name":     name,
		},
	}
}

// breakObjectTie links obj to the deterministic tie-break pick among the tied
// candidates and records an ambiguous-object-name attention item. A failure
// to record is logged and does not undo the link.
func breakObjectTie(ctx context.Context, reconciler ObjectReconciler, logger ApiTypes.JimoLogger, obj ArtifactObject, tied []ObjectNodeCandidate, llmOutcome string) ObjectReconcileResult {
	picked := pickTieBreakCandidate(obj, tied)
	result := ObjectReconcileResult{
		ObjectID:   picked.Node.ObjectID,
		Status:     ObjectReconcileAmbiguousResolved,
		Confidence: picked.Score,
		Method:     "tie_break_deterministic",
	}
	var itemID int64
	if reconciler.Attention != nil {
		rec, err := reconciler.Attention.Record(ctx, ambiguousObjectTieReport(obj, tied, picked, llmOutcome))
		if err != nil {
			if logger != nil {
				logger.Error("object reconciliation: recording attention item failed (ChenWeb#18)",
					"input_record_id", obj.InputRecordID,
					"artifact_type", obj.ArtifactType,
					"artifact_id", obj.ArtifactID,
					"object_name", obj.ObjectName,
					"picked_object_id", picked.Node.ObjectID,
					"error", err,
				)
			}
		} else {
			itemID = rec.ID
		}
	}
	if logger != nil {
		logger.Info("object reconciliation tie broken",
			"input_record_id", obj.InputRecordID,
			"artifact_type", obj.ArtifactType,
			"artifact_id", obj.ArtifactID,
			"object_name", obj.ObjectName,
			"top_score", picked.Score,
			"candidates", objectReconcileCandidateDisplay(tied),
			"picked_object_id", picked.Node.ObjectID,
			"llm_outcome", llmOutcome,
			"attention_item_id", itemID,
		)
	}
	return result
}

// FindTieOccurrenceRow returns the id of the current kb.artifact_objects row
// for the first occurrence, in the given order, that still has one. The
// second result is false when none does (every source record was deleted or
// re-extracted without that object).
func (s ArtifactObjectSQLStore) FindTieOccurrenceRow(ctx context.Context, occurrences []TieOccurrence) (int64, bool, error) {
	if s.DB == nil {
		return 0, false, fmt.Errorf("db is nil")
	}
	for _, o := range occurrences {
		var id int64
		err := s.DB.QueryRowContext(ctx, `
SELECT id FROM kb.artifact_objects
WHERE input_record_id = $1 AND artifact_type = $2 AND artifact_id = $3
  AND (object_name = $4 OR object_name_en = $4 OR object_name_zh = $4)
ORDER BY id DESC
LIMIT 1`, o.InputRecordID, o.ArtifactType, o.ArtifactID, o.ObjectName).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, false, err
		}
		return id, true, nil
	}
	return 0, false, nil
}
