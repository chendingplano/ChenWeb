package kbhandler

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// installPolicyDB swaps ApiTypes.ProjectDBHandle for a sqlmock-backed *sql.DB
// for the duration of the test, restoring the original on cleanup. Shared
// across handler tests that need a mocked DB (originally defined alongside
// the now-removed pipeline-policies handler tests).
func installPolicyDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	t.Cleanup(func() { ApiTypes.ProjectDBHandle = old; _ = db.Close() })
	return db, mock
}

// TestMain no-ops the pipeline_reload.go post-commit hooks for the whole
// package's test run: every pipeline/binding/rule mutating handler now
// calls reloadAfterPipelineWrite (ADR 2026081001 DR3 -- there is no more
// separate "activate a policy" step to hang a reload off of), which would
// otherwise require every such test to also mock the registry/binding/gate
// reload queries. Tests that specifically want to exercise reload/alarm
// behavior override these vars themselves and restore via t.Cleanup.
func TestMain(m *testing.M) {
	reloadProductionPipelineState = func(context.Context, *sql.DB) error { return nil }
	m.Run()
}
