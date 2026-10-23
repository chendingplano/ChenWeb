package meshbrowser

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// Record is the assembled search result view. Entity remains the unmodified RDF record.
type Record struct {
	Entity     *Entity         `json:"entity"`
	Release    string          `json:"release"`
	Concepts   []RecordConcept `json:"concepts"`
	Qualifiers []EntityRow     `json:"qualifiers"`
	Trees      []RecordTree    `json:"trees"`
}

type RecordConcept struct {
	Entity    *Entity   `json:"entity"`
	Preferred bool      `json:"preferred"`
	Terms     []*Entity `json:"terms"`
}

type RecordTreeNode struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	RecordID string `json:"record_id,omitempty"`
}

type RecordTree struct {
	ID       string           `json:"id"`
	Path     []RecordTreeNode `json:"path"`
	Children []RecordTreeNode `json:"children"`
}

func (h *Handler) Record(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "20261023-728")
	defer rc.Close()
	id := c.QueryParam("id")
	if !validID(id) {
		return badRequest(c, rc, "id must be a MeSH record id")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 20*time.Second)
	defer cancel()
	record, err := h.s().Record(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, map[string]any{"message": "mesh record not found"})
	}
	if err != nil {
		return fail(c, rc, "mesh record failed", err)
	}
	return c.JSON(http.StatusOK, record)
}

func values(e *Entity, predicates ...string) []Value {
	result := []Value{}
	for _, predicate := range predicates {
		for _, p := range e.Properties {
			if p.Predicate == predicate {
				result = append(result, p.Values...)
			}
		}
	}
	return result
}

func (s *Store) Record(ctx context.Context, id string) (*Record, error) {
	e, err := s.Entity(ctx, id)
	if err != nil {
		return nil, err
	}
	r := &Record{Entity: e, Concepts: []RecordConcept{}, Qualifiers: []EntityRow{}, Trees: []RecordTree{}}
	err = s.DB.QueryRowContext(ctx, `SELECT release FROM resources.mesh_import ORDER BY imported_at DESC LIMIT 1`).Scan(&r.Release)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	seen := map[string]bool{}
	preferred := values(e, "preferredConcept")
	conceptLinks := values(e, "preferredConcept", "concept")
	if e.Kind == "Concept" {
		conceptLinks = []Value{{ID: e.ID, Label: e.Label}}
		preferred = conceptLinks
	}
	for _, v := range conceptLinks {
		if v.ID == "" || seen[v.ID] {
			continue
		}
		seen[v.ID] = true
		concept := e
		if v.ID != e.ID {
			concept, err = s.Entity(ctx, v.ID)
			if err != nil {
				return nil, err
			}
		}
		c := RecordConcept{Entity: concept, Terms: []*Entity{}}
		for _, p := range preferred {
			if p.ID == v.ID {
				c.Preferred = true
			}
		}
		terms := map[string]bool{}
		for _, term := range values(concept, "preferredTerm", "term") {
			if term.ID == "" || terms[term.ID] {
				continue
			}
			terms[term.ID] = true
			t, err := s.Entity(ctx, term.ID)
			if err != nil {
				return nil, err
			}
			c.Terms = append(c.Terms, t)
		}
		r.Concepts = append(r.Concepts, c)
	}
	for _, v := range values(e, "allowableQualifier") {
		if v.ID != "" {
			r.Qualifiers = append(r.Qualifiers, EntityRow{ID: v.ID, Label: v.Label})
		}
	}
	sort.Slice(r.Qualifiers, func(i, j int) bool { return r.Qualifiers[i].Label < r.Qualifiers[j].Label })
	for _, v := range values(e, "treeNumber") {
		if v.ID == "" {
			continue
		}
		tree := RecordTree{ID: v.ID, Path: []RecordTreeNode{}, Children: []RecordTreeNode{}}
		if category, ok := topCategory(v.ID[:1]); ok {
			tree.Path = append(tree.Path, RecordTreeNode{ID: category.ID, Label: category.Label})
		}
		parts := strings.Split(v.ID, ".")
		for i := range parts {
			node, err := s.recordTreeNode(ctx, strings.Join(parts[:i+1], "."))
			if err != nil {
				return nil, err
			}
			tree.Path = append(tree.Path, node)
		}
		kids, err := s.Children(ctx, v.ID)
		if err != nil {
			return nil, err
		}
		for _, child := range kids {
			node, err := s.recordTreeNode(ctx, child.ID)
			if err != nil {
				return nil, err
			}
			tree.Children = append(tree.Children, node)
		}
		r.Trees = append(r.Trees, tree)
	}
	return r, nil
}

func (s *Store) recordTreeNode(ctx context.Context, id string) (RecordTreeNode, error) {
	var node RecordTreeNode
	err := s.DB.QueryRowContext(ctx, `SELECT e.id, `+treeLabel+`, COALESCE((
		SELECT l.subject_id FROM resources.mesh_link l WHERE l.predicate='treeNumber' AND l.object_id=e.id ORDER BY l.subject_id LIMIT 1), '')
		FROM resources.mesh_entity e WHERE e.id=$1`, id).Scan(&node.ID, &node.Label, &node.RecordID)
	return node, err
}
