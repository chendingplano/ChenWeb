package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// Doc processors are the pipeline's processing units (capsule §7). kb.doc_processors is the
// admin-managed catalog of those processors — an editorial/management table the pipeline
// execution machinery does not read at runtime (that metadata lives in kb.processor_registry
// and the Go literal productionProcessorSpecs). This handler exposes flat CRUD over the catalog.

type docProcessorRecord struct {
	NameAsID    string    `json:"name_as_id"`
	DisplayName string    `json:"display_name"`
	Description *string   `json:"description,omitempty"`
	Type        string    `json:"type"`
	RequireLLM  bool      `json:"require_llm"`
	Status      string    `json:"status"`
	Notes       *string   `json:"notes,omitempty"`
	Requires    []string  `json:"requires"`
	CreateTime  time.Time `json:"create_time"`
	ModifyTime  time.Time `json:"modify_time"`
}

type listDocProcessorsResponse struct {
	Status  bool                 `json:"status"`
	Results []docProcessorRecord `json:"results"`
	Total   int                  `json:"total"`
}

type docProcessorResponse struct {
	Status bool               `json:"status"`
	Record docProcessorRecord `json:"record"`
}

type docProcessorDeleteResponse struct {
	Status  bool `json:"status"`
	Deleted int  `json:"deleted"`
}

const docProcessorColumns = `
    name_as_id, display_name, description, type, require_llm, status, notes, requires, create_time, modify_time
FROM kb.doc_processors`

func scanDocProcessorRecord(scan func(dest ...any) error) (docProcessorRecord, error) {
	var (
		rec         docProcessorRecord
		description sql.NullString
		notes       sql.NullString
		requiresRaw []byte
	)
	if err := scan(
		&rec.NameAsID, &rec.DisplayName, &description, &rec.Type,
		&rec.RequireLLM, &rec.Status, &notes, &requiresRaw, &rec.CreateTime, &rec.ModifyTime,
	); err != nil {
		return docProcessorRecord{}, err
	}
	rec.Requires = []string{}
	if len(requiresRaw) > 0 {
		if err := json.Unmarshal(requiresRaw, &rec.Requires); err != nil {
			return docProcessorRecord{}, err
		}
	}
	if description.Valid && strings.TrimSpace(description.String) != "" {
		v := description.String
		rec.Description = &v
	}
	if notes.Valid && strings.TrimSpace(notes.String) != "" {
		v := notes.String
		rec.Notes = &v
	}
	return rec, nil
}

func fetchDocProcessorByID(db *sql.DB, name string) (docProcessorRecord, error) {
	query := "SELECT" + docProcessorColumns + "\nWHERE name_as_id = $1"
	return scanDocProcessorRecord(db.QueryRow(query, name).Scan)
}

// decodeRequiresList parses a requires payload: a JSON array of strings
// (null yields an empty list). Values are trimmed and de-duplicated.
func decodeRequiresList(raw json.RawMessage) ([]string, error) {
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out, nil
}

func validDocProcessorType(v string) bool {
	return v == "mandatory" || v == "configurable"
}

func validDocProcessorStatus(v string) bool {
	return v == "active" || v == "disabled" || v == "suspended"
}

// ListDocProcessors handles GET /api/v1/kb/doc-processors, optionally filtered
// by ?search= (ILIKE on name_as_id/display_name).
func ListDocProcessors(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DPR_001")
	defer rc.Close()
	logger := rc.GetLogger()

	search := strings.TrimSpace(c.QueryParam("search"))

	var query string
	var args []any
	if search != "" {
		like := "%" + search + "%"
		query = "SELECT" + docProcessorColumns + `
WHERE name_as_id ILIKE $1 OR display_name ILIKE $1
ORDER BY name_as_id`
		args = []any{like}
	} else {
		query = "SELECT" + docProcessorColumns + "\nORDER BY name_as_id"
	}

	db := ApiTypes.ProjectDBHandle
	rows, err := db.Query(query, args...)
	if err != nil {
		logger.Error("query doc_processors failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve doc processors (CWB_KB_DPR_002)"})
	}
	defer rows.Close()

	results := make([]docProcessorRecord, 0)
	for rows.Next() {
		rec, err := scanDocProcessorRecord(rows.Scan)
		if err != nil {
			logger.Error("scan doc_processor failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan doc processors (CWB_KB_DPR_003)"})
		}
		results = append(results, rec)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate doc_processors failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate doc processors (CWB_KB_DPR_004)"})
	}

	return c.JSON(http.StatusOK, listDocProcessorsResponse{Status: true, Results: results, Total: len(results)})
}

// CreateDocProcessor handles POST /api/v1/kb/doc-processors. name_as_id is
// required and unique; type/status must be valid enum values; status defaults
// to 'active' when omitted.
func CreateDocProcessor(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DPR_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_DPR_101)"})
	}

	var nameAsID, displayName string
	if raw, ok := payload["name_as_id"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil || value == nil || strings.TrimSpace(*value) == "" {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name_as_id is required (CWB_KB_DPR_102)"})
		}
		nameAsID = *value
	} else {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name_as_id is required (CWB_KB_DPR_102)"})
	}
	if raw, ok := payload["display_name"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil || value == nil || strings.TrimSpace(*value) == "" {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "display_name is required (CWB_KB_DPR_103)"})
		}
		displayName = *value
	} else {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "display_name is required (CWB_KB_DPR_103)"})
	}

	processorType := "configurable"
	if raw, ok := payload["type"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil || value == nil || !validDocProcessorType(*value) {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "type must be 'mandatory' or 'configurable' (CWB_KB_DPR_104)"})
		}
		processorType = *value
	}

	requireLLM := false
	if raw, ok := payload["require_llm"]; ok {
		if err := json.Unmarshal(raw, &requireLLM); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid require_llm: %v (CWB_KB_DPR_105)", err)})
		}
	}

	status := "active"
	if raw, ok := payload["status"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil || value == nil || !validDocProcessorStatus(*value) {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "status must be 'active', 'disabled', or 'suspended' (CWB_KB_DPR_106)"})
		}
		status = *value
	}

	requires := []string{}
	if raw, ok := payload["requires"]; ok {
		list, err := decodeRequiresList(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "requires must be an array of strings (CWB_KB_DPR_108)"})
		}
		requires = list
	}

	var description, notes any
	if raw, ok := payload["description"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid description: %v (CWB_KB_DPR_107)", err)})
		}
		if value != nil {
			description = *value
		}
	}
	if raw, ok := payload["notes"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid notes: %v (CWB_KB_DPR_109)", err)})
		}
		if value != nil {
			notes = *value
		}
	}

	db := ApiTypes.ProjectDBHandle
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM kb.doc_processors WHERE name_as_id = $1)`, nameAsID).Scan(&exists); err != nil {
		logger.Error("check duplicate doc processor failed", "name_as_id", nameAsID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create doc processor (CWB_KB_DPR_110)"})
	}
	if exists {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("a doc processor named %q already exists (CWB_KB_DPR_111)", nameAsID)})
	}

	requiresJSON, err := json.Marshal(requires)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to encode requires (CWB_KB_DPR_112)"})
	}
	if _, err := db.Exec(`
INSERT INTO kb.doc_processors (name_as_id, display_name, description, type, require_llm, status, notes, requires)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)`,
		nameAsID, displayName, description, processorType, requireLLM, status, notes, string(requiresJSON)); err != nil {
		logger.Error("insert doc processor failed", "name_as_id", nameAsID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create doc processor (CWB_KB_DPR_113)"})
	}

	rec, err := fetchDocProcessorByID(db, nameAsID)
	if err != nil {
		logger.Error("fetch created doc processor failed", "name_as_id", nameAsID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created doc processor (CWB_KB_DPR_114)"})
	}

	return c.JSON(http.StatusOK, docProcessorResponse{Status: true, Record: rec})
}

// UpdateDocProcessor handles PUT /api/v1/kb/doc-processors/:name. Only editable
// fields are updated; name_as_id is immutable (rename = create new + delete old).
func UpdateDocProcessor(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DPR_200")
	defer rc.Close()
	logger := rc.GetLogger()

	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_DPR_201)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_DPR_202)"})
	}

	if _, ok := payload["name_as_id"]; ok {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name_as_id is immutable; create a new processor to rename (CWB_KB_DPR_203)"})
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
		case "display_name":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil || strings.TrimSpace(*value) == "" {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "display_name is required (CWB_KB_DPR_204)"})
			}
			addSet(field, *value)
		case "description", "notes":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_DPR_205)", field, err)})
			}
			if value != nil {
				addSet(field, *value)
			} else {
				addSet(field, nil)
			}
		case "type":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil || !validDocProcessorType(*value) {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "type must be 'mandatory' or 'configurable' (CWB_KB_DPR_206)"})
			}
			addSet(field, *value)
		case "require_llm":
			var v bool
			if err := json.Unmarshal(raw, &v); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid require_llm: %v (CWB_KB_DPR_207)", err)})
			}
			addSet(field, v)
		case "status":
			value, err := decodeStringValue(raw, true)
			if err != nil || value == nil || !validDocProcessorStatus(*value) {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "status must be 'active', 'disabled', or 'suspended' (CWB_KB_DPR_208)"})
			}
			addSet(field, *value)
		case "requires":
			list, err := decodeRequiresList(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "requires must be an array of strings (CWB_KB_DPR_215)"})
			}
			encoded, err := json.Marshal(list)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to encode requires (CWB_KB_DPR_216)"})
			}
			args = append(args, string(encoded))
			sets = append(sets, fmt.Sprintf("requires = $%d::jsonb", len(args)))
		default:
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("unknown field %q (CWB_KB_DPR_209)", field)})
		}
	}

	if len(sets) <= 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_DPR_210)"})
	}

	db := ApiTypes.ProjectDBHandle
	query := fmt.Sprintf("UPDATE kb.doc_processors SET %s WHERE name_as_id = $%d", strings.Join(sets, ", "), len(args)+1)
	args = append(args, name)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update doc processor failed", "name_as_id", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update doc processor (CWB_KB_DPR_211)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected doc processor failed", "name_as_id", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify doc processor update (CWB_KB_DPR_212)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "doc processor not found (CWB_KB_DPR_213)"})
	}

	rec, err := fetchDocProcessorByID(db, name)
	if err != nil {
		logger.Error("fetch updated doc processor failed", "name_as_id", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve updated doc processor (CWB_KB_DPR_214)"})
	}

	return c.JSON(http.StatusOK, docProcessorResponse{Status: true, Record: rec})
}

// DeleteDocProcessor handles DELETE /api/v1/kb/doc-processors/:name.
func DeleteDocProcessor(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DPR_300")
	defer rc.Close()
	logger := rc.GetLogger()

	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_DPR_301)"})
	}

	db := ApiTypes.ProjectDBHandle
	result, err := db.Exec(`DELETE FROM kb.doc_processors WHERE name_as_id = $1`, name)
	if err != nil {
		logger.Error("delete doc processor failed", "name_as_id", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc processor (CWB_KB_DPR_302)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected delete doc processor failed", "name_as_id", name, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify doc processor delete (CWB_KB_DPR_303)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "doc processor not found (CWB_KB_DPR_304)"})
	}

	return c.JSON(http.StatusOK, docProcessorDeleteResponse{Status: true, Deleted: int(affected)})
}
