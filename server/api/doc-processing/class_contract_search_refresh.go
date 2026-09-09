package docprocessing

import (
	"context"

	"github.com/chendingplano/deepdoc/server/api/ontology/classcontractsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// refreshClassContractSearchForRecord re-indexes the class-contract
// hybrid-search rows (kb.ontology_class_contract_search) for every governed
// class a metric in recordID resolved to. It is called from Phase D's
// associate_semantics stage AFTER that stage's per-metric write transactions
// have committed — classcontractsearch.Reindex embeds text, which is network
// I/O that must never sit inside a metric-write transaction (openspec change
// analysis-node-related-metrics, design D8).
//
// Best-effort: every failure (and a panic) is logged and swallowed; it never
// affects the Phase D result.
func refreshClassContractSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil && logger != nil {
			logger.Error("class contract search refresh panicked", "record_id", recordID, "recover", r)
		}
	}()

	rows, err := db.QueryContext(ctx, `
SELECT DISTINCT a.instance_of_term_id
FROM kb.semantic_assertions a
JOIN kb.semantic_decision_candidates dc
     ON dc.resulting_assertion_id = a.id AND dc.source_artifact_type = 'metric'
JOIN kb.metrics m ON m.metric_id = dc.source_artifact_id
WHERE m.input_record_id = $1
  AND COALESCE(a.instance_of_term_id, '') <> ''`, recordID)
	if err != nil {
		if logger != nil {
			logger.Warn("class contract search refresh: load classes failed", "record_id", recordID, "error", err.Error())
		}
		return
	}
	var classIDs []string
	for rows.Next() {
		var termID string
		if err := rows.Scan(&termID); err != nil {
			rows.Close()
			if logger != nil {
				logger.Warn("class contract search refresh: scan failed", "record_id", recordID, "error", err.Error())
			}
			return
		}
		classIDs = append(classIDs, termID)
	}
	rows.Close()
	if len(classIDs) == 0 {
		return
	}

	store := classcontractsearch.Store{DB: db}
	var reindexed, failed int
	for _, termID := range classIDs {
		if ctx.Err() != nil {
			break
		}
		if err := store.Reindex(ctx, termID, embedQueryText); err != nil {
			failed++
			if logger != nil {
				logger.Warn("class contract search refresh: reindex failed",
					"record_id", recordID, "class_term_id", termID, "error", err.Error())
			}
			continue
		}
		reindexed++
	}
	if logger != nil {
		logger.Info("class contract search refresh done",
			"record_id", recordID, "classes", len(classIDs), "reindexed", reindexed, "failed", failed)
	}
}
