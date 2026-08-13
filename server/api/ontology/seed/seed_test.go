package seed

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestSeedReleaseStorePreservesActiveRelease(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := seedReleaseStore(db)
	if store.DB != db {
		t.Fatal("seed release store must use the supplied database")
	}
	if !store.PreserveActive {
		t.Fatal("seed release store must preserve an operator-selected active release")
	}
}

func TestEnsureCuratedModulesDefersMeasurementWithoutQuantityRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	var seeded [][]string
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_active_releases ar")).
		WithArgs("quantity").
		WillReturnError(sql.ErrNoRows)

	warnings, err := ensureCuratedModules(context.Background(), db, func(_ context.Context, _ *sql.DB, ids []string, authorOnly bool) error {
		if authorOnly {
			t.Fatal("service bootstrap must release strict modules")
		}
		seeded = append(seeded, append([]string(nil), ids...))
		return nil
	})
	if err != nil {
		t.Fatalf("EnsureCuratedModules: %v", err)
	}
	if len(seeded) != 1 || len(seeded[0]) != 2 || seeded[0][0] != "core" || seeded[0][1] != "document-authority" {
		t.Fatalf("strict modules seeded = %#v, want core and document-authority", seeded)
	}
	if len(warnings) != 1 || warnings[0].ModuleID != "measurement" || warnings[0].DependencyModuleID != "quantity" {
		t.Fatalf("warnings = %#v, want deferred measurement warning", warnings)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestCuratedReleaseVersionChangesWhenContentChanges(t *testing.T) {
	base := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "first", Labels: enPref("x")}}}
	changed := base
	changed.Terms = append([]seedTerm(nil), base.Terms...)
	changed.Terms[0].Def = "changed"

	if got, want := curatedReleaseVersion(base), curatedReleaseVersion(changed); got == want {
		t.Fatalf("content-derived versions must differ after a source edit: %q", got)
	}
	if got, want := curatedReleaseVersion(base), curatedReleaseVersion(base); got != want {
		t.Fatalf("content-derived versions must be deterministic: %q != %q", got, want)
	}
}

func TestReleaseAndActivateDoesNotReplaceExistingActiveRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform"}
	version := curatedReleaseVersion(mc)
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_module_releases\nWHERE module_id = $1 AND version = $2")).
		WithArgs("core", version).
		WillReturnRows(sqlmock.NewRows(releaseColumns()).AddRow(int64(11), "core", version, "Core", []byte(`{}`), "checksum", []byte(`{}`), nil, "ontology-seed", now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_active_releases ar")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(activeReleaseColumns()).AddRow(int64(3), "core", int64(7), "operator-version", now, "operator", nil))

	if err := releaseAndActivate(context.Background(), db, mc); err != nil {
		t.Fatalf("releaseAndActivate: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestAuthorModuleSupersedesChangedPreferredLabelBeforeReplacingIt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "x", Labels: enPref("new label")}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_modules\nWHERE module_id = $1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(moduleColumns()).AddRow(int64(1), "core", "Core", "platform", "", pq.Array([]string{}), "active", now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 1, "class", "core", "approved", "x", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(3), "core:x", 1, "old label", "en", "prefLabel", "approved", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_term_labels\nSET status = 'superseded', modify_time = NOW(), modify_by = $3\nWHERE term_id = $1 AND lang = $2 AND label_role = 'prefLabel'\n\tAND status NOT IN ('rejected', 'superseded')")).
		WithArgs("core:x", "en", "ontology-seed").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs("core:x", "new label", "en", "prefLabel", "approved", nil, "ontology-seed", "ontology-seed").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(4), "core:x", 1, "new label", "en", "prefLabel", "approved", nil, now, "ontology-seed", now, "ontology-seed"))

	if err := authorModule(context.Background(), db, mc); err != nil {
		t.Fatalf("authorModule: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestAuthorModuleVersionsReleasedTermWhenCuratedDefinitionChanges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "curated definition", Labels: enPref("x")}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_modules\nWHERE module_id = $1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(moduleColumns()).AddRow(int64(1), "core", "Core", "platform", "", pq.Array([]string{}), "active", now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 1, "class", "core", "included_in_release", "old definition", "", nil, "", "", nil, now, "prior-release", now, "prior-release"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs("core:x", "class", "core", "approved", "curated definition", nil, nil, nil, nil, nil, "ontology-seed", "ontology-seed").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(3), "core:x", 2, "class", "core", "approved", "curated definition", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(4), "core:x", 1, "x", "en", "prefLabel", "included_in_release", nil, now, "prior-release", now, "prior-release"))

	if err := authorModule(context.Background(), db, mc); err != nil {
		t.Fatalf("authorModule: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestAuthorModuleDoesNotVersionMatchingCuratedTerm(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "curated definition", Labels: enPref("x")}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_modules\nWHERE module_id = $1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(moduleColumns()).AddRow(int64(1), "core", "Core", "platform", "", pq.Array([]string{}), "active", now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 1, "class", "core", "included_in_release", "curated definition", "", nil, "", "", nil, now, "prior-release", now, "prior-release"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(4), "core:x", 1, "x", "en", "prefLabel", "included_in_release", nil, now, "prior-release", now, "prior-release"))

	if err := authorModule(context.Background(), db, mc); err != nil {
		t.Fatalf("authorModule: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("matching curated term must not receive a version: %v", err)
	}
}

func TestStageContentForNewCuratedReleaseStagesIncludedTermAfterLabelOnlyEdit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "x", Labels: enPref("new label")}}}
	// authorModule has already replaced the old released prefLabel with the
	// approved new label. The term itself remains included_in_release until
	// the new content-derived release is constructed.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 1, "class", "core", "included_in_release", "x", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs("core:x", "class", "core", "approved", "x", nil, nil, nil, nil, nil, "ontology-seed", "ontology-seed").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(4), "core:x", 2, "class", "core", "approved", "x", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()).
			AddRow(int64(3), "core:x", 2, "new label", "en", "prefLabel", "approved", nil, now, "ontology-seed", now, "ontology-seed").
			AddRow(int64(1), "core:x", 1, "old label", "en", "prefLabel", "superseded", nil, now, "ontology-seed", now, "ontology-seed"))

	if err := stageContentForNewCuratedRelease(context.Background(), db, mc); err != nil {
		t.Fatalf("stage content for post-release label edit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStageContentForNewCuratedReleaseStagesIncludedDesiredLabel(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "x", Labels: enPref("label")}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 2, "class", "core", "approved", "x", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(3), "core:x", 1, "label", "en", "prefLabel", "included_in_release", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs("core:x", "label", "en", "prefLabel", "approved", nil, "ontology-seed", "ontology-seed").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(4), "core:x", 2, "label", "en", "prefLabel", "approved", nil, now, "ontology-seed", now, "ontology-seed"))

	if err := stageContentForNewCuratedRelease(context.Background(), db, mc); err != nil {
		t.Fatalf("stage content for new release: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func moduleColumns() []string {
	return []string{"id", "module_id", "title", "owner", "description", "depends_on", "status", "create_time", "create_by", "modify_time", "modify_by"}
}

func termColumns() []string {
	return []string{"id", "term_id", "version", "term_kind", "module_id", "status", "definition", "scope", "source_candidate_id", "value_type", "range_type", "permitted_unit_term_ids", "create_time", "create_by", "modify_time", "modify_by"}
}

func labelColumns() []string {
	return []string{"id", "term_id", "version", "label", "lang", "label_role", "status", "source_candidate_id", "create_time", "create_by", "modify_time", "modify_by"}
}

func releaseColumns() []string {
	return []string{"id", "module_id", "version", "title", "payload", "content_checksum", "dependency_releases", "superseded_by_release_id", "released_by", "released_at"}
}

func activeReleaseColumns() []string {
	return []string{"id", "module_id", "release_id", "version", "activated_at", "activated_by", "deactivated_at"}
}
