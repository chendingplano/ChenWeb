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

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

type pipelineRecord struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	DisplayName      *string   `json:"display_name,omitempty"`
	Description      *string   `json:"description,omitempty"`
	Processors       []string  `json:"processors,omitempty"`
	LegacyEquivalent bool      `json:"legacy_equivalent"`
	IsSystemDefault  bool      `json:"is_system_default"`
	Version          int       `json:"version"`
	PipelineStatus   string    `json:"status"`
	CreateTime       time.Time `json:"create_time"`
	ModifyTime       time.Time `json:"modify_time"`
}

// pipelineRuleDraftPayload is one kb.pipeline_rules row (a processor gate
// and/or DR6 DAG-edge declaration) authored together with a new pipeline
// version, in the same request and the same transaction (ADR 2026081001
// DR2). Predicate/Effect may be omitted for a row that only declares
// DependsOnProcessors edges with no gate of its own.
type pipelineRuleDraftPayload struct {
	Name                string          `json:"name"`
	Priority            int             `json:"priority"`
	TargetProcessor     string          `json:"target_processor"`
	Effect              string          `json:"effect"`
	Predicate           json.RawMessage `json:"predicate"`
	DependsOnProcessors []string        `json:"depends_on_processors"`
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
    id, name, display_name, description, processors, legacy_equivalent, is_system_default,
    version, status, create_time, modify_time
FROM kb.pipelines`

func scanPipelineRecord(scan func(dest ...any) error) (pipelineRecord, error) {
	var (
		record      pipelineRecord
		displayName sql.NullString
		description sql.NullString
		processors  pq.StringArray
	)
	if err := scan(
		&record.ID, &record.Name, &displayName, &description, &processors, &record.LegacyEquivalent, &record.IsSystemDefault,
		&record.Version, &record.PipelineStatus, &record.CreateTime, &record.ModifyTime,
	); err != nil {
		return pipelineRecord{}, err
	}
	if displayName.Valid && strings.TrimSpace(displayName.String) != "" {
		v := displayName.String
		record.DisplayName = &v
	}
	if description.Valid && strings.TrimSpace(description.String) != "" {
		v := description.String
		record.Description = &v
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
		description      any
		processors       []string
		legacyEquivalent bool
		isSystemDefault  bool
		ruleDrafts       []pipelineRuleDraftPayload
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
	if raw, ok := payload["description"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid description: %v (CWB_KB_P_108)", err)})
		}
		if value != nil {
			description = *value
		}
	}
	if raw, ok := payload["is_system_default"]; ok {
		if err := json.Unmarshal(raw, &isSystemDefault); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid is_system_default: %v (CWB_KB_P_109)", err)})
		}
	}
	if raw, ok := payload["processors"]; ok {
		value, err := decodeStringArrayValue(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid processors: %v (CWB_KB_P_104)", err)})
		}
		if value != nil {
			processors = *value
		}
	}
	if raw, ok := payload["legacy_equivalent"]; ok {
		if err := json.Unmarshal(raw, &legacyEquivalent); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid legacy_equivalent: %v (CWB_KB_P_105)", err)})
		}
	}
	if raw, ok := payload["rules"]; ok {
		if err := json.Unmarshal(raw, &ruleDrafts); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid rules: %v (CWB_KB_P_110)", err)})
		}
	}

	gates := make([]docprocessing.PipelineGate, 0, len(ruleDrafts))
	for _, draft := range ruleDrafts {
		gate := docprocessing.PipelineGate{
			Name:                draft.Name,
			Priority:            draft.Priority,
			TargetProcessor:     draft.TargetProcessor,
			Effect:              draft.Effect,
			Active:              true,
			DependsOnProcessors: draft.DependsOnProcessors,
		}
		if len(draft.Predicate) > 0 {
			if err := json.Unmarshal(draft.Predicate, &gate.Predicate); err != nil || semrules.Validate(gate.Predicate) != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid predicate for rule %q (CWB_KB_P_111)", draft.Name)})
			}
			_, checksum, err := semrules.Canonicalize(gate.Predicate)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid predicate for rule %q (CWB_KB_P_111)", draft.Name)})
			}
			gate.PredicateChecksum = checksum
			gate.RequiredFacets = semrules.Analyze(gate.Predicate).RequiredDocumentFacets
		}
		gates = append(gates, gate)
	}

	// ADR 2026081001 DR8: reject at creation time, before anything is
	// written, naming the specific violation.
	if err := docprocessing.ValidatePipelineVersion(docprocessing.PipelineVersionDraft{Processors: processors, Rules: gates}); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("%v (CWB_KB_P_112)", err)})
	}

	db := ApiTypes.ProjectDBHandle
	tx, err := db.Begin()
	if err != nil {
		logger.Error("begin pipeline version transaction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
	}
	defer tx.Rollback()

	// ADR 2026081001 DR1: lock the current version (if any) so concurrent
	// authoring for the same name serializes instead of racing on the next
	// version number.
	var priorID sql.NullInt64
	var priorVersion int
	err = tx.QueryRow(`SELECT id, version FROM kb.pipelines WHERE name = $1 ORDER BY version DESC LIMIT 1 FOR UPDATE`, name).Scan(&priorID, &priorVersion)
	if err != nil && err != sql.ErrNoRows {
		logger.Error("lock prior pipeline version failed", "name", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
	}
	nextVersion := priorVersion + 1

	var id int64
	const insertQuery = `
INSERT INTO kb.pipelines (
    name, display_name, description, processors, legacy_equivalent, is_system_default, version, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 'active'
)
RETURNING id
`
	if err := tx.QueryRow(insertQuery, name, displayName, description, pq.Array(processors), legacyEquivalent, isSystemDefault, nextVersion).Scan(&id); err != nil {
		logger.Error("insert pipeline failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
	}

	if priorID.Valid {
		if _, err := tx.Exec(`UPDATE kb.pipelines SET status = 'superseded', modify_time = NOW() WHERE id = $1`, priorID.Int64); err != nil {
			logger.Error("supersede prior pipeline version failed", "prior_id", priorID.Int64, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
		}
	}

	for _, gate := range gates {
		var predicate, predicateChecksum any
		if gate.PredicateChecksum != "" {
			raw, err := json.Marshal(gate.Predicate)
			if err != nil {
				logger.Error("marshal rule predicate failed", "err", err)
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
			}
			predicate = raw
			predicateChecksum = gate.PredicateChecksum
		}
		var effect any
		if strings.TrimSpace(gate.Effect) != "" {
			effect = gate.Effect
		}
		var targetProcessor any
		if strings.TrimSpace(gate.TargetProcessor) != "" {
			targetProcessor = gate.TargetProcessor
		}
		requiredFacets, err := json.Marshal(gate.RequiredFacets)
		if err != nil {
			logger.Error("marshal rule required facets failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
		}
		if _, err := tx.Exec(`
INSERT INTO kb.pipeline_rules (
    name, priority, pipeline_id, target_processor, effect, predicate, predicate_checksum,
    required_facets, depends_on_processors, active, approval_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, true, 'approved'
)`, gate.Name, gate.Priority, id, targetProcessor, effect, predicate, predicateChecksum, requiredFacets, pq.Array(gate.DependsOnProcessors)); err != nil {
			logger.Error("insert pipeline rule failed", "name", gate.Name, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error("commit pipeline version transaction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create pipeline (CWB_KB_P_106)"})
	}

	record, err := fetchPipelineByID(db, id)
	if err != nil {
		logger.Error("fetch created pipeline failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created pipeline (CWB_KB_P_107)"})
	}
	reloadAfterPipelineWrite(c.Request().Context(), db, logger, "pipeline version create")

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
		case "description":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid description: %v (CWB_KB_P_212)", err)})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}
		case "is_system_default":
			var v bool
			if err := json.Unmarshal(raw, &v); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid is_system_default: %v (CWB_KB_P_213)", err)})
			}
			addSet(field, v)
		case "processors":
			// ADR 2026081001 DR1: a pipeline version's processors[] is
			// immutable. Changing it means authoring a new version via
			// CreatePipeline, never an in-place UPDATE.
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "processors cannot be edited in place; author a new pipeline version instead (CWB_KB_P_205)"})
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

// DeletePipeline no longer exists (ADR 2026081001 DR1): a pipeline version
// is immutable and cannot be deleted out from under whatever binding still
// points at it. Superseding it with a new version is the only "remove".
