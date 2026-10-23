package meshbrowser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type SearchOptions struct {
	Query, Field, Match, Words, Sort string
	Limit, Offset                    int
}

type SearchRow struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

func searchOptions(c echo.Context) (SearchOptions, error) {
	o := SearchOptions{Query: strings.TrimSpace(c.QueryParam("q")), Field: "all_terms", Match: "all", Words: "full", Sort: "label"}
	if o.Query == "" || utf8.RuneCountInString(o.Query) > 300 {
		return o, errors.New("query must contain 1 to 300 characters")
	}
	for _, p := range []struct {
		key     string
		dst     *string
		allowed string
	}{
		{"field", &o.Field, "all_terms descriptor qualifier scr id scope_note annotation registry pharmacological_action heading_mapped_to free_text"},
		{"match", &o.Match, "exact all any"},
		{"words", &o.Words, "full substring"},
		{"sort", &o.Sort, "label label_desc id"},
	} {
		if v := c.QueryParam(p.key); v != "" {
			if !slices.Contains(strings.Fields(p.allowed), v) {
				return o, fmt.Errorf("invalid %s", p.key)
			}
			*p.dst = v
		}
	}
	var err error
	o.Limit, o.Offset, err = paging(c)
	if len(strings.Fields(o.Query)) > 20 {
		return o, errors.New("query must contain at most 20 words")
	}
	return o, err
}

func (h *Handler) Search(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "20261023-487")
	defer rc.Close()
	o, err := searchOptions(c)
	if err != nil {
		return badRequest(c, rc, err.Error())
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 20*time.Second)
	defer cancel()
	rows, err := h.s().Search(ctx, o)
	if err != nil {
		return fail(c, rc, "mesh search failed", err)
	}
	hasMore := len(rows) > o.Limit
	if hasMore {
		rows = rows[:o.Limit]
	}
	return c.JSON(http.StatusOK, map[string]any{"items": rows, "has_more": hasMore})
}

// Search returns main records, including hits on their concept/entry-term labels.
// Vocabulary values remain source text; interface labels are translated by the client.
func (s *Store) Search(ctx context.Context, o SearchOptions) ([]SearchRow, error) {
	query, args := searchSQL(o)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search MeSH: %w", err)
	}
	defer rows.Close()
	out := []SearchRow{}
	for rows.Next() {
		var row SearchRow
		if err := rows.Scan(&row.ID, &row.Label, &row.Kind); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// SQL fragments below are fixed allowlisted expressions. User input is always a parameter.
func searchSQL(o SearchOptions) (string, []any) {
	kinds := `e.kind IN ('TopicalDescriptor','GeographicalDescriptor','PublicationType','Qualifier','SCR_Chemical','SCR_Organism','SCR_Disease','SCR_Protocol','SCR_Population','SCR_Anatomy','CheckTag')`
	switch o.Field {
	case "descriptor":
		kinds = `e.kind IN ('TopicalDescriptor','GeographicalDescriptor','PublicationType')`
	case "qualifier":
		kinds = `e.kind = 'Qualifier'`
	case "scr":
		kinds = `e.kind LIKE 'SCR\_%'`
	}
	parts := strings.Fields(o.Query)
	if o.Match == "exact" {
		parts = []string{o.Query}
	}
	args := []any{}
	conditions := []string{}
	ctes := []string{}
	for _, part := range parts {
		var pattern string
		op := "ILIKE"
		if o.Match == "exact" {
			op, pattern = "=", strings.ToLower(part)
		} else if o.Words == "substring" {
			pattern = "%" + likeEscaper.Replace(part) + "%"
		} else {
			op, pattern = "~*", `(^|[^[:alnum:]_])`+regexp.QuoteMeta(part)+`($|[^[:alnum:]_])`
		}
		args = append(args, pattern)
		match := func(expr string) string {
			if o.Match == "exact" {
				expr = "lower(" + expr + ")"
			}
			return fmt.Sprintf("%s %s $%d", expr, op, len(args))
		}
		if o.Field == "id" {
			conditions = append(conditions, match("e.id"))
			continue
		}
		name := fmt.Sprintf("hits%d", len(args))
		selectHits := `SELECT v.id FROM resources.mesh_entity v WHERE ` + match("v.label")
		propertyHits := func(predicates string) string {
			return `SELECT p.entity_id AS id FROM resources.mesh_property p WHERE ` + predicates + ` AND ` + match("p.value")
		}
		switch o.Field {
		case "scope_note":
			selectHits = propertyHits(`p.predicate IN ('scopeNote','note')`)
		case "annotation":
			selectHits = propertyHits(`p.predicate = 'annotation'`)
		case "registry":
			selectHits = propertyHits(`p.predicate IN ('registryNumber','relatedRegistryNumber','casn1_label')`)
		case "free_text":
			selectHits += " UNION " + propertyHits("TRUE")
		}
		ctes = append(ctes, name+` AS MATERIALIZED (`+selectHits+`)`)
		if o.Field == "pharmacological_action" || o.Field == "heading_mapped_to" {
			predicates := "'pharmacologicalAction'"
			if o.Field == "heading_mapped_to" {
				predicates = "'preferredMappedTo','mappedTo'"
			}
			// These fields also accept the target record's unique ID.
			ctes[len(ctes)-1] = name + ` AS MATERIALIZED (` + selectHits + ` OR ` + match("v.id") + `)`
			conditions = append(conditions, `e.id IN (SELECT l.subject_id FROM resources.mesh_link l JOIN `+name+` h ON h.id=l.object_id WHERE l.predicate IN (`+predicates+`))`)
			continue
		}
		// Walk backwards from matching entry terms to concepts and main records.
		// Matching source text once avoids a correlated scan for every main record.
		conditions = append(conditions, `e.id IN (
			SELECT id FROM `+name+`
			UNION SELECT l.subject_id FROM resources.mesh_link l JOIN `+name+` h ON h.id=l.object_id
			WHERE l.predicate IN ('concept','preferredConcept','term','preferredTerm')
			UNION SELECT c.subject_id FROM resources.mesh_link t JOIN `+name+` h ON h.id=t.object_id
			JOIN resources.mesh_link c ON c.object_id=t.subject_id
			WHERE t.predicate IN ('term','preferredTerm') AND c.predicate IN ('concept','preferredConcept'))`)

	}
	join := " AND "
	if o.Match == "any" {
		join = " OR "
	}
	order := "e.label, e.id"
	if o.Sort == "label_desc" {
		order = "e.label DESC, e.id"
	} else if o.Sort == "id" {
		order = "e.id"
	}
	prefix := ""
	if len(ctes) > 0 {
		prefix = "WITH " + strings.Join(ctes, ", ") + " "
	}
	query := prefix + `SELECT e.id, e.label, e.kind FROM resources.mesh_entity e WHERE ` + kinds + ` AND (` + strings.Join(conditions, join) + `) ORDER BY ` + order
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, o.Limit+1, o.Offset)
	return query, args
}
