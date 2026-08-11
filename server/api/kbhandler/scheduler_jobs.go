package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/deepdoc/server/api/llmusage"
	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
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
		"terminology_refresh": scheduler.JobDescriptor{
			Label: "Refresh External Terminology Resources",
			Run:   runTerminologyRefreshJob,
		},
		"collect_llm_usage": scheduler.JobDescriptor{
			Label: "Collect Coding Assistant LLM Usage",
			Run:   runCollectLLMUsageJob,
		},
	}
}

// runTerminologyRefreshJob re-fetches freely downloadable terminology
// resources into the local fetch store, writing fresh artifacts plus
// unapproved draft manifests for operator review. The "sources" param is a
// comma-separated list of resource IDs (default "wikidata", which updates
// weekly upstream); each re-fetch gets a fresh retrieved_at and, for
// Wikidata, a new dump-<date> release identity. Import still requires
// operator approval, so a scheduled refresh never imports anything.
func runTerminologyRefreshJob(ctx context.Context, params map[string]any, logger ApiTypes.JimoLogger) (map[string]any, error) {
	dir := terminology.Dir()
	if dir == "" {
		return nil, fmt.Errorf("TERMINOLOGY_DIR or DATA_HOME_DIR is not set")
	}
	raw := stringParam(params, "sources", "wikidata")
	results := make([]map[string]any, 0, 1)
	for _, part := range strings.Split(raw, ",") {
		id := terminology.ResourceID(strings.TrimSpace(part))
		if id == "" {
			continue
		}
		if _, ok := terminology.ResourceByID(id); !ok {
			results = append(results, map[string]any{"source": string(id), "error": "unknown terminology resource"})
			continue
		}
		st, err := terminology.Fetch(ctx, id, dir)
		if err != nil {
			results = append(results, map[string]any{"source": string(id), "error": err.Error()})
			continue
		}
		results = append(results, map[string]any{
			"source": string(id), "downloaded": st.Downloaded, "release": st.Release,
			"size_bytes": st.SizeBytes,
		})
	}
	return map[string]any{"sources": results}, nil
}

// runCollectLLMUsageJob reads the local coding-assistant usage logs (Qwen
// Code writes per-request records to ~/.qwen/usage/token-usage-*.jsonl) and
// upserts per-day, per-model aggregates into kb.llm_usage, the daily usage
// table shared by coding assistants. The "usage_dir" param overrides the log
// directory and "assistant" the tool name recorded in the table.
func runCollectLLMUsageJob(ctx context.Context, params map[string]any, logger ApiTypes.JimoLogger) (map[string]any, error) {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	dir := stringParam(params, "usage_dir", "")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home directory: %w", err)
		}
		dir = filepath.Join(home, ".qwen", "usage")
	}
	summary, err := llmusage.CollectAssistantUsage(ctx, db, dir, stringParam(params, "assistant", "qwen"))
	if err != nil {
		return nil, err
	}
	return structToMap(summary), nil
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
