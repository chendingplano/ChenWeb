package keywords

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

const keywordIdentityLockSQL = "SELECT pg_advisory_xact_lock(1264011588, 1);"

func expectKeywordIdentityLock(mock sqlmock.Sqlmock) {
	mock.ExpectExec(regexp.QuoteMeta(keywordIdentityLockSQL)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestMergeConceptTxUsesCallerDirectionAndTransaction(t *testing.T) {
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
	expectKeywordIdentityLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:provisional").WillReturnRows(conceptRow("kw:provisional", "provisional", nil))
	mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).
		WithArgs("kw:active").WillReturnRows(conceptRow("kw:active", "active", nil))
	mock.ExpectQuery(regexp.QuoteMeta(neverMergeSQL)).
		WithArgs("keyword", "kw:active", "kw:provisional").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE kb.keyword_concepts SET status = 'merged', merged_into = $2, modify_time = NOW() WHERE concept_id = $1")).
		WithArgs("kw:provisional", "kw:active").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(
		"UPDATE kb.keyword_surfaces SET concept_id = $2, origin_concept = COALESCE(origin_concept, $1), modify_time = NOW() WHERE concept_id = $1")).
		WithArgs("kw:provisional", "kw:active").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	if err := (ConceptStore{}).MergeConceptTx(ctx, tx, "kw:provisional", "kw:active"); err != nil {
		t.Fatalf("MergeConceptTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMergeAndDecisionAuditShareCallerTransaction(t *testing.T) {
	for _, commit := range []bool{false, true} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
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
			expectKeywordIdentityLock(mock)
			expectKeywordIdentityLock(mock)
			mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).WithArgs("kw:from").
				WillReturnRows(conceptRow("kw:from", "provisional", nil))
			mock.ExpectQuery(regexp.QuoteMeta(getConceptSQL)).WithArgs("kw:to").
				WillReturnRows(conceptRow("kw:to", "active", nil))
			mock.ExpectQuery(regexp.QuoteMeta(neverMergeSQL)).WithArgs("keyword", "kw:from", "kw:to").
				WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.keyword_concepts")).
				WithArgs("kw:from", "kw:to").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.keyword_surfaces")).
				WithArgs("kw:from", "kw:to").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.semid_decision_log")).
				WithArgs("keyword", nil, "{}", "{}", "merged", nil, nil, "tester", 0).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
			if commit {
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}

			if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
				t.Fatal(err)
			}
			if err := (ConceptStore{}).MergeConceptTx(ctx, tx, "kw:from", "kw:to"); err != nil {
				t.Fatal(err)
			}
			if _, err := (semid.DecisionLogStore{DB: tx}).Append(ctx, semid.DecisionLogEntry{
				Family: "keyword", Verdict: "merged", Actor: "tester",
			}); err != nil {
				t.Fatal(err)
			}
			if commit {
				err = tx.Commit()
			} else {
				err = tx.Rollback()
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConceptWritersLockInsideTransaction(t *testing.T) {
	ctx := context.Background()
	t.Run("create", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectBegin()
		expectKeywordIdentityLock(mock)
		mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.keyword_concepts")).
			WithArgs("kw:new", "New", nil, "_", "active", "none").
			WillReturnRows(conceptRow("kw:new", "active", nil))
		mock.ExpectCommit()
		if _, err := (ConceptStore{DB: db}).CreateConcept(ctx, Concept{ConceptID: "kw:new", PrefLabel: "New"}); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("update label", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectBegin()
		expectKeywordIdentityLock(mock)
		mock.ExpectQuery(regexp.QuoteMeta("UPDATE kb.keyword_concepts SET pref_label = $2, gloss = $3, modify_time = NOW() WHERE concept_id = $1")).
			WithArgs("kw:a", "Renamed", nil).
			WillReturnRows(conceptRow("kw:a", "active", nil))
		mock.ExpectCommit()
		if _, err := (ConceptStore{DB: db}).UpdateConceptLabel(ctx, "kw:a", "Renamed", ""); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("status", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectBegin()
		expectKeywordIdentityLock(mock)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT " + conceptColumns + " " + conceptFrom + " WHERE concept_id = $1")).
			WithArgs("kw:a").WillReturnRows(conceptRow("kw:a", "active", nil))
		mock.ExpectQuery(regexp.QuoteMeta("UPDATE kb.keyword_concepts SET status = $2, modify_time = NOW() WHERE concept_id = $1")).
			WithArgs("kw:a", "provisional").WillReturnRows(conceptRow("kw:a", "provisional", nil))
		mock.ExpectCommit()
		if _, err := (ConceptStore{DB: db}).TransitionStatus(ctx, "kw:a", "provisional"); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSurfaceWritersLockInsideTransaction(t *testing.T) {
	ctx := context.Background()
	t.Run("create", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		sf := Surface{ConceptID: "kw:a", Surface: "Lamp", AliasType: "exact", Provenance: "manual"}
		mock.ExpectBegin()
		expectKeywordIdentityLock(mock)
		mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.keyword_surfaces")).
			WithArgs(sqlmock.AnyArg(), "kw:a", "Lamp", "lamp", sqlmock.AnyArg(), "pref", "exact", "und", "_", 1.0, "manual", false, nil).
			WillReturnRows(sqlmock.NewRows([]string{
				"surface_id", "concept_id", "surface", "norm_key", "norm_version", "label_role", "alias_type", "lang", "scope", "confidence", "provenance", "locked", "evidence", "create_time", "modify_time",
			}).AddRow("kws_x", "kw:a", "Lamp", "lamp", 1, "pref", "exact", "und", "_", 1.0, "manual", false, nil, time.Now(), time.Now()))
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM kb.keyword_surface_keys")).
			WithArgs("kws_x").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_surface_keys")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.keyword_surface_keys")).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if _, err := (SurfaceStore{DB: db}).CreateSurface(ctx, sf); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("lock", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()
		mock.ExpectBegin()
		expectKeywordIdentityLock(mock)
		mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.keyword_surfaces SET locked = $2, modify_time = NOW() WHERE surface_id = $1")).
			WithArgs("kws_x", true).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if err := (SurfaceStore{DB: db}).UpdateSurfaceLock(ctx, "kws_x", true); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}
