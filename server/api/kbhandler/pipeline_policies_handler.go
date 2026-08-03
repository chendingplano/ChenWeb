package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

var newPipelinePolicyCompiler = func() docprocessing.PolicyCompiler {
	return docprocessing.PolicyCompilerSQLStore{DB: ApiTypes.ProjectDBHandle}
}

var recompilePipelinePolicyInTx = func(ctx context.Context, tx *sql.Tx, policyID int64) (docprocessing.CompiledPolicy, error) {
	return (docprocessing.PolicyCompilerSQLStore{DB: tx}).CompilePolicy(ctx, policyID)
}

// reloadPipelinePolicyBindingsAndGates reloads the in-process canonical
// binding and gate sets after a successful activation, so a running process
// picks up the new policy immediately -- without this, activation had no
// effect on a running process until restart, leaving E2's atomic activation
// endpoint inert (P5 review 2026080302 finding P5-12). Multi-process
// deployments still require an out-of-band restart/reload signal per
// process; that remains a documented carry-forward, not solved here.
var reloadPipelinePolicyBindingsAndGates = func(ctx context.Context, db *sql.DB) error {
	if err := docprocessing.LoadProductionPipelineBindings(ctx, docprocessing.PipelineBindingSQLStore{DB: db}); err != nil {
		return fmt.Errorf("reload pipeline bindings: %w", err)
	}
	if err := docprocessing.LoadProductionPipelineGates(ctx, docprocessing.PipelineGateSQLStore{DB: db}); err != nil {
		return fmt.Errorf("reload pipeline gates: %w", err)
	}
	return nil
}

// writeRoutingPolicyAlarm raises an operator alarm on the shared
// alarms_errors table. A reload failure after a successful activation
// commit uses this instead of failing the request -- the activation itself
// already committed and must not be misrepresented as rolled back.
var writeRoutingPolicyAlarm = func(ctx context.Context, db *sql.DB, alarm docprocessing.RoutingAlarm) error {
	return (docprocessing.RoutingAlarmSQLWriter{DB: db}).WriteAlarm(ctx, alarm)
}

type pipelinePolicyRecord struct {
	ID          int64      `json:"id"`
	Version     int        `json:"version"`
	Status      string     `json:"status"`
	SourceRef   *string    `json:"source_ref,omitempty"`
	Checksum    *string    `json:"checksum,omitempty"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
	ActivatedBy *string    `json:"activated_by,omitempty"`
	CreateTime  time.Time  `json:"create_time"`
	ModifyTime  time.Time  `json:"modify_time"`
}

type listPipelinePoliciesResponse struct {
	Status  bool                   `json:"status"`
	Results []pipelinePolicyRecord `json:"results"`
	Total   int                    `json:"total"`
}

type pipelinePolicyDetailResponse struct {
	Status bool                 `json:"status"`
	Record pipelinePolicyRecord `json:"record"`
	// Warning is set only when activation committed successfully but the
	// in-process reload (P5 review 2026080302 finding P5-12) failed --
	// the policy is active in the database but this process still needs a
	// restart to route/gate against it.
	Warning string `json:"warning,omitempty"`
}

const pipelinePolicySelectColumns = `
    id, version, status, source_ref, checksum, activated_at, activated_by, create_time, modify_time
FROM kb.pipeline_policies`

func scanPipelinePolicyRecord(scan func(dest ...any) error) (pipelinePolicyRecord, error) {
	var (
		record      pipelinePolicyRecord
		sourceRef   sql.NullString
		checksum    sql.NullString
		activatedAt sql.NullTime
		activatedBy sql.NullString
	)
	if err := scan(
		&record.ID, &record.Version, &record.Status, &sourceRef, &checksum, &activatedAt, &activatedBy,
		&record.CreateTime, &record.ModifyTime,
	); err != nil {
		return pipelinePolicyRecord{}, err
	}
	if sourceRef.Valid {
		record.SourceRef = &sourceRef.String
	}
	if checksum.Valid {
		record.Checksum = &checksum.String
	}
	if activatedAt.Valid {
		record.ActivatedAt = &activatedAt.Time
	}
	if activatedBy.Valid {
		record.ActivatedBy = &activatedBy.String
	}
	return record, nil
}

func fetchPipelinePolicyByID(db *sql.DB, id int64) (pipelinePolicyRecord, error) {
	query := "SELECT" + pipelinePolicySelectColumns + "\nWHERE id = $1"
	row := db.QueryRow(query, id)
	return scanPipelinePolicyRecord(row.Scan)
}

// activePipelinePolicyID returns the id of the currently active policy.
func activePipelinePolicyID(db *sql.DB) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1").Scan(&id)
	return id, err
}

func pipelinePolicyStatus(db *sql.DB, id int64) (string, error) {
	var status string
	err := db.QueryRow("SELECT status FROM kb.pipeline_policies WHERE id = $1", id).Scan(&status)
	return strings.ToLower(strings.TrimSpace(status)), err
}

// ListPipelinePolicies handles GET /api/v1/kb/pipeline-policies.
func ListPipelinePolicies(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PP_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	query := "SELECT" + pipelinePolicySelectColumns + "\nORDER BY version DESC"
	rows, err := db.Query(query)
	if err != nil {
		logger.Error("query pipeline policies failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve pipeline policies (CWB_KB_PP_002)"})
	}
	defer rows.Close()

	results := make([]pipelinePolicyRecord, 0)
	for rows.Next() {
		record, err := scanPipelinePolicyRecord(rows.Scan)
		if err != nil {
			logger.Error("scan pipeline policy failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan pipeline policies (CWB_KB_PP_003)"})
		}
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate pipeline policies failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate pipeline policies (CWB_KB_PP_004)"})
	}

	return c.JSON(http.StatusOK, listPipelinePoliciesResponse{Status: true, Results: results, Total: len(results)})
}

// CreatePipelinePolicy handles POST /api/v1/kb/pipeline-policies, authoring
// a new policy as a draft. It is not consulted by resolution until
// activated via ActivatePipelinePolicy.
func CreatePipelinePolicy(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PP_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		SourceRef *string `json:"source_ref"`
		Checksum  *string `json:"checksum"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PP_101)"})
	}

	db := ApiTypes.ProjectDBHandle
	const insertQuery = `
INSERT INTO kb.pipeline_policies (version, status, source_ref, checksum)
VALUES ((SELECT COALESCE(MAX(version), 0) + 1 FROM kb.pipeline_policies), 'draft', $1, $2)
RETURNING id
`
	var id int64
	if err := db.QueryRow(insertQuery, payload.SourceRef, payload.Checksum).Scan(&id); err != nil {
		logger.Error("insert pipeline policy failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline policy (CWB_KB_PP_102)"})
	}

	record, err := fetchPipelinePolicyByID(db, id)
	if err != nil {
		logger.Error("fetch created pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created pipeline policy (CWB_KB_PP_103)"})
	}

	return c.JSON(http.StatusOK, pipelinePolicyDetailResponse{Status: true, Record: record})
}

// ActivatePipelinePolicy handles POST /api/v1/kb/pipeline-policies/:id/activate.
// Archives whatever policy was previously active and activates the target
// one, atomically -- kb.pipeline_policies enforces at most one active row
// via a partial unique index, so this must happen in a single transaction.
func ActivatePipelinePolicy(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PP_200")
	defer rc.Close()
	logger := rc.GetLogger()
	user := rc.IsAuthenticated()
	if err := pipelineRoutingAuthorizer.Authorize(user, PolicyActionActivation); err != nil {
		return policyAuthorizationResponse(c, err)
	}

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_PP_201)"})
	}

	var payload struct{}
	if err := decodeStrictJSON(c, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PP_202)"})
	}

	compiled, err := newPipelinePolicyCompiler().CompilePolicy(c.Request().Context(), id)
	if err != nil {
		logger.Warn("pipeline policy compilation failed", "id", id, "err", err)
		return c.JSON(http.StatusUnprocessableEntity, errorResponse{Status: false, ErrorMsg: "pipeline policy compilation failed: " + err.Error()})
	}
	if compiled.PolicyID != id || strings.TrimSpace(compiled.Checksum) == "" {
		return c.JSON(http.StatusUnprocessableEntity, errorResponse{Status: false, ErrorMsg: "pipeline policy compilation returned invalid identity/checksum"})
	}

	db := ApiTypes.ProjectDBHandle
	tx, err := db.Begin()
	if err != nil {
		logger.Error("begin activate pipeline policy tx failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to activate pipeline policy (CWB_KB_PP_203)"})
	}
	defer tx.Rollback()

	var storedChecksum, targetStatus string
	if err := tx.QueryRow("SELECT COALESCE(checksum, ''), status FROM kb.pipeline_policies WHERE id = $1 FOR UPDATE", id).Scan(&storedChecksum, &targetStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PP_207)"})
		}
		logger.Error("lock target pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to activate pipeline policy (CWB_KB_PP_204)"})
	}
	if strings.ToLower(strings.TrimSpace(targetStatus)) != "draft" {
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "only a draft pipeline policy can be activated"})
	}
	recompiled, err := recompilePipelinePolicyInTx(c.Request().Context(), tx, id)
	if err != nil {
		logger.Warn("pipeline policy transaction recompile failed", "id", id, "err", err)
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "pipeline policy changed during compilation"})
	}
	if recompiled.PolicyID != compiled.PolicyID || recompiled.Checksum != compiled.Checksum {
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "pipeline policy changed during compilation"})
	}
	if storedChecksum != "" && storedChecksum != compiled.Checksum {
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "pipeline policy changed after compilation"})
	}
	if storedChecksum == "" {
		if _, err := tx.Exec("UPDATE kb.pipeline_policies SET checksum = $1, modify_time = NOW() WHERE id = $2", compiled.Checksum, id); err != nil {
			logger.Error("store compiled pipeline policy checksum failed", "id", id, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to activate pipeline policy (CWB_KB_PP_204)"})
		}
	}
	if _, err := tx.Exec("UPDATE kb.pipeline_policies SET status = 'archived', modify_time = NOW() WHERE status = 'active' AND id <> $1", id); err != nil {
		logger.Error("archive previous active pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to activate pipeline policy (CWB_KB_PP_204)"})
	}
	result, err := tx.Exec(
		"UPDATE kb.pipeline_policies SET status = 'active', activated_at = NOW(), activated_by = $1, checksum = $3, modify_time = NOW() WHERE id = $2 AND status = 'draft' AND checksum = $3",
		strings.TrimSpace(user.UserName), id, compiled.Checksum,
	)
	if err != nil {
		logger.Error("activate pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to activate pipeline policy (CWB_KB_PP_205)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected activate pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify pipeline policy activation (CWB_KB_PP_206)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PP_207)"})
	}
	if err := tx.Commit(); err != nil {
		logger.Error("commit activate pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to activate pipeline policy (CWB_KB_PP_208)"})
	}
	writePolicyAuditEvent(c, rc, logger, policyaudit.Event{
		Kind: policyaudit.EventPolicyActivated, PolicyID: id, PolicyVersion: compiled.Version,
		Detail: map[string]any{"checksum": compiled.Checksum},
	})

	record, err := fetchPipelinePolicyByID(db, id)
	if err != nil {
		logger.Error("fetch activated pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve activated pipeline policy (CWB_KB_PP_209)"})
	}

	response := pipelinePolicyDetailResponse{Status: true, Record: record}
	// The activation already committed above; a reload failure here must
	// not be reported as though activation itself failed, and must not
	// trigger a rollback of an already-committed transaction (P5 review
	// 2026080302 finding P5-12).
	if reloadErr := reloadPipelinePolicyBindingsAndGates(c.Request().Context(), db); reloadErr != nil {
		logger.Error("in-process pipeline policy reload failed after activation", "id", id, "err", reloadErr)
		alarm := docprocessing.RoutingAlarm{
			Kind: docprocessing.RoutingAlarmKindPolicyIntegrity, Severity: docprocessing.RoutingAlarmSeverityError,
			Message: fmt.Sprintf("pipeline policy %d activated but in-process reload failed: %s; this process requires a restart to apply it", id, reloadErr),
		}
		if err := writeRoutingPolicyAlarm(c.Request().Context(), db, alarm); err != nil {
			logger.Error("failed to write pipeline policy reload alarm", "id", id, "err", err)
		}
		response.Warning = "activated, but in-process reload failed; this process requires a restart to apply it"
	}

	return c.JSON(http.StatusOK, response)
}
