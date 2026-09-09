package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// ScopeNode is an accepted profile node flattened for retrieval: its identity
// keys (for lexical / product-record matching), its concept id (for direct
// concept-equality matching), its stored label embedding (for the vector half
// of hybrid search), and — for aspects — the relation types it maps to.
type ScopeNode struct {
	ID            int64
	Kind          string
	Label         string
	LabelEN       string
	AspectKey     string
	RelationTypes []string
	ConceptID     string
	ObjectID      string
	Grounding     string
	NameKeys      []string  // normalized identity strings
	Embedding     []float64 // nil when the node has no stored embedding
}

// labelText joins the node's human labels for a lexical query.
func (n ScopeNode) labelText() string {
	return strings.TrimSpace(strings.Join(dedupeStrings([]string{n.Label, n.LabelEN}), " "))
}

func (n ScopeNode) isAspect() bool  { return n.Kind == KindAspect }
func (n ScopeNode) isProduct() bool { return n.Kind == KindProduct }

// LoadScopeNodes loads a profile's accepted nodes and resolves each grounded
// node's object names into its identity key set.
func LoadScopeNodes(ctx context.Context, db *sql.DB, profileID int64) ([]ScopeNode, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, node_kind, label, label_en, COALESCE(aliases, '[]'::jsonb),
		       COALESCE(object_id, ''), COALESCE(concept_id, ''), grounding,
		       aspect_key, COALESCE(relation_types, '[]'::jsonb),
		       COALESCE(embedding::text, '')
		FROM kb.product_profile_nodes
		WHERE profile_id = $1 AND status = 'accepted'
		ORDER BY depth, id`, profileID)
	if err != nil {
		return nil, err
	}
	var nodes []ScopeNode
	objectIDs := map[string]bool{}
	func() {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var (
				n                  ScopeNode
				aliasesRaw, relRaw []byte
				embText            string
			)
			if err = rows.Scan(&n.ID, &n.Kind, &n.Label, &n.LabelEN, &aliasesRaw,
				&n.ObjectID, &n.ConceptID, &n.Grounding, &n.AspectKey, &relRaw, &embText); err != nil {
				return
			}
			var aliases []string
			_ = json.Unmarshal(aliasesRaw, &aliases)
			_ = json.Unmarshal(relRaw, &n.RelationTypes)
			n.NameKeys = normalizedKeys(append([]string{n.Label, n.LabelEN}, aliases...))
			n.Embedding = parseVectorText(embText)
			if n.ObjectID != "" {
				objectIDs[n.ObjectID] = true
			}
			nodes = append(nodes, n)
		}
		err = rows.Err()
	}()
	if err != nil {
		return nil, err
	}

	if len(objectIDs) > 0 {
		names, err := loadObjectNameKeys(ctx, db, keysOf(objectIDs))
		if err != nil {
			return nil, err
		}
		for i := range nodes {
			if extra, ok := names[nodes[i].ObjectID]; ok {
				nodes[i].NameKeys = dedupeStrings(append(nodes[i].NameKeys, extra...))
			}
		}
	}
	return nodes, nil
}

// RootNodeID returns the id of the `product` root, or 0.
func RootNodeID(nodes []ScopeNode) int64 {
	for _, n := range nodes {
		if n.isProduct() {
			return n.ID
		}
	}
	return 0
}

func loadObjectNameKeys(ctx context.Context, db *sql.DB, objectIDs []string) (map[string][]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT object_id, COALESCE(canonical_name, ''), COALESCE(canonical_name_en, ''),
		       COALESCE(normalized_names, '[]'::jsonb)
		FROM kb.object_nodes WHERE object_id = ANY($1)`, pq.Array(objectIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]string{}
	for rows.Next() {
		var (
			objectID, name, nameEN string
			normRaw                []byte
		)
		if err := rows.Scan(&objectID, &name, &nameEN, &normRaw); err != nil {
			return nil, err
		}
		var norm []string
		_ = json.Unmarshal(normRaw, &norm)
		out[objectID] = normalizedKeys(append([]string{name, nameEN}, norm...))
	}
	return out, rows.Err()
}

// normalizedKeys normalizes each input and returns the unique non-empty set.
func normalizedKeys(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		k := normalizeName(s)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// parseVectorText parses a pgvector `::text` literal ("[1,2,3]") to a float
// slice. Empty / malformed → nil.
func parseVectorText(s string) []float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil
		}
		out = append(out, f)
	}
	return out
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
