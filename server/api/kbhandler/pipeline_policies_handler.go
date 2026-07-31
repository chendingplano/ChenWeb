package kbhandler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

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
	Status  bool                    `json:"status"`
	Results []pipelinePolicyRecord `json:"results"`
	Total   int                     `json:"total"`
}

type pipelinePolicyDetailResponse struct {
	Status bool                  `json:"status"`
	Record pipelinePolicyRecord `json:"record"`
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

// activePipelinePolicyID returns the id of the currently active policy, used
// as the default policy_id for new pipeline_bindings/pipeline_rules rows
// when the caller doesn't specify one explicitly.
func activePipelinePolicyID(db *sql.DB) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1").Scan(&id)
	return id, err
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

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_PP_201)"})
	}

	var payload struct {
		ActivatedBy *string `json:"activated_by"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil && err.Error() != "EOF" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PP_202)"})
	}
	activatedBy := "unknown"
	if payload.ActivatedBy != nil && strings.TrimSpace(*payload.ActivatedBy) != "" {
		activatedBy = strings.TrimSpace(*payload.ActivatedBy)
	}

	db := ApiTypes.ProjectDBHandle
	tx, err := db.Begin()
	if err != nil {
		logger.Error("begin activate pipeline policy tx failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to activate pipeline policy (CWB_KB_PP_203)"})
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE kb.pipeline_policies SET status = 'archived', modify_time = NOW() WHERE status = 'active'"); err != nil {
		logger.Error("archive previous active pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to activate pipeline policy (CWB_KB_PP_204)"})
	}
	result, err := tx.Exec(
		"UPDATE kb.pipeline_policies SET status = 'active', activated_at = NOW(), activated_by = $1, modify_time = NOW() WHERE id = $2",
		activatedBy, id,
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

	record, err := fetchPipelinePolicyByID(db, id)
	if err != nil {
		logger.Error("fetch activated pipeline policy failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve activated pipeline policy (CWB_KB_PP_209)"})
	}

	return c.JSON(http.StatusOK, pipelinePolicyDetailResponse{Status: true, Record: record})
}
