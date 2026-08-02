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
)

type docFacetRecord struct {
	RecordID              int64     `json:"record_id"`
	KSStoreID             int64     `json:"ks_store_id"`
	KnowledgeStoreBinding string    `json:"knowledge_store_binding"`
	InputDocType          string    `json:"input_doc_type"`
	SourceLanguage        string    `json:"source_language"`
	HasDocumentNumber     bool      `json:"has_document_number"`
	CreateTime            time.Time `json:"create_time"`
	ModifyTime            time.Time `json:"modify_time"`
}

type listDocFacetsResponse struct {
	Status  bool             `json:"status"`
	Results []docFacetRecord `json:"results"`
	Total   int              `json:"total"`
}

type docFacetDetailResponse struct {
	Status bool           `json:"status"`
	Record docFacetRecord `json:"record"`
}

const docFacetListColumns = `
    record_id, ks_store_id, knowledge_store_binding, input_doc_type,
    source_language, has_document_number, create_time, modify_time
FROM kb.doc_facets`

func scanDocFacetRecord(scan func(dest ...any) error) (docFacetRecord, error) {
	var rec docFacetRecord
	if err := scan(
		&rec.RecordID, &rec.KSStoreID, &rec.KnowledgeStoreBinding,
		&rec.InputDocType, &rec.SourceLanguage, &rec.HasDocumentNumber,
		&rec.CreateTime, &rec.ModifyTime,
	); err != nil {
		return docFacetRecord{}, err
	}
	return rec, nil
}

func fetchDocFacetByID(db *sql.DB, recordID int64) (docFacetRecord, error) {
	query := "SELECT" + docFacetListColumns + "\nWHERE record_id = $1"
	return scanDocFacetRecord(db.QueryRow(query, recordID).Scan)
}

// ListDocFacets handles GET /api/v1/kb/doc-facets/list with optional search
// parameter ?q=... that matches against knowledge_store_binding, input_doc_type,
// and source_language.
func ListDocFacets(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DFC_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	q := strings.TrimSpace(c.QueryParam("q"))

	var query string
	var args []any
	if q != "" {
		like := "%" + q + "%"
		query = "SELECT" + docFacetListColumns + `
WHERE knowledge_store_binding ILIKE $1
   OR input_doc_type ILIKE $1
   OR source_language ILIKE $1
   OR CAST(record_id AS TEXT) LIKE $1
   OR CAST(ks_store_id AS TEXT) LIKE $1
ORDER BY record_id`
		args = []any{like}
	} else {
		query = "SELECT" + docFacetListColumns + "\nORDER BY record_id"
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		logger.Error("query doc_facets failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve doc facets (CWB_KB_DFC_002)"})
	}
	defer rows.Close()

	results := make([]docFacetRecord, 0)
	for rows.Next() {
		rec, err := scanDocFacetRecord(rows.Scan)
		if err != nil {
			logger.Error("scan doc_facet failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan doc facets (CWB_KB_DFC_003)"})
		}
		results = append(results, rec)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate doc_facets failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate doc facets (CWB_KB_DFC_004)"})
	}

	return c.JSON(http.StatusOK, listDocFacetsResponse{Status: true, Results: results, Total: len(results)})
}

// UpdateDocFacets handles PUT /api/v1/kb/doc-facets/:record_id.
func UpdateDocFacets(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DFC_100")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("record_id"))
	recordID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || recordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid record_id (CWB_KB_DFC_101)"})
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_DFC_102)"})
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
		case "ks_store_id":
			var v int64
			if err := json.Unmarshal(raw, &v); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid ks_store_id: %v (CWB_KB_DFC_103)", err)})
			}
			addSet(field, v)
		case "knowledge_store_binding":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid knowledge_store_binding: %v (CWB_KB_DFC_104)", err)})
			}
			if value != nil {
				addSet(field, *value)
			}
		case "input_doc_type":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid input_doc_type: %v (CWB_KB_DFC_105)", err)})
			}
			if value != nil {
				addSet(field, *value)
			}
		case "source_language":
			value, err := decodeStringValue(raw, true)
			if err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid source_language: %v (CWB_KB_DFC_106)", err)})
			}
			if value != nil {
				addSet(field, *value)
			}
		case "has_document_number":
			var v bool
			if err := json.Unmarshal(raw, &v); err != nil {
				return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("invalid has_document_number: %v (CWB_KB_DFC_107)", err)})
			}
			addSet(field, v)
		}
	}

	if len(sets) <= 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no editable fields in request (CWB_KB_DFC_108)"})
	}

	db := ApiTypes.ProjectDBHandle
	query := fmt.Sprintf("UPDATE kb.doc_facets SET %s WHERE record_id = $%d", strings.Join(sets, ", "), len(args)+1)
	args = append(args, recordID)
	result, err := db.Exec(query, args...)
	if err != nil {
		logger.Error("update doc_facet failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update doc facet (CWB_KB_DFC_109)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected doc_facet failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify doc facet update (CWB_KB_DFC_110)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_DFC_111)"})
	}

	rec, err := fetchDocFacetByID(db, recordID)
	if err != nil {
		logger.Error("fetch updated doc_facet failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve updated doc facet (CWB_KB_DFC_112)"})
	}

	return c.JSON(http.StatusOK, docFacetDetailResponse{Status: true, Record: rec})
}

// DeleteDocFacets handles DELETE /api/v1/kb/doc-facets/:record_id.
func DeleteDocFacets(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DFC_200")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.Param("record_id"))
	recordID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || recordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid record_id (CWB_KB_DFC_201)"})
	}

	db := ApiTypes.ProjectDBHandle
	result, err := db.Exec("DELETE FROM kb.doc_facets WHERE record_id = $1", recordID)
	if err != nil {
		logger.Error("delete doc_facet failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete doc facet (CWB_KB_DFC_202)"})
	}
	affected, err := result.RowsAffected()
	if err != nil {
		logger.Error("rows affected delete doc_facet failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify doc facet delete (CWB_KB_DFC_203)"})
	}
	if affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_DFC_204)"})
	}

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}
