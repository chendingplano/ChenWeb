// Command keyword-reconcile runs one offline reconciliation pass over the
// keyword lexicon (spec 2026080403 §19 step 11): it merges D11 auto-created
// provisional concepts into their true match using tier 5 (edit distance)
// and tier 6 (multilingual embedding) evidence. Not wired into any live
// server or cron -- run manually or via an external scheduler, per the
// spec's §22 Q2 decision to keep tier 6 off the online resolve path.
//
// Usage:
//
//	keyword-reconcile --scope=_
//
// Requires KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME to name an entry in
// .models.toml (e.g. qwen3-embedding-0-6b-llama-cpp or
// nomic-embed-v2-moe-llama-cpp -- both already locally hosted per .models.toml).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func main() {
	log.SetFlags(0)
	scope := flag.String("scope", "_", "keyword scope to reconcile")
	flag.Parse()

	db := connect()
	defer db.Close()
	ctx := context.Background()

	embedClient, modelName, err := embeddingClientFromEnv()
	if err != nil {
		log.Fatalf("embedding client: %v", err)
	}

	r := &keywords.Reconciler{
		DB:           db,
		ConceptStore: keywords.ConceptStore{DB: db},
		SurfaceStore: keywords.SurfaceStore{DB: db},
		DecisionLog:  semid.DecisionLogStore{DB: db},
		Embeddings:   &llmEmbeddingClient{client: embedClient, modelName: modelName},
		Scope:        *scope,
	}
	stats, err := r.Run(ctx)
	if err != nil {
		log.Fatalf("reconcile run failed: %v", err)
	}
	fmt.Printf("keyword-reconcile scope=%s scanned=%d merged=%d skipped_vetoed=%d skipped_no_candidate=%d\n",
		*scope, stats.Scanned, stats.Merged, stats.SkippedVetoed, stats.SkippedNoCandidate)
}

func connect() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=disable",
		envOr("PG_HOST", "/tmp"), envOr("PG_PORT", "5432"), envOr("PG_USER", "cding"),
		envOr("PG_DB_NAME", "chenweb_test"))
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	return db
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// llmEmbeddingClient adapts shared/go/api/llm's OpenAIJSONClient to
// keywords.EmbeddingClient, keeping the ontology/keywords package itself
// free of any LLM-client dependency (only this cmd binary wires them
// together, mirroring how doc-processing's entity reconciliation keeps its
// LLM seam (MergeAdjudicator) separate from the DB seam).
type llmEmbeddingClient struct {
	client    *llmclients.OpenAIJSONClient
	modelName string
}

func (c *llmEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	return c.client.EmbedBatch(ctx, llmclients.EmbedBatchInput{
		ModelName:  c.modelName,
		InputTexts: texts,
		CallReason: "keyword_reconcile_tier6",
		CallLoc:    "cmd/keyword-reconcile",
	})
}

func embeddingClientFromEnv() (*llmclients.OpenAIJSONClient, string, error) {
	modelRef := strings.TrimSpace(os.Getenv("KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME"))
	if modelRef == "" {
		return nil, "", fmt.Errorf("KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME is not set")
	}
	path, err := modelsFilePath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}
	var models ApiTypes.LLMModelsFile
	if err := toml.Unmarshal(raw, &models); err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", path, err)
	}
	def, ok := models[modelRef]
	if !ok {
		return nil, "", fmt.Errorf("model %q not found in %s", modelRef, path)
	}
	client, err := llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
		ModelName:            def.ModelName,
		APIKey:               def.APIKey,
		BaseURL:              def.BaseURL,
		Provider:             llmclients.ProviderOpenAICompatible,
		TimeoutSec:           def.TimeoutSec,
		EmbeddingDimensions:  kbsearch.EmbeddingDimensionsForModel(def.ModelName, def.BaseURL),
		MaxInflight:          def.MaxInflight,
		MaxRequestsPerMinute: def.MaxRequestsPerMinute,
		MaxTokensPerMinute:   def.MaxTokensPerMinute,
		TokenReservePerCall:  def.TokenReservePerCall,
	}, nil)
	if err != nil {
		return nil, "", err
	}
	return client, def.ModelName, nil
}

// modelsFilePath walks up from the working directory looking for
// .models.toml, mirroring doc-processing's resolveModelsFilePath.
func modelsFilePath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("MODELS_FILE")); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("MODELS_FILE %q is invalid: %w", override, err)
		}
		return override, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for {
		candidate := filepath.Join(cur, ".models.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", fmt.Errorf(".models.toml not found; set MODELS_FILE or place .models.toml in the current directory tree")
}
