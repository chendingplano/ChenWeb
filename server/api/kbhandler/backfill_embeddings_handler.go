package kbhandler

import (
	"context"
	"net/http"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

// BackfillSearchEmbeddings populates kb.search_artifacts.embedding for existing rows
// so historical artifacts become semantically searchable without re-running the full
// doc-processing pipeline per record.
//
// POST /kb/search/backfill-embeddings
//
//	?artifact_type=  (optional; "" / "all" = every type)
//	?limit=          (rows per call; default 200, call repeatedly until remaining=0)
//	?reembed_all=    (true = recompute every row, not just NULL embeddings)
//
// Requires the pgvector migration applied and EMBEDDING_MODEL_NAME configured. It is
// independent of SEARCH_SEMANTIC_ENABLED, so embeddings can be backfilled first and
// the read flag flipped afterwards.
func BackfillSearchEmbeddings(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_BFE_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_BFE_010)"})
	}

	client, modelName, ok := newSearchQueryEmbedder()
	if !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "no embedding model configured; set EMBEDDING_MODEL_NAME (CWB_KB_BFE_011)"})
	}

	artifactType := strings.TrimSpace(c.QueryParam("artifact_type"))
	limit := parsePositiveInt(c.QueryParam("limit"), 200)
	reembedAll := strings.EqualFold(strings.TrimSpace(c.QueryParam("reembed_all")), "true")

	embed := func(ctx context.Context, text string) ([]float64, bool) {
		vec, err := client.Embed(ctx, llmclients.EmbedInput{
			ModelName: modelName,
			InputText: truncateRunesForEmbedding(text, embeddingQueryMaxRunes),
		})
		if err != nil {
			logger.Warn("backfill embedding call failed", "err", err.Error())
			return nil, false
		}
		return vec, true
	}

	logger.Info("search embedding backfill started",
		"artifact_type", artifactType, "limit", limit, "reembed_all", reembedAll, "model", modelName)
	result, err := kbsearch.BackfillEmbeddings(c.Request().Context(), db, embed, artifactType, reembedAll, limit)
	if err != nil {
		logger.Error("search embedding backfill failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "backfill failed (CWB_KB_BFE_012)"})
	}
	logger.Info("search embedding backfill finished",
		"scanned", result.Scanned, "embedded", result.Embedded,
		"skipped", result.Skipped, "failed", result.Failed, "remaining", result.Remaining)

	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"model":  modelName,
		"result": result,
	})
}
