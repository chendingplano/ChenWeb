package assertions

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Task 5.7 reader compatibility certification: classification_projection.go
// had zero test coverage before this. primaryClassificationFor is the
// accepted-only source of truth ProjectSemantics.Run rebuilds
// kb.object_nodes.primary_class_term_id from (DR8 Phase D, "semantic
// projection" in consumer-lifecycle-policy.md) -- a represented or
// unsupported core:instance_of assertion must never become an object's
// primary class.
func TestPrimaryClassificationForFiltersToAcceptedStatusOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT object_ref_id, id, revision
FROM kb.semantic_assertions
WHERE subject_object_id = $1
  AND predicate_term_id = 'core:instance_of'
  AND object_ref_kind = 'ontology_term'
  AND status = 'accepted'
ORDER BY id ASC
LIMIT 1`)).
		WithArgs("obj-1").
		WillReturnRows(sqlmock.NewRows([]string{"object_ref_id", "id", "revision"}).
			AddRow("core:device", int64(5), 2))

	termID, assertionID, revision, found, err := primaryClassificationFor(context.Background(), db, "obj-1")
	if err != nil {
		t.Fatalf("primaryClassificationFor: %v", err)
	}
	if !found || termID != "core:device" || assertionID != 5 || revision != 2 {
		t.Fatalf("primaryClassificationFor = (%q, %d, %d, %v), want (core:device, 5, 2, true)", termID, assertionID, revision, found)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("query no longer filters to accepted-only: %v", err)
	}
}

// Task 5.7: an object whose only core:instance_of assertion is represented
// (not yet accepted) must report not-found, not a false classification --
// this is what keeps a raw-preserved/represented claim from being treated as
// this consumer's endorsed truth.
func TestPrimaryClassificationForReportsNotFoundWhenOnlyNonAcceptedExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(`WHERE subject_object_id = $1
  AND predicate_term_id = 'core:instance_of'
  AND object_ref_kind = 'ontology_term'
  AND status = 'accepted'`)).
		WithArgs("obj-2").
		WillReturnRows(sqlmock.NewRows([]string{"object_ref_id", "id", "revision"}))

	_, _, _, found, err := primaryClassificationFor(context.Background(), db, "obj-2")
	if err != nil {
		t.Fatalf("primaryClassificationFor: %v", err)
	}
	if found {
		t.Fatal("a represented-only core:instance_of assertion must not resolve to a primary classification")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
