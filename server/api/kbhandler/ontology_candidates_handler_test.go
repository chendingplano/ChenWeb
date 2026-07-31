package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/candidates"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newOntologyCandidateContext(t *testing.T, method, target, body string, params map[string]string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(target)
	if len(params) > 0 {
		names := make([]string, 0, len(params))
		values := make([]string, 0, len(params))
		for k, v := range params {
			names = append(names, k)
			values = append(values, v)
		}
		c.SetParamNames(names...)
		c.SetParamValues(values...)
	}
	return c, rec
}

func TestCreateOntologyCandidateReturnsFingerprintAndReused(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	payload := []byte(`{"term_id":"core:assertion","term_kind":"class","module_id":"core"}`)
	fp, err := candidates.Fingerprint(payload, "llm", "rec:1", "core")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_candidates")).
		WithArgs("term", string(payload), "core", "llm", "rec:1", "null", nil, nil, fp, "null",
			"discovered", nil, "tester", "tester").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "candidate_kind", "proposed_payload", "proposed_module_id", "source_type",
			"source_ref", "source_line_spans", "discovery_method", "confidence", "fingerprint",
			"candidate_matches", "status", "decision_reason", "dependency_fingerprint",
			"proposed_by", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(int64(1), "term", payload, "core", "llm", "rec:1", []byte("null"), nil, nil, fp,
			[]byte("null"), "discovered", nil, nil, nil, now, "tester", now, "tester"))

	body := `{"candidate_kind":"term","proposed_payload":{"term_id":"core:assertion","term_kind":"class","module_id":"core"},"proposed_module_id":"core","source_type":"llm","source_ref":"rec:1","create_by":"tester"}`
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/candidates", body, nil)
	if err := CreateOntologyCandidate(c); err != nil {
		t.Fatalf("CreateOntologyCandidate: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status bool                   `json:"status"`
		Record map[string]interface{} `json:"record"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Status || resp.Record["fingerprint"] != fp || resp.Record["status"] != "discovered" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPromoteOntologyCandidateRequiresApproved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_candidates")).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "candidate_kind", "proposed_payload", "proposed_module_id", "source_type",
			"source_ref", "source_line_spans", "discovery_method", "confidence", "fingerprint",
			"candidate_matches", "status", "decision_reason", "dependency_fingerprint",
			"proposed_by", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(int64(1), "term", []byte(`{}`), "core", "llm", nil, []byte("null"), nil, nil, "fp",
			[]byte("null"), "draft", nil, nil, nil, now, nil, now, nil))

	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/candidates/1/promote", `{}`, map[string]string{"id": "1"})
	if err := PromoteOntologyCandidate(c); err != nil {
		t.Fatalf("PromoteOntologyCandidate: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-approved promotion, got %d", rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
