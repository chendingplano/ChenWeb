package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// Hybrid-search tuning. rrfK is the standard Reciprocal Rank Fusion constant;
// hybridCandidateLimit bounds each candidate list (lexical / semantic) before fusion.
const (
	rrfK                 = 60
	hybridCandidateLimit = 200
)

type lexicalBackend string

const (
	lexicalBackendPostgres lexicalBackend = "postgres"
	lexicalBackendParadeDB lexicalBackend = "paradedb"
)

type registrySearchConfig struct {
	dictionary      string
	defaultPageSize int
	maxPageSize     int
	previewMaxWords int
	phraseFriendly  bool
	minRank         float64
	lexicalWeight   float64    // hybrid RRF weight of the lexical list
	semanticWeight  float64    // hybrid RRF weight of the semantic list
	call            searchCall // run ID and user for the search's LLM usage events
	candidateLimit  int        // hybrid candidates per list; 0 = hybridCandidateLimit
	// trace, when set, collects each hybrid search's candidate lists (benchmark runs only).
	trace *searchTraceSink
}

func (c registrySearchConfig) candidates() int {
	if c.candidateLimit > 0 {
		return c.candidateLimit
	}
	return hybridCandidateLimit
}

func loadRegistrySearchConfig() registrySearchConfig {
	cfg := appconfig.GetArtifactSearchConfig()
	return registrySearchConfig{
		dictionary:      sanitizeTSConfig(cfg.Dictionary),
		defaultPageSize: cfg.DefaultPageSize,
		maxPageSize:     cfg.MaxPageSize,
		previewMaxWords: cfg.PreviewMaxWords,
		phraseFriendly:  cfg.PhraseFriendly,
		minRank:         cfg.MinRank,
		lexicalWeight:   cfg.HybridLexicalWeight,
		semanticWeight:  cfg.HybridSemanticWeight,
	}
}

func registryLexicalBackend() lexicalBackend {
	return lexicalBackend(searchengine.BackendFromEnv())
}

// CheckSearchBackend verifies the configured search backend is available.
// When the effective backend is paradedb (the default), it confirms the
// pg_search extension is installed and returns a fatal-worthy error if not.
// Call this once at startup after migrations have run.
func CheckSearchBackend(db *sql.DB) error {
	if strings.TrimSpace(os.Getenv("SEARCH_SEMANTIC_ENABLED")) == "" {
		return fmt.Errorf("SEARCH_SEMANTIC_ENABLED is not set; set it to true or false explicitly")
	}
	if registryLexicalBackend() != lexicalBackendParadeDB {
		return nil
	}
	return CheckParadeDBInstalled(db)
}

// CheckParadeDBInstalled verifies that the pg_search extension is present in the
// database. Call this at startup when the lexical backend is paradedb; the
// extension must be installed before any BM25 query can run.
func CheckParadeDBInstalled(db *sql.DB) error {
	var exists bool
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_search')`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("could not query pg_extension for pg_search: %w", err)
	}
	if !exists {
		return fmt.Errorf("ParadeDB (pg_search) extension is not installed; " +
			"install pg_search on the PostgreSQL server or set SEARCH_LEXICAL_BACKEND=postgres to use tsvector")
	}
	return nil
}

// searchArtifactPartitionTypes are the artifact types with a kb.search_artifacts_<type> partition.
var searchArtifactPartitionTypes = searchengine.PartitionTypes()

// searchArtifactPartition names the kb.search_artifacts partition of an artifact type (the table is
// LIST partitioned by artifact_type).
func searchArtifactPartition(artifactType string) string {
	return "kb.search_artifacts_" + artifactType
}

// The query functions below keep their signatures and map onto the hybrid search engine
// (server/api/searchengine), which owns the lexical and vector queries, fusion and diagnostics.

// engineConfig is the engine's configuration for one search: the process settings with this search's
// weights, candidate limit and snippet size.
func engineConfig(cfg registrySearchConfig) searchengine.Config {
	ec := searchengine.LoadConfig()
	ec.Dictionary = cfg.dictionary
	ec.LexicalWeight, ec.SemanticWeight = cfg.lexicalWeight, cfg.semanticWeight
	ec.CandidateLimit = cfg.candidateLimit
	ec.MinLexicalScore = cfg.minRank
	ec.PhraseFriendly = cfg.phraseFriendly
	ec.PreviewWords = cfg.previewMaxWords
	return ec
}

// engineFilters maps the HTTP filters; the route's artifact type takes precedence over
// filters.ArtifactTypes, as before.
func engineFilters(artifactType string, f artifactSearchFilters) searchengine.Filters {
	out := searchengine.Filters{
		ArtifactTypes:    f.ArtifactTypes,
		KnowledgeStoreID: f.KnowledgeStoreID,
		DocumentGroup:    f.DocumentGroup,
		Structural: searchengine.StructuralFilters{
			CategoryPath: f.CategoryPath, TopicType: f.TopicType, SceneType: f.SceneType,
			ProvisionType: f.ProvisionType, ProductType: f.ProductType, RelationType: f.RelationType,
			InventoryCategory: f.InventoryCategory, Manufacturer: f.Manufacturer, Brand: f.Brand,
			ModelNumber: f.ModelNumber, PartNumber: f.PartNumber, ValidationStatus: f.ValidationStatus,
		},
	}
	if t := strings.TrimSpace(artifactType); t != "" && t != "all" {
		out.ArtifactTypes = []string{t}
	}
	if f.InputRecordID != nil {
		out.RecordIDs = []int64{*f.InputRecordID}
	}
	return out
}

// engineKeywordMode maps the per-search keyword switch (benchmarks) to the engine's.
func engineKeywordMode(call searchCall) searchengine.KeywordExtraction {
	switch {
	case call.keywords == nil:
		return searchengine.KeywordExtractionDefault
	case *call.keywords:
		return searchengine.KeywordExtractionOn
	}
	return searchengine.KeywordExtractionOff
}

// The HTTP handlers call the search through these; tests replace them.
var (
	registrySearchCount = countRegistrySearchResults
	registrySearchQuery = queryRegistrySearchResults
)

func countRegistrySearchResults(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, cfg registrySearchConfig) (int64, error) {
	ctx := withSearchCall(context.Background(), cfg.call)
	return searchengine.Count(ctx, db, engineConfig(cfg), searchengine.Request{
		Query:   searchengine.Query{Text: queryText},
		Filters: engineFilters(artifactType, filters),
		Options: searchengine.Options{KeywordExtraction: engineKeywordMode(cfg.call)},
	})
}

// queryRegistrySearchResults runs one search through the engine and returns the requested page of its
// fused list. When the engine fails (for example the vector list), it retries on the keyword list
// alone, as the search always degraded to lexical-only.
func queryRegistrySearchResults(db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, page int, pageSize int, cfg registrySearchConfig) ([]artifactSearchResult, error) {
	ctx := withSearchCall(context.Background(), cfg.call)
	ec := engineConfig(cfg)
	results, err := searchViaEngine(ctx, db, artifactType, queryText, filters, page, pageSize, cfg, ec)
	if err != nil && ec.SemanticEnabled {
		ec.SemanticEnabled = false
		if retry, retryErr := searchViaEngine(ctx, db, artifactType, queryText, filters, page, pageSize, cfg, ec); retryErr == nil {
			return retry, nil
		}
	}
	return results, err
}

// queryRegistrySearchResultsCtx is the strict form (no degradation), for callers that measure one
// ranking, such as the benchmark's semantic-only mode.
func queryRegistrySearchResultsCtx(ctx context.Context, db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, page int, pageSize int, cfg registrySearchConfig) ([]artifactSearchResult, error) {
	return searchViaEngine(ctx, db, artifactType, queryText, filters, page, pageSize, cfg, engineConfig(cfg))
}

func searchViaEngine(ctx context.Context, db *sql.DB, artifactType string, queryText string, filters artifactSearchFilters, page int, pageSize int, cfg registrySearchConfig, ec searchengine.Config) ([]artifactSearchResult, error) {
	if page < 1 {
		page = 1
	}
	req := searchengine.Request{
		Query:   searchengine.Query{Text: queryText},
		Filters: engineFilters(artifactType, filters),
		Options: searchengine.Options{
			Limit: pageSize, Offset: (page - 1) * pageSize,
			KeywordExtraction: engineKeywordMode(cfg.call), Trace: cfg.trace != nil,
		},
	}
	resp, err := searchengine.Search(ctx, db, ec, req)
	if err != nil {
		return nil, err
	}
	results := make([]artifactSearchResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		results = append(results, artifactSearchResultFromEngine(r))
	}
	if cfg.trace != nil && resp.Trace != nil {
		cfg.trace.add(queryText, resp, results, req.Options.Offset)
	}
	return results, nil
}

func artifactSearchResultFromEngine(r searchengine.Result) artifactSearchResult {
	spans, payload := r.SourceLineSpans, r.SemanticPayload
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	return artifactSearchResult{
		ArtifactType: r.ArtifactType, ArtifactID: r.ArtifactID, InputRecordID: r.InputRecordID,
		PrimaryLabel: r.PrimaryLabel, SecondaryLabel: r.SecondaryLabel, Snippet: r.Snippet, Score: r.Score,
		SourceTitle: r.SourceTitle, SourceFilename: r.SourceFilename,
		SourceLineSpans: jsonArrayOrEmpty(spans), SemanticPayload: payload, Keywords: r.Keywords,
	}
}
