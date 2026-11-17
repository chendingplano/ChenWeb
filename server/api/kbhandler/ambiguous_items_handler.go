package kbhandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/attention"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type ambiguousItemDeps struct {
	getItem func(ctx context.Context, id int64) (attention.Item, error)
	findRow func(ctx context.Context, occurrences []docprocessing.TieOccurrence) (int64, bool, error)
}

// GetAmbiguousItemArtifactObject handles
// GET /api/v1/kb/objects/ambiguous-items/:id/artifact-object. For an
// ambiguous-object-name attention item it returns the id of the current
// kb.artifact_objects row of the newest occurrence that still has one, so the
// Resolve Ambiguous Objects page can open its existing artifact-object detail.
func GetAmbiguousItemArtifactObject(c echo.Context) error {
	db := ApiTypes.ProjectDBHandle
	return ambiguousItemArtifactObject(c, ambiguousItemDeps{
		getItem: attention.SQLStore{DB: db}.Get,
		findRow: docprocessing.ArtifactObjectSQLStore{DB: db}.FindTieOccurrenceRow,
	})
}

func ambiguousItemArtifactObject(c echo.Context, deps ambiguousItemDeps) error {
	rc := EchoFactory.NewFromEcho(c, "20261117-512")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid attention item id"})
	}
	ctx := c.Request().Context()
	item, err := deps.getItem(ctx, id)
	if errors.Is(err, attention.ErrNotFound) {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "attention item not found"})
	}
	if err != nil {
		logger.Error("ambiguous item: load failed", "id", id, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load attention item"})
	}
	if item.ItemType != attention.TypeAmbiguousObjectName {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "not an ambiguous-object-name item"})
	}
	var details struct {
		Occurrences []docprocessing.TieOccurrence `json:"occurrences"`
	}
	if err := json.Unmarshal(item.Details, &details); err != nil {
		logger.Error("ambiguous item: decode details failed", "id", id, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read attention item details"})
	}
	rowID, found, err := deps.findRow(ctx, details.Occurrences)
	if err != nil {
		logger.Error("ambiguous item: find artifact object failed", "id", id, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to find artifact object"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "found": found, "artifact_object_id": rowID})
}
