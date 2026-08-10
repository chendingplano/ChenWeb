package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
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
	BindingKind       string          `json:"binding_kind"`
	Predicate         json.RawMessage `json:"predicate,omitempty"`
	PredicateChecksum string          `json:"predicate_checksum,omitempty"`
	Active            bool            `json:"active"`
	TenantID          string          `json:"tenant_id,omitempty"`
	UserID            string          `json:"user_id,omitempty"`
	InputRecordID     int64           `json:"input_record_id,omitempty"`
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
    b.pipeline_id, p.name, b.binding_kind,
    COALESCE(b.predicate, '{}'::jsonb)::text, COALESCE(b.predicate_checksum, ''), b.active,
    COALESCE(b.tenant_id, '-'), COALESCE(b.user_id, ''), COALESCE(b.input_record_id, 0),
    b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id`

func scanPipelineBindingRecord(scan func(dest ...any) error) (pipelineBindingRecord, error) {
	var record pipelineBindingRecord
	var predicateRaw string
	if err := scan(
		&record.ID, &record.Name, &record.Priority, &record.KSStoreID,
		&record.PipelineID, &record.PipelineName, &record.BindingKind,
		&predicateRaw, &record.PredicateChecksum, &record.Active,
		&record.TenantID, &record.UserID, &record.InputRecordID,
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
		BindingKind       string          `json:"binding_kind"`
		Predicate         json.RawMessage `json:"predicate"`
		PredicateChecksum string          `json:"predicate_checksum"`
		Active            *bool           `json:"active"`
		TenantID          string          `json:"tenant_id"`
		UserID            string          `json:"user_id"`
		InputRecordID     *int64          `json:"input_record_id"`
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
	if payload.InputRecordID != nil && *payload.InputRecordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid input_record_id (CWB_KB_PB_114)"})
	}
	tenantID := strings.TrimSpace(payload.TenantID)
	if tenantID == "" {
		tenantID = "-"
	}
	userID := strings.TrimSpace(payload.UserID)
	var inputRecordID any
	if payload.InputRecordID != nil {
		inputRecordID = *payload.InputRecordID
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
	var id int64
	if err := db.QueryRow(
		"INSERT INTO kb.pipeline_bindings (name, priority, ks_store_id, pipeline_id, binding_kind, predicate, predicate_checksum, active, tenant_id, user_id, input_record_id) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11) RETURNING id",
		strings.TrimSpace(payload.Name), priority, ksStoreID, *payload.PipelineID, bindingKind, predicate, predicateChecksum, active, tenantID, userID, inputRecordID,
	).Scan(&id); err != nil {
		logger.Error("insert pipeline binding failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline binding (CWB_KB_PB_104)"})
	}

	record, err := fetchPipelineBindingByID(db, id)
	if err != nil {
		logger.Error("fetch created pipeline binding failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created pipeline binding (CWB_KB_PB_105)"})
	}
	writePolicyAuditEvent(c, rc, logger, policyaudit.Event{
		Kind: policyaudit.EventBindingAuthored, SubjectKind: bindingKind, SubjectID: id,
		Detail: map[string]any{"predicate_checksum": predicateChecksum, "pipeline_id": *payload.PipelineID, "binding_kind": bindingKind},
	})
	reloadAfterPipelineWrite(c.Request().Context(), db, logger, "pipeline binding create")

	return c.JSON(http.StatusOK, pipelineBindingDetailResponse{Status: true, Record: record})
}

// UpdatePipelineBinding handles PUT /api/v1/kb/pipeline-bindings/:id.
// It permits draft-policy binding metadata and predicate updates, preserving the
// predicate checksum as a function of the validated SemRules document.
func UpdatePipelineBinding(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PB_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_PB_201)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_PB_202)"})
	}

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	sets := make([]string, 0, len(fields)+1)
	args := make([]any, 0, len(fields)+2)
	addSet := func(column, expression string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = %s", column, fmt.Sprintf(expression, len(args))))
	}

	var requestedChecksum *string
	if raw, ok := payload["predicate_checksum"]; ok {
		var checksum string
		if err := json.Unmarshal(raw, &checksum); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate_checksum (CWB_KB_PB_210)"})
		}
		requestedChecksum = &checksum
	}

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "name":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name cannot be empty (CWB_KB_PB_203)"})
			}
			addSet("name", "$%d", strings.TrimSpace(value))
		case "priority":
			var value int
			if err := json.Unmarshal(raw, &value); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid priority (CWB_KB_PB_204)"})
			}
			addSet("priority", "$%d", value)
		case "pipeline_id":
			var value int64
			if err := json.Unmarshal(raw, &value); err != nil || value <= 0 {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid pipeline_id (CWB_KB_PB_205)"})
			}
			addSet("pipeline_id", "$%d", value)
		case "tenant_id":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid tenant_id (CWB_KB_PB_216)"})
			}
			value = strings.TrimSpace(value)
			if value == "" {
				value = "-"
			}
			addSet("tenant_id", "$%d", value)
		case "user_id":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid user_id (CWB_KB_PB_217)"})
			}
			addSet("user_id", "$%d", strings.TrimSpace(value))
		case "ks_store_id", "input_record_id":
			var value *int64
			if err := json.Unmarshal(raw, &value); err != nil || (value != nil && *value <= 0) {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid %s (CWB_KB_PB_218)", field)})
			}
			if value == nil {
				addSet(field, "$%d", nil)
			} else {
				addSet(field, "$%d", *value)
			}
		case "binding_kind":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil || (value != "conditional" && value != "store_default") {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid binding_kind (CWB_KB_PB_206)"})
			}
			addSet("binding_kind", "$%d", value)
		case "active":
			var value bool
			if err := json.Unmarshal(raw, &value); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid active (CWB_KB_PB_207)"})
			}
			addSet("active", "$%d", value)
		case "predicate":
			var doc semrules.Document
			if err := json.Unmarshal(raw, &doc); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PB_211)"})
			}
			if err := semrules.Validate(doc); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PB_212)"})
			}
			_, checksum, err := semrules.Canonicalize(doc)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid predicate (CWB_KB_PB_213)"})
			}
			if requestedChecksum != nil && strings.TrimSpace(*requestedChecksum) != checksum {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "predicate_checksum mismatch (CWB_KB_PB_214)"})
			}
			addSet("predicate", "$%d::jsonb", string(raw))
			addSet("predicate_checksum", "$%d", checksum)
		case "predicate_checksum":
			// It is verified when predicate is supplied and cannot be changed on its own.
		default:
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_PB_215)"})
		}
	}
	if len(sets) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_PB_215)"})
	}

	db := ApiTypes.ProjectDBHandle
	sets = append(sets, "modify_time = NOW()")
	args = append(args, id)
	query := fmt.Sprintf("UPDATE kb.pipeline_bindings SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args))
	result, err := db.Exec(query, args...)
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
	reloadAfterPipelineWrite(c.Request().Context(), db, logger, "pipeline binding update")

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
	reloadAfterPipelineWrite(c.Request().Context(), db, logger, "pipeline binding delete")

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}
