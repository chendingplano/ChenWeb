package keywords

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
		"constraint fk_keyword_surface_evidence_source_release foreign key (source, release)",
		"constraint fk_keyword_external_ids_source_release foreign key (source, release)",
		"references kb.keyword_sources (source, release) on update restrict on delete restrict",
		"create table kb.keyword_source_artifacts",
		"create table kb.keyword_catalog_entries",
		"create table kb.keyword_catalog_labels",
		"create table kb.keyword_catalog_relations",
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

	down := strings.Index(sql, "-- +goose down")
	guard := strings.Index(sql[down:], "duplicate (surface_id, source, external_id)")
	dropRelease := strings.Index(sql[down:], "drop column release")
	if down < 0 || guard < 0 || dropRelease < 0 || guard > dropRelease {
		t.Error("Down must guard against duplicate collapse before dropping evidence release")
	}
	if strings.Contains(sql[down:], "delete from kb.keyword_sources") {
		t.Error("Down must retain migration-created source placeholders")
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
