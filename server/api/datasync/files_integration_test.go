package datasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scratchFileTable(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id          BIGSERIAL PRIMARY KEY,
			stored_path TEXT NOT NULL,
			name        TEXT NOT NULL DEFAULT '',
			update_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (stored_path)
		)`, name))
	if err != nil {
		t.Fatalf("create scratch file table %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP TABLE IF EXISTS " + name); err != nil {
			t.Logf("cleanup: drop table %s: %v", name, err)
		}
	})
}

func fileItemFor(table string) TableSyncItem {
	return TableSyncItem{
		ID:         "test_file_item",
		Table:      table,
		CursorCol:  "update_time",
		NaturalKey: []string{"stored_path"},
		Columns:    []string{"stored_path", "name", "update_time"},
		Kind:       KindTableWithFiles,
		FileColumn: "stored_path",
	}
}

// TestApplyTableWithFilesEndToEndAgainstRealPostgres exercises the whole
// table_with_files pipeline against a real database: a row on a "source"
// scratch table points at a real file on disk; fetching + materializing +
// upserting must land a copy of that file under the target's configured
// directory and rewrite the upserted row's file column to point at it.
//
// This calls fetchChangesPage/materializeFiles/upsertRows directly rather
// than going through HandleApplySync/ResolveItem, the same way
// TestUpsertRowsAgainstRealPostgres bypasses HandleApplySync -- source and
// target here share one physical TEST_DATABASE_URL database under
// differently-named scratch tables, so there's no way to give the same
// itemId two different Table values through the shared global registry the
// way two genuinely separate ChenWeb instances would. HandleGetItemFile
// itself (the real handler, auth included) is covered separately by
// TestFetchSourceItemDefinitionsRoundTrip-style single-role tests; this test
// stands in a minimal equivalent handler backed directly by sourceItem/db.
func TestApplyTableWithFilesEndToEndAgainstRealPostgres(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sourceTable := fmt.Sprintf("public.datasync_it_files_src_%d", time.Now().UnixNano())
	targetTable := fmt.Sprintf("public.datasync_it_files_tgt_%d", time.Now().UnixNano())
	scratchFileTable(t, db, sourceTable)
	scratchFileTable(t, db, targetTable)
	sourceItem := fileItemFor(sourceTable)

	sourceDir := t.TempDir()
	sourceFilePath := filepath.Join(sourceDir, "clip_1.mp4")
	const fileBody = "real bytes on the source filesystem"
	if err := os.WriteFile(sourceFilePath, []byte(fileBody), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	_, err := db.Exec(fmt.Sprintf(`INSERT INTO %s (stored_path, name) VALUES ($1, $2)`, sourceTable),
		sourceFilePath, "clip one")
	if err != nil {
		t.Fatalf("seed source row: %v", err)
	}

	fileSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var keyValues []string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("key")), &keyValues); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		path, err := lookupItemFilePath(db, sourceItem, keyValues)
		if err != nil || path == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		defer f.Close()
		_, _ = io.Copy(w, f)
	}))
	defer fileSrv.Close()

	page, err := fetchChangesPage(db, sourceItem, "", 100)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1", len(page.Rows))
	}

	targetDir := t.TempDir()
	targetItem := fileItemFor(targetTable)
	t.Setenv("TEST_TARGET_FILE_DIR", targetDir)
	targetItem.FileDirEnv = "TEST_TARGET_FILE_DIR"

	if err := materializeFiles(ctx, targetItem, fileSrv.URL, "test-secret", page.Rows); err != nil {
		t.Fatalf("materializeFiles() error = %v", err)
	}
	if err := upsertRows(ctx, db, targetItem, page.Rows); err != nil {
		t.Fatalf("upsertRows() error = %v", err)
	}

	var newStoredPath, name string
	err = db.QueryRow(fmt.Sprintf("SELECT stored_path, name FROM %s", targetTable)).Scan(&newStoredPath, &name)
	if err != nil {
		t.Fatalf("read target row: %v", err)
	}
	if name != "clip one" {
		t.Fatalf("name = %q, want %q", name, "clip one")
	}
	if !strings.HasPrefix(newStoredPath, targetDir) {
		t.Fatalf("stored_path = %q, want it rewritten under %q", newStoredPath, targetDir)
	}
	if newStoredPath == sourceFilePath {
		t.Fatalf("stored_path was not rewritten -- still points at the source path %q", sourceFilePath)
	}
	got, err := os.ReadFile(newStoredPath)
	if err != nil {
		t.Fatalf("read materialized file at %q: %v", newStoredPath, err)
	}
	if string(got) != fileBody {
		t.Fatalf("materialized file content = %q, want %q", got, fileBody)
	}
}
