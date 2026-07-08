package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const objectSearchDefaultPageSize = 50
const objectSearchMaxPageSize = 200

type objectSearchRequest struct {
	Table    string `json:"table"`
	Query    string `json:"query"`
	ObjectID string `json:"object_id"`
	RecordID int64  `json:"record_id"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type objectNodeSummaryDTO struct {
	ID              int64  `json:"id"`
	ObjectID        string `json:"object_id"`
	CanonicalName   string `json:"canonical_name"`
	CanonicalNameEn string `json:"canonical_name_en"`
	ObjectType      string `json:"object_type"`
	ReconcileStatus string `json:"reconcile_status"`
}

type artifactObjectSummaryRowDTO struct {
	ID              int64  `json:"id"`
	ArtifactType    string `json:"artifact_type"`
	ArtifactID      string `json:"artifact_id"`
	ObjectName      string `json:"object_name"`
	ObjectNameEn    string `json:"object_name_en"`
	ObjectID        string `json:"object_id"`
	ReconcileStatus string `json:"reconcile_status"`
}

// normalizeSearchTable validates the requested table, defaulting to
// object_nodes. The second return is false for an unrecognized table.
func normalizeSearchTable(t string) (string, bool) {
	switch strings.TrimSpace(t) {
	case "", "object_nodes":
		return "object_nodes", true
	case "artifact_objects":
		return "artifact_objects", true
	default:
		return "", false
	}
}

func clampObjectSearchPageSize(n int) int {
	if n <= 0 {
		return objectSearchDefaultPageSize
	}
	if n > objectSearchMaxPageSize {
		return objectSearchMaxPageSize
	}
	return n
}

func searchObjectNodes(ctx context.Context, db *sql.DB, req objectSearchRequest, pageSize, offset int) ([]objectNodeSummaryDTO, error) {
	const base = `SELECT id, object_id, canonical_name, COALESCE(canonical_name_en, ''), object_type, reconcile_status FROM kb.object_nodes`
	var (
		query string
		args  []any
	)
	switch {
	case req.RecordID > 0:
		query = base + ` WHERE id = $1 ORDER BY id`
		args = []any{req.RecordID}
	case req.Query != "":
		query = base + ` WHERE (canonical_name ILIKE $1 OR object_id ILIKE $1 OR COALESCE(search_document, '') ILIKE $1) ORDER BY id LIMIT $2 OFFSET $3`
		args = []any{"%" + req.Query + "%", pageSize, offset}
	default:
		query = base + ` ORDER BY id LIMIT $1 OFFSET $2`
		args = []any{pageSize, offset}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]objectNodeSummaryDTO, 0, pageSize)
	for rows.Next() {
		var r objectNodeSummaryDTO
		if err := rows.Scan(&r.ID, &r.ObjectID, &r.CanonicalName, &r.CanonicalNameEn, &r.ObjectType, &r.ReconcileStatus); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func searchArtifactObjects(ctx context.Context, db *sql.DB, req objectSearchRequest, pageSize, offset int) ([]artifactObjectSummaryRowDTO, error) {
	const base = `SELECT id, artifact_type, artifact_id, object_name, COALESCE(object_name_en, ''), COALESCE(object_id, ''), reconcile_status FROM kb.artifact_objects`
	var (
		query string
		args  []any
	)
	switch {
	case req.RecordID > 0:
		query = base + ` WHERE id = $1 ORDER BY id`
		args = []any{req.RecordID}
	case req.ObjectID != "":
		query = base + ` WHERE object_id = $1 ORDER BY id LIMIT $2 OFFSET $3`
		args = []any{req.ObjectID, pageSize, offset}
	case req.Query != "":
		query = base + ` WHERE (object_name ILIKE $1 OR COALESCE(object_name_en, '') ILIKE $1 OR artifact_id ILIKE $1) ORDER BY id LIMIT $2 OFFSET $3`
		args = []any{"%" + req.Query + "%", pageSize, offset}
	default:
		query = base + ` ORDER BY id LIMIT $1 OFFSET $2`
		args = []any{pageSize, offset}
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]artifactObjectSummaryRowDTO, 0, pageSize)
	for rows.Next() {
		var r artifactObjectSummaryRowDTO
		if err := rows.Scan(&r.ID, &r.ArtifactType, &r.ArtifactID, &r.ObjectName, &r.ObjectNameEn, &r.ObjectID, &r.ReconcileStatus); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchObjects handles POST /api/v1/kb/objects/search — the Left Panel Record
// list. It lists or searches either kb.object_nodes (default) or
// kb.artifact_objects, or fetches a single row by integer id via record_id.
func SearchObjects(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OSR_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req objectSearchRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OSR_002)"})
	}
	table, ok := normalizeSearchTable(req.Table)
	if !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid table (CWB_KB_OSR_003)"})
	}
	req.Query = strings.TrimSpace(req.Query)
	req.ObjectID = strings.TrimSpace(req.ObjectID)
	pageSize := clampObjectSearchPageSize(req.PageSize)
	page := req.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_OSR_010)"})
	}
	ctx := c.Request().Context()

	if table == "artifact_objects" {
		rows, err := searchArtifactObjects(ctx, db, req, pageSize, offset)
		if err != nil {
			logger.Error("search artifact objects failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to search artifact objects (CWB_KB_OSR_011)"})
		}
		return c.JSON(http.StatusOK, map[string]any{"status": true, "table": table, "rows": rows})
	}

	rows, err := searchObjectNodes(ctx, db, req, pageSize, offset)
	if err != nil {
		logger.Error("search object nodes failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to search object nodes (CWB_KB_OSR_012)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "table": table, "rows": rows})
}
