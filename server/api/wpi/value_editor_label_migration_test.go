package wpi

import (
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"
)

const valueEditorLabelMigrationFile = "../../../project_migrations/20261104000004_wpi_value_editor_label_string_editor.sql"

// Run with WPI_TEST_DB=1. The stored element classes, other than this
// migration's own rows, are copied into a temporary table and the migration is
// exercised there; no stored descriptors are changed by this test.
func TestValueEditorLabelMigrationDatabase(t *testing.T) {
	if os.Getenv("WPI_TEST_DB") != "1" {
		t.Skip("set WPI_TEST_DB=1 for PostgreSQL migration checks")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TEMP TABLE wpi_label_descriptors AS
		SELECT kind, descriptor_id, version, schema_version, body, status, created_by, updated_by
		FROM kb.wpi_descriptors
		WHERE kind = 'element' AND created_by <> 'migration:wpi-value-editor-label-string-editor'`); err != nil {
		t.Fatal(err)
	}
	type row struct {
		version int
		status  string
		body    map[string]any
	}
	// latest returns each class's newest draft or active version, as served.
	latest := func() map[string]row {
		t.Helper()
		rows, err := tx.Query(`SELECT DISTINCT ON (descriptor_id) descriptor_id, version, status, body
			FROM wpi_label_descriptors WHERE status IN ('draft','active') ORDER BY descriptor_id, version DESC`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]row{}
		for rows.Next() {
			var id string
			var r row
			var raw []byte
			if err := rows.Scan(&id, &r.version, &r.status, &raw); err != nil {
				t.Fatal(err)
			}
			r.body = decode(t, string(raw))
			out[id] = r
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := latest()
	editors := 0
	for _, r := range before {
		if r.body["code_module_id"] == "value-editor" {
			editors++
		}
	}
	if editors == 0 {
		t.Fatal("no value-editor classes read")
	}
	data, err := os.ReadFile(valueEditorLabelMigrationFile)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(data), "-- +goose Down")
	up := strings.ReplaceAll(parts[0], "kb.wpi_descriptors", "pg_temp.wpi_label_descriptors")
	down := strings.ReplaceAll(parts[1], "kb.wpi_descriptors", "pg_temp.wpi_label_descriptors")
	check := func(applied bool) {
		t.Helper()
		after := latest()
		for id, old := range before {
			got := after[id]
			if old.body["code_module_id"] != "value-editor" || !applied {
				if got.version != old.version || !reflect.DeepEqual(got.body, old.body) {
					t.Fatalf("%s changed: version %d -> %d", id, old.version, got.version)
				}
				continue
			}
			if got.version != old.version+1 || got.status != old.status {
				t.Fatalf("%s: version %d %s, want %d %s", id, got.version, got.status, old.version+1, old.status)
			}
			editor := got.body["label"].(map[string]any)["value_editor"].(map[string]any)
			options := editor["options"].(map[string]any)
			if editor["id"] != "string-editor" || options["mode"] != "multilingual" ||
				!reflect.DeepEqual(options["languages"], []any{"en", "zh-cn"}) {
				t.Fatalf("%s: label editor %s", id, mustJSON(editor))
			}
			// Everything else in the class is carried over unchanged.
			want := decode(t, mustJSON(old.body))
			want["label"].(map[string]any)["value_editor"] = got.body["label"].(map[string]any)["value_editor"]
			if !reflect.DeepEqual(got.body, want) {
				t.Fatalf("%s: other attributes changed: %s", id, mustJSON(got.body))
			}
			if issues := validateElementClass(got.body, got.status == "draft"); len(issues) != 0 {
				t.Fatalf("%s: invalid migrated class: %+v", id, issues)
			}
		}
	}
	for _, sqlText := range []string{up, up, down, up} {
		if _, err := tx.Exec(sqlText); err != nil {
			t.Fatal(err)
		}
		check(sqlText != down)
	}
	t.Logf("checked %d value-editor classes among %d element classes", editors, len(before))
}
