package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

func testAuditLogger(t *testing.T) ApiTypes.JimoLogger {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	rc := EchoFactory.NewFromEcho(c, "TEST_OAL_001")
	return rc.GetLogger()
}

func TestLogObjectAuditInsertsRowWithActor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := map[string]json.RawMessage{"object_id": json.RawMessage(`"obj_b"`)}

	insertQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(insertQuery).
		WithArgs("kb.artifact_objects", "42", "resolve_object_id", `{"object_id":"obj_b"}`, "alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	logObjectAudit(context.Background(), db, testAuditLogger(t), "kb.artifact_objects", "42", "resolve_object_id", "alice", payload)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestLogObjectAuditInsertsNullActorWhenUnauthenticated(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := map[string]json.RawMessage{"description": json.RawMessage(`"fixed typo"`)}

	insertQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(insertQuery).
		WithArgs("kb.object_nodes", "obj_a", "edit_fields", `{"description":"fixed typo"}`, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	logObjectAudit(context.Background(), db, testAuditLogger(t), "kb.object_nodes", "obj_a", "edit_fields", "", payload)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestLogObjectAuditSwallowsInsertError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	payload := map[string]json.RawMessage{"description": json.RawMessage(`"x"`)}

	insertQuery := regexp.QuoteMeta(
		"INSERT INTO kb.object_audit_log (table_name, row_key, action, changes, actor) VALUES ($1,$2,$3,$4,$5)",
	)
	mock.ExpectExec(insertQuery).WillReturnError(fmt.Errorf("boom"))

	// Must not panic and must not return an error (best-effort, fire-and-forget).
	logObjectAudit(context.Background(), db, testAuditLogger(t), "kb.artifact_objects", "1", "edit_fields", "", payload)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
