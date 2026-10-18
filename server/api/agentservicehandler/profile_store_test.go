package agentservicehandler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func profileStoreFixture(t *testing.T) (*SQLProfileVersionStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLProfileVersionStore(db), mock
}

func profileRegistryFixture(t *testing.T) *ProfileRegistry {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	registry, err := LoadProfileRegistry(filepath.Clean(filepath.Join(filepath.Dir(file), "../../../prompts")))
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestProfileStoreEnsureSeededStoresBothResolvedPromptSnapshots(t *testing.T) {
	store, mock := profileStoreFixture(t)
	registry := profileRegistryFixture(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT pg_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_xact_lock"}).AddRow(nil))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	for _, slug := range []string{"knowledge-guide", "problem-diagnostics"} {
		profile := registry.versions[slug][registry.active[slug]]
		promptHash := sha256.Sum256([]byte(profile.SystemPrompt))
		mock.ExpectExec("INSERT INTO kb.agentic_service_profile_versions").WithArgs(slug, "v1", sqlmock.AnyArg(), profile.PromptFile, hex.EncodeToString(promptHash[:]), profile.SystemPrompt).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO kb.agentic_service_profiles").WithArgs(slug, "v1").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	if err := store.EnsureSeeded(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreEnsureSeededSkipsWhenAnyProfileHasVersions(t *testing.T) {
	store, mock := profileStoreFixture(t)
	registry := profileRegistryFixture(t)
	// A version for just one slug means the persisted store is authoritative;
	// initialization must not fill in the other slug from environment defaults.
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT pg_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_xact_lock"}).AddRow(nil))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectCommit()
	if err := store.EnsureSeeded(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreEnsureSeededRollsBackPartialInitialization(t *testing.T) {
	store, mock := profileStoreFixture(t)
	registry := profileRegistryFixture(t)
	first := registry.versions["knowledge-guide"][registry.active["knowledge-guide"]]
	firstHash := sha256.Sum256([]byte(first.SystemPrompt))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT pg_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_xact_lock"}).AddRow(nil))
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec("INSERT INTO kb.agentic_service_profile_versions").WithArgs(first.Slug, "v1", sqlmock.AnyArg(), first.PromptFile, hex.EncodeToString(firstHash[:]), first.SystemPrompt).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.agentic_service_profiles").WithArgs(first.Slug, "v1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO kb.agentic_service_profile_versions").WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()
	if err := store.EnsureSeeded(context.Background(), registry); err == nil {
		t.Fatal("EnsureSeeded() error = nil, want failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreListActive(t *testing.T) {
	store, mock := profileStoreFixture(t)
	profile := AgentServiceProfile{Slug: "knowledge-guide", Version: "v2", FriendlyName: "Guide", SystemPrompt: "saved prompt", PilotUsers: []string{"alice"}}
	data, err := jsonMarshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT v.profile_json, v.prompt_snapshot").WillReturnRows(sqlmock.NewRows([]string{"profile_json", "prompt_snapshot"}).AddRow(data, "saved prompt"))
	got, err := store.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Version != "v2" || got[0].SystemPrompt != "saved prompt" || len(got[0].PilotUsers) != 1 || got[0].PilotUsers[0] != "alice" {
		t.Fatalf("ListActive() = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreLoadVersionUsesStoredPromptSnapshot(t *testing.T) {
	store, mock := profileStoreFixture(t)
	profile := AgentServiceProfile{Slug: "problem-diagnostics", Version: "v1", PromptFile: "removed-or-changed.md", PilotUsers: []string{"alice"}}
	data, err := jsonMarshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT profile_json, prompt_snapshot").WithArgs(profile.Slug, profile.Version).WillReturnRows(sqlmock.NewRows([]string{"profile_json", "prompt_snapshot"}).AddRow(data, "historical prompt"))
	got, err := store.LoadVersion(context.Background(), profile.Slug, profile.Version)
	if err != nil {
		t.Fatal(err)
	}
	if got.SystemPrompt != "historical prompt" || len(got.PilotUsers) != 1 || got.PilotUsers[0] != "alice" {
		t.Fatalf("loaded historical profile = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreActivateVersionRejectsStaleExpectedVersion(t *testing.T) {
	store, mock := profileStoreFixture(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT active_version FROM kb.agentic_service_profiles").WithArgs("knowledge-guide").WillReturnRows(sqlmock.NewRows([]string{"active_version"}).AddRow("v3"))
	mock.ExpectRollback()
	_, _, err := store.ActivateVersion(context.Background(), AgentServiceProfile{Slug: "knowledge-guide"}, "v2", "user-1")
	if !errors.Is(err, ErrProfileVersionConflict) {
		t.Fatalf("ActivateVersion() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreActivateVersionCommitsNextImmutableVersion(t *testing.T) {
	store, mock := profileStoreFixture(t)
	profile := AgentServiceProfile{
		Slug: "knowledge-guide", FriendlyName: "Updated Guide", PromptFile: "prompt-v2.md",
		SystemPrompt: "resolved prompt v2", Enabled: true, PilotUsers: []string{"alice"},
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT active_version FROM kb.agentic_service_profiles WHERE service_slug=\\$1 FOR UPDATE").
		WithArgs("knowledge-guide").
		WillReturnRows(sqlmock.NewRows([]string{"active_version"}).AddRow("v3"))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(SUBSTRING\\(version FROM 2\\)::integer\\), 0\\)").
		WithArgs("knowledge-guide").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(3))
	profile.Version = "v4"
	profileJSON, err := marshalStoredProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(profile.SystemPrompt))
	mock.ExpectExec("INSERT INTO kb.agentic_service_profile_versions").
		WithArgs("knowledge-guide", "v4", profileJSON, profile.PromptFile, hex.EncodeToString(hash[:]), profile.SystemPrompt, "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE kb.agentic_service_profiles SET active_version=\\$2, updated_at=NOW()").
		WithArgs("knowledge-guide", "v4").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	got, previous, err := store.ActivateVersion(context.Background(), AgentServiceProfile{
		Slug: "knowledge-guide", FriendlyName: "Updated Guide", PromptFile: "prompt-v2.md",
		SystemPrompt: "resolved prompt v2", Enabled: true, PilotUsers: []string{"alice"},
	}, "v3", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "v4" || previous != "v3" {
		t.Fatalf("ActivateVersion() = version %q, previous %q; want v4, v3", got.Version, previous)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreRestoreActiveVersionIsConditional(t *testing.T) {
	t.Run("restores just-activated version", func(t *testing.T) {
		store, mock := profileStoreFixture(t)
		mock.ExpectExec("UPDATE kb.agentic_service_profiles").WithArgs("knowledge-guide", "v2", "v1").WillReturnResult(sqlmock.NewResult(0, 1))
		if err := store.RestoreActiveVersion(context.Background(), "knowledge-guide", "v2", "v1"); err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("conflicts when active version has moved", func(t *testing.T) {
		store, mock := profileStoreFixture(t)
		mock.ExpectExec("UPDATE kb.agentic_service_profiles").WithArgs("knowledge-guide", "v2", "v1").WillReturnResult(sqlmock.NewResult(0, 0))
		if err := store.RestoreActiveVersion(context.Background(), "knowledge-guide", "v2", "v1"); !errors.Is(err, ErrProfileVersionConflict) {
			t.Fatalf("RestoreActiveVersion() error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

// jsonMarshal keeps test setup focused on the store behavior.
func jsonMarshal(profile AgentServiceProfile) ([]byte, error) { return marshalStoredProfile(profile) }
