// Package meshbrowser serves read-only views over the normalized MeSH tables in the
// resources schema (mesh_entity, mesh_property, mesh_link), loaded by scripts/mesh-import.sql.
package meshbrowser

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Store runs the queries.
type Store struct {
	DB *sql.DB
}

type Overview struct {
	Release string      `json:"release"`
	Total   int64       `json:"source_triples"`
	Kinds   []KindCount `json:"kinds"`
}

type KindCount struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
}

type EntityRow struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Value is one object of a property. Links to other MeSH records carry ID and Label
// so the page can navigate to them; literals carry Text and optionally Lang.
type Value struct {
	Text  string `json:"text,omitempty"`
	Lang  string `json:"lang,omitempty"`
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
}

type Property struct {
	Predicate string  `json:"predicate"`
	Values    []Value `json:"values"`
}

type Entity struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Label      string      `json:"label"`
	Properties []Property  `json:"properties"`
	Filed      []EntityRow `json:"filed,omitempty"`
}

// Overview returns the release and the number of records per kind.
func (s *Store) Overview(ctx context.Context) (*Overview, error) {
	ov := &Overview{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT release, source_rows FROM resources.mesh_import ORDER BY imported_at DESC LIMIT 1`).
		Scan(&ov.Release, &ov.Total)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("read import info: %w", err)
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT kind, count(*) FROM resources.mesh_entity GROUP BY kind ORDER BY count(*) DESC`)
	if err != nil {
		return nil, fmt.Errorf("count kinds: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kc KindCount
		if err := rows.Scan(&kc.Kind, &kc.Count); err != nil {
			return nil, err
		}
		ov.Kinds = append(ov.Kinds, kc)
	}
	return ov, rows.Err()
}

// ListKind returns one page of records of a kind, optionally filtered by label substring.
func (s *Store) ListKind(ctx context.Context, kind, query string, limit, offset int) ([]EntityRow, error) {
	pattern := "%" + likeEscaper.Replace(query) + "%"
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, label FROM resources.mesh_entity
		WHERE kind = $1 AND ($2 = '' OR label ILIKE $3)
		ORDER BY label, id
		LIMIT $4 OFFSET $5`, kind, query, pattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", kind, err)
	}
	return scanRows(rows)
}

// topCategories are the 16 top-level MeSH categories. The RDF release has no tree
// number for them (its shallowest tree numbers are A01, A02, ...), so they are
// defined here and sit above the tree numbers that start with their letter.
var topCategories = []EntityRow{
	{ID: "A", Label: "A — Anatomy"},
	{ID: "B", Label: "B — Organisms"},
	{ID: "C", Label: "C — Diseases"},
	{ID: "D", Label: "D — Chemicals and Drugs"},
	{ID: "E", Label: "E — Analytical, Diagnostic and Therapeutic Techniques, and Equipment"},
	{ID: "F", Label: "F — Psychiatry and Psychology"},
	{ID: "G", Label: "G — Phenomena and Processes"},
	{ID: "H", Label: "H — Disciplines and Occupations"},
	{ID: "I", Label: "I — Anthropology, Education, Sociology, and Social Phenomena"},
	{ID: "J", Label: "J — Technology, Industry, and Agriculture"},
	{ID: "K", Label: "K — Humanities"},
	{ID: "L", Label: "L — Information Science"},
	{ID: "M", Label: "M — Named Groups"},
	{ID: "N", Label: "N — Health Care"},
	{ID: "V", Label: "V — Publication Characteristics"},
	{ID: "Z", Label: "Z — Geographicals"},
}

// topCategory returns the top-level category with the given letter.
func topCategory(id string) (EntityRow, bool) {
	for _, c := range topCategories {
		if c.ID == id {
			return c, true
		}
	}
	return EntityRow{}, false
}

// treeLabel selects the label of tree number e: the name of the descriptor filed under it
// (Body Regions for A01), since a tree number carries only its code as a label.
const treeLabel = `COALESCE((
	SELECT d.label FROM resources.mesh_link tl
	JOIN resources.mesh_entity d ON d.id = tl.subject_id
	WHERE tl.predicate = 'treeNumber' AND tl.object_id = e.id
	ORDER BY d.id LIMIT 1), e.label)`

// Children returns the tree numbers directly below parent, or the 16 top-level
// categories when parent is empty. A category's children are the tree numbers
// without a dot that start with its letter (A01, A02, ...).
func (s *Store) Children(ctx context.Context, parent string) ([]EntityRow, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if parent == "" {
		return append([]EntityRow(nil), topCategories...), nil
	}
	if _, ok := topCategory(parent); ok {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT e.id, `+treeLabel+`
			FROM resources.mesh_entity e
			WHERE e.kind = 'TreeNumber' AND position('.' in e.id) = 0 AND e.id LIKE $1
			ORDER BY e.id`, parent+"%")
	} else {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT e.id, `+treeLabel+`
			FROM resources.mesh_link l
			JOIN resources.mesh_entity e ON e.id = l.subject_id
			WHERE l.predicate = 'parentTreeNumber' AND l.object_id = $1
			ORDER BY e.id`, parent)
	}
	if err != nil {
		return nil, fmt.Errorf("list tree children: %w", err)
	}
	return scanRows(rows)
}

// Entity loads one record with its attributes and links. For a tree number it also
// lists the descriptors filed directly under it.
func (s *Store) Entity(ctx context.Context, id string) (*Entity, error) {
	if c, ok := topCategory(id); ok {
		return &Entity{ID: c.ID, Kind: "TreeNumber", Label: c.Label}, nil
	}
	ent := &Entity{}
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, kind, label FROM resources.mesh_entity WHERE id = $1`, id).
		Scan(&ent.ID, &ent.Kind, &ent.Label)
	if err != nil {
		return nil, err
	}

	byPred := map[string][]Value{}
	propRows, err := s.DB.QueryContext(ctx,
		`SELECT predicate, value, lang FROM resources.mesh_property WHERE entity_id = $1 ORDER BY predicate, value`, id)
	if err != nil {
		return nil, fmt.Errorf("load properties: %w", err)
	}
	defer propRows.Close()
	for propRows.Next() {
		var pred string
		var v Value
		if err := propRows.Scan(&pred, &v.Text, &v.Lang); err != nil {
			return nil, err
		}
		byPred[pred] = append(byPred[pred], v)
	}
	if err := propRows.Err(); err != nil {
		return nil, err
	}

	linkRows, err := s.DB.QueryContext(ctx, `
		SELECT l.predicate, l.object_id, COALESCE(o.label, '')
		FROM resources.mesh_link l
		LEFT JOIN resources.mesh_entity o ON o.id = l.object_id
		WHERE l.subject_id = $1
		ORDER BY l.predicate, o.label, l.object_id`, id)
	if err != nil {
		return nil, fmt.Errorf("load links: %w", err)
	}
	defer linkRows.Close()
	for linkRows.Next() {
		var pred string
		var v Value
		if err := linkRows.Scan(&pred, &v.ID, &v.Label); err != nil {
			return nil, err
		}
		byPred[pred] = append(byPred[pred], v)
	}
	if err := linkRows.Err(); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(byPred))
	for name := range byPred {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		ent.Properties = append(ent.Properties, Property{Predicate: name, Values: byPred[name]})
	}

	if ent.Kind == "TreeNumber" {
		filed, err := s.filedUnder(ctx, id)
		if err != nil {
			return nil, err
		}
		ent.Filed = filed
	}
	return ent, nil
}

func (s *Store) filedUnder(ctx context.Context, treeID string) ([]EntityRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT e.id, e.label
		FROM resources.mesh_link l
		JOIN resources.mesh_entity e ON e.id = l.subject_id
		WHERE l.predicate = 'treeNumber' AND l.object_id = $1
		ORDER BY e.label
		LIMIT 200`, treeID)
	if err != nil {
		return nil, fmt.Errorf("list filed descriptors: %w", err)
	}
	return scanRows(rows)
}

func scanRows(rows *sql.Rows) ([]EntityRow, error) {
	defer rows.Close()
	var out []EntityRow
	for rows.Next() {
		var r EntityRow
		if err := rows.Scan(&r.ID, &r.Label); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Node is a tree number for the tree chart: the chart root and its child tree numbers. The
// children are capped so large branches stay readable; the cut-off count is reported in Hidden.
// ChildCount is the number of tree numbers directly below the node, whether or not they are
// loaded, so the chart can offer to expand it.
type Node struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	ChildCount int     `json:"child_count,omitempty"`
	Hidden     int     `json:"hidden,omitempty"`
	Children   []*Node `json:"children,omitempty"`
}

const maxChildren = 80

// Neighborhood returns the tree number root and one level of children below it.
func (s *Store) Neighborhood(ctx context.Context, root string) (*Node, error) {
	node := &Node{ID: root}
	if c, ok := topCategory(root); ok {
		node.Label = c.Label
	} else if err := s.DB.QueryRowContext(ctx,
		`SELECT `+treeLabel+` FROM resources.mesh_entity e WHERE e.id = $1`, root).Scan(&node.Label); err != nil {
		return nil, err
	}

	kids, err := s.Children(ctx, root)
	if err != nil {
		return nil, err
	}
	node.ChildCount = len(kids)
	if len(kids) > maxChildren {
		node.Hidden = len(kids) - maxChildren
		kids = kids[:maxChildren]
	}
	counts, err := s.childCounts(ctx, kids)
	if err != nil {
		return nil, err
	}
	for _, k := range kids {
		node.Children = append(node.Children, &Node{ID: k.ID, Label: k.Label, ChildCount: counts[k.ID]})
	}
	return node, nil
}

// childCounts returns, for each tree number, how many tree numbers are directly below it.
func (s *Store) childCounts(ctx context.Context, nodes []EntityRow) (map[string]int, error) {
	out := map[string]int{}
	if len(nodes) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(nodes))
	holders := make([]string, 0, len(nodes))
	for _, n := range nodes {
		args = append(args, n.ID)
		holders = append(holders, fmt.Sprintf("$%d", len(args)))
	}
	rows, err := s.DB.QueryContext(ctx, fmt.Sprintf(`
		SELECT object_id, count(*) FROM resources.mesh_link
		WHERE predicate = 'parentTreeNumber' AND object_id IN (%s)
		GROUP BY object_id`, strings.Join(holders, ",")), args...)
	if err != nil {
		return nil, fmt.Errorf("count children of %d nodes: %w", len(nodes), err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
