package productreviews

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// installGlobalMockDB swaps ApiTypes.ProjectDBHandle (used directly by the
// productdrawings package) for a sqlmock-backed *sql.DB, and returns a Store
// wrapping the same handle so both packages' queries land on one mock queue.
func installGlobalMockDB(t *testing.T) (Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	t.Cleanup(func() { ApiTypes.ProjectDBHandle = old; _ = db.Close() })
	return Store{DB: db}, mock
}

func writeExplodedViewPromptTemplate(t *testing.T, dir string) {
	t.Helper()
	tmpl := "Draw {{PRODUCT_NAME}}.{{COMPONENTS_SENTENCE}}\n"
	if err := os.WriteFile(filepath.Join(dir, "prompt-product-drawing-exploded-view-v2.md"), []byte(tmpl), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testLogger() ApiTypes.JimoLogger {
	return loggerutil.CreateDefaultLogger("00000-tst")
}

// Scenario: an existing kb.product_drawings row is bound without generating
// a new image (spec: product-review-auto-drawing).
func TestEnsureProfileDrawingReusesExisting(t *testing.T) {
	store, mock := installGlobalMockDB(t)

	mock.ExpectQuery(rx("WHERE LOWER(TRIM(name)) = LOWER(TRIM($1))")).
		WithArgs("Ventilator").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "prompt", "keywords", "notes", "filename", "model", "model_name", "created_at", "updated_at",
		}).AddRow(42, "Ventilator", "", "prompt", "", "", "ventilator-1.png", "Qwen", "wan2.2-t2i-flash", time.Now(), time.Now()))
	mock.ExpectExec(rx("UPDATE kb.product_profiles")).
		WithArgs(int64(1), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	profile := &Profile{ID: 1, Name: "Ventilator"}
	ensureProfileDrawing(context.Background(), testLogger(), store, profile, "Qwen")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// Scenario: no matching drawing exists, so one is generated and bound (spec:
// product-review-auto-drawing).
func TestEnsureProfileDrawingGeneratesWhenMissing(t *testing.T) {
	store, mock := installGlobalMockDB(t)
	promptDir := t.TempDir()
	writeExplodedViewPromptTemplate(t, promptDir)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"data":[{"b64_json":"iVBORw0KGgo="}]}`)
	}))
	defer server.Close()
	t.Setenv("PROMPTS_DIR", promptDir)
	t.Setenv("PRODUCT_DRAWINGS_DIR", t.TempDir())
	t.Setenv("IMAGE_GEN_BASE_URL", server.URL)
	t.Setenv("IMAGE_GEN_API_KEY", "test-key")

	mock.ExpectQuery(rx("WHERE LOWER(TRIM(name)) = LOWER(TRIM($1))")).
		WithArgs("Ventilator").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1,
			nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator", Status: StatusAccepted},
			nodeRow{ID: 2, ParentID: ptr(int64(1)), Kind: KindPart, Label: "Oxygen Sensor", Status: StatusAccepted},
		))
	mock.ExpectQuery(rx("INSERT INTO kb.product_drawings")).
		WithArgs("Ventilator", "", sqlmock.AnyArg(), "", "", sqlmock.AnyArg(), sqlmock.AnyArg(), "Qwen", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(99)))
	mock.ExpectExec(rx("UPDATE kb.product_profiles")).
		WithArgs(int64(1), int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	profile := &Profile{ID: 1, Name: "Ventilator"}
	ensureProfileDrawing(context.Background(), testLogger(), store, profile, "Qwen")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// Scenario: a profile that already has a drawing_id is left untouched — no
// lookup, no generation (spec: product-review-auto-drawing).
func TestEnsureProfileDrawingSkipsWhenAlreadySet(t *testing.T) {
	store, mock := installGlobalMockDB(t)

	profile := &Profile{ID: 1, Name: "Ventilator", DrawingID: ptr(int64(7))}
	ensureProfileDrawing(context.Background(), testLogger(), store, profile, "Qwen")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// Scenario: the generation provider fails — the profile is left with no
// drawing_id and no error is raised to the caller (design.md Decision 6).
func TestEnsureProfileDrawingIgnoresGenerationFailure(t *testing.T) {
	store, mock := installGlobalMockDB(t)
	promptDir := t.TempDir()
	writeExplodedViewPromptTemplate(t, promptDir)
	t.Setenv("PROMPTS_DIR", promptDir)
	t.Setenv("IMAGE_GEN_BASE_URL", "")
	t.Setenv("IMAGE_GEN_API_KEY", "")

	mock.ExpectQuery(rx("WHERE LOWER(TRIM(name)) = LOWER(TRIM($1))")).
		WithArgs("Ventilator").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(rx("FROM kb.product_profile_nodes WHERE profile_id = $1")).
		WithArgs(int64(1)).
		WillReturnRows(nodeRowsFrom(1, nodeRow{ID: 1, Kind: KindProduct, Label: "Ventilator", Status: StatusAccepted}))

	profile := &Profile{ID: 1, Name: "Ventilator"}
	ensureProfileDrawing(context.Background(), testLogger(), store, profile, "Qwen")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
