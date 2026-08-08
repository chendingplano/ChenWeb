// Command keyword-reconcile runs two offline reconciliation passes over the
// keyword lexicon (spec 2026080403 §13). First, it merges D11 auto-created
// provisional concepts into their true match using tier 5 (edit distance)
// and tier 6 (multilingual embedding) evidence (§19 step 11). Second, it
// re-ranks `ambiguous`-verdict backlog rows (tied candidates the online
// path could not confidently pick between) by embedding the original query
// against each still-live tied concept, auto-applying only when a clear
// margin or independent lexical corroboration backs the top pick -- the
// same two-signal discipline as the merge pass, never cosine alone. Not
// wired into any live server or cron -- run manually or via an external
// scheduler, per the spec's §22 Q2 decision to keep tier 6 off the online
// resolve path.
//
// Usage:
//
//	keyword-reconcile --scope=_ --identity-deployment-key=qudt-production
//
// Requires KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME to name an entry in
// .models.toml (e.g. qwen3-embedding-0-6b-llama-cpp or
// nomic-embed-v2-moe-llama-cpp -- both already locally hosted per .models.toml).
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func main() {
	// Timestamps on every log line: this binary is typically run under an
	// external scheduler, and the run's audit trail needs them.
	log.SetFlags(log.LstdFlags | log.LUTC)
	scope := flag.String("scope", "_", "keyword scope to reconcile")
	var deploymentKeys repeatedFlag
	flag.Var(&deploymentKeys, "identity-deployment-key", "enabled governed identity deployment key (repeatable; at least one required)")
	flag.Parse()
	keys, err := canonicalDeploymentKeys(deploymentKeys)
	if err != nil {
		log.Fatalf("identity deployment allowlist: %v", err)
	}

	db := connect()
	defer db.Close()
	ctx := context.Background()

	embedClient, modelName, err := embeddingClientFromEnv()
	if err != nil {
		log.Fatalf("embedding client: %v", err)
	}

	r := &keywords.Reconciler{
		DB: db,
		ConceptStore: keywords.ConceptStore{
			DB: db,
			Alignments: keywords.AlignmentsStore{
				Assertions:  assertions.AssertionStore{DB: db},
				DecisionLog: semid.DecisionLogStore{DB: db},
				Scope:       "_",
			},
		},
		EvidenceProviders: []keywords.IdentityEvidenceProvider{
			keywords.TripleEvidenceProvider{DeploymentKeys: keys},
		},
		Embeddings:       &llmEmbeddingClient{client: embedClient, modelName: modelName},
		EmbeddingModelID: modelName,
		Scope:            *scope,
	}
	stats, err := r.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s error=%v\n", formatStats(*scope, stats), err)
		os.Exit(1)
	}
	fmt.Println(formatStats(*scope, stats))

	// Second pass, same run: re-rank backlog rows an `ambiguous` verdict
	// logged (spec 2026080403 §13). Independent of the merge pass above --
	// it never touches D11 provisional concepts, only tied-candidate
	// backlog rows -- so a failure here does not roll back Run's results
	// (already committed row by row) and is reported separately.
	ambiguousStats, err := r.ReconcileAmbiguous(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s error=%v\n", formatAmbiguousStats(*scope, ambiguousStats), err)
		os.Exit(1)
	}
	fmt.Println(formatAmbiguousStats(*scope, ambiguousStats))
}

type repeatedFlag []string

func (f *repeatedFlag) String() string         { return strings.Join(*f, ",") }
func (f *repeatedFlag) Set(value string) error { *f = append(*f, value); return nil }

func canonicalDeploymentKeys(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("at least one --identity-deployment-key is required")
	}
	keys := make([]string, len(values))
	for i, value := range values {
		keys[i] = strings.TrimSpace(value)
		if keys[i] == "" {
			return nil, errors.New("deployment key is blank")
		}
	}
	sort.Strings(keys)
	for i := 1; i < len(keys); i++ {
		if keys[i] == keys[i-1] {
			return nil, fmt.Errorf("duplicate deployment key %q", keys[i])
		}
	}
	return keys, nil
}

func formatStats(scope string, stats keywords.ReconcileStats) string {
	return fmt.Sprintf("keyword-reconcile scope=%s scanned=%d decided=%d failed=%d merged=%d deferred_unvalidated=%d rejected=%d no_candidate=%d",
		scope, stats.Scanned, stats.Decided, stats.Failed, stats.Merged, stats.DeferredUnvalidated, stats.Rejected, stats.NoCandidate)
}

func formatAmbiguousStats(scope string, stats keywords.AmbiguousReconcileStats) string {
	return fmt.Sprintf("keyword-reconcile-ambiguous scope=%s scanned=%d decided=%d failed=%d auto_applied=%d collapsed=%d deferred=%d no_active_candidate=%d",
		scope, stats.Scanned, stats.Decided, stats.Failed, stats.AutoApplied, stats.Collapsed, stats.Deferred, stats.NoCandidate)
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
		// Name the entry that failed so the operator can tell which model
		// config the client-construction error refers to.
		return nil, "", fmt.Errorf("create embedding client for KEYWORD_RECONCILE_EMBEDDING_MODEL_NAME entry %q (model %s): %w", modelRef, def.ModelName, err)
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
