package kbhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type rebindArtifactObjectsRequest struct {
	ArtifactObjectIDs []int64 `json:"artifact_object_ids"`
	SurvivorObjectID  string  `json:"survivor_object_id"`
}

func validateRebindArtifactObjects(ids []int64, survivorObjectID string) error {
	if strings.TrimSpace(survivorObjectID) == "" {
		return fmt.Errorf("survivor_object_id is required")
	}
	if len(ids) == 0 {
		return fmt.Errorf("artifact_object_ids is required")
	}
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("artifact_object_ids must contain only positive ids")
		}
	}
	return nil
}

// RebindArtifactObjectsToMaster handles POST /api/v1/kb/objects/rebind. It
// repoints the selected kb.artifact_objects rows to the chosen master
// kb.object_nodes.object_id without merging or mutating the source node.
func RebindArtifactObjectsToMaster(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ORB_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req rebindArtifactObjectsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_ORB_002)"})
	}
	req.SurvivorObjectID = strings.TrimSpace(req.SurvivorObjectID)
	if err := validateRebindArtifactObjects(req.ArtifactObjectIDs, req.SurvivorObjectID); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error() + " (CWB_KB_ORB_003)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_ORB_010)"})
	}
	ctx := c.Request().Context()

	var present int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kb.object_nodes WHERE object_id = $1`,
		req.SurvivorObjectID).Scan(&present); err != nil {
		logger.Error("rebind survivor existence check failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify survivor object node (CWB_KB_ORB_011)"})
	}
	if present == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "survivor object node not found (CWB_KB_ORB_004)"})
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		logger.Error("rebind begin tx failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to start rebind (CWB_KB_ORB_012)"})
	}
	defer func() { _ = tx.Rollback() }()

	const updateStmt = `UPDATE kb.artifact_objects
SET object_id = $1,
    ext_info = COALESCE(ext_info, '{}'::jsonb) || $2::jsonb
WHERE id = $3`

	updated := 0
	for _, artifactObjectID := range req.ArtifactObjectIDs {
		res, err := tx.ExecContext(ctx, updateStmt, req.SurvivorObjectID, `{"reconcile_method":"manual_admin"}`, artifactObjectID)
		if err != nil {
			logger.Error("rebind artifact object failed", "artifact_object_id", artifactObjectID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to rebind artifact objects (CWB_KB_ORB_013)"})
		}
		affected, err := res.RowsAffected()
		if err != nil {
			logger.Error("rebind rows affected failed", "artifact_object_id", artifactObjectID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to verify artifact rebind (CWB_KB_ORB_014)"})
		}
		if affected == 0 {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "artifact object not found (CWB_KB_ORB_005)"})
		}
		updated += int(affected)
	}

	if err := tx.Commit(); err != nil {
		logger.Error("rebind commit failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to commit artifact rebind (CWB_KB_ORB_015)"})
	}

	objectIDJSON, _ := json.Marshal(req.SurvivorObjectID)
	for _, artifactObjectID := range req.ArtifactObjectIDs {
		payload := map[string]json.RawMessage{"object_id": objectIDJSON}
		logObjectAudit(ctx, db, logger, "kb.artifact_objects", strconv.FormatInt(artifactObjectID, 10), objectAuditActionResolveObjectID, structureActor(rc), payload)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":              true,
		"artifact_object_ids": req.ArtifactObjectIDs,
		"survivor_object_id":  req.SurvivorObjectID,
		"updated":             updated,
	})
}
