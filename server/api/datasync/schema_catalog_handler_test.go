package datasync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListTablesIncludesCurrentDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = previous }()
	mock.ExpectQuery("SELECT current_database\\(\\)").WillReturnRows(sqlmock.NewRows([]string{"current_database"}).AddRow("project"))
	mock.ExpectQuery("SELECT table_schema, table_name").WillReturnRows(sqlmock.NewRows([]string{"table_schema", "table_name"}).AddRow("kb", "people"))
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	if err := HandleListSyncableTables(ctx); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Database string     `json:"database"`
		Tables   []tableRef `json:"tables"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || body.Database != "project" || len(body.Tables) != 1 || body.Tables[0] != (tableRef{Schema: "kb", Table: "people"}) {
		t.Fatalf("unexpected response %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
