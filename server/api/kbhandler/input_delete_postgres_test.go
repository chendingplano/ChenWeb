package kbhandler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Use an empty disposable database: this exercises actual FK cascades, transaction-local
// counters and durable audit SQL, without depending on the application's staging database.
func TestDeleteInputPostgres(t *testing.T) {
	dsn := os.Getenv("KB_INPUT_DELETE_TEST_DSN")
	if dsn == "" {
		t.Skip("set KB_INPUT_DELETE_TEST_DSN to an empty disposable PostgreSQL database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'kb')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("integration test requires a disposable database without a kb schema")
	}
	if _, err := db.Exec(`CREATE SCHEMA kb`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(`DROP SCHEMA kb CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	for _, stmt := range []string{
		`CREATE TABLE kb.inputs (id bigint PRIMARY KEY, file_name text, backup_filename text, result_filename text)`,
		`CREATE TABLE kb.doc_facets (record_id bigint REFERENCES kb.inputs ON DELETE CASCADE)`,
		`CREATE TABLE kb.delete_test_children (id bigint PRIMARY KEY, input_id bigint REFERENCES kb.inputs ON DELETE CASCADE)`,
		`CREATE TABLE kb.delete_test_grandchildren (child_id bigint REFERENCES kb.delete_test_children ON DELETE CASCADE)`,
		`CREATE TABLE kb.doc_proc_logs (id bigserial PRIMARY KEY, call_reason text, doc_proc_name text, entry_type text, record_id bigint, extra_info jsonb, log_loc text, errors text)`,
		`INSERT INTO kb.inputs VALUES (430, 'Artifacts/0/430/a.pdf', '', 'Artifacts/0/430/a.json')`,
		`INSERT INTO kb.doc_facets VALUES (430)`,
		`INSERT INTO kb.delete_test_children VALUES (1,430),(2,430)`,
		`INSERT INTO kb.delete_test_grandchildren VALUES (1),(2)`,
		`INSERT INTO kb.doc_proc_logs (entry_type, record_id, extra_info) VALUES ('delete_input',430,'{}'),('extract_metrics',430,'{}')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()
	home, archive, web := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("DATA_HOME_DIR", home)
	t.Setenv("ARTIFACT_DIR", filepath.Join(home, "Artifacts"))
	t.Setenv("ARCHIVE_DIR", archive)
	t.Setenv("ARTIFACT_WEB_DIR", web)
	pdf := filepath.Join(home, "Artifacts", "0", "430", "a.pdf")
	result := filepath.Join(filepath.Dir(pdf), "a.json")
	image := filepath.Join(filepath.Dir(pdf), "images", "p1.png")
	index, metadata := filepath.Join(web, "topic", "metrics.txt"), filepath.Join(web, "topic", "metadata.txt")
	for _, path := range []string{pdf, result, image, index, metadata} {
		writeTestFile(t, path)
	}
	if err := os.WriteFile(index, []byte("430_metric_1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, rec := newDeleteInputContext(t, "430")
	c.QueryParams().Set("delete_file", "true")
	if err := DeleteInput(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	var encoded []byte
	if err := db.QueryRow(`SELECT extra_info FROM kb.doc_proc_logs WHERE record_id=430 AND entry_type='delete_input' ORDER BY id DESC LIMIT 1`).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var info struct {
		Status   string              `json:"cleanup_status"`
		Archived string              `json:"archived_file"`
		Files    []string            `json:"deleted_files"`
		Tables   []inputDeletedTable `json:"deleted_tables"`
		Planned  []string            `json:"planned_files"`
	}
	if err := json.Unmarshal(encoded, &info); err != nil {
		t.Fatal(err)
	}
	if info.Status != "completed" {
		t.Fatalf("unfinished audit: %s", encoded)
	}
	for _, path := range []string{result, image, index, metadata} {
		if !slices.Contains(info.Files, path) {
			t.Fatalf("missing deleted file %s: %s", path, encoded)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("file remains %s: %v", path, err)
		}
	}
	for _, want := range []inputDeletedTable{{"kb.inputs", 1}, {"kb.doc_facets", 1}, {"kb.delete_test_children", 2}, {"kb.delete_test_grandchildren", 2}, {"kb.doc_proc_logs", 1}} {
		if !slices.Contains(info.Tables, want) {
			t.Fatalf("missing cascade count %+v: %s", want, encoded)
		}
	}
	if !slices.Contains(info.Planned, image) {
		t.Fatalf("final audit lost original cleanup plan: %s", encoded)
	}
	if body, err := os.ReadFile(info.Archived); err != nil || string(body) != "data" {
		t.Fatalf("archive: %q %v", body, err)
	}
	var auditCount int
	if err := db.QueryRow(`SELECT count(*) FROM kb.doc_proc_logs WHERE entry_type='delete_input'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("previous deletion audit was removed: got %d entries", auditCount)
	}
}
