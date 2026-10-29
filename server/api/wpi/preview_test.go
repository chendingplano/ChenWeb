package wpi

import (
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"strings"
	"testing"
)

func TestPreviewListReadsUnsavedSource(t *testing.T) {
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
	h := testHandler(project, adminUser)
	h.Databases = map[string]*sql.DB{"shared": shared}
	sharedMock.ExpectQuery(catalogQuery).WithArgs("kb", "people").WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("name"))
	sharedMock.ExpectQuery(`SELECT "name" FROM "kb"\."people" LIMIT \$1`).WithArgs(200).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow("alice"))
	rec := call(t, h.PreviewList, "POST", `{"source":{"action":"load","table":"shared.kb.people","result":"list","fields":[{"column":"name","attribute":"name"}]}}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"alice"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if err := projectMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if err := sharedMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewListRejectsNonAdminAndWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if rec := call(t, testHandler(db, plainUser).PreviewList, "POST", `{}`); rec.Code != 403 {
		t.Fatalf("expected 403: %s", rec.Body.String())
	}
	if rec := call(t, testHandler(db, adminUser).PreviewList, "POST", `{"source":{"action":"save"}}`); rec.Code != 400 {
		t.Fatalf("expected 400: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewListRejectsInvalidAndUnlistedSources(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h := testHandler(db, adminUser)
	for _, body := range []string{
		`{"source":{"action":"query","sql_stmt_id":"test","result":"list"}}`,
		`{"source":{"action":"load","table":"kb.people;DROP","result":"list","fields":[{"column":"name","attribute":"name"}]}}`,
		`{"source":{"action":"load","table":"private.kb.people","result":"list","fields":[{"column":"name","attribute":"name"}]}}`,
		`{"source":{"action":"load","table":"kb.people","result":"list","fields":[]}}`,
	} {
		rec := call(t, h.PreviewList, "POST", body)
		if rec.Code != 400 {
			t.Fatalf("source %s: expected 400, got %d %s", body, rec.Code, rec.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
