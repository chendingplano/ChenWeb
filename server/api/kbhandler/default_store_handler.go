package kbhandler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

type defaultKnowledgeStoreResponse struct {
	Status   bool                  `json:"status"`
	Record   *knowledgeStoreRecord `json:"record,omitempty"`
	ErrorMsg string                `json:"error_msg,omitempty"`
}

// resolveDefaultKnowledgeStore returns the knowledge store named ksName. The
// name must select exactly one row: an empty name, no match, or several matches
// are all errors, and leave the caller without a default store.
func resolveDefaultKnowledgeStore(db *sql.DB, ksName string) (knowledgeStoreRecord, error) {
	ksName = strings.TrimSpace(ksName)
	if ksName == "" {
		return knowledgeStoreRecord{}, fmt.Errorf("[frontend].default_knowledge_store is not configured (CWB_KB_S_401)")
	}

	storeTable, err := resolveKnowledgeStoreTable(db)
	if err != nil {
		return knowledgeStoreRecord{}, fmt.Errorf("failed to resolve knowledge store table: %v (CWB_KB_S_402)", err)
	}

	query := fmt.Sprintf("SELECT id FROM %s WHERE ks_name = $1 ORDER BY id", storeTable)
	rows, err := db.Query(query, ksName)
	if err != nil {
		return knowledgeStoreRecord{}, fmt.Errorf("failed to query knowledge stores named %q: %v (CWB_KB_S_403)", ksName, err)
	}
	defer rows.Close()

	ids := make([]int64, 0, 2)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return knowledgeStoreRecord{}, fmt.Errorf("failed to scan knowledge stores named %q: %v (CWB_KB_S_404)", ksName, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return knowledgeStoreRecord{}, fmt.Errorf("failed to iterate knowledge stores named %q: %v (CWB_KB_S_405)", ksName, err)
	}

	if len(ids) == 0 {
		return knowledgeStoreRecord{}, fmt.Errorf("[frontend].default_knowledge_store %q matches no knowledge store (CWB_KB_S_406)", ksName)
	}
	if len(ids) > 1 {
		return knowledgeStoreRecord{}, fmt.Errorf("[frontend].default_knowledge_store %q matches %d knowledge stores; it must match exactly one (CWB_KB_S_407)", ksName, len(ids))
	}

	record, err := fetchKnowledgeStoreByID(db, storeTable, ids[0])
	if err != nil {
		return knowledgeStoreRecord{}, fmt.Errorf("failed to retrieve knowledge store %q: %v (CWB_KB_S_408)", ksName, err)
	}
	return record, nil
}

// GetDefaultKnowledgeStore resolves [frontend].default_knowledge_store against
// kb.knowledge_store so /home3/knowledge can preselect a store on entry.
// Endpoint: GET /api/v1/kb/default-store.
//
// A store that cannot be resolved is not a server fault: the response carries
// status=false plus the reason, and the frontend asks the user to select a
// store manually.
func GetDefaultKnowledgeStore(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_S_400")
	defer rc.Close()
	logger := rc.GetLogger()

	ksName := appconfig.GetDefaultKnowledgeStoreName()
	record, err := resolveDefaultKnowledgeStore(ApiTypes.ProjectDBHandle, ksName)
	if err != nil {
		logger.Warn("resolve default knowledge store failed", "ks_name", ksName, "err", err)
		return c.JSON(http.StatusOK, defaultKnowledgeStoreResponse{Status: false, ErrorMsg: err.Error()})
	}

	logger.Info("resolved default knowledge store", "ks_name", record.KSName, "id", record.ID)
	return c.JSON(http.StatusOK, defaultKnowledgeStoreResponse{Status: true, Record: &record})
}
