package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/documentgeometry"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestGetTableGeometryInvalidID(t *testing.T) {
	e := echo.New()
	r := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), r)
	c.SetParamNames("id")
	c.SetParamValues("bad")
	if err := GetTableGeometry(c); err != nil {
		t.Fatal(err)
	}
	if r.Code != 400 {
		t.Fatalf("status=%d", r.Code)
	}
}
func TestGetTableGeometryMissingCompanionReturnsEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	dir := t.TempDir()
	result := filepath.Join(dir, "sample_mineru.json")
	os.WriteFile(filepath.Join(dir, "sample_mineru.txt"), []byte("1\t1\tparagraph\tf\t12\t[0,0,1,1]\ttext\n"), 0600)
	expectResolveInputTablePlural(mock)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT i.result_filename, i.file_name FROM kb.inputs i WHERE i.id = $1")).WithArgs(int64(416)).WillReturnRows(sqlmock.NewRows([]string{"result_filename", "file_name"}).AddRow(result, filepath.Join(dir, "sample.pdf")))
	e := echo.New()
	r := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), r)
	c.SetParamNames("id")
	c.SetParamValues("416")
	if err := GetTableGeometry(c); err != nil {
		t.Fatal(err)
	}
	if r.Code != 200 {
		t.Fatalf("response=%s", r.Body.String())
	}
	var got documentgeometry.Document
	if err = json.Unmarshal(r.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.Tables) != 0 {
		t.Fatalf("geometry=%+v", got)
	}
	if r.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("geometry must be revalidated after regeneration")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
