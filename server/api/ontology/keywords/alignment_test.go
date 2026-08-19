package keywords

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

const (
	testConceptID   = "concept_a"
	testTermID      = "mea:Luminance"
	testOtherTermID = "mea:Radiance"
	testMethod      = "term_exact"
	testEvidence    = "pref_label exact match"
	testLK          = "kwc:concept_a:core:aligns_to_term:mea:Luminance"
)

var testScore = 1.0

// testQualifiers is the exact JSON the implementation writes (map keys are
// sorted by encoding/json: evidence < method) and the value every mocked
// RETURNING/read row returns.
const testQualifiers = `{"evidence":"pref_label exact match","method":"term_exact"}`

// alignmentAssertionRow is a full 37-column kb.semantic_assertions row as
// returned by CreateAssertion/CreateRevision/GetLatest, carrying an accepted
// keyword_concept -> ontology_term alignment.
func alignmentAssertionRow(id int64, lk, conceptID, termID, evidence string, qualifiers []byte, confidence float64) *sqlmock.Rows {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "logical_identity_key", "revision", "subject_ref_kind", "subject_ref_id",
		"subject_object_id", "predicate_term_id", "object_ref_kind", "object_ref_id",
		"object_object_id", "object_literal", "assertion_kind_term_id", "polarity",
		"modality", "qualifiers", "confidence", "value_form", "numeric_value", "lower_value",
		"upper_value", "lower_inclusive", "upper_inclusive", "comparator", "unit_term_id",
		"quantity_kind_term_id", "raw_text", "status", "decision_reason",
		"dependency_fingerprint", "superseded_by", "valid_time_start", "valid_time_end",
		"transaction_time", "create_time", "create_by", "modify_time", "modify_by",
	}).AddRow(
		id, lk, 1, "keyword_concept", conceptID,
		nil, "core:aligns_to_term", "ontology_term", termID,
		nil, []byte("null"), nil, "positive",
		nil, qualifiers, confidence, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, "", "accepted", evidence,
		nil, nil, nil, nil,
		now, now, nil, now, nil,
	)
}

// alignmentReadRow is the focused AcceptedForConcept row (8 columns).
func alignmentReadRow(id int64, conceptID, termID string, qualifiers []byte, confidence float64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "subject_ref_id", "object_ref_kind", "object_ref_id", "status", "qualifiers", "confidence", "decision_reason",
	}).AddRow(id, conceptID, "ontology_term", termID, "accepted", qualifiers, confidence, testEvidence)
}

func noAlignmentRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "subject_ref_id", "object_ref_kind", "object_ref_id", "status", "qualifiers", "confidence", "decision_reason",
	})
}

// getLatestByLK is the GetLatest probe (substring of AssertionStore.GetLatest's
// query text).
const getLatestByLK = "FROM kb.semantic_assertions\nWHERE logical_identity_key = $1"

// TestAlignmentsStoreEnsureAccepted verifies the observe-path producer: a full
// write (conflict check -> released guard -> identity-key idempotency ->
// CreateAssertion -> decision-log audit), then a no-op on the same concept+term
// (the reads rerun, but no second INSERT and no second decision-log row).
func TestAlignmentsStoreEnsureAccepted(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	}
	ctx := context.Background()

	// --- First call: full write path ---
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	// 1. Conflict gate: no existing accepted alignment.
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(testConceptID).
		WillReturnRows(noAlignmentRow())
	// 2. Released guard: the object term is a released metric_definition and
	//    the predicate a released property.
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).
		WithArgs(testTermID, "metric_definition").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).
		WithArgs(alignPredicateTermID, "property").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// 3. Idempotency: no prior revision for this identity key.
	mock.ExpectQuery(regexp.QuoteMeta(getLatestByLK)).
		WithArgs(testLK).
		WillReturnError(sql.ErrNoRows)
	// 4. CreateAssertion INSERT...RETURNING: pin the whole written row.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_assertions")).
		WithArgs(
			testLK, "keyword_concept", testConceptID, nil,
			alignPredicateTermID, "ontology_term", testTermID, nil, "null", nil,
			"positive", nil, testQualifiers, testScore, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, "accepted", nil, nil, nil, nil,
		).
		WillReturnRows(alignmentAssertionRow(1, testLK, testConceptID, testTermID, testEvidence, []byte(testQualifiers), testScore))
	// 5. DR15 audit row (observe path only).
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semid_decision_log")).
		WithArgs("keyword_align", "_", `{"concept_id":"concept_a","term_id":"mea:Luminance"}`, sqlmock.AnyArg(), "accepted", nil, nil, "auto-align", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectCommit()

	al, err := store.EnsureAccepted(ctx, testConceptID, testTermID, testMethod, testScore, testEvidence)
	if err != nil {
		t.Fatalf("EnsureAccepted (first): %v", err)
	}
	if al.ID != 1 || al.ConceptID != testConceptID || al.ObjectTermID != testTermID {
		t.Errorf("unexpected alignment: %+v", al)
	}
	if al.Method != testMethod {
		t.Errorf("expected method %q, got %q", testMethod, al.Method)
	}
	if al.Score == nil || *al.Score != testScore {
		t.Errorf("expected score %v, got %v", testScore, al.Score)
	}
	if al.Evidence != testEvidence {
		t.Errorf("expected evidence %q, got %q", testEvidence, al.Evidence)
	}

	// --- Second call (same concept, same term): a no-op ---
	// The conflict check, the released guard, and GetLatest rerun, but no
	// further INSERT / decision-log expectations are set, so ExpectationsWereMet
	// fails the test if a second write were attempted.
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(testConceptID).
		WillReturnRows(alignmentReadRow(1, testConceptID, testTermID, []byte(testQualifiers), testScore))
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).
		WithArgs(testTermID, "metric_definition").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).
		WithArgs(alignPredicateTermID, "property").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(getLatestByLK)).
		WithArgs(testLK).
		WillReturnRows(alignmentAssertionRow(1, testLK, testConceptID, testTermID, testEvidence, []byte(testQualifiers), testScore))
	mock.ExpectCommit()

	al2, err := store.EnsureAccepted(ctx, testConceptID, testTermID, testMethod, testScore, testEvidence)
	if err != nil {
		t.Fatalf("EnsureAccepted (no-op): %v", err)
	}
	if al2.ObjectTermID != testTermID || al2.Method != testMethod || al2.Evidence != testEvidence {
		t.Errorf("unexpected no-op alignment: %+v", al2)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestAlignmentsStoreConflict verifies spec §14.2: once a concept is aligned to
// a term, aligning it to a *different* term is refused before any write.
func TestAlignmentsStoreConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	}
	ctx := context.Background()

	// First: align to termA (full write path).
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(testConceptID).
		WillReturnRows(noAlignmentRow())
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).
		WithArgs(testTermID, "metric_definition").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).
		WithArgs(alignPredicateTermID, "property").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(getLatestByLK)).
		WithArgs(testLK).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_assertions")).
		WithArgs(
			testLK, "keyword_concept", testConceptID, nil,
			alignPredicateTermID, "ontology_term", testTermID, nil, "null", nil,
			"positive", nil, testQualifiers, testScore, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, "accepted", nil, nil, nil, nil,
		).
		WillReturnRows(alignmentAssertionRow(1, testLK, testConceptID, testTermID, testEvidence, []byte(testQualifiers), testScore))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semid_decision_log")).
		WithArgs("keyword_align", "_", `{"concept_id":"concept_a","term_id":"mea:Luminance"}`, sqlmock.AnyArg(), "accepted", nil, nil, "auto-align", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectCommit()

	if _, err := store.EnsureAccepted(ctx, testConceptID, testTermID, testMethod, testScore, testEvidence); err != nil {
		t.Fatalf("first EnsureAccepted: %v", err)
	}

	// Second: the same concept to a *different* term is refused at the conflict
	// gate; nothing else runs (no more expectations).
	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(testConceptID).
		WillReturnRows(alignmentReadRow(1, testConceptID, testTermID, []byte(testQualifiers), testScore))
	mock.ExpectRollback()

	_, err = store.EnsureAccepted(ctx, testConceptID, testOtherTermID, testMethod, testScore, testEvidence)
	if !errors.Is(err, ErrAlignmentConflict) {
		t.Fatalf("expected ErrAlignmentConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestAlignmentsStoreAcceptedForConcept verifies the projection read: an
// accepted alignment for a concept, and nil when it has none.
func TestAlignmentsStoreAcceptedForConcept(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := AlignmentsStore{}
	ctx := context.Background()

	t.Run("returns accepted alignment", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
			WithArgs(testConceptID).
			WillReturnRows(alignmentReadRow(1, testConceptID, testTermID, []byte(testQualifiers), testScore))

		al, err := store.AcceptedForConcept(ctx, db, testConceptID)
		if err != nil {
			t.Fatalf("AcceptedForConcept: %v", err)
		}
		if al == nil {
			t.Fatal("expected an alignment")
		}
		if al.ID != 1 || al.ConceptID != testConceptID || al.ObjectTermID != testTermID {
			t.Errorf("unexpected alignment: %+v", al)
		}
		if al.Method != testMethod || al.Evidence != testEvidence {
			t.Errorf("unexpected method/evidence: %+v", al)
		}
		if al.Score == nil || *al.Score != testScore {
			t.Errorf("expected score %v, got %v", testScore, al.Score)
		}
	})

	t.Run("none when concept is unaligned", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
			WithArgs("concept_ghost").
			WillReturnRows(noAlignmentRow())

		al, err := store.AcceptedForConcept(ctx, db, "concept_ghost")
		if err != nil {
			t.Fatalf("AcceptedForConcept: %v", err)
		}
		if al != nil {
			t.Errorf("expected nil alignment, got %+v", al)
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestAlignmentsStoreFollowMerge issues the accepted-alignment re-point on a
// caller-owned transaction (Task 4's MergeConcept depends on this): the single
// UPDATE runs on the *sql.Tx, so it is atomic with the tombstone.
func TestAlignmentsStoreFollowMerge(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(followMergeSQL)).
		WithArgs("kwc_absorbed", "kwc_survivor").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := (AlignmentsStore{}).FollowMerge(ctx, tx, "kwc_absorbed", "kwc_survivor"); err != nil {
		t.Fatalf("FollowMerge: %v", err)
	}

	mock.ExpectCommit()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestAlignmentsStoreEnsureAcceptedRollsBackAssertionWhenAuditFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock(1264011588, 1);")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).WithArgs(testConceptID).WillReturnRows(noAlignmentRow())
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).WithArgs(testTermID, "metric_definition").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).WithArgs(alignPredicateTermID, "property").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(getLatestByLK)).WithArgs(testLK).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_assertions")).
		WillReturnRows(alignmentAssertionRow(1, testLK, testConceptID, testTermID, testEvidence, []byte(testQualifiers), testScore))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semid_decision_log")).
		WillReturnError(errors.New("audit unavailable"))
	mock.ExpectRollback()

	if _, err := store.EnsureAccepted(context.Background(), testConceptID, testTermID, testMethod, testScore, testEvidence); err == nil {
		t.Fatal("EnsureAccepted succeeded despite failed audit")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestAlignmentsStoreMergeConflict verifies spec §14.2's conflict rule: true
// only when both concepts have accepted alignments to *different* terms.
func TestAlignmentsStoreMergeConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := AlignmentsStore{}
	ctx := context.Background()

	t.Run("both aligned to different terms is a conflict", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
			WithArgs("concept_a").
			WillReturnRows(alignmentReadRow(1, "concept_a", testTermID, []byte(testQualifiers), testScore))
		mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
			WithArgs("concept_b").
			WillReturnRows(alignmentReadRow(2, "concept_b", testOtherTermID, []byte(testQualifiers), testScore))

		conflict, err := store.MergeConflict(ctx, db, "concept_a", "concept_b")
		if err != nil {
			t.Fatalf("MergeConflict: %v", err)
		}
		if !conflict {
			t.Error("expected conflict when both concepts align to different terms")
		}
	})

	t.Run("unaligned participant is not a conflict", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
			WithArgs("concept_a").
			WillReturnRows(alignmentReadRow(1, "concept_a", testTermID, []byte(testQualifiers), testScore))
		mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
			WithArgs("concept_b").
			WillReturnRows(noAlignmentRow())

		conflict, err := store.MergeConflict(ctx, db, "concept_a", "concept_b")
		if err != nil {
			t.Fatalf("MergeConflict: %v", err)
		}
		if conflict {
			t.Error("expected no conflict when one concept is unaligned")
		}
	})

	t.Run("both aligned to the same term is not a conflict", func(t *testing.T) {
		mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
			WithArgs("concept_a").
			WillReturnRows(alignmentReadRow(1, "concept_a", testTermID, []byte(testQualifiers), testScore))
		mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
			WithArgs("concept_b").
			WillReturnRows(alignmentReadRow(2, "concept_b", testTermID, []byte(testQualifiers), testScore))

		conflict, err := store.MergeConflict(ctx, db, "concept_a", "concept_b")
		if err != nil {
			t.Fatalf("MergeConflict: %v", err)
		}
		if conflict {
			t.Error("expected no conflict when both concepts align to the same term")
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestAlignmentsStoreEnsureAcceptedOrCreateReusesExisting verifies ADR
// 2026081201 DR1's first branch: a concept that already has an accepted
// alignment is returned as-is -- no term is created, no label is written.
// ExpectationsWereMet fails the test if EnsureAcceptedOrCreate attempted any
// of that unnecessary work.
func TestAlignmentsStoreEnsureAcceptedOrCreateReusesExisting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	}

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(testConceptID).
		WillReturnRows(alignmentReadRow(1, testConceptID, testTermID, []byte(testQualifiers), testScore))
	mock.ExpectCommit()

	al, err := store.EnsureAcceptedOrCreate(context.Background(), testConceptID,
		TermSynthesisInput{CanonicalName: "should not be used"}, testMethod, testScore, testEvidence)
	if err != nil {
		t.Fatalf("EnsureAcceptedOrCreate: %v", err)
	}
	if al.ObjectTermID != testTermID {
		t.Errorf("expected the existing term %q reused, got %q", testTermID, al.ObjectTermID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet (no term/label write expected): %v", err)
	}
}

// TestAlignmentsStoreEnsureAcceptedOrCreateAutoCreatesTerm verifies ADR
// 2026081201 DR1's create branch end to end: no existing alignment -> a new
// auto-promoted term is inserted (DR2/DR3 fields), a prefLabel and one
// altLabel are written (the alias equal to the canonical name is skipped),
// then the same EnsureAccepted write sequence aligns the concept to it.
func TestAlignmentsStoreEnsureAcceptedOrCreateAutoCreatesTerm(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := AlignmentsStore{
		Assertions:  assertions.AssertionStore{DB: db},
		DecisionLog: semid.DecisionLogStore{DB: db},
		Scope:       "_",
	}

	const (
		conceptID = "concept_new"
		newTermID = "measurement:concept_new"
		newLK     = "kwc:concept_new:core:aligns_to_term:measurement:concept_new"
	)
	now := time.Now()
	permittedUnitsJSON := []byte(`["quantity:unit_x"]`)

	mock.ExpectBegin()
	expectKeywordIdentityLock(mock)

	// D1 step 1: no existing alignment for this concept.
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(conceptID).
		WillReturnRows(noAlignmentRow())

	// D1 step 2 / DR2 / DR3: create the term.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs(
			newTermID, "metric_definition", "measurement", "auto-promoted",
			"def text", "document-derived, auto-promoted (ADR 2026081201)", nil,
			"number", "exact", permittedUnitsJSON, nil, nil,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "term_kind", "module_id", "status",
			"definition", "scope", "source_candidate_id", "value_type", "range_type",
			"permitted_unit_term_ids", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(100, newTermID, 1, "metric_definition", "measurement", "auto-promoted",
			"def text", "document-derived, auto-promoted (ADR 2026081201)", nil,
			"number", "exact", permittedUnitsJSON, now, nil, now, nil))

	// prefLabel: existence check, then insert.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
		WithArgs(newTermID, "en").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs(newTermID, "Test Metric", "en", "prefLabel", "auto-promoted", nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "label", "lang", "label_role", "status",
			"source_candidate_id", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(200, newTermID, 1, "Test Metric", "en", "prefLabel", "auto-promoted", nil, now, nil, now, nil))

	// altLabel: "TM" only -- the "Test Metric" alias equals the canonical
	// name and must be skipped, and altLabel never checks prefLabelExists.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs(newTermID, "TM", "en", "altLabel", "auto-promoted", nil, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "version", "label", "lang", "label_role", "status",
			"source_candidate_id", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(201, newTermID, 1, "TM", "en", "altLabel", "auto-promoted", nil, now, nil, now, nil))

	// ensureAccepted's own sequence: conflict gate, released guard x2,
	// idempotency check, the assertion write, the audit row.
	mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
		WithArgs(conceptID).
		WillReturnRows(noAlignmentRow())
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).
		WithArgs(newTermID, "metric_definition").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(releasedTermExistsSQL)).
		WithArgs(alignPredicateTermID, "property").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(getLatestByLK)).
		WithArgs(newLK).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semantic_assertions")).
		WithArgs(
			newLK, "keyword_concept", conceptID, nil,
			alignPredicateTermID, "ontology_term", newTermID, nil, "null", nil,
			"positive", nil, testQualifiers, testScore, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, "accepted", nil, nil, nil, nil,
		).
		WillReturnRows(alignmentAssertionRow(2, newLK, conceptID, newTermID, testEvidence, []byte(testQualifiers), testScore))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semid_decision_log")).
		WithArgs("keyword_align", "_", `{"concept_id":"concept_new","term_id":"measurement:concept_new"}`,
			sqlmock.AnyArg(), "accepted", nil, nil, "auto-align", 0).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectCommit()

	al, err := store.EnsureAcceptedOrCreate(context.Background(), conceptID,
		TermSynthesisInput{
			CanonicalName:        "Test Metric",
			Aliases:              []string{"TM", "Test Metric"},
			Definition:           "def text",
			ValueType:            "number",
			RangeType:            "exact",
			PermittedUnitTermIDs: []string{"quantity:unit_x"},
		}, testMethod, testScore, testEvidence)
	if err != nil {
		t.Fatalf("EnsureAcceptedOrCreate: %v", err)
	}
	if al.ObjectTermID != newTermID {
		t.Errorf("expected new term %q, got %q", newTermID, al.ObjectTermID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestAlignmentsStoreEnsureAcceptedOrCreateAssignsLabelLanguages(t *testing.T) {
	tests := []struct {
		name          string
		canonicalName string
		alias         string
		canonicalLang string
		aliasLang     string
	}{
		{
			name:          "Chinese preferred and alias",
			canonicalName: "显示亮度",
			alias:         "屏幕亮度",
			canonicalLang: "zh",
			aliasLang:     "zh",
		},
		{
			name:          "English preferred and alternate",
			canonicalName: "Display luminance",
			alias:         "Screen brightness",
			canonicalLang: "en",
			aliasLang:     "en",
		},
		{
			name:          "Chinese preferred and Latin alternate",
			canonicalName: "显示亮度",
			alias:         "Display luminance",
			canonicalLang: "zh",
			aliasLang:     "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			const conceptID = "concept_language"
			termID := autoPromotedTermID(conceptID)
			stop := errors.New("stop after label writes")
			store := AlignmentsStore{
				Assertions:  assertions.AssertionStore{DB: db},
				DecisionLog: semid.DecisionLogStore{DB: db},
				Scope:       "_",
			}

			mock.ExpectBegin()
			expectKeywordIdentityLock(mock)
			mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
				WithArgs(conceptID).
				WillReturnRows(noAlignmentRow())
			mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
				WithArgs(
					termID, "metric_definition", "measurement", "auto-promoted",
					"definition", "document-derived, auto-promoted (ADR 2026081201)", nil,
					"number", "exact", nil, nil, nil,
				).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "term_id", "version", "term_kind", "module_id", "status",
					"definition", "scope", "source_candidate_id", "value_type", "range_type",
					"permitted_unit_term_ids", "create_time", "create_by", "modify_time", "modify_by",
				}).AddRow(100, termID, 1, "metric_definition", "measurement", "auto-promoted",
					"definition", "document-derived, auto-promoted (ADR 2026081201)", nil,
					"number", "exact", nil, time.Now(), nil, time.Now(), nil))
			mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS")).
				WithArgs(termID, tt.canonicalLang).
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
				WithArgs(termID, tt.canonicalName, tt.canonicalLang, "prefLabel", "auto-promoted", nil, nil, nil).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "term_id", "version", "label", "lang", "label_role", "status",
					"source_candidate_id", "create_time", "create_by", "modify_time", "modify_by",
				}).AddRow(200, termID, 1, tt.canonicalName, tt.canonicalLang, "prefLabel", "auto-promoted", nil, time.Now(), nil, time.Now(), nil))
			mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
				WithArgs(termID, tt.alias, tt.aliasLang, "altLabel", "auto-promoted", nil, nil, nil).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "term_id", "version", "label", "lang", "label_role", "status",
					"source_candidate_id", "create_time", "create_by", "modify_time", "modify_by",
				}).AddRow(201, termID, 1, tt.alias, tt.aliasLang, "altLabel", "auto-promoted", nil, time.Now(), nil, time.Now(), nil))
			mock.ExpectQuery(regexp.QuoteMeta(acceptedForConceptSQL)).
				WithArgs(conceptID).
				WillReturnError(stop)
			mock.ExpectRollback()

			_, err = store.EnsureAcceptedOrCreate(context.Background(), conceptID, TermSynthesisInput{
				CanonicalName: tt.canonicalName,
				Aliases:       []string{tt.alias},
				Definition:    "definition",
				ValueType:     "number",
				RangeType:     "exact",
			}, testMethod, testScore, testEvidence)
			if !errors.Is(err, stop) {
				t.Fatalf("EnsureAcceptedOrCreate error = %v, want %v", err, stop)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Task 5.7 reader compatibility certification: TestAlignmentsStoreAcceptedForConcept
// mocks against the acceptedForConceptSQL constant itself, so it would still
// pass even if the accepted-only clause were accidentally removed from that
// constant. This test independently types the WHERE clause so a regression
// to the accepted-only guarantee (a represented or unsupported aligns_to_term
// row must never be treated as this concept's alignment) breaks a test.
func TestAcceptedForConceptSQLFiltersToAcceptedStatusOnly(t *testing.T) {
	if !regexp.MustCompile(`(?s)status = 'accepted'`).MatchString(acceptedForConceptSQL) {
		t.Fatalf("acceptedForConceptSQL no longer filters to accepted status:\n%s", acceptedForConceptSQL)
	}
	if !regexp.MustCompile(`(?s)subject_ref_kind = 'keyword_concept'`).MatchString(acceptedForConceptSQL) {
		t.Fatalf("acceptedForConceptSQL no longer scopes to keyword_concept:\n%s", acceptedForConceptSQL)
	}
}
