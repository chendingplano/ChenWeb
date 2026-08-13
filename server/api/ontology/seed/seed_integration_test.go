package seed

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// These tests exercise EnsureCuratedModules against a live, freshly migrated
// PostgreSQL database with no manual seed step -- the fresh-database
// integration test named as remediation item 10 in bug doc-2026081301, and
// missing through every round of that report's review. It is what would have
// caught finding 1 (circular measurement/quantity dependency on a fresh
// database) and finding H (unbounded release churn from a superseded curated
// term) automatically instead of by hand.
//
// Unlike this package's SQL-mock tests, and unlike the other TEST_DATABASE_URL
// integration tests in this repo that reuse a persistent, already-migrated
// database (chenweb_test), this test must run against a database that starts
// with zero ontology content. A shared chenweb_test database already carries
// an imported quantity module from prior runs, which would silently skip the
// deferral path this test exists to check -- and per the ChenWeb bug-doc
// record, some other tests unconditionally wipe module_id='quantity' rows
// there, so sharing state with them is unsafe in both directions. This test
// therefore treats TEST_DATABASE_URL only as a connection template: it opens
// an administrative connection with the same parameters, CREATEs a uniquely
// named scratch database, runs the full project_migrations set against it,
// and drops it afterward. The connecting role needs CREATEDB.
//
//	TEST_DATABASE_URL='host=/tmp user=cding dbname=postgres sslmode=disable' \
//	    go test ./server/api/ontology/seed/ -run TestEnsureCuratedModulesFreshDatabaseLifecycle -v
func freshOntologyTestDB(t *testing.T) *sql.DB {
	t.Helper()
	template := os.Getenv("TEST_DATABASE_URL")
	if template == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	dbnamePattern := regexp.MustCompile(`dbname=\S+`)
	if !dbnamePattern.MatchString(template) {
		t.Fatalf("TEST_DATABASE_URL must be a key=value libpq string with dbname=..., got %q", template)
	}

	admin, err := sql.Open("postgres", dbnamePattern.ReplaceAllString(template, "dbname=postgres"))
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	// t.Cleanup runs LIFO: registering the close before the drop below means
	// the drop (which needs admin still open) runs first, then the close.
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatalf("ping admin connection: %v", err)
	}

	name := fmt.Sprintf("ontology_seed_fresh_%d_%d", time.Now().UnixNano(), rand.Intn(1_000_000))
	if _, err := admin.Exec(`CREATE DATABASE ` + pq.QuoteIdentifier(name)); err != nil {
		t.Fatalf("create scratch database %s: %v", name, err)
	}
	t.Cleanup(func() {
		// A leaked connection from a failed test would otherwise block DROP
		// DATABASE; terminate any backends still attached to the scratch
		// database before dropping it.
		_, _ = admin.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, name)
		if _, err := admin.Exec(`DROP DATABASE IF EXISTS ` + pq.QuoteIdentifier(name)); err != nil {
			t.Errorf("drop scratch database %s: %v", name, err)
		}
	})

	db, err := sql.Open("postgres", dbnamePattern.ReplaceAllString(template, "dbname="+name))
	if err != nil {
		t.Fatalf("open scratch database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping scratch database: %v", err)
	}

	_, file, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(file), "../../../../project_migrations")
	goose.SetDialect("postgres")
	if err := goose.Up(db, migrationsDir); err != nil {
		t.Fatalf("run migrations against scratch database: %v", err)
	}
	return db
}

// ontologyRowCounts is a coarse fingerprint of everything EnsureCuratedModules
// can touch. Comparing two snapshots is how the idempotence subtests assert
// "no additional module, term, label, or release rows" without hand-listing
// every row.
type ontologyRowCounts struct {
	modules, releases, terms, labels, activeReleases int
}

func snapshotOntologyRowCounts(t *testing.T, db *sql.DB) ontologyRowCounts {
	t.Helper()
	var c ontologyRowCounts
	for table, dest := range map[string]*int{
		"kb.ontology_modules":         &c.modules,
		"kb.ontology_module_releases": &c.releases,
		"kb.ontology_terms":           &c.terms,
		"kb.ontology_term_labels":     &c.labels,
	} {
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(dest); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
	}
	if err := db.QueryRow(`SELECT count(*) FROM kb.ontology_active_releases WHERE deactivated_at IS NULL`).Scan(&c.activeReleases); err != nil {
		t.Fatalf("count active releases: %v", err)
	}
	return c
}

// stubQuantityActiveRelease authors and activates a minimal quantity module
// through the real modules/terms stores, standing in for the QUDT import that
// normally gives quantity its release. EnsureCuratedModules only checks for
// an active quantity release (seed.go's ensureCuratedModules), so a single
// term is sufficient to unblock measurement.
func stubQuantityActiveRelease(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if _, err := (modules.ModuleStore{DB: db}).CreateModule(ctx, modules.Module{
		ModuleID: "quantity", Title: "Quantity", Owner: "platform", Status: "active",
		DependsOn: []string{}, CreateBy: "test", ModifyBy: "test",
	}); err != nil {
		t.Fatalf("create quantity module: %v", err)
	}
	if _, err := (terms.TermStore{DB: db}).CreateTerm(ctx, terms.Term{
		TermID: "qty:stub_kind", TermKind: "quantity_kind", ModuleID: "quantity",
		Definition: "stub quantity kind for the fresh-database integration test",
		Status:     "approved", CreateBy: "test", ModifyBy: "test",
	}); err != nil {
		t.Fatalf("author quantity stub term: %v", err)
	}
	rs := modules.ReleaseStore{DB: db}
	rel, err := rs.CreateRelease(ctx, "quantity", "1.0.0", "test")
	if err != nil {
		t.Fatalf("release quantity: %v", err)
	}
	if _, err := rs.Activate(ctx, "quantity", rel.ID, "test"); err != nil {
		t.Fatalf("activate quantity release: %v", err)
	}
}

// assertMeasurementGateTermsReleased checks the exact condition
// AssociateSemantics.termExists gates on (associate_semantics.go: status =
// 'included_in_release') for the two governed terms bug doc-2026081301 was
// filed over: mea:measured_by and mea:lower_bound_requirement.
func assertMeasurementGateTermsReleased(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, termID := range []string{"mea:measured_by", "mea:lower_bound_requirement"} {
		var exists bool
		const stmt = `SELECT EXISTS (SELECT 1 FROM kb.ontology_terms WHERE term_id = $1 AND status = 'included_in_release')`
		if err := db.QueryRow(stmt, termID).Scan(&exists); err != nil {
			t.Fatalf("check gate term %s: %v", termID, err)
		}
		if !exists {
			t.Fatalf("%s is not included_in_release; associate_semantics.termExists would still defer on it", termID)
		}
	}
}

func TestEnsureCuratedModulesFreshDatabaseLifecycle(t *testing.T) {
	db := freshOntologyTestDB(t)
	ctx := context.Background()

	t.Run("defers measurement until quantity is releasable", func(t *testing.T) {
		warnings, err := EnsureCuratedModules(ctx, db)
		if err != nil {
			t.Fatalf("EnsureCuratedModules on a fresh database: %v", err)
		}
		if len(warnings) != 1 || warnings[0].Kind != WarningDeferredDependency ||
			warnings[0].ModuleID != "measurement" || warnings[0].DependencyModuleID != "quantity" {
			t.Fatalf("warnings = %#v, want exactly one deferred-dependency warning for measurement on quantity", warnings)
		}

		for _, moduleID := range []string{"core", "document-authority"} {
			var released int
			if err := db.QueryRow(`SELECT count(*) FROM kb.ontology_terms WHERE module_id = $1 AND status = 'included_in_release'`, moduleID).Scan(&released); err != nil {
				t.Fatalf("count released terms for %s: %v", moduleID, err)
			}
			if released == 0 {
				t.Fatalf("%s has no included_in_release terms after bootstrap", moduleID)
			}
		}

		var measurementModuleRows int
		if err := db.QueryRow(`SELECT count(*) FROM kb.ontology_modules WHERE module_id = 'measurement'`).Scan(&measurementModuleRows); err != nil {
			t.Fatalf("count measurement module rows: %v", err)
		}
		if measurementModuleRows != 0 {
			t.Fatalf("measurement module was registered despite the deferral, rows=%d", measurementModuleRows)
		}
	})

	t.Run("is idempotent while measurement is deferred", func(t *testing.T) {
		before := snapshotOntologyRowCounts(t, db)
		warnings, err := EnsureCuratedModules(ctx, db)
		if err != nil {
			t.Fatalf("EnsureCuratedModules re-run: %v", err)
		}
		if len(warnings) != 1 || warnings[0].Kind != WarningDeferredDependency {
			t.Fatalf("warnings = %#v, want the deferral warning to persist unchanged", warnings)
		}
		after := snapshotOntologyRowCounts(t, db)
		if before != after {
			t.Fatalf("re-running while deferred is not a no-op: before=%+v after=%+v", before, after)
		}
	})

	t.Run("releases measurement once quantity has an active release", func(t *testing.T) {
		stubQuantityActiveRelease(t, ctx, db)

		warnings, err := EnsureCuratedModules(ctx, db)
		if err != nil {
			t.Fatalf("EnsureCuratedModules after quantity is releasable: %v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("warnings = %#v, want none once quantity is releasable", warnings)
		}
		assertMeasurementGateTermsReleased(t, db)
	})

	t.Run("is idempotent once fully bootstrapped", func(t *testing.T) {
		before := snapshotOntologyRowCounts(t, db)
		for i := 0; i < 2; i++ {
			warnings, err := EnsureCuratedModules(ctx, db)
			if err != nil {
				t.Fatalf("EnsureCuratedModules re-run %d: %v", i+1, err)
			}
			if len(warnings) != 0 {
				t.Fatalf("re-run %d warnings = %#v, want none", i+1, warnings)
			}
		}
		after := snapshotOntologyRowCounts(t, db)
		if before != after {
			t.Fatalf("re-running a fully bootstrapped database is not a no-op: before=%+v after=%+v", before, after)
		}
		assertMeasurementGateTermsReleased(t, db)
	})
}
