package productreviews

import (
	"context"
	"database/sql"
	"strings"

	"github.com/lib/pq"
)

// EmbedFunc embeds text for the semantic half of grounding. ok=false tells the
// grounder to fall back to lexical matching for that node.
type EmbedFunc func(ctx context.Context, text string) (vec []float64, ok bool)

// Grounder resolves each proposed node against the governed identity spaces
// (spec: Grounding pass). It never mutates the node set, so it does not bump the
// profile version — a run's node set stays stable across a re-ground.
type Grounder struct {
	Store           Store
	DB              *sql.DB
	Embed           EmbedFunc
	SemanticEnabled bool
	Thresholds      GroundingConfig
}

// Ground attempts to ground every not-yet-grounded, non-aspect, non-rejected
// node of the profile. Nodes that match nothing keep grounding `ungrounded`.
func (g Grounder) Ground(ctx context.Context, profileID int64) error {
	nodes, err := g.Store.loadScanned(ctx, g.DB, profileID)
	if err != nil {
		return err
	}
	semantic := g.SemanticEnabled && g.Embed != nil

	for _, n := range nodes {
		if n.NodeKind == KindAspect || n.Status == StatusRejected {
			continue
		}
		if n.Grounding != "" && n.Grounding != GroundingUngrounded {
			continue
		}

		var labelVec []float64
		if semantic && !n.HasEmbedding {
			if vec, ok := g.Embed(ctx, n.Label); ok {
				labelVec = vec
				if err := g.setEmbedding(ctx, n.ID, vec); err != nil {
					return err
				}
			}
		}

		if objectID, reconcile, ok, err := g.groundToObject(ctx, n.ProfileNode, labelVec, semantic); err != nil {
			return err
		} else if ok {
			if err := g.apply(ctx, n.ID, GroundingObjectNode, "object_id", objectID, reconcile); err != nil {
				return err
			}
			continue
		}
		if conceptID, ok, err := g.groundToConcept(ctx, n.Label); err != nil {
			return err
		} else if ok {
			if err := g.apply(ctx, n.ID, GroundingKeywordConcept, "concept_id", conceptID, ""); err != nil {
				return err
			}
			continue
		}
		if termID, ok, err := g.groundToTerm(ctx, n.Label); err != nil {
			return err
		} else if ok {
			if err := g.apply(ctx, n.ID, GroundingOntologyTerm, "term_id", termID, ""); err != nil {
				return err
			}
			continue
		}
		// no match — node stays `ungrounded` (its default).
	}
	return nil
}

// groundToObject tries an exact/alias match on kb.object_nodes, then (when
// semantic search is on and a label vector is available) an embedding match at
// or above the configured threshold.
func (g Grounder) groundToObject(ctx context.Context, n ProfileNode, labelVec []float64, semantic bool) (objectID, reconcile string, ok bool, err error) {
	names := dedupeStrings(append([]string{n.Label, n.LabelEN}, n.Aliases...))
	normed := make([]string, 0, len(names))
	for _, nm := range names {
		if v := normalizeName(nm); v != "" {
			normed = append(normed, v)
		}
	}
	if len(normed) > 0 {
		err = g.DB.QueryRowContext(ctx, `
			SELECT object_id, COALESCE(reconcile_status, '')
			FROM kb.object_nodes
			WHERE reconcile_status NOT IN ('rejected', 'merged')
			  AND (normalized_names ?| $1 OR lower(canonical_name) = $2
			       OR lower(canonical_name_en) = $2 OR lower(canonical_name_zh) = $2)
			ORDER BY id
			LIMIT 1`, pq.Array(normed), strings.ToLower(strings.TrimSpace(n.Label)),
		).Scan(&objectID, &reconcile)
		if err == nil {
			return objectID, reconcile, true, nil
		}
		if err != sql.ErrNoRows {
			return "", "", false, err
		}
	}

	if !semantic || len(labelVec) == 0 {
		return "", "", false, nil
	}
	var sim float64
	err = g.DB.QueryRowContext(ctx, `
		SELECT object_id, COALESCE(reconcile_status, ''),
		       1 - (embedding <=> $1::vector) AS sim
		FROM kb.object_nodes
		WHERE embedding IS NOT NULL
		  AND reconcile_status NOT IN ('rejected', 'merged')
		ORDER BY embedding <=> $1::vector
		LIMIT 1`, formatVector(labelVec),
	).Scan(&objectID, &reconcile, &sim)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if sim < g.Thresholds.ObjectEmbeddingMin {
		return "", "", false, nil
	}
	return objectID, reconcile, true, nil
}

// groundToConcept matches a kb.keyword_concepts row in scope `metric_subject`
// by exact (case-insensitive) pref_label.
func (g Grounder) groundToConcept(ctx context.Context, label string) (conceptID string, ok bool, err error) {
	err = g.DB.QueryRowContext(ctx, `
		SELECT concept_id FROM kb.keyword_concepts
		WHERE scope = 'metric_subject' AND status = 'active'
		  AND lower(pref_label) = lower($1)
		ORDER BY concept_id
		LIMIT 1`, strings.TrimSpace(label)).Scan(&conceptID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return conceptID, true, nil
}

// groundToTerm matches a kb.ontology_term_labels label (any non-rejected role).
func (g Grounder) groundToTerm(ctx context.Context, label string) (termID string, ok bool, err error) {
	err = g.DB.QueryRowContext(ctx, `
		SELECT term_id FROM kb.ontology_term_labels
		WHERE status <> 'rejected' AND lower(label) = lower($1)
		ORDER BY term_id
		LIMIT 1`, strings.TrimSpace(label)).Scan(&termID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return termID, true, nil
}

// apply writes a node's grounding result. reconcile is stored verbatim so the UI
// can flag an ambiguous / pending-review object.
func (g Grounder) apply(ctx context.Context, nodeID int64, grounding, refCol, refID, reconcile string) error {
	_, err := g.DB.ExecContext(ctx, `
		UPDATE kb.product_profile_nodes
		SET grounding = $2, `+refCol+` = $3, reconcile_status = $4, updated_at = NOW()
		WHERE id = $1`, nodeID, grounding, refID, reconcile)
	return err
}

func (g Grounder) setEmbedding(ctx context.Context, nodeID int64, vec []float64) error {
	_, err := g.DB.ExecContext(ctx, `
		UPDATE kb.product_profile_nodes
		SET embedding = $2::vector, updated_at = NOW()
		WHERE id = $1 AND embedding IS NULL`, nodeID, formatVector(vec))
	return err
}
