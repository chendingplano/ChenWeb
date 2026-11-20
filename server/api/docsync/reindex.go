package docsync

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type indexer func(context.Context, int64, ApiTypes.JimoLogger) error

type derivedIndex struct {
	name string
	run  indexer
}

var indexes = map[string]derivedIndex{
	"extract_summaries":            {"summary", docprocessing.ReindexSummarySearchForRecord},
	"generate_summaries":           {"summary", docprocessing.ReindexSummarySearchForRecord},
	"generate_topics":              {"topic", docprocessing.ReindexTopicSearchForRecord},
	"extract_metrics":              {"metric", docprocessing.ReindexMetricSearchForRecord},
	"extract_metrics_simple":       {"metric", docprocessing.ReindexMetricSearchForRecord},
	"extract_provisions":           {"provision", docprocessing.ReindexProvisionSearchForRecord},
	"extract_products":             {"product", docprocessing.ReindexProductSearchForRecord},
	"extract_entity":               {"entity", docprocessing.ReindexEntitySearchForRecord},
	"extract_relation":             {"relation", docprocessing.ReindexRelationSearchForRecord},
	"extract_inventory_items":      {"inventory_item", docprocessing.ReindexInventoryItemSearchForRecord},
	"extract_semantic_projections": {"semantic_projection", docprocessing.ReindexSemanticProjectionSearchForRecord},
	"extract_structured_knowledge": {"knowledge", docprocessing.ReindexKnowledgeSearchForRecord},
	"generate_scene_blocks":        {"scene_block", docprocessing.ReindexSceneBlockSearchForRecord},
	"chunking":                     {"chunk", nil},
	"project_semantics":            {"assertion_projections", nil},
}

func (r *Runner) queueIndex(ctx context.Context, tx *sql.Tx, id int64, processor string) error {
	index, exists := indexes[processor]
	if !exists {
		return nil
	}
	if index.name != "assertion_projections" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM kb.search_artifacts WHERE input_record_id=$1 AND artifact_type=$2`, id, index.name); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO kb.doc_sync_reindex(input_id,processor) VALUES($1,$2)
		ON CONFLICT (input_id,processor) DO NOTHING`, id, processor)
	return err
}

func (r *Runner) rebuild(ctx context.Context, runID int64) error {
	rows, err := r.DB.QueryContext(ctx, `SELECT input_id,processor FROM kb.doc_sync_reindex ORDER BY input_id,processor LIMIT 500`)
	if err != nil {
		return err
	}
	type job struct {
		id        int64
		processor string
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.processor); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, end := r.Window.bounds(time.Now())
		if !time.Now().Before(end) {
			return nil
		}
		release, ok, err := docprocessing.AcquireDocumentLease(ctx, r.DB, job.id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		err = r.rebuildOne(ctx, job.id, job.processor)
		release()
		if err == nil {
			if _, err := r.DB.ExecContext(ctx, `DELETE FROM kb.doc_sync_reindex WHERE input_id=$1 AND processor=$2`, job.id, job.processor); err != nil {
				return err
			}
		} else if err := r.event(ctx, runID, "reindex_deferred", workItem{Kind: job.processor}, job.id, map[string]any{"reason": err.Error()}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) rebuildOne(ctx context.Context, id int64, processor string) error {
	switch processor {
	case "chunking":
		_, err := docprocessing.BackfillChunkSearch(ctx, r.DB, 0, id, 1, r.Logger)
		return err
	case "project_semantics":
		report, err := (assertions.ProjectSemantics{DB: r.DB}).Run(ctx, id)
		if err == nil && report.Errors > 0 {
			err = fmt.Errorf("%d assertion projections failed to rebuild", report.Errors)
		}
		return err
	}
	if index := indexes[processor]; index.run != nil {
		return index.run(ctx, id, r.Logger)
	}
	return nil
}
