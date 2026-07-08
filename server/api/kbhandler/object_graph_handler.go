package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// sqlObjectGraphSource is the database-backed objectGraphSource used by the
// Object Relation Chart endpoint.
type sqlObjectGraphSource struct {
	DB *sql.DB
}

func (s sqlObjectGraphSource) LoadArtifactObjectByID(ctx context.Context, id int64) (docprocessing.ArtifactObject, bool, error) {
	return docprocessing.ArtifactObjectSQLStore{DB: s.DB}.LoadByID(ctx, id)
}

func (s sqlObjectGraphSource) scanArtifactObjects(rows *sql.Rows) ([]docprocessing.ArtifactObject, error) {
	defer func() { _ = rows.Close() }()
	var out []docprocessing.ArtifactObject
	for rows.Next() {
		var o docprocessing.ArtifactObject
		if err := rows.Scan(&o.ID, &o.ArtifactType, &o.ArtifactID, &o.ObjectID, &o.ObjectName, &o.ObjectNameEn, &o.ReconcileStatus); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s sqlObjectGraphSource) ObjectsByObjectID(ctx context.Context, objectID string) ([]docprocessing.ArtifactObject, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, artifact_type, artifact_id, COALESCE(object_id, ''), object_name, COALESCE(object_name_en, ''), reconcile_status
FROM kb.artifact_objects
WHERE object_id = $1
ORDER BY id`, objectID)
	if err != nil {
		return nil, err
	}
	return s.scanArtifactObjects(rows)
}

func (s sqlObjectGraphSource) ObjectsByArtifact(ctx context.Context, artifactType, artifactID string) ([]docprocessing.ArtifactObject, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, artifact_type, artifact_id, COALESCE(object_id, ''), object_name, COALESCE(object_name_en, ''), reconcile_status
FROM kb.artifact_objects
WHERE artifact_type = $1 AND artifact_id = $2
ORDER BY id`, artifactType, artifactID)
	if err != nil {
		return nil, err
	}
	return s.scanArtifactObjects(rows)
}

func (s sqlObjectGraphSource) ObjectNodeByID(ctx context.Context, objectID string) (docprocessing.ObjectNode, bool, error) {
	var n docprocessing.ObjectNode
	err := s.DB.QueryRowContext(ctx, `
SELECT object_id, canonical_name, COALESCE(canonical_name_en, ''), object_type
FROM kb.object_nodes
WHERE object_id = $1`, objectID).Scan(&n.ObjectID, &n.CanonicalName, &n.CanonicalNameEn, &n.ObjectType)
	if err == sql.ErrNoRows {
		return docprocessing.ObjectNode{}, false, nil
	}
	if err != nil {
		return docprocessing.ObjectNode{}, false, err
	}
	return n, true, nil
}

func (s sqlObjectGraphSource) SimilarArtifacts(ctx context.Context, artifactType, artifactID string, topN int) ([]docprocessing.OnTheFlySemanticMatch, error) {
	return docprocessing.FindSimilarArtifactsOnTheFly(ctx, s.DB, artifactType, artifactID, "", topN)
}

// GetConnectedNodesByObjectId builds the relation graph seeded from a canonical
// object identity (kb.object_nodes.object_id).
func GetConnectedNodesByObjectId(ctx context.Context, src objectGraphSource, objectID string, opts objectGraphOptions) (ObjectGraph, error) {
	return BuildObjectGraph(ctx, src, objectGraphSeed{ObjectID: objectID}, opts)
}

// GetConnectedNodesByArtifactObjectId builds the relation graph seeded from a
// single kb.artifact_objects mention row. If the mention is already resolved
// (object_id set), the seed is that object; otherwise the mention is a terminal
// node so an unresolved object is still reachable and resolvable.
func GetConnectedNodesByArtifactObjectId(ctx context.Context, src objectGraphSource, id int64, opts objectGraphOptions) (ObjectGraph, bool, error) {
	obj, found, err := src.LoadArtifactObjectByID(ctx, id)
	if err != nil {
		return ObjectGraph{}, false, err
	}
	if !found {
		return ObjectGraph{}, false, nil
	}
	seed := objectGraphSeed{}
	if strings.TrimSpace(obj.ObjectID) != "" {
		seed.ObjectID = obj.ObjectID
	} else {
		seed.Mention = &obj
	}
	g, err := BuildObjectGraph(ctx, src, seed, opts)
	return g, true, err
}

func envIntDefault(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func objectGraphOptionsFromEnv() objectGraphOptions {
	return objectGraphOptions{
		SimilarTopN:    envIntDefault("SIMILAR_ARTIFACT_TOP_N", defaultSimilarArtifactTopN),
		RecursiveLevel: envIntDefault("OBJECT_CHART_RECURSIVE_LEVEL", defaultObjectChartRecursion),
		MaxNodes:       envIntDefault("OBJECT_CHART_MAX_NODES", defaultObjectChartMaxNodes),
	}
}

type objectGraphRequest struct {
	ObjectID         string `json:"object_id"`
	ArtifactObjectID int64  `json:"artifact_object_id"`
	TopN             int    `json:"top_n"`
	Level            int    `json:"level"`
	MaxNodes         int    `json:"max_nodes"`
}

// BuildObjectGraphHandler handles POST /api/v1/kb/objects/graph — the Object
// Relation Chart data source. It accepts either an object_id or an
// artifact_object_id and returns the bounded relation graph. Traversal bounds
// default from env (SIMILAR_ARTIFACT_TOP_N, OBJECT_CHART_RECURSIVE_LEVEL,
// OBJECT_CHART_MAX_NODES) and may be overridden per request.
func BuildObjectGraphHandler(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OGR_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req objectGraphRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OGR_002)"})
	}
	req.ObjectID = strings.TrimSpace(req.ObjectID)
	if req.ObjectID == "" && req.ArtifactObjectID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "object_id or artifact_object_id is required (CWB_KB_OGR_003)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_OGR_010)"})
	}

	opts := objectGraphOptionsFromEnv()
	if req.TopN > 0 {
		opts.SimilarTopN = req.TopN
	}
	if req.Level > 0 {
		opts.RecursiveLevel = req.Level
	}
	if req.MaxNodes > 0 {
		opts.MaxNodes = req.MaxNodes
	}

	src := sqlObjectGraphSource{DB: db}
	ctx := c.Request().Context()

	var (
		graph ObjectGraph
		err   error
	)
	if req.ArtifactObjectID > 0 {
		var found bool
		graph, found, err = GetConnectedNodesByArtifactObjectId(ctx, src, req.ArtifactObjectID, opts)
		if err == nil && !found {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "artifact object not found (CWB_KB_OGR_004)"})
		}
	} else {
		graph, err = GetConnectedNodesByObjectId(ctx, src, req.ObjectID, opts)
	}
	if err != nil {
		logger.Error("build object graph failed", "object_id", req.ObjectID, "artifact_object_id", req.ArtifactObjectID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to build object graph (CWB_KB_OGR_011)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true, "graph": graph})
}
