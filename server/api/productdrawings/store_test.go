package productdrawings

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func rx(s string) string { return regexp.QuoteMeta(s) }

// installMockDB swaps ApiTypes.ProjectDBHandle for a sqlmock-backed *sql.DB
// for the duration of the test, restoring the original on cleanup.
func installMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	t.Cleanup(func() { ApiTypes.ProjectDBHandle = old; _ = db.Close() })
	return mock
}

func TestFindByNameNoMatch(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(rx("WHERE LOWER(TRIM(name)) = LOWER(TRIM($1))")).
		WithArgs("Ventilator").
		WillReturnError(sql.ErrNoRows)

	d, err := FindByName(context.Background(), "Ventilator")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if d != nil {
		t.Fatalf("d = %+v, want nil (no match)", d)
	}
}

func TestFindByNameMatchIgnoresCaseAndWhitespace(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(rx("WHERE LOWER(TRIM(name)) = LOWER(TRIM($1))")).
		WithArgs("  ventilator  ").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "prompt", "keywords", "notes", "filename", "model", "model_name", "created_at", "updated_at",
		}).AddRow(7, "Ventilator", "", "prompt", "", "", "ventilator-1.png", "Qwen", "wan2.2-t2i-flash", time.Now(), time.Now()))

	d, err := FindByName(context.Background(), "  ventilator  ")
	if err != nil {
		t.Fatalf("FindByName: %v", err)
	}
	if d == nil || d.ID != 7 || d.Name != "Ventilator" {
		t.Fatalf("d = %+v, want id 7 / name Ventilator", d)
	}
}

func TestFindByNameRequiresDB(t *testing.T) {
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = nil
	defer func() { ApiTypes.ProjectDBHandle = old }()

	if _, err := FindByName(context.Background(), "Ventilator"); err == nil {
		t.Fatal("FindByName with no DB configured: want error, got nil")
	}
}

func TestGenerateAndSaveInsertsRow(t *testing.T) {
	mock := installMockDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":[{"b64_json":"iVBORw0KGgo="}]}`)
	}))
	defer server.Close()
	t.Setenv("PRODUCT_DRAWINGS_DIR", t.TempDir())
	t.Setenv("IMAGE_GEN_BASE_URL", server.URL)
	t.Setenv("IMAGE_GEN_API_KEY", "test-key")

	mock.ExpectQuery(rx("INSERT INTO kb.product_drawings")).
		WithArgs("Ventilator", "", "prompt", "", "", sqlmock.AnyArg(), sqlmock.AnyArg(), "Qwen", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))

	d, err := GenerateAndSave(context.Background(), GenerateAndSaveInput{
		Name: "Ventilator", Prompt: "prompt", Model: "Qwen",
	})
	if err != nil {
		t.Fatalf("GenerateAndSave: %v", err)
	}
	if d == nil || d.ID != 11 {
		t.Fatalf("d = %+v, want id 11", d)
	}
}

func TestGenerateAndSaveRejectsUnsupportedModel(t *testing.T) {
	if _, err := GenerateAndSave(context.Background(), GenerateAndSaveInput{Name: "x", Prompt: "p", Model: "bogus"}); err == nil {
		t.Fatal("GenerateAndSave with unsupported model: want error, got nil")
	}
}

func TestGenerateAndSavePropagatesProviderError(t *testing.T) {
	t.Setenv("IMAGE_GEN_BASE_URL", "")
	t.Setenv("IMAGE_GEN_API_KEY", "")

	if _, err := GenerateAndSave(context.Background(), GenerateAndSaveInput{Name: "x", Prompt: "p", Model: "Qwen"}); err == nil {
		t.Fatal("GenerateAndSave with unconfigured provider: want error, got nil")
	}
}
