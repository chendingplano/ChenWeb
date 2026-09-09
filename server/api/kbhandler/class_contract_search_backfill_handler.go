package kbhandler

import (
	"context"
	"net/http"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/classcontractsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

// BackfillClassContractSearch populates kb.ontology_class_contract_search for
// governed class terms so the Metric Ontology Explorer's "Metrics of Similar
// Classes" node has an index to hybrid-search.
//
// POST /kb/ontology/class-contracts/backfill-search
//
//	?limit=       (class terms per call; default 200, call until remaining=0)
//	?reembed_all= (true = reindex every class term, not just missing / un-embedded)
//
// It works with or without an embedding model configured: without one, rows are
// written lexical-only. Independent of SEARCH_SEMANTIC_ENABLED for the write, so
// documents can be indexed first and embeddings backfilled later.
func BackfillClassContractSearch(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_CCSB_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_CCSB_010)"})
	}

	limit := parsePositiveInt(c.QueryParam("limit"), 200)
	reembedAll := strings.EqualFold(strings.TrimSpace(c.QueryParam("reembed_all")), "true")

	modelName := ""
	var embed classcontractsearch.EmbedFunc
	if client, name, ok := newSearchQueryEmbedder(); ok {
		modelName = name
		embed = func(ctx context.Context, text string) ([]float64, bool) {
			vec, err := client.Embed(ctx, llmclients.EmbedInput{
				ModelName: name,
				InputText: truncateRunesForEmbedding(text, embeddingQueryMaxRunes),
			})
			if err != nil {
				logger.Warn("class contract search backfill embedding call failed", "err", err.Error())
				return nil, false
			}
			return vec, true
		}
	}

	logger.Info("class contract search backfill started", "limit", limit, "reembed_all", reembedAll, "model", modelName)
	result, err := classcontractsearch.BackfillClassContractSearch(c.Request().Context(), db, embed, limit, reembedAll)
	if err != nil {
		logger.Error("class contract search backfill failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "backfill failed (CWB_KB_CCSB_011)"})
	}
	logger.Info("class contract search backfill finished",
		"scanned", result.Scanned, "embedded", result.Embedded,
		"failed", result.Failed, "remaining", result.Remaining)

	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"model":  modelName,
		"result": result,
	})
}
