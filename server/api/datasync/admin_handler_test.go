package datasync

import (
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
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
