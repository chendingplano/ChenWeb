package agentservicehandler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	for _, slug := range []string{"knowledge-guide", "problem-diagnostics"} {
		profile := registry.versions[slug][registry.active[slug]]
		promptHash := sha256.Sum256([]byte(profile.SystemPrompt))
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT EXISTS").WithArgs(slug).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("INSERT INTO kb.agentic_service_profile_versions").WithArgs(slug, "v1", sqlmock.AnyArg(), profile.PromptFile, hex.EncodeToString(promptHash[:]), profile.SystemPrompt).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO kb.agentic_service_profiles").WithArgs(slug, "v1").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
	}
	if err := store.EnsureSeeded(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreEnsureSeededRepeatDoesNotOverwrite(t *testing.T) {
	store, mock := profileStoreFixture(t)
	registry := profileRegistryFixture(t)
	for i := 0; i < 2; i++ {
		for _, slug := range []string{"knowledge-guide", "problem-diagnostics"} {
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT EXISTS").WithArgs(slug).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(i == 1))
			if i == 0 {
				mock.ExpectExec("INSERT INTO kb.agentic_service_profile_versions").WithArgs(slug, "v1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("INSERT INTO kb.agentic_service_profiles").WithArgs(slug, "v1").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			mock.ExpectCommit()
		}
		if err := store.EnsureSeeded(context.Background(), registry); err != nil {
			t.Fatal(err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreListActive(t *testing.T) {
	store, mock := profileStoreFixture(t)
	profile := AgentServiceProfile{Slug: "knowledge-guide", Version: "v2", FriendlyName: "Guide", SystemPrompt: "saved prompt"}
	data, err := jsonMarshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT v.profile_json, v.prompt_snapshot").WillReturnRows(sqlmock.NewRows([]string{"profile_json", "prompt_snapshot"}).AddRow(data, "saved prompt"))
	got, err := store.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Version != "v2" || got[0].SystemPrompt != "saved prompt" {
		t.Fatalf("ListActive() = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileStoreLoadVersionUsesStoredPromptSnapshot(t *testing.T) {
	store, mock := profileStoreFixture(t)
	profile := AgentServiceProfile{Slug: "problem-diagnostics", Version: "v1", PromptFile: "removed-or-changed.md"}
	data, err := jsonMarshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT profile_json, prompt_snapshot").WithArgs(profile.Slug, profile.Version).WillReturnRows(sqlmock.NewRows([]string{"profile_json", "prompt_snapshot"}).AddRow(data, "historical prompt"))
	got, err := store.LoadVersion(context.Background(), profile.Slug, profile.Version)
	if err != nil {
		t.Fatal(err)
	}
	if got.SystemPrompt != "historical prompt" {
		t.Fatalf("prompt = %q", got.SystemPrompt)
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

// jsonMarshal keeps test setup focused on the store behavior.
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
