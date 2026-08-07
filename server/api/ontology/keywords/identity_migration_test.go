package keywords

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func TestKeywordIdentitySourceMigrationContract(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate migration test")
	}
	path := filepath.Join(filepath.Dir(file), "../../../../project_migrations/20260807000001_govern_keyword_identity_sources.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(raw))

	required := []string{
		"-- +goose up",
		"add column release text",
		"add column identity_authority boolean not null default false",
		"provider_id text",
		"source_subset text",
		"content_checksum text",
		"license_review_status text",
		"authority_role text",
		"authoritative_relations text[]",
		"allowed_scopes text[]",
		"languages text[]",
		"adapter_version text",
		"provenance_locator text",
		"approved_by text",
		"approved_at timestamptz",
		"update kb.keyword_surface_evidence set release = '' where release is null",
		"from kb.keyword_surface_evidence",
		"select distinct source, ''",
		"from kb.keyword_external_ids",
		"select distinct source, release",
		"on conflict (source, release) do nothing",
		"alter column release set not null",
		"alter column release set default ''",
		"drop constraint keyword_surface_evidence_surface_id_source_external_id_key",
		"constraint uq_keyword_surface_evidence_source_external_release unique (surface_id, source, external_id, release)",
		"constraint ck_keyword_sources_nonblank_source check (btrim(source) <> '')",
		"constraint ck_keyword_external_ids_nonblank_external_id check (btrim(external_id) <> '')",
		"btrim(provider_id) <> ''",
		"retrieved_at is not null",
		"cardinality(input_values) > 0",
		"bool_and(value is not null and value !~ ''^[[:space:]]*$'')",
		"coalesce('exact_equivalent' = any(authoritative_relations), false)",
		"constraint ck_keyword_sources_authoritative_relations",
		"value in (''exact_equivalent'', ''related'', ''broader'', ''narrower'', ''translation'', ''probabilistic'', ''other'')",
		"kb.keyword_nonblank_text_array(allowed_scopes)",
		"kb.keyword_nonblank_text_array(languages)",
		"constraint fk_keyword_surface_evidence_source_release foreign key (source, release)",
		"constraint fk_keyword_external_ids_source_release foreign key (source, release)",
		"references kb.keyword_sources (source, release) on update restrict on delete restrict",
		"create table kb.keyword_source_artifacts",
		"create table kb.keyword_catalog_entries",
		"create table kb.keyword_catalog_labels",
		"create table kb.keyword_catalog_relations",
		"object_source",
		"object_release",
		"primary key (source, release, subject_external_id, relation, object_source, object_release, object_external_id)",
		"primary key (source, release, subject_external_id, object_source, object_release, object_external_id, relation)",
		"create table kb.keyword_catalog_negative_decisions",
		"create table kb.keyword_ucum_codes",
		"create table kb.keyword_identity_deployments",
		"create table kb.keyword_identity_deployment_history",
		"create trigger keyword_sources_immutable",
		"create trigger keyword_source_artifacts_immutable",
		"create trigger keyword_identity_deployment_history_append_only",
		"-- +goose down",
		"duplicate (surface_id, source, external_id)",
		"drop constraint fk_keyword_surface_evidence_source_release",
		"drop constraint fk_keyword_external_ids_source_release",
		"drop constraint ck_keyword_sources_nonblank_source",
		"drop constraint ck_keyword_external_ids_nonblank_external_id",
		"drop constraint uq_keyword_surface_evidence_source_external_release",
		"constraint keyword_surface_evidence_surface_id_source_external_id_key unique (surface_id, source, external_id)",
		"drop column release",
		"drop column identity_authority",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration is missing contract fragment %q", fragment)
		}
	}

	if strings.Contains(sql, "keyword_catalog_entries") && strings.Contains(sql, "identity_authority boolean") {
		for _, table := range []string{
			"keyword_catalog_entries", "keyword_catalog_labels", "keyword_catalog_relations",
			"keyword_catalog_negative_decisions", "keyword_ucum_codes",
		} {
			block := createTableBlock(sql, "kb."+table)
			if strings.Contains(block, "identity_authority") || strings.Contains(block, "authoritative") {
				t.Errorf("staging table %s must not carry authority state", table)
			}
		}
	}
	for _, table := range []string{"keyword_catalog_relations", "keyword_catalog_negative_decisions"} {
		block := createTableBlock(sql, "kb."+table)
		for _, column := range []string{"object_source", "object_release", "object_external_id"} {
			if !strings.Contains(block, column) {
				t.Errorf("staging table %s is missing cross-source target column %s", table, column)
			}
		}
	}

	down := strings.Index(sql, "-- +goose down")
	guard := strings.Index(sql[down:], "duplicate (surface_id, source, external_id)")
	dropRelease := strings.Index(sql[down:], "drop column release")
	if down < 0 || guard < 0 || dropRelease < 0 || guard > dropRelease {
		t.Error("Down must guard against duplicate collapse before dropping evidence release")
	}
	if strings.Contains(sql[down:], "delete from kb.keyword_sources") {
		t.Error("Down must retain migration-created source placeholders")
	}
	if got := strings.Count(sql, "-- +goose statementbegin"); got != 3 {
		t.Errorf("compound migration statements need 3 StatementBegin blocks, got %d", got)
	}
	if got := strings.Count(sql, "-- +goose statementend"); got != 3 {
		t.Errorf("compound migration statements need 3 StatementEnd blocks, got %d", got)
	}
}

func TestKeywordIdentitySourceMigrationThroughGoose(t *testing.T) {
	dsn := os.Getenv("KEYWORD_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set KEYWORD_MIGRATION_TEST_DSN to a disposable PostgreSQL database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS public.goose_keyword_identity_test_version;
		DROP SCHEMA IF EXISTS kb CASCADE; CREATE SCHEMA kb;
		CREATE TABLE kb.keyword_concepts (concept_id TEXT PRIMARY KEY);
		CREATE TABLE kb.keyword_surfaces (surface_id TEXT PRIMARY KEY);
		INSERT INTO kb.keyword_concepts VALUES ('c1');
		INSERT INTO kb.keyword_surfaces VALUES ('s1');
		CREATE TABLE kb.keyword_sources (source TEXT NOT NULL, release TEXT NOT NULL DEFAULT '', license TEXT NOT NULL DEFAULT '', retrieved_at TIMESTAMPTZ, notes TEXT NOT NULL DEFAULT '', PRIMARY KEY (source, release));
		CREATE TABLE kb.keyword_external_ids (source TEXT NOT NULL, external_id TEXT NOT NULL, release TEXT NOT NULL DEFAULT '', concept_id TEXT NOT NULL REFERENCES kb.keyword_concepts(concept_id), PRIMARY KEY (source, external_id, release));
		CREATE TABLE kb.keyword_surface_evidence (id BIGSERIAL PRIMARY KEY, surface_id TEXT NOT NULL REFERENCES kb.keyword_surfaces(surface_id) ON DELETE CASCADE, source TEXT NOT NULL, external_id TEXT NOT NULL DEFAULT '', evidence TEXT NOT NULL DEFAULT '', confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0, UNIQUE (surface_id, source, external_id));
		INSERT INTO kb.keyword_surface_evidence (surface_id, source, external_id) VALUES ('s1', 'legacy-evidence', '');
		INSERT INTO kb.keyword_external_ids (source, external_id, release, concept_id) VALUES ('legacy-map', 'M1', '2026.1', 'c1')`); err != nil {
		t.Fatalf("prepare legacy schema: %v", err)
	}
	defer db.ExecContext(ctx, `DROP SCHEMA IF EXISTS kb CASCADE`)
	defer db.ExecContext(ctx, `DROP TABLE IF EXISTS public.goose_keyword_identity_test_version`)

	_, file, _, _ := runtime.Caller(0)
	migrationPath := filepath.Join(filepath.Dir(file), "../../../../project_migrations/20260807000001_govern_keyword_identity_sources.sql")
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(migrationPath)), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_keyword_identity_test_version")
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		t.Fatalf("goose Up: %v", err)
	}

	var placeholders int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.keyword_sources WHERE identity_authority = FALSE AND source IN ('legacy-evidence', 'legacy-map')`).Scan(&placeholders); err != nil || placeholders != 2 {
		t.Fatalf("legacy placeholders = %d, err = %v", placeholders, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO kb.keyword_sources
		(source, release, license, license_review_status, authority_role, authoritative_relations, approved_by, approved_at, identity_authority)
		VALUES ('unsafe', '1', 'license', 'approved', 'exact_identity_authority', ARRAY['exact_equivalent'], 'reviewer', now(), TRUE)`); err == nil {
		t.Fatal("database accepted identity authority with incomplete structured governance")
	}
	const governedInsert = `INSERT INTO kb.keyword_sources
		(provider_id, source, source_subset, release, retrieved_at, content_checksum, license,
		 license_review_status, authority_role, authoritative_relations, allowed_scopes, languages,
		 adapter_version, provenance_locator, approved_by, approved_at, identity_authority)
		VALUES ('provider', $1, 'subset', '1', $2, repeat('a', 64), 'license',
		 'approved', 'exact_identity_authority', $3, $4, $5,
		 'adapter/v1', 'file:///artifact', 'reviewer', now(), TRUE)`
	if _, err := db.ExecContext(ctx, governedInsert, "valid-authority", time.Now(), pq.Array([]string{"exact_equivalent"}), pq.Array([]string{"scope"}), pq.Array([]string{"en"})); err != nil {
		t.Fatalf("database rejected complete identity authority: %v", err)
	}
	exact, scope, english := "exact_equivalent", "scope", "en"
	for _, tc := range []struct {
		name      string
		source    string
		retrieved any
		relations any
		scopes    any
		languages any
	}{
		{"missing retrieval", "unsafe-retrieved", nil, pq.Array([]string{exact}), pq.Array([]string{scope}), pq.Array([]string{english})},
		{"blank scope", "unsafe-scope", time.Now(), pq.Array([]string{exact}), pq.Array([]string{" "}), pq.Array([]string{english})},
		{"blank language", "unsafe-language", time.Now(), pq.Array([]string{exact}), pq.Array([]string{scope}), pq.Array([]string{"\t"})},
		{"NULL-only scope", "unsafe-null-scope", time.Now(), pq.Array([]string{exact}), pq.Array([]*string{nil}), pq.Array([]string{english})},
		{"mixed NULL scope", "unsafe-mixed-scope", time.Now(), pq.Array([]string{exact}), pq.Array([]*string{&scope, nil}), pq.Array([]string{english})},
		{"NULL-only language", "unsafe-null-language", time.Now(), pq.Array([]string{exact}), pq.Array([]string{scope}), pq.Array([]*string{nil})},
		{"mixed NULL language", "unsafe-mixed-language", time.Now(), pq.Array([]string{exact}), pq.Array([]string{scope}), pq.Array([]*string{&english, nil})},
		{"NULL authoritative relation", "unsafe-null-relation", time.Now(), pq.Array([]*string{nil}), pq.Array([]string{scope}), pq.Array([]string{english})},
	} {
		if _, err := db.ExecContext(ctx, governedInsert, tc.source, tc.retrieved, tc.relations, tc.scopes, tc.languages); err == nil {
			t.Errorf("database accepted identity authority with %s", tc.name)
		}
	}
	for _, table := range []string{"keyword_catalog_relations", "keyword_catalog_negative_decisions"} {
		for _, column := range []string{"object_source", "object_release"} {
			var exists bool
			if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='kb' AND table_name=$1 AND column_name=$2)`, table, column).Scan(&exists); err != nil || !exists {
				t.Fatalf("%s column %s exists = %v, err = %v", table, column, exists, err)
			}
		}
	}
	db.SetMaxOpenConns(4)
	registerConcurrently := func(left, right SourcePolicy) [2]error {
		t.Helper()
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, policy := range []SourcePolicy{left, right} {
			policy := policy
			go func() {
				<-start
				results <- (SourcePolicyStore{DB: db}).Register(ctx, policy)
			}()
		}
		close(start)
		return [2]error{<-results, <-results}
	}
	identical := validExactSourcePolicy()
	identical.Source = "concurrent-identical"
	identical.Release = "1"
	if errs := registerConcurrently(identical, identical); errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent identical registration errors = %v", errs)
	}
	left := identical
	left.Source = "concurrent-conflict"
	right := left
	right.ContentChecksum = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	errs := registerConcurrently(left, right)
	var successes, immutable int
	for _, err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrImmutableSourceRelease) {
			immutable++
		} else {
			t.Fatalf("unexpected concurrent conflict error: %v", err)
		}
	}
	if successes != 1 || immutable != 1 {
		t.Fatalf("concurrent conflict results = %v, want one success and one immutable rejection", errs)
	}
	if err := goose.DownContext(ctx, db, dir); err != nil {
		t.Fatalf("goose Down: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.keyword_sources WHERE source IN ('legacy-evidence', 'legacy-map')`).Scan(&placeholders); err != nil || placeholders != 2 {
		t.Fatalf("retained placeholders = %d, err = %v", placeholders, err)
	}
}

func createTableBlock(sql, table string) string {
	start := strings.Index(sql, "create table "+table)
	if start < 0 {
		return ""
	}
	rest := sql[start:]
	end := strings.Index(rest, ";")
	if end < 0 {
		return rest
	}
	return rest[:end]
}
