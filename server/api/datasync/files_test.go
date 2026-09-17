package datasync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFileDirPrefersEnvOverDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TEST_FILE_DIR_VAR", dir)
	item := TableSyncItem{ID: "x", FileDirEnv: "TEST_FILE_DIR_VAR", FileDirDefaultSubdir: "unused"}
	got, err := resolveFileDir(item)
	if err != nil || got != dir {
		t.Fatalf("resolveFileDir() = (%q, %v), want (%q, nil)", got, err, dir)
	}
}

func TestResolveFileDirFallsBackToDataHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DATA_HOME_DIR", home)
	item := TableSyncItem{ID: "x", FileDirDefaultSubdir: "Videos"}
	got, err := resolveFileDir(item)
	want := filepath.Join(home, "Videos")
	if err != nil || got != want {
		t.Fatalf("resolveFileDir() = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestResolveFileDirErrorsWhenNothingConfigured(t *testing.T) {
	item := TableSyncItem{ID: "x"}
	if _, err := resolveFileDir(item); err == nil {
		t.Fatalf("resolveFileDir() error = nil, want an error when neither FileDirEnv nor FileDirDefaultSubdir is set")
	}
}

func TestMaterializeFilesCopiesAndRewritesFileColumn(t *testing.T) {
	const fileBody = "fake video bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(fileBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("DATA_HOME_DIR", dir)
	item := TableSyncItem{
		ID:                   "mat_test",
		Kind:                 KindTableWithFiles,
		NaturalKey:           []string{"stored_path"},
		FileColumn:           "stored_path",
		FileDirDefaultSubdir: "Videos",
	}

	row := Row{"stored_path": jsonStr(t, "/mac-only/path/clip_1.mp4")}
	rows := []Row{row}

	if err := materializeFiles(context.Background(), item, srv.URL, "test-secret", rows); err != nil {
		t.Fatalf("materializeFiles() error = %v", err)
	}

	var newPath string
	if err := json.Unmarshal(row["stored_path"], &newPath); err != nil {
		t.Fatalf("decode rewritten stored_path: %v", err)
	}
	wantDir := filepath.Join(dir, "Videos")
	if !strings.HasPrefix(newPath, wantDir) {
		t.Fatalf("rewritten stored_path = %q, want it under %q", newPath, wantDir)
	}
	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read materialized file: %v", err)
	}
	if string(got) != fileBody {
		t.Fatalf("materialized file content = %q, want %q", got, fileBody)
	}
}

func TestMaterializeFilesSkipsEmptyFileColumn(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte("should not be fetched"))
	}))
	defer srv.Close()

	t.Setenv("DATA_HOME_DIR", t.TempDir())
	item := TableSyncItem{
		ID:                   "mat_test_empty",
		Kind:                 KindTableWithFiles,
		NaturalKey:           []string{"stored_path"},
		FileColumn:           "stored_path",
		FileDirDefaultSubdir: "Videos",
	}

	row := Row{"stored_path": nullJSON}
	if err := materializeFiles(context.Background(), item, srv.URL, "test-secret", []Row{row}); err != nil {
		t.Fatalf("materializeFiles() error = %v", err)
	}
	if called {
		t.Fatalf("source file endpoint was called for a row with an empty file column")
	}
	if string(row["stored_path"]) != string(nullJSON) {
		t.Fatalf("row was modified for a row with an empty file column: %s", row["stored_path"])
	}
}

func TestMaterializeFilesReturnsErrorOnFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("DATA_HOME_DIR", t.TempDir())
	item := TableSyncItem{
		ID:                   "mat_test_fail",
		Kind:                 KindTableWithFiles,
		NaturalKey:           []string{"stored_path"},
		FileColumn:           "stored_path",
		FileDirDefaultSubdir: "Videos",
	}

	row := Row{"stored_path": jsonStr(t, "/mac-only/path/clip_1.mp4")}
	err := materializeFiles(context.Background(), item, srv.URL, "test-secret", []Row{row})
	if err == nil {
		t.Fatalf("materializeFiles() error = nil, want an error when the source returns a failure")
	}
}
