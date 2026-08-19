package dbmainthandler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func setupOrphanedLabelsDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	return mock, func() {
		ApiTypes.ProjectDBHandle = old
		db.Close()
	}
}

func TestListOrphanedLabelsReturnsOnlyLabelsWithoutTerms(t *testing.T) {
	mock, cleanup := setupOrphanedLabelsDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM kb.ontology_term_labels l WHERE NOT EXISTS")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT l.id, l.term_id, l.label, l.lang, l.label_role, l.status")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "label", "lang", "label_role", "status", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(42, "measurement:orphan", "孤立标签", "zh-cn", "prefLabel", "draft", now, "seed", now, "seed"))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/ontology-term-labels/orphans", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := ListOrphanedLabels(c); err != nil {
		t.Fatalf("ListOrphanedLabels: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Results []OrphanedLabelRow `json:"results"`
		Total   int64              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Total != 1 || len(body.Results) != 1 || body.Results[0].TermID != "measurement:orphan" {
		t.Fatalf("unexpected response: %+v", body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListOrphanedLabelsAcceptsNullableAuditFields(t *testing.T) {
	mock, cleanup := setupOrphanedLabelsDB(t)
	defer cleanup()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM kb.ontology_term_labels l WHERE NOT EXISTS")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT l.id, l.term_id, l.label, l.lang, l.label_role, l.status,\n l.create_time, COALESCE(l.create_by, ''), l.modify_time, COALESCE(l.modify_by, '')")).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "label", "lang", "label_role", "status", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(42, "measurement:orphan", "orphan", "en", "altLabel", "draft", now, "", now, ""))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/ontology-term-labels/orphans", nil)
	rec := httptest.NewRecorder()
	if err := ListOrphanedLabels(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListOrphanedLabels: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListOrphanedLabelsAppliesSearchFilters(t *testing.T) {
	mock, cleanup := setupOrphanedLabelsDB(t)
	defer cleanup()

	mock.ExpectQuery(`SELECT count\(\*\) FROM kb\.ontology_term_labels l WHERE NOT EXISTS[\s\S]*l\.term_id ILIKE \$1[\s\S]*l\.lang = \$2[\s\S]*l\.label_role = \$3`).
		WithArgs("%垃圾%", "zh-cn", "prefLabel").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT l\.id, l\.term_id, l\.label, l\.lang, l\.label_role, l\.status[\s\S]*l\.term_id ILIKE \$1[\s\S]*l\.lang = \$2[\s\S]*l\.label_role = \$3`).
		WithArgs("%垃圾%", "zh-cn", "prefLabel").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "term_id", "label", "lang", "label_role", "status", "create_time", "create_by", "modify_time", "modify_by",
		}))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/?q=%E5%9E%83%E5%9C%BE&lang=zh-cn&label_role=prefLabel", nil)
	rec := httptest.NewRecorder()
	if err := ListOrphanedLabels(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListOrphanedLabels: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestResolveOrphanedLabelsDeletesSubmittedOrphansAndLogs(t *testing.T) {
	mock, cleanup := setupOrphanedLabelsDB(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM kb.ontology_term_labels l")).
		WithArgs(int64(42), int64(43)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.db_maintenance_logs (operation, result_data) VALUES ($1, $2)")).
		WithArgs("resolve-orphaned-ontology-term-labels", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"ids":[42,43],"q":"垃圾","lang":"zh-cn","label_role":"prefLabel"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := ResolveOrphanedLabels(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ResolveOrphanedLabels: %v", err)
	}
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"deleted_count":2`)) {
		t.Fatalf("unexpected response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestResolveOrphanedLabelsDoesNothingForEmptyIDs(t *testing.T) {
	_, cleanup := setupOrphanedLabelsDB(t)
	defer cleanup()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"ids":[]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := ResolveOrphanedLabels(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ResolveOrphanedLabels: %v", err)
	}
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"deleted_count":0`)) {
		t.Fatalf("unexpected response: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
