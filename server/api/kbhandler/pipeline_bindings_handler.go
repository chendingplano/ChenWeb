package kbhandler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type pipelineBindingRecord struct {
	ID                int64           `json:"id"`
	Name              string          `json:"name,omitempty"`
	Priority          int             `json:"priority"`
	KSStoreID         int64           `json:"ks_store_id,omitempty"`
	PipelineID        int64           `json:"pipeline_id"`
	PipelineName      string          `json:"pipeline_name"`
	PolicyID          int64           `json:"policy_id"`
	BindingKind       string          `json:"binding_kind"`
	Predicate         json.RawMessage `json:"predicate,omitempty"`
	PredicateChecksum string          `json:"predicate_checksum,omitempty"`
	Active            bool            `json:"active"`
	CreateTime        time.Time       `json:"create_time"`
	ModifyTime        time.Time       `json:"modify_time"`
}

type listPipelineBindingsResponse struct {
	Status  bool                    `json:"status"`
	Results []pipelineBindingRecord `json:"results"`
	Total   int                     `json:"total"`
}

type pipelineBindingDetailResponse struct {
	Status bool                  `json:"status"`
	Record pipelineBindingRecord `json:"record"`
}

const pipelineBindingSelectColumns = `
    b.id, COALESCE(b.name, ''), b.priority, COALESCE(b.ks_store_id, 0),
    b.pipeline_id, p.name, b.policy_id, b.binding_kind,
    COALESCE(b.predicate, '{}'::jsonb)::text, COALESCE(b.predicate_checksum, ''), b.active,
    b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id`

func scanPipelineBindingRecord(scan func(dest ...any) error) (pipelineBindingRecord, error) {
	var record pipelineBindingRecord
	var predicateRaw string
	if err := scan(
		&record.ID, &record.Name, &record.Priority, &record.KSStoreID,
		&record.PipelineID, &record.PipelineName, &record.PolicyID, &record.BindingKind,
		&predicateRaw, &record.PredicateChecksum, &record.Active,
		&record.CreateTime, &record.ModifyTime,
	); err != nil {
		return pipelineBindingRecord{}, err
	}
	if strings.TrimSpace(predicateRaw) != "" && strings.TrimSpace(predicateRaw) != "{}" {
		record.Predicate = json.RawMessage(predicateRaw)
	}
	return record, nil
}

func fetchPipelineBindingByID(db *sql.DB, id int64) (pipelineBindingRecord, error) {
	query := "SELECT" + pipelineBindingSelectColumns + "\nWHERE b.id = $1"
	row := db.QueryRow(query, id)
	return scanPipelineBindingRecord(row.Scan)
}

// ListPipelineBindings handles GET /api/v1/kb/pipeline-bindings, optionally
// filtered by ?ks_store_id=N to look up the binding for one store.
func ListPipelineBindings(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PB_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	query := "SELECT" + pipelineBindingSelectColumns
	args := []any{}
	if raw := strings.TrimSpace(c.QueryParam("ks_store_id")); raw != "" {
		ksStoreID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ksStoreID <= 0 {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid ks_store_id (CWB_KB_PB_002)"})
		}
		query += "\nWHERE b.ks_store_id = $1"
		args = append(args, ksStoreID)
	}
	query += "\nORDER BY b.id"

	rows, err := db.Query(query, args...)
	if err != nil {
		logger.Error("query pipeline bindings failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve pipeline bindings (CWB_KB_PB_003)"})
	}
	defer rows.Close()

	results := make([]pipelineBindingRecord, 0)
	for rows.Next() {
		record, err := scanPipelineBindingRecord(rows.Scan)
		if err != nil {
			logger.Error("scan pipeline binding failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan pipeline bindings (CWB_KB_PB_004)"})
		}
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate pipeline bindings failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate pipeline bindings (CWB_KB_PB_005)"})
	}

	return c.JSON(http.StatusOK, listPipelineBindingsResponse{Status: true, Results: results, Total: len(results)})
}

// CreatePipelineBinding handles POST /api/v1/kb/pipeline-bindings. A store
// may have at most one binding (kb.pipeline_bindings.ks_store_id is
// UNIQUE); creating a second binding for an already-bound store fails.
func CreatePipelineBinding(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PB_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		Name              string          `json:"name"`
		Priority          *int            `json:"priority"`
		KSStoreID         *int64          `json:"ks_store_id"`
		PipelineID        *int64          `json:"pipeline_id"`
		PolicyID          *int64          `json:"policy_id"`
		BindingKind       string          `json:"binding_kind"`
		Predicate         json.RawMessage `json:"predicate"`
		PredicateChecksum string          `json:"predicate_checksum"`
		Active            *bool           `json:"active"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PB_101)"})
	}
	if payload.PipelineID == nil || *payload.PipelineID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "pipeline_id is required (CWB_KB_PB_103)"})
	}
	priority := 0
	if payload.Priority != nil {
		priority = *payload.Priority
	}
	active := true
	if payload.Active != nil {
		active = *payload.Active
	}
	bindingKind := strings.TrimSpace(payload.BindingKind)
	if bindingKind == "" {
		bindingKind = "store_default"
	}
	if bindingKind == "store_default" && (payload.KSStoreID == nil || *payload.KSStoreID <= 0) {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "ks_store_id is required (CWB_KB_PB_102)"})
	}
	if bindingKind == "conditional" && len(payload.Predicate) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "predicate is required (CWB_KB_PB_111)"})
	}
	var ksStoreID any
	if payload.KSStoreID != nil && *payload.KSStoreID > 0 {
		ksStoreID = *payload.KSStoreID
	}
	predicate := any(nil)
	predicateChecksum := strings.TrimSpace(payload.PredicateChecksum)
	if len(payload.Predicate) > 0 {
		var doc semrules.Document
		if err := json.Unmarshal(payload.Predicate, &doc); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PB_107)"})
		}
		if err := semrules.Validate(doc); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PB_108)"})
		}
		_, checksum, err := semrules.Canonicalize(doc)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PB_109)"})
		}
		if predicateChecksum != "" && predicateChecksum != checksum {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "predicate_checksum mismatch (CWB_KB_PB_110)"})
		}
		predicateChecksum = checksum
		predicate = string(payload.Predicate)
	}

	db := ApiTypes.ProjectDBHandle
	policyID := payload.PolicyID
	if policyID == nil {
		active, err := activePipelinePolicyID(db)
		if err != nil {
			logger.Error("resolve active pipeline policy failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve active pipeline policy (CWB_KB_PB_106)"})
		}
		policyID = &active
	}

	var id int64
	if err := db.QueryRow(
		"INSERT INTO kb.pipeline_bindings (name, priority, ks_store_id, pipeline_id, policy_id, binding_kind, predicate, predicate_checksum, active) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9) RETURNING id",
		strings.TrimSpace(payload.Name), priority, ksStoreID, *payload.PipelineID, *policyID, bindingKind, predicate, predicateChecksum, active,
	).Scan(&id); err != nil {
		logger.Error("insert pipeline binding failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline binding (CWB_KB_PB_104)"})
	}

	record, err := fetchPipelineBindingByID(db, id)
	if err != nil {
		logger.Error("fetch created pipeline binding failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created pipeline binding (CWB_KB_PB_105)"})
	}

	return c.JSON(http.StatusOK, pipelineBindingDetailResponse{Status: true, Record: record})
}

// UpdatePipelineBinding handles PUT /api/v1/kb/pipeline-bindings/:id,
// rebinding an existing store to a different pipeline.
func UpdatePipelineBinding(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PB_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_PB_201)"})
	}

	var payload struct {
		PipelineID *int64 `json:"pipeline_id"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PB_202)"})
	}
	if payload.PipelineID == nil || *payload.PipelineID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "pipeline_id is required (CWB_KB_PB_203)"})
	}

	db := ApiTypes.ProjectDBHandle
	result, err := db.Exec(
		"UPDATE kb.pipeline_bindings SET pipeline_id = $1, modify_time = NOW() WHERE id = $2",
		*payload.PipelineID, id,
	)
	if err != nil {
		logger.Error("update pipeline binding failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update pipeline binding (CWB_KB_PB_204)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected pipeline binding failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify pipeline binding update (CWB_KB_PB_205)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PB_206)"})
	}

	record, err := fetchPipelineBindingByID(db, id)
	if err != nil {
		logger.Error("fetch updated pipeline binding failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve updated pipeline binding (CWB_KB_PB_207)"})
	}

	return c.JSON(http.StatusOK, pipelineBindingDetailResponse{Status: true, Record: record})
}

// DeletePipelineBinding handles DELETE /api/v1/kb/pipeline-bindings/:id,
// unbinding a store so it falls back to the system default pipeline.
func DeletePipelineBinding(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PB_300")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_PB_301)"})
	}

	db := ApiTypes.ProjectDBHandle
	result, err := db.Exec("DELETE FROM kb.pipeline_bindings WHERE id = $1", id)
	if err != nil {
		logger.Error("delete pipeline binding failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete pipeline binding (CWB_KB_PB_302)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected delete pipeline binding failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify pipeline binding delete (CWB_KB_PB_303)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_PB_304)"})
	}

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}
