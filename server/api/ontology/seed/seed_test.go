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

	warnings, err := ensureCuratedModules(context.Background(), db, func(_ context.Context, _ *sql.DB, ids []string, authorOnly bool) ([]BootstrapWarning, error) {
		if authorOnly {
			t.Fatal("service bootstrap must release strict modules")
		}
		seeded = append(seeded, append([]string(nil), ids...))
		return nil, nil
	})
	if err != nil {
		t.Fatalf("EnsureCuratedModules: %v", err)
	}
	// semantic-processing and provision join the strict batch (ADR 2026081801
	// Phase 1 task 2.1, Phase 4 task 7.6): both depend only on core, so unlike
	// measurement neither has a deferred dependency on the separately
	// imported quantity module.
	wantStrict := []string{"core", "document-authority", "semantic-processing", "provision"}
	if len(seeded) != 1 || !sameStringSlice(seeded[0], wantStrict) {
		t.Fatalf("strict modules seeded = %#v, want %#v", seeded, wantStrict)
	}
	if len(warnings) != 1 || warnings[0].Kind != WarningDeferredDependency || warnings[0].ModuleID != "measurement" || warnings[0].DependencyModuleID != "quantity" {
		t.Fatalf("warnings = %#v, want deferred measurement warning", warnings)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestCuratedReleaseVersionUsesDeclaredBaseVersion(t *testing.T) {
	mc := moduleContent{ModuleID: "core", Version: "2.1.0", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "x"}}}
	if got := curatedReleaseVersion(mc); len(got) < len(mc.Version) || got[:len(mc.Version)] != mc.Version {
		t.Fatalf("release version = %q, want base %q", got, mc.Version)
	}
}

func TestNextCuratedReleaseVersionReleasesRevertedContentAgain(t *testing.T) {
	base := "1.0.0+seed.abc123"
	if got := nextCuratedReleaseVersion(base, map[string]bool{base: true}); got != base+".r2" {
		t.Fatalf("next release version = %q, want %q", got, base+".r2")
	}
	if got := nextCuratedReleaseVersion(base, map[string]bool{base: true, base + ".r2": true}); got != base+".r3" {
		t.Fatalf("next release version = %q, want %q", got, base+".r3")
	}
}

func TestBootstrapWarningReportsPreservedActiveRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Version: "1.0.0", Title: "Core"}
	version := curatedReleaseVersion(mc)
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_module_releases\nWHERE module_id = $1 AND version = $2")).
		WithArgs("core", version).
		WillReturnRows(sqlmock.NewRows(releaseColumns()).AddRow(int64(11), "core", version, "Core", []byte(`{}`), "checksum", []byte(`{}`), nil, "ontology-seed", now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_module_releases\nWHERE module_id = $1\nORDER BY released_at DESC, id DESC\nLIMIT 1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(releaseColumns()).AddRow(int64(11), "core", version, "Core", []byte(`{}`), "checksum", []byte(`{}`), nil, "ontology-seed", now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_active_releases ar")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(activeReleaseColumns()).AddRow(int64(3), "core", int64(7), "operator-version", now, "operator", nil))

	warnings, err := releaseAndActivate(context.Background(), db, mc)
	if err != nil {
		t.Fatalf("releaseAndActivate: %v", err)
	}
	if len(warnings) != 1 || warnings[0].Kind != WarningPendingActivation {
		t.Fatalf("warnings = %#v, want pending activation warning", warnings)
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
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_module_releases\nWHERE module_id = $1\nORDER BY released_at DESC, id DESC\nLIMIT 1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(releaseColumns()).AddRow(int64(11), "core", version, "Core", []byte(`{}`), "checksum", []byte(`{}`), nil, "ontology-seed", now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_active_releases ar")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(activeReleaseColumns()).AddRow(int64(3), "core", int64(7), "operator-version", now, "operator", nil))

	if _, err := releaseAndActivate(context.Background(), db, mc); err != nil {
		t.Fatalf("releaseAndActivate: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestAuthorModuleUpdatesExistingModuleMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "New title", Owner: "new-owner", DependsOn: []string{"quantity"}, Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "x", Labels: enPref("x")}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_modules\nWHERE module_id = $1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(moduleColumns()).AddRow(int64(1), "core", "Old title", "old-owner", "", pq.Array([]string{}), "active", now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_modules\nSET title = $2, owner = $3, depends_on = $4, modify_time = NOW(), modify_by = $5\nWHERE module_id = $1")).
		WithArgs("core", "New title", "new-owner", pq.Array([]string{"quantity"}), "ontology-seed").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_modules\nWHERE module_id = $1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(moduleColumns()).AddRow(int64(1), "core", "New title", "new-owner", "", pq.Array([]string{"quantity"}), "active", now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs("core:x", "class", "core", "approved", "x", nil, nil, nil, nil, nil, "ontology-seed", "ontology-seed").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 1, "class", "core", "approved", "x", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS (\n\tSELECT 1 FROM kb.ontology_term_labels")).
		WithArgs("core:x", "en").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs("core:x", "x", "en", "prefLabel", "approved", nil, "ontology-seed", "ontology-seed").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(3), "core:x", 1, "x", "en", "prefLabel", "approved", nil, now, "ontology-seed", now, "ontology-seed"))

	if err := authorModule(context.Background(), db, mc); err != nil {
		t.Fatalf("authorModule: %v", err)
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

func TestAuthorModuleReauthorsSupersededCuratedTerm(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "x", Labels: enPref("x")}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_modules\nWHERE module_id = $1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(moduleColumns()).AddRow(int64(1), "core", "Core", "platform", "", pq.Array([]string{}), "active", now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 1, "class", "core", "superseded", "x", "", nil, "", "", nil, now, "operator", now, "operator"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_terms")).
		WithArgs("core:x", "class", "core", "approved", "x", nil, nil, nil, nil, nil, "ontology-seed", "ontology-seed").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(3), "core:x", 2, "class", "core", "approved", "x", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(4), "core:x", 1, "x", "en", "prefLabel", "superseded", nil, now, "operator", now, "operator"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_term_labels")).
		WithArgs("core:x", "x", "en", "prefLabel", "approved", nil, "ontology-seed", "ontology-seed").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(5), "core:x", 2, "x", "en", "prefLabel", "approved", nil, now, "ontology-seed", now, "ontology-seed"))

	if err := authorModule(context.Background(), db, mc); err != nil {
		t.Fatalf("authorModule: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("superseded curated term must be re-authored as approved: %v", err)
	}
}

func TestReleaseAndActivateAdvancesSeedOwnedActiveRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Version: "1.0.0", Title: "Core"}
	version := curatedReleaseVersion(mc)
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_module_releases\nWHERE module_id = $1 AND version = $2")).
		WithArgs("core", version).
		WillReturnRows(sqlmock.NewRows(releaseColumns()).AddRow(int64(11), "core", version, "Core", []byte(`{}`), "checksum", []byte(`{}`), nil, "ontology-seed", now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_module_releases\nWHERE module_id = $1\nORDER BY released_at DESC, id DESC\nLIMIT 1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(releaseColumns()).AddRow(int64(15), "core", version+".r2", "Core", []byte(`{}`), "checksum", []byte(`{}`), nil, "ontology-seed", now.Add(time.Minute)))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_active_releases ar")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(activeReleaseColumns()).AddRow(int64(3), "core", int64(3), "1.0.0", now, "ontology-seed", nil))
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_active_releases")).
		WithArgs("core", "ontology-seed").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_active_releases")).
		WithArgs("core", int64(15), "ontology-seed").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ar.id, ar.module_id, ar.release_id, r.version, ar.activated_at")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows([]string{"id", "module_id", "release_id", "release_version", "activated_at", "activated_by", "deactivated_at"}).
			AddRow(int64(9), "core", int64(15), version+".r2", now, "ontology-seed", nil))

	warnings, err := releaseAndActivate(context.Background(), db, mc)
	if err != nil {
		t.Fatalf("releaseAndActivate: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none after advancing seed-owned activation", warnings)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("seed-owned active release must advance to the newest curated release: %v", err)
	}
}

func TestCuratedContentReleasedRejectsStaleNewestReleasePins(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "x", Labels: enPref("x")}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 1, "class", "core", "included_in_release", "x", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(3), "core:x", 1, "x", "en", "prefLabel", "included_in_release", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_module_releases\nWHERE module_id = $1\nORDER BY released_at DESC, id DESC\nLIMIT 1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(releaseColumns()).AddRow(int64(15), "core", "1.0.0+seed.stale", "Core", []byte(`{}`), "checksum", []byte(`{"quantity":"1.0.0"}`), nil, "ontology-seed", now))

	released, _, err := curatedContentReleased(context.Background(), db, mc)
	if err != nil {
		t.Fatalf("curatedContentReleased: %v", err)
	}
	if released {
		t.Fatal("curated content must not count as released when the newest release pins a stale dependency set")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCuratedContentReleasedAcceptsMatchingNewestReleasePins(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mc := moduleContent{ModuleID: "core", Title: "Core", Owner: "platform", DependsOn: []string{"quantity"}, Terms: []seedTerm{{ID: "core:x", Kind: "class", Def: "x", Labels: enPref("x")}}}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE term_id = $1\nORDER BY version DESC\nLIMIT 1")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(termColumns()).AddRow(int64(2), "core:x", 1, "class", "core", "included_in_release", "x", "", nil, "", "", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_term_labels\nWHERE term_id = $1\nORDER BY version DESC")).
		WithArgs("core:x").
		WillReturnRows(sqlmock.NewRows(labelColumns()).AddRow(int64(3), "core:x", 1, "x", "en", "prefLabel", "included_in_release", nil, now, "ontology-seed", now, "ontology-seed"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_module_releases\nWHERE module_id = $1\nORDER BY released_at DESC, id DESC\nLIMIT 1")).
		WithArgs("core").
		WillReturnRows(sqlmock.NewRows(releaseColumns()).AddRow(int64(15), "core", "1.0.0+seed.cur", "Core", []byte(`{}`), "checksum", []byte(`{"quantity":"1.0.0"}`), nil, "ontology-seed", now))

	released, _, err := curatedContentReleased(context.Background(), db, mc)
	if err != nil {
		t.Fatalf("curatedContentReleased: %v", err)
	}
	if !released {
		t.Fatal("curated content must count as released when the newest release pins the curated dependency set")
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
