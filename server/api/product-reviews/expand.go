package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/lib/pq"
)

// structuralPredicates are the accepted assertion predicates the expansion pass
// walks (spec: Graph expansion pass).
var structuralPredicates = []string{"core:part_of", "core:component_of"}

// productPartRelations are the kb.products relation types that name a
// part/containment link.
var productPartRelations = []string{"component_of", "contains_product"}

// Expander pulls in real parts the model never named, from the accepted
// structural graph and from kb.products containment rows. New nodes carry
// origin `graph_expanded` and a reference to the edge that introduced them.
type Expander struct {
	Store   Store
	DB      *sql.DB
	Budgets BudgetsConfig
}

type expandItem struct {
	nodeID   int64
	objectID string
	depth    int
}

// Expand traverses outward from every grounded node, bounded by the depth and
// node budgets. It bumps the profile version only if it inserted a node.
func (e Expander) Expand(ctx context.Context, profileID int64) error {
	budgets := e.Budgets.withDefaults()

	nodes, err := e.Store.loadScanned(ctx, e.DB, profileID)
	if err != nil {
		return err
	}
	nodeByObject := map[string]int64{}
	for _, n := range nodes {
		if n.ObjectID != "" {
			nodeByObject[n.ObjectID] = n.ID
		}
	}
	nodeCount := len(nodes)

	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var queue []expandItem
	for _, n := range nodes {
		if n.ObjectID != "" && n.Status != StatusRejected {
			queue = append(queue, expandItem{nodeID: n.ID, objectID: n.ObjectID, depth: n.Depth})
		}
	}
	visited := map[string]bool{} // objectID already expanded from
	inserted := 0

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur.objectID] || cur.depth >= budgets.MaxDepth {
			continue
		}
		visited[cur.objectID] = true

		neighbors, err := e.neighbors(ctx, tx, cur.objectID)
		if err != nil {
			return err
		}
		for _, nb := range neighbors {
			if existingID, ok := nodeByObject[nb.objectID]; ok {
				if err := appendSourceRef(ctx, tx, existingID, nb.ref); err != nil {
					return err
				}
				continue
			}
			if nodeCount >= budgets.MaxNodes {
				continue
			}
			refs, _ := json.Marshal([]SourceRef{nb.ref})
			var newID int64
			err := tx.QueryRowContext(ctx, `
				INSERT INTO kb.product_profile_nodes
					(profile_id, parent_node_id, node_kind, label, label_en, depth,
					 origin, status, object_id, grounding, reconcile_status, source_refs)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
				RETURNING id`,
				profileID, cur.nodeID, KindPart, nb.label, nb.labelEN, cur.depth+1,
				OriginGraphExpanded, StatusProposed, nb.objectID, GroundingObjectNode,
				nb.reconcile, refs,
			).Scan(&newID)
			if err != nil {
				return err
			}
			nodeByObject[nb.objectID] = newID
			nodeCount++
			inserted++
			queue = append(queue, expandItem{nodeID: newID, objectID: nb.objectID, depth: cur.depth + 1})
		}
	}

	if inserted > 0 {
		if err := bumpVersion(ctx, tx, profileID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type neighbor struct {
	objectID  string
	label     string
	labelEN   string
	reconcile string
	ref       SourceRef
}

// neighbors returns the objects linked to objectID by an accepted structural
// assertion or a kb.products containment row.
func (e Expander) neighbors(ctx context.Context, q queryer, objectID string) ([]neighbor, error) {
	out := make([]neighbor, 0, 8)
	seen := map[string]bool{objectID: true}

	rows, err := q.QueryContext(ctx, `
		SELECT sa.id, sa.predicate_term_id,
		       CASE WHEN sa.subject_object_id = $1 THEN sa.object_object_id
		            ELSE sa.subject_object_id END AS other_id,
		       COALESCE(o.canonical_name, ''), COALESCE(o.canonical_name_en, ''),
		       COALESCE(o.reconcile_status, '')
		FROM kb.semantic_assertions sa
		JOIN kb.object_nodes o ON o.object_id = CASE
		        WHEN sa.subject_object_id = $1 THEN sa.object_object_id
		        ELSE sa.subject_object_id END
		WHERE sa.status = 'accepted'
		  AND sa.predicate_term_id = ANY($2)
		  AND (sa.subject_object_id = $1 OR sa.object_object_id = $1)
		  AND o.reconcile_status NOT IN ('rejected', 'merged')`,
		objectID, pq.Array(structuralPredicates))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			assertionID    int64
			predicate, oid string
			name, nameEN   string
			reconcile      string
		)
		if err := rows.Scan(&assertionID, &predicate, &oid, &name, &nameEN, &reconcile); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if oid == "" || seen[oid] {
			continue
		}
		seen[oid] = true
		out = append(out, neighbor{
			objectID: oid, label: name, labelEN: fallback(nameEN, name), reconcile: reconcile,
			ref: SourceRef{Kind: "assertion", ID: assertionID, Detail: predicate},
		})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	prodRows, err := q.QueryContext(ctx, `
		SELECT id, relation_type, COALESCE(related_products, '[]'::jsonb)
		FROM kb.products
		WHERE relation_type = ANY($2)
		  AND status = 'active'
		  AND lower(canonical_name) = ANY($1)`,
		pq.Array(e.objectNameKeys(ctx, objectID)), pq.Array(productPartRelations))
	if err != nil {
		return nil, err
	}
	defer func() { _ = prodRows.Close() }()
	for prodRows.Next() {
		var (
			prodID       int64
			relationType string
			relatedRaw   []byte
		)
		if err := prodRows.Scan(&prodID, &relationType, &relatedRaw); err != nil {
			return nil, err
		}
		var related []struct {
			Name   string `json:"name"`
			NameEN string `json:"name_en"`
		}
		_ = json.Unmarshal(relatedRaw, &related)
		for _, rp := range related {
			label := strings.TrimSpace(rp.Name)
			if label == "" || seen[normalizeName(label)] {
				continue
			}
			seen[normalizeName(label)] = true
			out = append(out, neighbor{
				objectID: "", label: label, labelEN: fallback(strings.TrimSpace(rp.NameEN), label),
				ref: SourceRef{Kind: "product", ID: prodID, Detail: relationType},
			})
		}
	}
	return out, prodRows.Err()
}

// objectNameKeys returns the lower-cased canonical names of an object, used to
// match kb.products rows to a grounded node.
func (e Expander) objectNameKeys(ctx context.Context, objectID string) []string {
	var name, nameEN string
	err := e.DB.QueryRowContext(ctx, `
		SELECT COALESCE(canonical_name, ''), COALESCE(canonical_name_en, '')
		FROM kb.object_nodes WHERE object_id = $1`, objectID).Scan(&name, &nameEN)
	if err != nil {
		return nil
	}
	return dedupeStrings([]string{strings.ToLower(name), strings.ToLower(nameEN)})
}

func appendSourceRef(ctx context.Context, q queryer, nodeID int64, ref SourceRef) error {
	raw, _ := json.Marshal([]SourceRef{ref})
	_, err := q.ExecContext(ctx, `
		UPDATE kb.product_profile_nodes
		SET source_refs = COALESCE(source_refs, '[]'::jsonb) || $2::jsonb, updated_at = NOW()
		WHERE id = $1`, nodeID, raw)
	return err
}

func fallback(v, alt string) string {
	if strings.TrimSpace(v) == "" {
		return alt
	}
	return v
}
