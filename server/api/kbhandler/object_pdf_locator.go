package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// pdfLocatorDTO tells the reused Provisions PDF viewer which document and source
// line spans to open for a clicked chart node. The viewer maps line spans to
// page numbers client-side (via its loaded line/page structure, exactly like
// the Provisions viewer), so the backend intentionally does not return a page.
type pdfLocatorDTO struct {
	ArtifactObjectID int64    `json:"artifact_object_id"`
	InputRecordID    int64    `json:"input_record_id"`
	Document         string   `json:"document"`
	SourceLineSpans  []string `json:"source_line_spans"`
}

// resolveLocatorMention loads the mention row to locate. For an artifact_object_id
// it loads that row directly; for an object_id it picks a representative mention
// (highest confidence, then most recently modified) among the node's mentions.
func resolveLocatorMention(ctx context.Context, db *sql.DB, artifactObjectID int64, objectID string) (id, inputRecordID int64, spansRaw []byte, found bool, err error) {
	var row *sql.Row
	if artifactObjectID > 0 {
		row = db.QueryRowContext(ctx, `
SELECT id, input_record_id, COALESCE(source_line_spans, '[]'::jsonb)
FROM kb.artifact_objects
WHERE id = $1`, artifactObjectID)
	} else {
		row = db.QueryRowContext(ctx, `
SELECT id, input_record_id, COALESCE(source_line_spans, '[]'::jsonb)
FROM kb.artifact_objects
WHERE object_id = $1
ORDER BY confidence DESC, modify_time DESC, id DESC
LIMIT 1`, objectID)
	}
	err = row.Scan(&id, &inputRecordID, &spansRaw)
	if err == sql.ErrNoRows {
		return 0, 0, nil, false, nil
	}
	if err != nil {
		return 0, 0, nil, false, err
	}
	return id, inputRecordID, spansRaw, true, nil
}

// GetObjectPDFLocator handles GET /api/v1/kb/objects/pdf-locator — it accepts
// either artifact_object_id or object_id and returns the document plus source
// line spans for the reused PDF viewer to open and highlight (ADR DR4).
func GetObjectPDFLocator(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OPL_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var artifactObjectID int64
	if raw := strings.TrimSpace(c.QueryParam("artifact_object_id")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			artifactObjectID = n
		}
	}
	objectID := strings.TrimSpace(c.QueryParam("object_id"))
	if artifactObjectID <= 0 && objectID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "artifact_object_id or object_id is required (CWB_KB_OPL_002)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_OPL_010)"})
	}
	ctx := c.Request().Context()

	id, inputRecordID, spansRaw, found, err := resolveLocatorMention(ctx, db, artifactObjectID, objectID)
	if err != nil {
		logger.Error("resolve locator mention failed", "artifact_object_id", artifactObjectID, "object_id", objectID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve mention (CWB_KB_OPL_011)"})
	}
	if !found {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "no mention found to locate (CWB_KB_OPL_003)"})
	}

	var document string
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(file_name, '') FROM kb.inputs WHERE id = $1`, inputRecordID).Scan(&document); err != nil && err != sql.ErrNoRows {
		logger.Error("load document name failed", "input_record_id", inputRecordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load document (CWB_KB_OPL_012)"})
	}

	spans := []string{}
	_ = json.Unmarshal(spansRaw, &spans)

	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"locator": pdfLocatorDTO{
			ArtifactObjectID: id,
			InputRecordID:    inputRecordID,
			Document:         document,
			SourceLineSpans:  spans,
		},
	})
}
