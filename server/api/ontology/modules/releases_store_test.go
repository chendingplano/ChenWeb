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

func TestCreateReleasePreserveActiveExcludesCurrentActiveReleaseFromSupersession(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	expectCreateReleasePrerequisites(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.ontology_module_releases`)).
		WithArgs("core", "1.0.0+seed.changed", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "ontology-seed").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(101)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_terms SET status = 'included_in_release'`)).
		WithArgs(int64(101), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_candidates`)).
		WithArgs(int64(101)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// This is the seed path: a new release may supersede inactive historical
	// releases, but must leave the operator-selected active release intact.
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_module_releases
SET superseded_by_release_id = $1, modify_time = NOW()
WHERE module_id = $2 AND id <> $1 AND superseded_by_release_id IS NULL
	AND id NOT IN (
		SELECT release_id FROM kb.ontology_active_releases
		WHERE module_id = $2 AND deactivated_at IS NULL
	)`)).
		WithArgs(int64(101), "core").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_applicability_proposals`)).
		WithArgs(int64(101), "core").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_module_releases
WHERE id = $1`)).
		WithArgs(int64(101)).
		WillReturnRows(releaseRows().AddRow(int64(101), "core", "1.0.0+seed.changed", "Core", []byte(`{}`), "checksum", []byte(`{}`), nil, "ontology-seed", timeNow()))

	_, err = (ReleaseStore{DB: db, PreserveActive: true}).CreateRelease(context.Background(), "core", "1.0.0+seed.changed", "ontology-seed")
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateReleaseNormallySupersedesAllPriorReleases(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	expectCreateReleasePrerequisites(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.ontology_module_releases`)).
		WithArgs("core", "1.0.1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "compiler").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(102)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_terms SET status = 'included_in_release'`)).
		WithArgs(int64(102), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_candidates`)).
		WithArgs(int64(102)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_module_releases
SET superseded_by_release_id = $1, modify_time = NOW()
WHERE module_id = $2 AND id <> $1 AND superseded_by_release_id IS NULL`)).
		WithArgs(int64(102), "core").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.ontology_applicability_proposals`)).
		WithArgs(int64(102), "core").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_module_releases
WHERE id = $1`)).
		WithArgs(int64(102)).
		WillReturnRows(releaseRows().AddRow(int64(102), "core", "1.0.1", "Core", []byte(`{}`), "checksum", []byte(`{}`), nil, "compiler", timeNow()))

	_, err = (ReleaseStore{DB: db}).CreateRelease(context.Background(), "core", "1.0.1", "compiler")
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectCreateReleasePrerequisites(mock sqlmock.Sqlmock) {
	now := timeNow()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_modules
WHERE module_id = $1`)).
		WithArgs("core").
		WillReturnRows(moduleRows().AddRow(int64(1), "core", "Core", "platform", "", "{}", "active", now, "tester", now, "tester"))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_terms
WHERE ($1 = '' OR module_id = $1)
  AND ($2 = '' OR status = $2)`)).
		WithArgs("core", "approved").
		WillReturnRows(termRows().AddRow(int64(10), "core:term", 2, "class", "core", "approved", "term", "", nil, "", "", nil, now, "tester", now, "tester"))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_term_labels
WHERE term_id = $1`)).
		WithArgs("core:term").
		WillReturnRows(sqlmock.NewRows([]string{"id", "term_id", "version", "label", "lang", "label_role", "status", "source_candidate_id", "create_time", "create_by", "modify_time", "modify_by"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_axioms
WHERE ($1 = '' OR module_id = $1)`)).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows([]string{"id", "axiom_id", "version", "axiom_kind", "subject_term_id", "predicate_term_id", "object_term_id", "object_iri", "module_id", "status", "source_candidate_id", "create_time", "create_by", "modify_time", "modify_by"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_mappings
WHERE ($1 = '' OR module_id = $1)`)).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows([]string{"id", "mapping_id", "version", "from_term_id", "to_term_id", "to_iri", "relation", "evidence", "approval_status", "module_id", "status", "source_candidate_id", "create_time", "create_by", "modify_time", "modify_by"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_profiles
WHERE module_id = $1 AND status = 'approved'`)).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows([]string{"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "create_time", "create_by", "modify_time", "modify_by"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_profile_rules pr`)).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows([]string{"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "create_time", "create_by", "modify_time", "modify_by"}))
	mock.ExpectQuery(regexp.QuoteMeta(`FROM kb.ontology_modules
ORDER BY module_id`)).
		WillReturnRows(moduleRows().AddRow(int64(1), "core", "Core", "platform", "", "{}", "active", now, "tester", now, "tester"))
}

func moduleRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "module_id", "title", "owner", "description", "depends_on", "status", "create_time", "create_by", "modify_time", "modify_by"})
}

func termRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "term_id", "version", "term_kind", "module_id", "status", "definition", "scope", "source_candidate_id", "value_type", "range_type", "permitted_unit_term_ids", "create_time", "create_by", "modify_time", "modify_by"})
}

func releaseRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "module_id", "version", "title", "payload", "content_checksum", "dependency_releases", "superseded_by_release_id", "released_by", "released_at"})
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
