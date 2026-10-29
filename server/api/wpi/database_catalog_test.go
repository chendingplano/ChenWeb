package wpi

import (
	"context"
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestConfiguredDatabaseTableResolution(t *testing.T) {
	project, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer project.Close()
	shared, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	h := New(project)
	h.Databases = map[string]*sql.DB{"shared": shared}
	mock.ExpectQuery("SELECT column_name FROM information_schema.columns").WithArgs("kb", "people").WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("id"))
	cols, err := h.columns(context.Background(), "shared.kb.people")
	if err != nil || !cols["id"] {
		t.Fatalf("columns=%v err=%v", cols, err)
	}
	if _, _, err := h.tableDatabase("other.kb.people"); err == nil {
		t.Fatal("unconfigured database accepted")
	}
	if db, table, err := h.tableDatabase("kb.people"); err != nil || db != project || table != "kb.people" {
		t.Fatal("legacy table did not resolve to project database")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredDatabaseListUsesItsPool(t *testing.T) {
	project, projectMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer project.Close()
	shared, sharedMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	h := testHandler(project, plainUser)
	h.Databases = map[string]*sql.DB{"shared": shared}
	stored(projectMock, KindPage, "customers", strings.ReplaceAll(customersPage, "kb.wpi_demo_customers", "shared.kb.wpi_demo_customers"))
	storedClass(projectMock, "load")
	customerColumns(sharedMock)
	sharedMock.ExpectQuery(`ORDER BY`).WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(2, "a@example.com"))
	runAction(t, projectMock, h, `{`+loadListRef+`}`, 200, `"id":2`)
	if err := sharedMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogOnlyExposesConfiguredDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := testHandler(db, plainUser)
	h.DatabaseNames = []string{"miner", "shared"}
	h.Databases = map[string]*sql.DB{"shared": db}
	rec := call(t, h.TableCatalog, "GET", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"miner"`) || !strings.Contains(rec.Body.String(), `"shared"`) {
		t.Fatal(rec.Body.String())
	}
	mock.ExpectQuery("SELECT table_schema, table_name").WillReturnRows(sqlmock.NewRows([]string{"table_schema", "table_name"}).AddRow("kb", "items"))
	e := echo.New()
	rec = httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest("GET", "/?database=shared", nil), rec)
	if err := h.TableCatalog(c); err != nil || rec.Code != 200 || !strings.Contains(rec.Body.String(), `"table":"items"`) {
		t.Fatalf("%v %s", err, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	c = e.NewContext(httptest.NewRequest("GET", "/?database=other", nil), rec)
	if err := h.TableCatalog(c); err != nil || rec.Code != 503 {
		t.Fatalf("%v %s", err, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTableColumnsUsesSelectedDatabaseAndColumnOrder(t *testing.T) {
	project, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer project.Close()
	shared, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	h := testHandler(project, plainUser)
	h.Databases = map[string]*sql.DB{"shared": shared}
	mock.ExpectQuery("SELECT column_name FROM information_schema.columns").WithArgs("kb", "people").WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("id").AddRow("name"))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest("GET", "/?table=shared.kb.people", nil), rec)
	if err := h.TableColumns(c); err != nil || rec.Code != 200 || !strings.Contains(rec.Body.String(), `"columns":["id","name"]`) {
		t.Fatalf("%v %s", err, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTableColumnsRejectsUnconfiguredOrInvalidReference(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := testHandler(db, plainUser)
	for _, table := range []string{"other.kb.people", "kb.people;DROP"} {
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest("GET", "/?table="+url.QueryEscape(table), nil), rec)
		if err := h.TableColumns(c); err != nil || rec.Code != 400 {
			t.Fatalf("table %q: %v %s", table, err, rec.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
