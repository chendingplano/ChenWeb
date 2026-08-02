package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type fakePolicyCompiler struct {
	compiled docprocessing.CompiledPolicy
	err      error
	calls    int
}

func (f *fakePolicyCompiler) CompilePolicy(context.Context, int64) (docprocessing.CompiledPolicy, error) {
	f.calls++
	return f.compiled, f.err
}

func newPipelinePolicyContext(t *testing.T, method, target, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newPipelinePolicyIDContext(t *testing.T, id, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	c, rec := newPipelinePolicyContext(t, http.MethodPost, "/api/v1/kb/pipeline-policies/"+id+"/activate", body)
	c.SetPath("/api/v1/kb/pipeline-policies/:id/activate")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func installPolicyActivationFakes(t *testing.T, user *ApiTypes.UserInfo, compiler docprocessing.PolicyCompiler) func() {
	t.Helper()
	oldAuth, oldCompiler, oldRecompile := EchoFactory.DefaultAuthenticator, newPipelinePolicyCompiler, recompilePipelinePolicyInTx
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) { return user, nil }
	newPipelinePolicyCompiler = func() docprocessing.PolicyCompiler { return compiler }
	recompilePipelinePolicyInTx = func(ctx context.Context, _ *sql.Tx, policyID int64) (docprocessing.CompiledPolicy, error) {
		return compiler.CompilePolicy(ctx, policyID)
	}
	return func() {
		EchoFactory.DefaultAuthenticator = oldAuth
		newPipelinePolicyCompiler = oldCompiler
		recompilePipelinePolicyInTx = oldRecompile
	}
}

func installPolicyDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	t.Cleanup(func() { ApiTypes.ProjectDBHandle = old; _ = db.Close() })
	return db, mock
}

func TestListPipelinePoliciesOrderedByVersionDesc(t *testing.T) {
	_, mock := installPolicyDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT" + pipelinePolicySelectColumns + "\nORDER BY version DESC")).WillReturnRows(policyRows().AddRow(int64(1), 1, "active", "bootstrap", nil, time.Now(), "system", time.Now(), time.Now()))
	c, rec := newPipelinePolicyContext(t, http.MethodGet, "/api/v1/kb/pipeline-policies", "")
	if err := ListPipelinePolicies(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload listPipelinePoliciesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || len(payload.Results) != 1 {
		t.Fatalf("payload=%+v err=%v", payload, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePipelinePolicyInsertsAsDraft(t *testing.T) {
	_, mock := installPolicyDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.pipeline_policies (version, status, source_ref, checksum)")).WithArgs(nil, nil).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT" + pipelinePolicySelectColumns + "\nWHERE id = $1")).WithArgs(int64(2)).WillReturnRows(policyRows().AddRow(int64(2), 2, "draft", nil, nil, nil, nil, time.Now(), time.Now()))
	c, rec := newPipelinePolicyContext(t, http.MethodPost, "/api/v1/kb/pipeline-policies", `{}`)
	if err := CreatePipelinePolicy(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActivatePipelinePolicyCompilesLocksAndActivatesAtomically(t *testing.T) {
	_, mock := installPolicyDB(t)
	compiler := &fakePolicyCompiler{compiled: docprocessing.CompiledPolicy{PolicyID: 2, Version: 2, Checksum: "sha256:compiled"}}
	restore := installPolicyActivationFakes(t, &ApiTypes.UserInfo{UserName: "owner@example.com", IsOwner: true}, compiler)
	defer restore()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(checksum, ''), status FROM kb.pipeline_policies WHERE id = $1 FOR UPDATE")).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"checksum", "status"}).AddRow("", "draft"))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.pipeline_policies SET checksum = $1, modify_time = NOW() WHERE id = $2")).WithArgs("sha256:compiled", int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.pipeline_policies SET status = 'archived', modify_time = NOW() WHERE status = 'active' AND id <> $1")).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.pipeline_policies SET status = 'active', activated_at = NOW(), activated_by = $1, checksum = $3, modify_time = NOW() WHERE id = $2 AND status = 'draft' AND checksum = $3")).WithArgs("owner@example.com", int64(2), "sha256:compiled").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT" + pipelinePolicySelectColumns + "\nWHERE id = $1")).WithArgs(int64(2)).WillReturnRows(policyRows().AddRow(int64(2), 2, "active", nil, "sha256:compiled", time.Now(), "owner@example.com", time.Now(), time.Now()))
	c, rec := newPipelinePolicyIDContext(t, "2", `{}`)
	if err := ActivatePipelinePolicy(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || compiler.calls != 2 {
		t.Fatalf("code=%d calls=%d body=%s", rec.Code, compiler.calls, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActivatePipelinePolicyRejectsIdentityAndAuthorizationFailures(t *testing.T) {
	tests := []struct {
		name string
		user *ApiTypes.UserInfo
		body string
		want int
	}{
		{"unauthenticated", nil, `{}`, http.StatusUnauthorized},
		{"unauthorized", &ApiTypes.UserInfo{UserName: "reader"}, `{}`, http.StatusForbidden},
		{"body actor", &ApiTypes.UserInfo{UserName: "owner", IsOwner: true}, `{"activated_by":"forged"}`, http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiler := &fakePolicyCompiler{}
			restore := installPolicyActivationFakes(t, test.user, compiler)
			defer restore()
			c, rec := newPipelinePolicyIDContext(t, "2", test.body)
			if err := ActivatePipelinePolicy(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != test.want || compiler.calls != 0 {
				t.Fatalf("code=%d calls=%d body=%s", rec.Code, compiler.calls, rec.Body.String())
			}
		})
	}
}

func TestActivatePipelinePolicyCompilerFailureLeavesPriorActiveUntouched(t *testing.T) {
	compiler := &fakePolicyCompiler{err: errors.New("overlapping bindings")}
	restore := installPolicyActivationFakes(t, &ApiTypes.UserInfo{UserName: "owner", IsOwner: true}, compiler)
	defer restore()
	c, rec := newPipelinePolicyIDContext(t, "2", `{}`)
	if err := ActivatePipelinePolicy(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnprocessableEntity || compiler.calls != 1 {
		t.Fatalf("code=%d calls=%d body=%s", rec.Code, compiler.calls, rec.Body.String())
	}
}

func TestActivatePipelinePolicyTransactionFailureRollsBackArchive(t *testing.T) {
	_, mock := installPolicyDB(t)
	compiler := &fakePolicyCompiler{compiled: docprocessing.CompiledPolicy{PolicyID: 2, Version: 2, Checksum: "sha256:compiled"}}
	restore := installPolicyActivationFakes(t, &ApiTypes.UserInfo{UserName: "admin", Admin: true}, compiler)
	defer restore()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE\\(checksum").WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"checksum", "status"}).AddRow("sha256:compiled", "draft"))
	mock.ExpectExec("UPDATE kb.pipeline_policies SET status = 'archived'").WithArgs(int64(2)).WillReturnError(errors.New("write failed"))
	mock.ExpectRollback()
	c, rec := newPipelinePolicyIDContext(t, "2", `{}`)
	if err := ActivatePipelinePolicy(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActivatePipelinePolicyRejectsChecksumChangedAfterCompile(t *testing.T) {
	_, mock := installPolicyDB(t)
	compiler := &fakePolicyCompiler{compiled: docprocessing.CompiledPolicy{PolicyID: 2, Version: 2, Checksum: "sha256:compiled"}}
	restore := installPolicyActivationFakes(t, &ApiTypes.UserInfo{UserName: "admin", Roles: []string{"admin"}}, compiler)
	defer restore()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE\\(checksum").WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"checksum", "status"}).AddRow("sha256:changed", "draft"))
	mock.ExpectRollback()
	c, rec := newPipelinePolicyIDContext(t, "2", `{}`)
	if err := ActivatePipelinePolicy(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActivatePipelinePolicyRejectsContentChangedDuringTransaction(t *testing.T) {
	_, mock := installPolicyDB(t)
	compiler := &fakePolicyCompiler{compiled: docprocessing.CompiledPolicy{PolicyID: 2, Version: 2, Checksum: "sha256:before"}}
	restore := installPolicyActivationFakes(t, &ApiTypes.UserInfo{UserName: "owner", IsOwner: true}, compiler)
	defer restore()
	recompilePipelinePolicyInTx = func(context.Context, *sql.Tx, int64) (docprocessing.CompiledPolicy, error) {
		return docprocessing.CompiledPolicy{PolicyID: 2, Version: 2, Checksum: "sha256:after"}, nil
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE\\(checksum").WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{"checksum", "status"}).AddRow("", "draft"))
	mock.ExpectRollback()
	c, rec := newPipelinePolicyIDContext(t, "2", `{}`)
	if err := ActivatePipelinePolicy(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func policyRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "version", "status", "source_ref", "checksum", "activated_at", "activated_by", "create_time", "modify_time"})
}
