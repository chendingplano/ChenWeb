package keywords

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
)

var errBoom = errors.New("boom")

// TestSynthesizeClassNoConceptCreatesTermDirectly covers metric-class-synthesis-seam's
// "Metric occurrence with no resolved concept" scenario: no ConceptID means
// no transaction is opened (no ExpectBegin/ExpectCommit expected) and no
// alignment assertion is written, only the term itself.
func TestSynthesizeClassNoConceptCreatesTermDirectly(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const candidateTermID = "measurement:auto:defname_abc123"
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs(
			candidateTermID, "metric_definition", "measurement", "auto-promoted",
			nil, "document-derived, auto-promoted (ADR 2026081201)", nil,
			nil, nil, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "properties",
			"create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(300, candidateTermID, 1, "metric_definition", "measurement", "auto-promoted",
			"", "document-derived, auto-promoted (ADR 2026081201)", nil, nil, now, nil, now, nil))

	termID, created, err := synthesizeClass(context.Background(), db, candidateTermID, assertions.ClassSynthesisInput{
		CanonicalName: "No Concept Metric",
	})
	if err != nil {
		t.Fatalf("synthesizeClass: %v", err)
	}
	if termID != candidateTermID {
		t.Errorf("termID = %q, want %q", termID, candidateTermID)
	}
	if !created {
		t.Error("expected created = true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet (no alignment write expected): %v", err)
	}
}

// TestSynthesizeClassWithConceptReusesExisting covers "class synthesis
// preserves the keyword-concept alignment link" for an already-aligned
// concept: it returns the existing term without writing a new one, and --
// unlike EnsureAcceptedOrCreate -- opens no transaction of its own.
func TestSynthesizeClassWithConceptReusesExisting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(testConceptID).
		WillReturnRows(alignmentReadRow(1, testConceptID, testTermID, []byte(testQualifiers), testScore))

	termID, created, err := synthesizeClass(context.Background(), db, "measurement:auto:unused", assertions.ClassSynthesisInput{
		ConceptID:     testConceptID,
		CanonicalName: "should not be used",
	})
	if err != nil {
		t.Fatalf("synthesizeClass: %v", err)
	}
	if termID != testTermID {
		t.Errorf("termID = %q, want reused %q", termID, testTermID)
	}
	if created {
		t.Error("expected created = false for a reused term")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet (no term/label/alignment write expected): %v", err)
	}
}

// TestSynthesizeClassWithConceptCreatesTermThenDelegatesAlignment covers the
// create branch's synthesizeClass-specific glue: a concept with no existing
// alignment gets a new term and labels created directly against the
// caller-supplied db, with no transaction opened by synthesizeClass itself
// (the caller -- metric_lossless_writer.go -- owns that transaction; unlike
// EnsureAcceptedOrCreate, there is no ExpectBegin/ExpectCommit here). The
// remaining step -- writing the core:aligns_to_term alignment assertion --
// is ensureAccepted's own already-tested logic
// (TestAlignmentsStoreEnsureAcceptedOrCreateAutoCreatesTerm exercises it in
// full); this test stops at the conflict-gate query synthesizeClass reaches
// next, confirming the handoff happens rather than re-verifying that
// existing assertion-write mechanics.
func TestSynthesizeClassWithConceptCreatesTermThenDelegatesAlignment(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const (
		conceptID = "concept_new"
		newTermID = "measurement:concept_new"
	)
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(conceptID).
		WillReturnRows(noAlignmentRow())
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs(
			newTermID, "metric_definition", "measurement", "auto-promoted",
			"def text", "document-derived, auto-promoted (ADR 2026081201)", nil,
			nil, nil, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "properties",
			"create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(101, newTermID, 1, "metric_definition", "measurement", "auto-promoted",
			"def text", "document-derived, auto-promoted (ADR 2026081201)", nil, nil, now, nil, now, nil))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
		WithArgs(newTermID, "en").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs(newTermID, "Test Metric", "en", "prefLabel", "auto-promoted", nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "label", "lang", "label_role", "status",
			"source_candidate_id", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(201, newTermID, 1, "Test Metric", "en", "prefLabel", "auto-promoted", nil, now, nil, now, nil))
	// synthesizeClass hands off to ensureAccepted here (its own conflict-gate
	// read); returning an error is enough to prove the handoff happened,
	// without re-mocking ensureAccepted's already-tested write sequence.
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(conceptID).
		WillReturnError(errBoom)

	_, _, err = synthesizeClass(context.Background(), db, newTermID, assertions.ClassSynthesisInput{
		ConceptID:     conceptID,
		CanonicalName: "Test Metric",
		Definition:    "def text",
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("synthesizeClass error = %v, want errBoom from the alignment handoff", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
