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

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

type pipelineRecord struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	DisplayName      *string   `json:"display_name,omitempty"`
	Processors       []string  `json:"processors,omitempty"`
	LegacyEquivalent bool      `json:"legacy_equivalent"`
	CreateTime       time.Time `json:"create_time"`
	ModifyTime       time.Time `json:"modify_time"`
}

type listPipelinesResponse struct {
	Status  bool             `json:"status"`
	Results []pipelineRecord `json:"results"`
	Total   int              `json:"total"`
}

type pipelineDetailResponse struct {
	Status bool           `json:"status"`
	Record pipelineRecord `json:"record"`
}

const pipelineListColumns = `
    id, name, display_name, processors, legacy_equivalent,
    create_time, modify_time
FROM kb.pipelines`

func scanPipelineRecord(scan func(dest ...any) error) (pipelineRecord, error) {
	var (
		record      pipelineRecord
		displayName sql.NullString
		processors  pq.StringArray
	)
	if err := scan(
		&record.ID, &record.Name, &displayName, &processors, &record.LegacyEquivalent,
		&record.CreateTime, &record.ModifyTime,
	); err != nil {
		return pipelineRecord{}, err
	}
	if displayName.Valid && strings.TrimSpace(displayName.String) != "" {
		v := displayName.String
		record.DisplayName = &v
	}
	record.Processors = []string(processors)
	return record, nil
}

func fetchPipelineByID(db *sql.DB, id int64) (pipelineRecord, error) {
	query := "SELECT" + pipelineListColumns + "\nWHERE id = $1"
	row := db.QueryRow(query, id)
	return scanPipelineRecord(row.Scan)
}

func ListPipelines(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_P_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	query := "SELECT" + pipelineListColumns + "\nORDER BY id"
	rows, err := db.Query(query)
	if err != nil {
		logger.Error("query pipelines failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve pipelines (CWB_KB_P_002)"})
	}
	defer rows.Close()

	results := make([]pipelineRecord, 0)
	for rows.Next() {
		record, err := scanPipelineRecord(rows.Scan)
		if err != nil {
			logger.Error("scan pipeline failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan pipelines (CWB_KB_P_003)"})
		}
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate pipelines failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate pipelines (CWB_KB_P_004)"})
	}

	return c.JSON(http.StatusOK, listPipelinesResponse{Status: true, Results: results, Total: len(results)})
}

func CreatePipeline(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_P_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_P_101)"})
	}

	var (
		name             string
		displayName      any
		processors       any = pq.Array([]string{})
		legacyEquivalent bool
	)

	if raw, ok := payload["name"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil || value == nil || strings.TrimSpace(*value) == "" {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_P_102)"})
		}
		name = *value
	} else {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_P_102)"})
	}
	if raw, ok := payload["display_name"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid display_name: %v (CWB_KB_P_103)", err)})
		}
		if value != nil {
			displayName = *value
		}
	}
	if raw, ok := payload["processors"]; ok {
		value, err := decodeStringArrayValue(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid processors: %v (CWB_KB_P_104)", err)})
		}
		if value != nil {
			processors = pq.Array(*value)
		}
	}
	if raw, ok := payload["legacy_equivalent"]; ok {
		if err := json.Unmarshal(raw, &legacyEquivalent); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid legacy_equivalent: %v (CWB_KB_P_105)", err)})
		}
	}

	db := ApiTypes.ProjectDBHandle
	const insertQuery = `
INSERT INTO kb.pipelines (
    name, display_name, processors, legacy_equivalent
) VALUES (
    $1, $2, $3, $4
)
RETURNING id
`
	var id int64
	if err := db.QueryRow(insertQuery, name, displayName, processors, legacyEquivalent).Scan(&id); err != nil {
		logger.Error("insert pipeline failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
	}

	record, err := fetchPipelineByID(db, id)
	if err != nil {
		logger.Error("fetch created pipeline failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created pipeline (CWB_KB_P_107)"})
	}

	return c.JSON(http.StatusOK, pipelineDetailResponse{Status: true, Record: record})
}

func UpdatePipeline(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_P_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_P_201)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_P_202)"})
	}

	sets := []string{"modify_time = NOW()"}
	args := make([]any, 0, len(payload)+1)
	addSet := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	fields := make([]string, 0, len(payload))
	for field := range payload {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	for _, field := range fields {
		raw := payload[field]
		switch field {
		case "name":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil || strings.TrimSpace(*value) == "" {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name cannot be empty (CWB_KB_P_203)"})
			}
			addSet(field, *value)
		case "display_name":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid display_name: %v (CWB_KB_P_204)", err)})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}
		case "processors":
			value, err := decodeStringArrayValue(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid processors: %v (CWB_KB_P_205)", err)})
			}
			if value == nil {
				addSet(field, pq.Array([]string{}))
			} else {
				addSet(field, pq.Array(*value))
			}
		case "legacy_equivalent":
			var v bool
			if err := json.Unmarshal(raw, &v); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid legacy_equivalent: %v (CWB_KB_P_206)", err)})
			}
			addSet(field, v)
		}
	}

	if len(sets) <= 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_P_207)"})
	}

	db := ApiTypes.ProjectDBHandle
	query := fmt.Sprintf("UPDATE kb.pipelines SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args)+1)
	args = append(args, id)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update pipeline failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update pipeline (CWB_KB_P_208)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected pipeline failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify pipeline update (CWB_KB_P_209)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_P_210)"})
	}

	record, err := fetchPipelineByID(db, id)
	if err != nil {
		logger.Error("fetch updated pipeline failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve updated pipeline (CWB_KB_P_211)"})
	}

	return c.JSON(http.StatusOK, pipelineDetailResponse{Status: true, Record: record})
}

func DeletePipeline(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_P_300")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_P_301)"})
	}

	db := ApiTypes.ProjectDBHandle
	result, err := db.Exec("DELETE FROM kb.pipelines WHERE id = $1", id)
	if err != nil {
		logger.Error("delete pipeline failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete pipeline (CWB_KB_P_302)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected delete pipeline failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify pipeline delete (CWB_KB_P_303)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_P_304)"})
	}

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}
