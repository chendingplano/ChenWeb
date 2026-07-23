package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/deepdoc/server/api/scheduler"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// DefaultSchedulerRegistry wires the scheduler's generic engine
// (server/api/scheduler) to the concrete repeatable backlog-drain jobs this
// codebase already exposes as one-off HTTP endpoints:
// resolve_entity_objects (ADR 2026070101 Phase 4), resolve_ambiguous_objects
// (ADR 2026070701 DR5), and backfill_search_embeddings. Each job function
// here does exactly what its corresponding handler does, minus the HTTP
// envelope, so scheduled runs and manual endpoint calls behave identically.
func DefaultSchedulerRegistry() scheduler.Registry {
	return scheduler.Registry{
		"resolve_entity_objects": scheduler.JobDescriptor{
			Label: "Resolve Entity Objects",
			Run:   runResolveEntityObjectsJob,
		},
		"resolve_ambiguous_objects": scheduler.JobDescriptor{
			Label: "Resolve Ambiguous Objects",
			Run:   runResolveAmbiguousObjectsJob,
		},
		"backfill_search_embeddings": scheduler.JobDescriptor{
			Label: "Backfill Search Embeddings",
			Run:   runBackfillSearchEmbeddingsJob,
		},
	}
}

func runResolveEntityObjectsJob(ctx context.Context, params map[string]any, logger ApiTypes.JimoLogger) (map[string]any, error) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	classifier, cfg, err := docprocessing.NewEntityObjectClassifierFromEnv()
	if err != nil {
		return nil, err
	}
	if classifier == nil {
		return nil, fmt.Errorf("entity object resolution is not configured: set ENTITY_OBJECT_RESOLVE_MODEL_NAME")
	}
	store := docprocessing.EntityObjectResolveSQLStore{DB: db}
	objectStore := docprocessing.ArtifactObjectSQLStore{DB: db}
	reconciler := docprocessing.ObjectReconciler{
		Store:   docprocessing.ObjectNodeSQLStore{DB: db},
		Options: docprocessing.ObjectReconcileOptionsFromEnv(),
	}
	result, err := docprocessing.ResolveEntityObjects(ctx, store, objectStore, reconciler, classifier, cfg, intParam(params, "limit", 200), logger)
	if err != nil {
		return nil, err
	}
	return structToMap(result), nil
}

func runResolveAmbiguousObjectsJob(ctx context.Context, params map[string]any, logger ApiTypes.JimoLogger) (map[string]any, error) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	store := docprocessing.ArtifactObjectSQLStore{DB: db}
	reconciler := docprocessing.ObjectReconciler{
		Store:   docprocessing.ObjectNodeSQLStore{DB: db},
		Options: docprocessing.ObjectReconcileOptionsFromEnv(),
	}
	result, err := docprocessing.ResolveAmbiguousArtifactObjects(ctx, store, reconciler, logger, intParam(params, "limit", 200))
	if err != nil {
		return nil, err
	}
	return structToMap(result), nil
}

func runBackfillSearchEmbeddingsJob(ctx context.Context, params map[string]any, logger ApiTypes.JimoLogger) (map[string]any, error) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	client, modelName, ok := newSearchQueryEmbedder()
	if !ok {
		return nil, fmt.Errorf("no embedding model configured: set EMBEDDING_MODEL_NAME")
	}
	embed := func(ctx context.Context, text string) ([]float64, bool) {
		vec, err := client.Embed(ctx, llmclients.EmbedInput{
			ModelName: modelName,
			InputText: truncateRunesForEmbedding(text, embeddingQueryMaxRunes),
		})
		if err != nil {
			if logger != nil {
				logger.Warn("scheduled backfill embedding call failed", "err", err.Error())
			}
			return nil, false
		}
		return vec, true
	}
	artifactType := stringParam(params, "artifact_type", "")
	reembedAll := boolParam(params, "reembed_all", false)
	result, err := kbsearch.BackfillEmbeddings(ctx, db, embed, artifactType, reembedAll, intParam(params, "limit", 200))
	if err != nil {
		return nil, err
	}
	return structToMap(result), nil
}

func intParam(params map[string]any, key string, fallback int) int {
	switch v := params[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return fallback
	}
}

func stringParam(params map[string]any, key, fallback string) string {
	if v, ok := params[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func boolParam(params map[string]any, key string, fallback bool) bool {
	if v, ok := params[key].(bool); ok {
		return v
	}
	return fallback
}

// structToMap round-trips a result struct through JSON so it can be stored
// in kb.scheduled_job_runs.result (JSONB), regardless of the struct's field
// tags.
func structToMap(v any) map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}
