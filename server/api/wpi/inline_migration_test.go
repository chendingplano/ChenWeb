package wpi

import (
	"database/sql"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const inlineMigrationFile = "../../../project_migrations/20261102000002_wpi_inline_attribute_definitions.sql"

// Run with WPI_TEST_DB=1. The migration is exercised against a temporary table;
// no stored descriptors are changed by this test.
func TestInlineAttributeMigrationDatabase(t *testing.T) {
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
	if _, err := tx.Exec(`CREATE TEMP TABLE wpi_inline_descriptors (
		kind text, descriptor_id text, version int, schema_version text, body jsonb,
		status text, updated_by text, updated_at timestamptz,
		PRIMARY KEY(kind,descriptor_id,version))`); err != nil {
		t.Fatal(err)
	}
	raw := `{"descriptor_id":"button","schema_version":"0.4","code_module_id":"button","label":{"default_value":"Button","help_tip":{"en":"Caption","zh-cn":"标题"}},"on_click":{"default_value":null},"attribute_definitions":{"/label":{"scope":"instance","description":{"en":"Caption","zh-cn":"标题"},"value_editor":{"id":"localized-text","options":{"rows":2}}}}}`
	for version := 3; version <= 4; version++ {
		if _, err := tx.Exec(`INSERT INTO wpi_inline_descriptors VALUES ('element','button',$1,'0.4',$2,'active','before',NOW())`, version, raw); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(inlineMigrationFile)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(data), "-- +goose Down")
	up := strings.ReplaceAll(parts[0], "kb.wpi_descriptors", "pg_temp.wpi_inline_descriptors")
	down := strings.ReplaceAll(parts[1], "kb.wpi_descriptors", "pg_temp.wpi_inline_descriptors")
	check := func(inline bool) {
		t.Helper()
		var got []byte
		var version int
		var status string
		if err := tx.QueryRow(`SELECT version,status,body FROM wpi_inline_descriptors WHERE descriptor_id='button' AND version=4`).Scan(&version, &status, &got); err != nil {
			t.Fatal(err)
		}
		body := decode(t, string(got))
		label := body["label"].(map[string]any)
		if version != 4 || status != "active" || label["default_value"] != "Button" {
			t.Fatalf("record changed: %s", got)
		}
		help := label["help_tip"].(map[string]any)
		if help["en"] != "Caption" || help["zh-cn"] != "标题" {
			t.Fatalf("help changed: %s", got)
		}
		if inline {
			if label["scope"] != "instance" {
				t.Fatalf("scope missing: %s", got)
			}
			editor := label["value_editor"].(map[string]any)
			if editor["id"] != "localized-text" || editor["options"].(map[string]any)["rows"] != json.Number("2") {
				t.Fatalf("editor changed: %s", got)
			}
			if _, duplicate := body["attribute_definitions"].(map[string]any)["/label"]; duplicate {
				t.Fatalf("duplicate label settings: %s", got)
			}
			if issues := validateElementClass(body, false); len(issues) != 0 {
				t.Fatalf("invalid migrated class: %+v", issues)
			}
		} else if _, exists := label["scope"]; exists {
			t.Fatalf("Down retained inline scope: %s", got)
		}
		var historical []byte
		if err := tx.QueryRow(`SELECT body FROM wpi_inline_descriptors WHERE descriptor_id='button' AND version=3`).Scan(&historical); err != nil {
			t.Fatal(err)
		}
		if _, exists := decode(t, string(historical))["label"].(map[string]any)["scope"]; exists {
			t.Fatalf("historical version changed: %s", historical)
		}
	}
	for _, sqlText := range []string{up, up, down, up} {
		if _, err := tx.Exec(sqlText); err != nil {
			t.Fatal(err)
		}
		check(sqlText != down)
	}
}

// Read-back proof against the current stored class records after deployment.
func TestStoredInlineAttributeDefinitionsDatabase(t *testing.T) {
	if os.Getenv("WPI_TEST_DB") != "1" {
		t.Skip("set WPI_TEST_DB=1 for stored class checks")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT DISTINCT ON (descriptor_id) descriptor_id,status,body FROM kb.wpi_descriptors WHERE kind='element' AND status IN ('draft','active') AND schema_version='0.4' ORDER BY descriptor_id,version DESC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id string
		var status string
		var raw []byte
		if err := rows.Scan(&id, &status, &raw); err != nil {
			t.Fatal(err)
		}
		body := decode(t, string(raw))
		if issues := validateElementClass(body, status == "draft"); len(issues) != 0 {
			t.Fatalf("%s: %+v", id, issues)
		}
		for name, value := range body {
			if slices.Contains(classAttrs, name) {
				continue
			}
			attr, ok := value.(map[string]any)
			if !ok || attr["scope"] == nil || attr["value_editor"] == nil {
				t.Fatalf("%s.%s lacks stored settings", id, name)
			}
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no classes read")
	}
	t.Logf("validated %d latest stored element classes with inline definitions", count)
}

// Uses a unique temporary draft record to verify the real handlers and SQL agree.
// The record is deleted before the test returns; no user class is edited.
func TestInlineAttributeAPIDatabase(t *testing.T) {
	if os.Getenv("WPI_TEST_DB") != "1" {
		t.Skip("set WPI_TEST_DB=1 for real class API checks")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := "inline-editor-test-" + time.Now().Format("20060102150405.000000000")
	id = strings.ReplaceAll(id, ".", "-")
	defer func() {
		if _, err := db.Exec(`DELETE FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id=$1`, id); err != nil {
			t.Errorf("temporary class cleanup: %v", err)
		}
	}()
	body := map[string]any{"descriptor_id": id, "schema_version": "0.4", "description": "Temporary class API regression test", "code_module_id": "button",
		"label": map[string]any{"scope": "instance", "default_value": "Before", "value_editor": map[string]any{"id": "localized-text", "options": map[string]any{}}}}
	h := testHandler(db, adminUser)
	raw, _ := json.Marshal(body)
	if result := call(t, h.CreateElementClass, "POST", string(raw)); result.Code != 201 {
		t.Fatalf("create: %d %s", result.Code, result.Body)
	}
	body["label"] = map[string]any{"scope": "class", "instance_modifiable": false, "help_tip": map[string]any{"en": "Saved caption", "zh-cn": "已保存的标题"}, "default_value": "Saved default", "value_editor": map[string]any{"id": "text", "options": map[string]any{"commitOnBlur": true}}}
	raw, _ = json.Marshal(body)
	if result := call(t, h.UpdateClass, "PUT", string(raw), "kind", "element", "classId", id); result.Code != 200 {
		t.Fatalf("update: %d %s", result.Code, result.Body)
	}
	var stored []byte
	var version int
	var status string
	if err := db.QueryRow(`SELECT version,status,body FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id=$1`, id).Scan(&version, &status, &stored); err != nil {
		t.Fatal(err)
	}
	if version != 1 || status != "draft" || !reflect.DeepEqual(decode(t, string(stored)), decode(t, string(raw))) {
		t.Fatalf("stored body differs: %s", stored)
	}
	result := call(t, h.GetClass, "GET", "", "kind", "element", "classId", id)
	if result.Code != 200 {
		t.Fatalf("read: %d %s", result.Code, result.Body)
	}
	var response ClassRow
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decode(t, string(response.Class)), decode(t, string(stored))) {
		t.Fatalf("GET body differs: %s", result.Body)
	}
	t.Logf("real API PUT, SQL body and GET match; label=%s", mustJSON(body["label"]))
}

func mustJSON(value any) string { raw, _ := json.Marshal(value); return string(raw) }
