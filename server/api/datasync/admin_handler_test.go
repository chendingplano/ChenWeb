package datasync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestPreviewCountsWithoutFetchingRowsOrWritingState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	withProjectDBHandle(t, db)
	mock.ExpectQuery("SELECT last_cursor::text").WithArgs("kb_product_names").WillReturnRows(
		sqlmock.NewRows([]string{"last_cursor", "last_synced_at", "last_row_count", "last_error"}).AddRow("2026-11-01T00:00:00Z", nil, 0, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/count") || r.URL.Query().Get("since") != "2026-11-01T00:00:00Z" {
			t.Errorf("preview requested rows instead of count: %s", r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"changed_row_count":6000}`)
	}))
	defer srv.Close()
	t.Setenv("DATA_SYNC_SOURCE_URL", srv.URL)
	t.Setenv("DATA_SYNC_SHARED_SECRET", "secret")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/preview", nil), rec)
	c.SetParamNames("itemId")
	c.SetParamValues("kb_product_names")
	if err := HandlePreviewSync(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"changed_row_count":6000`) {
		t.Fatalf("preview response: %d %s", rec.Code, rec.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCountEndpointRequiresSecret(t *testing.T) {
	for _, configured := range []string{"", "secret"} {
		t.Run(configured, func(t *testing.T) {
			t.Setenv("DATA_SYNC_SHARED_SECRET", configured)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/count", nil), rec)
			if err := HandleCountChanges(c); err != nil {
				t.Fatal(err)
			}
			want := http.StatusUnauthorized
			if configured == "" {
				want = http.StatusInternalServerError
			}
			if rec.Code != want {
				t.Fatalf("status=%d want=%d", rec.Code, want)
			}
		})
	}
}

func TestApplyReturnsPersistedTimestampWithoutListingItems(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	withProjectDBHandle(t, db)
	columns := []string{"last_cursor", "last_synced_at", "last_row_count", "last_error"}
	mock.ExpectQuery("SELECT last_cursor::text").WithArgs("kb_product_names").WillReturnRows(sqlmock.NewRows(columns))
	mock.ExpectExec("INSERT INTO kb.data_sync_state").WithArgs("kb_product_names", nil, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT last_cursor::text").WithArgs("kb_product_names").WillReturnRows(sqlmock.NewRows(columns).AddRow(nil, "2026-11-16 12:34:56+00", 0, nil))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM kb.product_names").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7046))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/changes") {
			t.Errorf("unexpected source discovery: %s", r.URL)
		}
		_, _ = io.WriteString(w, `{"rows":[],"has_more":false,"next_cursor":""}`)
	}))
	defer srv.Close()
	t.Setenv("DATA_SYNC_SOURCE_URL", srv.URL)
	t.Setenv("DATA_SYNC_SHARED_SECRET", "secret")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/apply", nil), rec)
	c.SetParamNames("itemId")
	c.SetParamValues("kb_product_names")
	if err := HandleApplySync(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"last_synced_at":"2026-11-16 12:34:56+00"`) {
		t.Fatalf("apply response: %d %s", rec.Code, rec.Body)
	}
	var result struct {
		TargetCount *int64 `json:"target_row_count"`
		SyncedCount int    `json:"synced_row_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TargetCount == nil || *result.TargetCount != 7046 || result.SyncedCount != 0 {
		t.Fatalf("no-change sync must retain destination count: %s", rec.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTargetRowCountDoesNotUseSourceFilterOrSyncCount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	item := testItem() // source filter excludes proposed rows; destination count must include them
	mock.ExpectQuery("^SELECT count\\(\\*\\) FROM kb.product_names$").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7046))
	count := targetRowCount(context.Background(), db, item)
	if count == nil || *count != 7046 {
		t.Fatalf("destination count=%v", count)
	}
	mock.ExpectQuery("^SELECT count\\(\\*\\) FROM kb.product_names$").WillReturnError(errors.New("table unavailable"))
	if count := targetRowCount(context.Background(), db, item); count != nil {
		t.Fatalf("failed count must be unknown, got %v", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListReportsDestinationCountSeparatelyFromLastTransfer(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	withProjectDBHandle(t, db)
	previousRegistry := Registry
	Registry = []TableSyncItem{testItem()}
	t.Cleanup(func() { Registry = previousRegistry })
	t.Setenv("DATA_SYNC_SOURCE_URL", "")
	mock.ExpectQuery("SELECT id, kind, origin").WillReturnRows(sqlmock.NewRows([]string{"id", "kind", "origin", "table_name", "cursor_col", "natural_key", "columns", "json_columns", "filter", "file_column", "file_dir_env", "file_dir_default_subdir"}))
	mock.ExpectQuery("SELECT last_cursor::text").WithArgs("kb_product_names").WillReturnRows(sqlmock.NewRows([]string{"last_cursor", "last_synced_at", "last_row_count", "last_error"}).AddRow(nil, nil, 0, nil))
	mock.ExpectQuery("^SELECT count\\(\\*\\) FROM kb.product_names$").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7046))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/items", nil), rec)
	if err := HandleListSyncItems(c); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Items []struct {
			TargetCount *int64 `json:"target_row_count"`
			LastCount   int    `json:"last_row_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(result.Items) != 1 || result.Items[0].TargetCount == nil || *result.Items[0].TargetCount != 7046 || result.Items[0].LastCount != 0 {
		t.Fatalf("list response: %d %s", rec.Code, rec.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
