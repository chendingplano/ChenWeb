package kbhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const objectAuditActionMergeNodes = "merge_nodes"

type mergeObjectNodesRequest struct {
	LoserObjectID    string `json:"loser_object_id"`
	SurvivorObjectID string `json:"survivor_object_id"`
}

// validateMergeObjectNodes checks that both object ids are present and distinct.
func validateMergeObjectNodes(loser, survivor string) error {
	if strings.TrimSpace(loser) == "" {
		return fmt.Errorf("loser_object_id is required")
	}
	if strings.TrimSpace(survivor) == "" {
		return fmt.Errorf("survivor_object_id is required")
	}
	if loser == survivor {
		return fmt.Errorf("loser_object_id and survivor_object_id must differ")
	}
	return nil
}

// MergeObjectNodes handles POST /api/v1/kb/objects/merge — reconcile two
// canonical object nodes into one (ADR 2026070101 DR3). The loser's evidence
// mention rows are repointed to the survivor (never deleted), and the loser
// node is marked `merged` with `canonical_object_id` set to the survivor so
// the identity redirects. The update runs in a transaction; the audit entry is
// best-effort after commit.
func MergeObjectNodes(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OMG_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req mergeObjectNodesRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OMG_002)"})
	}
	req.LoserObjectID = strings.TrimSpace(req.LoserObjectID)
	req.SurvivorObjectID = strings.TrimSpace(req.SurvivorObjectID)
	if err := validateMergeObjectNodes(req.LoserObjectID, req.SurvivorObjectID); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error() + " (CWB_KB_OMG_003)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_OMG_010)"})
	}
	ctx := c.Request().Context()

	var present int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kb.object_nodes WHERE object_id = $1 OR object_id = $2`,
		req.LoserObjectID, req.SurvivorObjectID).Scan(&present); err != nil {
		logger.Error("merge existence check failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify object nodes (CWB_KB_OMG_011)"})
	}
	if present < 2 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "both object nodes must exist (CWB_KB_OMG_004)"})
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		logger.Error("merge begin tx failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to start merge (CWB_KB_OMG_012)"})
	}
	defer func() { _ = tx.Rollback() }()

	repointRes, err := tx.ExecContext(ctx,
		`UPDATE kb.artifact_objects SET object_id = $1 WHERE object_id = $2`,
		req.SurvivorObjectID, req.LoserObjectID)
	if err != nil {
		logger.Error("merge repoint mentions failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to repoint mentions (CWB_KB_OMG_013)"})
	}
	repointed, _ := repointRes.RowsAffected()

	if _, err := tx.ExecContext(ctx,
		`UPDATE kb.object_nodes SET canonical_object_id = $1, reconcile_status = 'merged' WHERE object_id = $2`,
		req.SurvivorObjectID, req.LoserObjectID); err != nil {
		logger.Error("merge mark loser failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to mark node merged (CWB_KB_OMG_014)"})
	}

	if err := tx.Commit(); err != nil {
		logger.Error("merge commit failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to commit merge (CWB_KB_OMG_015)"})
	}

	survivorJSON, _ := json.Marshal(req.SurvivorObjectID)
	repointedJSON, _ := json.Marshal(repointed)
	logObjectAudit(ctx, db, logger, "kb.object_nodes", req.LoserObjectID, objectAuditActionMergeNodes, structureActor(rc),
		map[string]json.RawMessage{
			"survivor_object_id": survivorJSON,
			"repointed_mentions": repointedJSON,
		})

	return c.JSON(http.StatusOK, map[string]any{
		"status":             true,
		"survivor_object_id": req.SurvivorObjectID,
		"repointed_mentions": repointed,
	})
}
