package productreviews

import (
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// rx quotes a literal SQL fragment for sqlmock's regexp query matcher, so a
// distinctive substring can be matched without hand-escaping every paren.
func rx(s string) string { return regexp.QuoteMeta(s) }

func newMockStore(t *testing.T) (Store, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	return Store{DB: db}, mock, func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sqlmock expectations: %v", err)
		}
		_ = db.Close()
	}
}

// profileRows builds the RETURNING / SELECT row set for a kb.product_profiles row.
func profileRows(id int64, name string, version int, status string, truncated bool, truncatedCount int) *sqlmock.Rows {
	return profileRowsWithDrawing(id, name, version, status, truncated, truncatedCount, nil)
}

// profileRowsWithDrawing is profileRows plus an explicit drawing_id (nil = no
// drawing associated yet — spec: product-review-results-layout).
func profileRowsWithDrawing(id int64, name string, version int, status string, truncated bool, truncatedCount int, drawingID *int64) *sqlmock.Rows {
	var drawing any
	if drawingID != nil {
		drawing = *drawingID
	}
	return sqlmock.NewRows([]string{
		"id", "tenant_id", "name", "name_cn", "name_en", "product_description", "keywords", "notes", "version", "status",
		"truncated", "truncated_count", "drawing_id", "created_at", "updated_at",
	}).AddRow(id, "-", name, name, "", "", []byte("[]"), "", version, status, truncated, truncatedCount, drawing, time.Now(), time.Now())
}

// nodeCols is the column order of the nodeColumns SELECT list.
var nodeCols = []string{
	"id", "profile_id", "parent_node_id", "node_kind", "label", "label_en",
	"aliases", "depth", "origin", "status", "confidence", "rationale",
	"object_id", "concept_id", "term_id", "grounding", "reconcile_status",
	"aspect_key", "relation_types", "match_mode", "source_refs", "has_embedding",
}

type nodeRow struct {
	ID           int64
	ParentID     *int64
	Kind         string
	Label        string
	Depth        int
	Origin       string
	Status       string
	ObjectID     string
	ConceptID    string
	TermID       string
	Grounding    string
	AspectKey    string
	HasEmbedding bool
}

func (r nodeRow) values(profileID int64) []driver.Value {
	var parent any
	if r.ParentID != nil {
		parent = *r.ParentID
	}
	grounding := r.Grounding
	if grounding == "" {
		grounding = GroundingUngrounded
	}
	kind := r.Kind
	if kind == "" {
		kind = KindPart
	}
	status := r.Status
	if status == "" {
		status = StatusProposed
	}
	origin := r.Origin
	if origin == "" {
		origin = OriginLLMProposed
	}
	return []driver.Value{
		r.ID, profileID, parent, kind, r.Label, r.Label,
		[]byte("[]"), r.Depth, origin, status, 0.0, "",
		r.ObjectID, r.ConceptID, r.TermID, grounding, "",
		r.AspectKey, []byte("[]"), "", []byte("[]"), r.HasEmbedding,
	}
}

func nodeRowsFrom(profileID int64, rows ...nodeRow) *sqlmock.Rows {
	out := sqlmock.NewRows(nodeCols)
	for _, r := range rows {
		out.AddRow(r.values(profileID)...)
	}
	return out
}
