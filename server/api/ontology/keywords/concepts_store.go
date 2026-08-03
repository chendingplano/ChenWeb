package keywords

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DBX is the subset of database/sql that keyword stores need. Both
// *sql.DB and *sql.Tx satisfy it.
type DBX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Concept is one keyword concept row. ConceptID is opaque and immutable;
// PrefLabel is mutable display. Concepts are ungoverned — status transitions
// are linear (active → provisional → merged → deprecated) with no draft/review.
type Concept struct {
	ConceptID   string    `json:"concept_id"`
	PrefLabel   string    `json:"pref_label"`
	Gloss       *string   `json:"gloss"`
	Scope       string    `json:"scope"`
	Status      string    `json:"status"`
	MergedInto  *string   `json:"merged_into"`
	GlossSource string    `json:"gloss_source"`
	CreateTime  time.Time `json:"create_time"`
	ModifyTime  time.Time `json:"modify_time"`
}

// AllowedConceptStatuses is the ungoverned keyword-concept lifecycle.
var AllowedConceptStatuses = map[string]bool{
	"active":      true,
	"provisional": true,
	"merged":      true,
	"deprecated":  true,
}

var errIllegalConceptTransition = errors.New("illegal concept status transition")

// conceptStatusTransitions lists allowed status changes for keyword concepts.
// Ungoverned: provisional can go back to active; merged sets merged_into.
var conceptStatusTransitions = map[string]map[string]bool{
	"active":      {"provisional": true, "merged": true, "deprecated": true},
	"provisional": {"active": true, "merged": true, "deprecated": true},
	"merged":      {},
	"deprecated":  {},
}

// ConceptStore persists keyword concept rows.
type ConceptStore struct {
	DB DBX
}

const conceptColumns = `concept_id, pref_label, gloss, scope, status, merged_into, gloss_source, create_time, modify_time`

const conceptFrom = `FROM kb.keyword_concepts`

func scanConcept(scan func(dest ...any) error) (Concept, error) {
	var (
		c          Concept
		gloss      sql.NullString
		mergedInto sql.NullString
	)
	if err := scan(
		&c.ConceptID, &c.PrefLabel, &gloss, &c.Scope, &c.Status,
		&mergedInto, &c.GlossSource, &c.CreateTime, &c.ModifyTime,
	); err != nil {
		return Concept{}, err
	}
	if gloss.Valid {
		c.Gloss = &gloss.String
	}
	if mergedInto.Valid {
		v := mergedInto.String
		c.MergedInto = &v
	}
	return c, nil
}

func validateConcept(c Concept) error {
	if c.ConceptID == "" {
		return fmt.Errorf("concept_id is required")
	}
	if c.PrefLabel == "" {
		return fmt.Errorf("pref_label is required")
	}
	if c.Scope == "" {
		return fmt.Errorf("scope is required")
	}
	if !AllowedConceptStatuses[c.Status] {
		return fmt.Errorf("invalid status: %s", c.Status)
	}
	return nil
}

// CreateConcept inserts a new keyword concept.
func (s ConceptStore) CreateConcept(ctx context.Context, c Concept) (Concept, error) {
	if c.Status == "" {
		c.Status = "active"
	}
	if c.Scope == "" {
		c.Scope = "_"
	}
	if c.GlossSource == "" {
		c.GlossSource = "none"
	}
	if err := validateConcept(c); err != nil {
		return Concept{}, err
	}
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO kb.keyword_concepts (concept_id, pref_label, gloss, scope, status, gloss_source)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+conceptColumns,
		c.ConceptID, c.PrefLabel, nullableString(func() string {
			if c.Gloss != nil {
				return *c.Gloss
			}
			return ""
		}()), c.Scope, c.Status, c.GlossSource,
	)
	return scanConcept(row.Scan)
}

// GetConcept retrieves a concept by its immutable concept_id.
func (s ConceptStore) GetConcept(ctx context.Context, conceptID string) (Concept, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT `+conceptColumns+`
		`+conceptFrom+`
		WHERE concept_id = $1`, conceptID)
	return scanConcept(row.Scan)
}

// ListConcepts returns concepts, optionally filtered by scope. When scope is
// empty, all active and provisional concepts are returned.
func (s ConceptStore) ListConcepts(ctx context.Context, scope string) ([]Concept, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if scope == "" {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT `+conceptColumns+`
			`+conceptFrom+`
			WHERE status IN ('active', 'provisional')
			ORDER BY concept_id`)
	} else {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT `+conceptColumns+`
			`+conceptFrom+`
			WHERE scope = $1 AND status IN ('active', 'provisional')
			ORDER BY concept_id`, scope)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Concept
	for rows.Next() {
		c, err := scanConcept(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateConceptLabel updates the display attributes of a concept.
func (s ConceptStore) UpdateConceptLabel(ctx context.Context, conceptID, prefLabel, gloss string) (Concept, error) {
	if prefLabel == "" {
		return Concept{}, fmt.Errorf("pref_label is required")
	}
	row := s.DB.QueryRowContext(ctx, `
		UPDATE kb.keyword_concepts
		SET pref_label = $2, gloss = $3, modify_time = NOW()
		WHERE concept_id = $1
		RETURNING `+conceptColumns,
		conceptID, prefLabel, nullableString(gloss))
	return scanConcept(row.Scan)
}

// TransitionStatus enforces the concept status state machine.
func (s ConceptStore) TransitionStatus(ctx context.Context, conceptID, to string) (Concept, error) {
	if !AllowedConceptStatuses[to] {
		return Concept{}, fmt.Errorf("invalid status: %s", to)
	}

	current, err := s.GetConcept(ctx, conceptID)
	if err != nil {
		return Concept{}, err
	}

	if to == current.Status {
		return current, nil
	}

	allowed, ok := conceptStatusTransitions[current.Status]
	if !ok || !allowed[to] {
		return Concept{}, fmt.Errorf("%w: %s → %s", errIllegalConceptTransition, current.Status, to)
	}

	row := s.DB.QueryRowContext(ctx, `
		UPDATE kb.keyword_concepts
		SET status = $2, modify_time = NOW()
		WHERE concept_id = $1
		RETURNING `+conceptColumns,
		conceptID, to)
	return scanConcept(row.Scan)
}

// MergeConcept sets the from concept's merged_into to the to concept and
// transitions from to 'merged'. The to concept is returned.
func (s ConceptStore) MergeConcept(ctx context.Context, fromID, toID string) (Concept, error) {
	if fromID == toID {
		return Concept{}, fmt.Errorf("cannot merge a concept into itself")
	}

	// Verify the target exists.
	if _, err := s.GetConcept(ctx, toID); err != nil {
		return Concept{}, fmt.Errorf("target concept %s: %w", toID, err)
	}

	if _, err := s.DB.ExecContext(ctx, `
		UPDATE kb.keyword_concepts
		SET status = 'merged', merged_into = $2, modify_time = NOW()
		WHERE concept_id = $1`,
		fromID, toID); err != nil {
		return Concept{}, fmt.Errorf("merge concept %s → %s: %w", fromID, toID, err)
	}

	return s.GetConcept(ctx, toID)
}
