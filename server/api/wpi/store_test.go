package wpi

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

const latestAllQuery = `SELECT DISTINCT ON \(descriptor_id\) descriptor_id, version, body FROM kb\.wpi_descriptors WHERE kind = \$1 AND status = 'active' ORDER BY descriptor_id, version DESC`

const latestQuery = `SELECT DISTINCT ON \(descriptor_id\) descriptor_id, version, body FROM kb\.wpi_descriptors WHERE kind = \$1 AND status = 'active' AND descriptor_id = ANY\(\$2\) ORDER BY descriptor_id, version DESC`

const latestAppsQuery = `SELECT DISTINCT ON \(descriptor_id\) descriptor_id, version, body FROM kb\.wpi_apps WHERE status = 'active' AND descriptor_id = ANY\(\$1\) ORDER BY descriptor_id, version DESC`

const latestPagesQuery = `SELECT DISTINCT ON \(descriptor_id\) descriptor_id, version, body FROM kb\.frontend_pages WHERE status = 'active' AND descriptor_id = ANY\(\$1\) ORDER BY descriptor_id, version DESC`

func TestStoreLatest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The query returns one row per descriptor (DISTINCT ON); drafts and
	// older versions are filtered out by SQL, so only version 2 arrives here.
	mock.ExpectQuery(latestPagesQuery).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "body"}).
			AddRow("customer-form", 2, []byte(`{"descriptor_id": "customer-form", "limit": 5}`)))
	got, err := store{db}.latest(context.Background(), KindPage, []string{"customer-form", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := got["customer-form"]
	if !ok || d.Version != 2 || d.Kind != KindPage || len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if n, _ := asInt(d.Body["limit"]); n != 5 {
		t.Fatalf("numbers must decode as json.Number: %#v", d.Body["limit"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreLatestAll(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(latestAllQuery).WithArgs("element").
		WillReturnRows(descriptorRows([2]string{"input", `{}`}, [2]string{"text", `{}`}))
	got, err := store{db}.latest(context.Background(), KindElement, nil)
	if err != nil || len(got) != 2 || got["text"].Kind != KindElement {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreLatestRejectsNonObjectBody(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(latestPagesQuery).WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "body"}).AddRow("x", 1, []byte(`null`)))
	if _, err := (store{db}).latest(context.Background(), KindPage, []string{"x"}); err == nil {
		t.Fatal("a null body must be an error")
	}
}

const insertDraftQuery = `INSERT INTO kb\.wpi_descriptors .+ SELECT 'element', \$1::text, 1, .+ 'draft', .+ WHERE NOT EXISTS \(SELECT 1 FROM kb\.wpi_descriptors WHERE kind = 'element' AND descriptor_id = \$1::text\)`

const elementClassesQuery = `SELECT DISTINCT ON \(descriptor_id\) descriptor_id, version, status, body\s+FROM kb\.wpi_descriptors WHERE kind = 'element' AND status IN \('draft', 'active'\) ORDER BY descriptor_id, version DESC`

func TestStoreInsertDraftClass(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	ctx := context.Background()
	mock.ExpectExec(insertDraftQuery).WithArgs("slider", ElementSchemaVersion, `{"a":1}`, "cding@example.com").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := (store{db}).insertDraftClass(ctx, "slider", []byte(`{"a":1}`), "cding@example.com"); err != nil {
		t.Fatal(err)
	}
	// A row of the ID exists: nothing inserted.
	mock.ExpectExec(insertDraftQuery).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := (store{db}).insertDraftClass(ctx, "input", []byte(`{}`), "x"); !errors.Is(err, errClassExists) {
		t.Fatalf("existing ID: got %v", err)
	}
	// A concurrent create won the race.
	mock.ExpectExec(insertDraftQuery).WillReturnError(&pq.Error{Code: "23505"})
	if err := (store{db}).insertDraftClass(ctx, "slider", []byte(`{}`), "x"); !errors.Is(err, errClassExists) {
		t.Fatalf("unique violation: got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreElementClasses(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(elementClassesQuery).WillReturnRows(sqlmock.NewRows([]string{"descriptor_id", "version", "status", "body"}).
		AddRow("input", 2, "active", []byte(`{"descriptor_id": "input", "schema_version": "0.3", "description": {"en": "A field"}, "label": {"default_value": null}}`)).
		AddRow("slider", 1, "draft", []byte(`{"descriptor_id": "slider", "schema_version": "0.3", "implement_notes": "x"}`)))
	got, err := store{db}.elementClasses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Status != "active" || got[0].Description.(map[string]any)["en"] != "A field" || got[1].Status != "draft" || got[1].Description != nil {
		t.Fatalf("got %+v", got)
	}
	// Only instance attributes: the class attributes stay out.
	if len(got[0].Attributes) != 1 || got[0].Attributes["label"].(map[string]any)["default_value"] != nil || len(got[1].Attributes) != 0 {
		t.Fatalf("attributes %+v, %+v", got[0].Attributes, got[1].Attributes)
	}
}

const elementClassQuery = `SELECT version, status, body FROM kb\.wpi_descriptors\s+WHERE kind = 'element' AND descriptor_id = \$1 AND status IN \('draft', 'active'\) ORDER BY version DESC LIMIT 1`

const updateClassQuery = `UPDATE kb\.wpi_descriptors SET schema_version = \$2::text, body = \$3::jsonb, updated_by = \$4::text, updated_at = NOW\(\) WHERE kind = 'element' AND descriptor_id = \$1::text AND version = \$5 AND status = \$6::text RETURNING version`

func TestStoreElementClass(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	ctx := context.Background()
	mock.ExpectQuery(elementClassQuery).WithArgs("slider").
		WillReturnRows(sqlmock.NewRows([]string{"version", "status", "body"}).AddRow(2, "draft", []byte(`{"a":1}`)))
	got, found, err := store{db}.elementClass(ctx, "slider")
	if err != nil || !found || got.Version != 2 || got.Status != "draft" || string(got.Class) != `{"a":1}` {
		t.Fatalf("got %+v, %v, %v", got, found, err)
	}
	mock.ExpectQuery(elementClassQuery).WithArgs("missing").WillReturnRows(sqlmock.NewRows([]string{"version", "status", "body"}))
	if _, found, err := (store{db}).elementClass(ctx, "missing"); found || err != nil {
		t.Fatalf("missing class: found %v, %v", found, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreUpdateClass(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(updateClassQuery).WithArgs("slider", ElementSchemaVersion, `{"a":1}`, "cding@example.com", 2, "draft").
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(2))
	v, err := store{db}.updateClass(context.Background(), "slider", ClassRow{Version: 2, Status: "draft"}, []byte(`{"a":1}`), "cding@example.com")
	if err != nil || v != 2 {
		t.Fatalf("got %d, %v", v, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreUpdateClassMissing(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(updateClassQuery).WithArgs("slider", ElementSchemaVersion, `{"a":1}`, "cding@example.com", 2, "draft").
		WillReturnRows(sqlmock.NewRows([]string{"version"}))
	_, err := store{db}.updateClass(context.Background(), "slider", ClassRow{Version: 2, Status: "draft"}, []byte(`{"a":1}`), "cding@example.com")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected missing-row error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPageSaveArchivesCurrentVersion(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs("customers").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO archive\.frontend_pages`).WithArgs("customers").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`UPDATE kb\.frontend_pages`).WithArgs("customers", SchemaVersion, `{"a":1}`, "editor").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))
	mock.ExpectExec(`DELETE FROM archive\.frontend_pages`).WithArgs("customers", "editor").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	v, err := (store{db}).savePage(context.Background(), "customers", []byte(`{"a":1}`), "editor", "", false)
	if err != nil || v != 3 {
		t.Fatalf("got version %d, error %v", v, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPageSaveRollsBackIfArchiveFails(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs("customers").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO archive\.frontend_pages`).WithArgs("customers").WillReturnError(errors.New("archive failed"))
	mock.ExpectRollback()
	if _, err := (store{db}).savePage(context.Background(), "customers", []byte(`{}`), "editor", "", false); err == nil {
		t.Fatal("expected archive error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// activeModulesQuery finds which code modules element classes may name: active
// element modules in kb.code_modules (OpenSpec wpi-code-modules).
const activeModulesQuery = `SELECT code_module_id FROM kb\.code_modules WHERE module_type = 'element' AND status = 'active' AND code_module_id = ANY\(\$1\)`

// typedModulesQuery finds the active code modules of a module type.
func typedModulesQuery(moduleType string) string {
	return `SELECT code_module_id FROM kb\.code_modules WHERE module_type = '` + moduleType + `' AND status = 'active' AND code_module_id = ANY\(\$1\)`
}

const listCodeModulesQuery = `SELECT code_module_id, module_name, description, module_type, module_doc, notes, status\s+FROM kb\.code_modules ORDER BY module_type, code_module_id`

func TestStoreCodeModules(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	ctx := context.Background()
	mock.ExpectQuery(activeModulesQuery).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"code_module_id"}).AddRow("button"))
	found, err := store{db}.registered(ctx, storedModule, []string{"button", "slider"})
	if err != nil || !found["button"] || found["slider"] {
		t.Fatalf("found %v, %v", found, err)
	}
	mock.ExpectQuery(listCodeModulesQuery).WillReturnRows(sqlmock.NewRows([]string{"code_module_id", "module_name", "description", "module_type", "module_doc", "notes", "status"}).
		AddRow("button", "Button", "A button", "element", "button.md", "", "active"))
	modules, err := store{db}.listCodeModules(ctx)
	if err != nil || len(modules) != 1 || modules[0] != (CodeModule{"button", "Button", "A button", "element", "button.md", "", "active"}) {
		t.Fatalf("modules %+v, %v", modules, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
