package modules

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func timeNow() time.Time { return time.Now() }

// TestActivateCallsPromoteHookInsideTransaction proves the approved-proposal
// promotion runs inside Activate's own transaction: the Promote hook receives
// the release id and its content checksum (resolved on the same tx), and the
// transaction commits only after the hook succeeds (P5 review 2026080302
// finding P5-4 -- promotion was previously a post-hoc CLI call with a warning).
func TestActivateCallsPromoteHookInsideTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_active_releases`)).
		WithArgs("ventilator", "alice").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.ontology_active_releases`)).
		WithArgs("ventilator", int64(42), "alice").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT content_checksum FROM kb.ontology_module_releases WHERE id = $1`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"content_checksum"}).AddRow("sha256:xyz"))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT ar.id, ar.module_id, ar.release_id, r.version, ar.activated_at`)).
		WithArgs("ventilator").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "module_id", "release_id", "release_version", "activated_at", "activated_by", "deactivated_at",
		}).AddRow(int64(9), "ventilator", int64(42), "0.2.0", timeNow(), "alice", nil))

	var gotReleaseID int64
	var gotChecksum string
	rs := ReleaseStore{
		DB: db,
		Promote: func(_ context.Context, tx *sql.Tx, releaseID int64, releaseChecksum string) error {
			if tx == nil {
				t.Fatal("Promote hook must receive the release transaction")
			}
			gotReleaseID, gotChecksum = releaseID, releaseChecksum
			return nil
		},
	}

	active, err := rs.Activate(context.Background(), "ventilator", 42, "alice")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if active.ReleaseID != 42 {
		t.Fatalf("active release id = %d, want 42", active.ReleaseID)
	}
	if gotReleaseID != 42 || gotChecksum != "sha256:xyz" {
		t.Fatalf("promote hook received release=%d checksum=%q, want 42/sha256:xyz", gotReleaseID, gotChecksum)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestMarkApprovedProposalsIncluded proves the release transaction marks only
// the module's approved proposals as included_in_release, keyed on module and
// approved status -- a proposal can never claim a release that did not carry
// it (P5 review 2026080302 finding P5-17), and there is no manual
// approved -> included_in_release HTTP transition anymore.
func TestMarkApprovedProposalsIncluded(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_applicability_proposals`)).
		WithArgs(int64(99), "ventilator").
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := markApprovedProposalsIncluded(context.Background(), tx, "ventilator", 99); err != nil {
		t.Fatalf("markApprovedProposalsIncluded: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestActivateRollsBackWhenPromoteFails proves a promotion failure rolls the
// activation transaction back instead of leaving the release activated without
// its draft policy.
func TestActivateRollsBackWhenPromoteFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_active_releases`)).
		WithArgs("ventilator", "alice").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.ontology_active_releases`)).
		WithArgs("ventilator", int64(42), "alice").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT content_checksum FROM kb.ontology_module_releases WHERE id = $1`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"content_checksum"}).AddRow("sha256:xyz"))
	mock.ExpectRollback()

	rs := ReleaseStore{
		DB: db,
		Promote: func(context.Context, *sql.Tx, int64, string) error {
			return errors.New("promotion failed")
		},
	}
	_, err = rs.Activate(context.Background(), "ventilator", 42, "alice")
	if err == nil || !strings.Contains(err.Error(), "promotion failed") {
		t.Fatalf("expected the promotion failure to roll back activation, err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
