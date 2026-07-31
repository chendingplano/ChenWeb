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

const (
	kbKnowledgeStoreTableSingular = "kb.knowledge_store"
	kbKnowledgeStoreTablePlural   = "kb.knowledge_stores"
)

type knowledgeStoreRecord struct {
	ID          int64           `json:"id"`
	TenantID    *string         `json:"tenant_id,omitempty"`
	KSType      *string         `json:"ks_type,omitempty"`
	KSName      string          `json:"ks_name"`
	KSDesc      *string         `json:"ks_desc,omitempty"`
	KSSyncMode  *string         `json:"ks_sync_mode,omitempty"`
	KSSources   []string        `json:"ks_sources,omitempty"`
	Status      *string         `json:"status,omitempty"`
	Notes       *string         `json:"notes,omitempty"`
	ErrorMsg    *string         `json:"error_msg,omitempty"`
	PublicInfo  json.RawMessage `json:"public_info,omitempty"`
	PrivateInfo json.RawMessage `json:"private_info,omitempty"`
	CreateTime  time.Time       `json:"create_time"`
	ModifyTime  time.Time       `json:"modify_time"`
}

type listKnowledgeStoresResponse struct {
	Status  bool                   `json:"status"`
	Results []knowledgeStoreRecord `json:"results"`
	Total   int                    `json:"total"`
}

type knowledgeStoreDetailResponse struct {
	Status bool                 `json:"status"`
	Record knowledgeStoreRecord `json:"record"`
}

func resolveKnowledgeStoreTable(db *sql.DB) (string, error) {
	const query = `
SELECT
	to_regclass($1)::text AS singular,
	to_regclass($2)::text AS plural
`

	var singular sql.NullString
	var plural sql.NullString
	if err := db.QueryRow(query, kbKnowledgeStoreTableSingular, kbKnowledgeStoreTablePlural).Scan(&singular, &plural); err != nil {
		return "", err
	}
	if singular.Valid && strings.TrimSpace(singular.String) != "" {
		return singular.String, nil
	}
	if plural.Valid && strings.TrimSpace(plural.String) != "" {
		return plural.String, nil
	}
	return "", fmt.Errorf("neither %s nor %s exists", kbKnowledgeStoreTableSingular, kbKnowledgeStoreTablePlural)
}

func fetchKnowledgeStoreByID(db *sql.DB, storeTable string, id int64) (knowledgeStoreRecord, error) {
	query := fmt.Sprintf(`
SELECT
    id, tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode,
    ks_sources, status, notes, error_msg, public_info, private_info,
    create_time, modify_time
FROM %s
WHERE id = $1
`, storeTable)

	row := db.QueryRow(query, id)
	var (
		record              knowledgeStoreRecord
		sources             pq.StringArray
		publicInfoNullable  sql.NullString
		privateInfoNullable sql.NullString
	)
	if err := row.Scan(
		&record.ID, &record.TenantID, &record.KSType, &record.KSName, &record.KSDesc, &record.KSSyncMode,
		&sources, &record.Status, &record.Notes, &record.ErrorMsg, &publicInfoNullable, &privateInfoNullable,
		&record.CreateTime, &record.ModifyTime,
	); err != nil {
		return knowledgeStoreRecord{}, err
	}
	record.KSSources = []string(sources)
	if publicInfoNullable.Valid && strings.TrimSpace(publicInfoNullable.String) != "" {
		record.PublicInfo = json.RawMessage([]byte(publicInfoNullable.String))
	}
	if privateInfoNullable.Valid && strings.TrimSpace(privateInfoNullable.String) != "" {
		record.PrivateInfo = json.RawMessage([]byte(privateInfoNullable.String))
	}
	return record, nil
}

func decodeStringArrayValue(raw json.RawMessage) (*[]string, error) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("must be string[] or null")
	}
	clean := make([]string, 0, len(arr))
	for _, item := range arr {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		clean = append(clean, value)
	}
	return &clean, nil
}

func ListKnowledgeStores(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_S_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	storeTable, err := resolveKnowledgeStoreTable(db)
	if err != nil {
		logger.Error("resolve knowledge store table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve knowledge store table (CWB_KB_S_010)"})
	}

	query := fmt.Sprintf(`
SELECT
    id, tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode,
    ks_sources, status, notes, error_msg, public_info, private_info,
    create_time, modify_time
FROM %s
ORDER BY modify_time DESC, id DESC
`, storeTable)

	rows, err := db.Query(query)
	if err != nil {
		logger.Error("query knowledge stores failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve knowledge stores (CWB_KB_S_011)"})
	}
	defer rows.Close()

	results := make([]knowledgeStoreRecord, 0)
	for rows.Next() {
		var (
			record              knowledgeStoreRecord
			sources             pq.StringArray
			publicInfoNullable  sql.NullString
			privateInfoNullable sql.NullString
		)
		if err := rows.Scan(
			&record.ID, &record.TenantID, &record.KSType, &record.KSName, &record.KSDesc, &record.KSSyncMode,
			&sources, &record.Status, &record.Notes, &record.ErrorMsg, &publicInfoNullable, &privateInfoNullable,
			&record.CreateTime, &record.ModifyTime,
		); err != nil {
			logger.Error("scan knowledge store failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan knowledge stores (CWB_KB_S_012)"})
		}
		record.KSSources = []string(sources)
		if publicInfoNullable.Valid && strings.TrimSpace(publicInfoNullable.String) != "" {
			record.PublicInfo = json.RawMessage([]byte(publicInfoNullable.String))
		}
		if privateInfoNullable.Valid && strings.TrimSpace(privateInfoNullable.String) != "" {
			record.PrivateInfo = json.RawMessage([]byte(privateInfoNullable.String))
		}
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate knowledge stores failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate knowledge stores (CWB_KB_S_013)"})
	}

	return c.JSON(http.StatusOK, listKnowledgeStoresResponse{
		Status:  true,
		Results: results,
		Total:   len(results),
	})
}

func CreateKnowledgeStore(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_S_100")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_S_101)"})
	}

	db := ApiTypes.ProjectDBHandle
	storeTable, err := resolveKnowledgeStoreTable(db)
	if err != nil {
		logger.Error("resolve knowledge store table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve knowledge store table (CWB_KB_S_102)"})
	}

	var (
		tenantID    any = "-"
		ksType      any
		ksName      string
		ksDesc      any
		ksSyncMode  any = "manual"
		ksSources   any = pq.Array([]string{})
		status      any = "active"
		notes       any
		publicInfo  any = "{}"
		privateInfo any = "{}"
	)

	if raw, ok := payload["tenant_id"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid tenant_id: %v (CWB_KB_S_103)", err)})
		}
		if value != nil {
			tenantID = *value
		}
	}
	if raw, ok := payload["ks_type"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid ks_type: %v (CWB_KB_S_104)", err)})
		}
		if value != nil {
			ksType = *value
		}
	}
	if raw, ok := payload["ks_name"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil || value == nil || strings.TrimSpace(*value) == "" {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "ks_name is required (CWB_KB_S_105)"})
		}
		ksName = *value
	} else {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "ks_name is required (CWB_KB_S_105)"})
	}
	if raw, ok := payload["ks_desc"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid ks_desc: %v (CWB_KB_S_106)", err)})
		}
		if value != nil {
			ksDesc = *value
		}
	}
	if raw, ok := payload["ks_sync_mode"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid ks_sync_mode: %v (CWB_KB_S_107)", err)})
		}
		if value != nil {
			ksSyncMode = *value
		}
	}
	if raw, ok := payload["ks_sources"]; ok {
		value, err := decodeStringArrayValue(raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid ks_sources: %v (CWB_KB_S_108)", err)})
		}
		if value != nil {
			ksSources = pq.Array(*value)
		}
	}
	if raw, ok := payload["status"]; ok {
		value, err := decodeStringValue(raw, true)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid status: %v (CWB_KB_S_109)", err)})
		}
		if value != nil {
			status = *value
		}
	}
	if raw, ok := payload["notes"]; ok {
		value, err := decodeStringValue(raw, false)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid notes: %v (CWB_KB_S_110)", err)})
		}
		if value != nil {
			notes = *value
		}
	}
	if raw, ok := payload["public_info"]; ok {
		if strings.TrimSpace(string(raw)) == "null" {
			publicInfo = "{}"
		} else {
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid public_info: %v (CWB_KB_S_111)", err)})
			}
			publicInfo = compact
		}
	}
	if raw, ok := payload["private_info"]; ok {
		if strings.TrimSpace(string(raw)) == "null" {
			privateInfo = "{}"
		} else {
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid private_info: %v (CWB_KB_S_112)", err)})
			}
			privateInfo = compact
		}
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
    tenant_id, ks_type, ks_name, ks_desc, ks_sync_mode, ks_sources, status, notes, public_info, private_info
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING id
`, storeTable)

	var id int64
	if err := db.QueryRow(query, tenantID, ksType, ksName, ksDesc, ksSyncMode, ksSources, status, notes, publicInfo, privateInfo).Scan(&id); err != nil {
		logger.Error("insert knowledge store failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create knowledge store (CWB_KB_S_113)"})
	}

	record, err := fetchKnowledgeStoreByID(db, storeTable, id)
	if err != nil {
		logger.Error("fetch created knowledge store failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created knowledge store (CWB_KB_S_114)"})
	}

	return c.JSON(http.StatusOK, knowledgeStoreDetailResponse{Status: true, Record: record})
}

func UpdateKnowledgeStore(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_S_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_S_201)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_S_202)"})
	}

	db := ApiTypes.ProjectDBHandle
	storeTable, err := resolveKnowledgeStoreTable(db)
	if err != nil {
		logger.Error("resolve knowledge store table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve knowledge store table (CWB_KB_S_203)"})
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
		case "tenant_id", "ks_type", "ks_name", "ks_sync_mode", "status":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_S_204)", field, err)})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}
		case "ks_desc", "notes", "error_msg":
			value, err := decodeStringValue(raw, false)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_S_205)", field, err)})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, *value)
			}
		case "ks_sources":
			value, err := decodeStringArrayValue(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid ks_sources: %v (CWB_KB_S_206)", err)})
			}
			if value == nil {
				addSet(field, nil)
			} else {
				addSet(field, pq.Array(*value))
			}
		case "public_info", "private_info":
			if strings.TrimSpace(string(raw)) == "null" {
				addSet(field, nil)
				break
			}
			compact, err := compactJSONRaw(raw)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid %s: %v (CWB_KB_S_207)", field, err)})
			}
			addSet(field, compact)
		}
	}

	if len(sets) <= 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_S_208)"})
	}

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d", storeTable, strings.Join(sets, ", "), len(args)+1)
	args = append(args, id)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update knowledge store failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update knowledge store (CWB_KB_S_209)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected knowledge store failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify knowledge store update (CWB_KB_S_210)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_S_211)"})
	}

	record, err := fetchKnowledgeStoreByID(db, storeTable, id)
	if err != nil {
		logger.Error("fetch updated knowledge store failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve updated knowledge store (CWB_KB_S_212)"})
	}

	return c.JSON(http.StatusOK, knowledgeStoreDetailResponse{Status: true, Record: record})
}

func DeleteKnowledgeStore(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_S_300")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_S_301)"})
	}

	db := ApiTypes.ProjectDBHandle
	storeTable, err := resolveKnowledgeStoreTable(db)
	if err != nil {
		logger.Error("resolve knowledge store table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve knowledge store table (CWB_KB_S_302)"})
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", storeTable)
	result, err := db.Exec(query, id)
	if err != nil {
		logger.Error("delete knowledge store failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete knowledge store (CWB_KB_S_303)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected delete knowledge store failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify knowledge store delete (CWB_KB_S_304)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_S_305)"})
	}

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}
