package searchengine

import (
	"os"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

type Backend string

const (
	BackendParadeDB Backend = "paradedb"
	BackendPostgres Backend = "postgres"
)

// Config is the engine's tuning: today's [artifact_search] weights and the lexical backend.
type Config struct {
	Backend         Backend
	Dictionary      string // Postgres full-text dictionary
	LexicalWeight   float64
	SemanticWeight  float64
	SemanticEnabled bool
	// CandidateLimit bounds each candidate list before fusion; 0 = 200.
	CandidateLimit int
	// MinLexicalScore drops keyword-list hits scoring below it when the request has no query vector
	// (the keyword-only fallback); 0 = no floor. Hybrid searches ignore it.
	MinLexicalScore float64
	// PhraseFriendly makes Postgres full text read a raw text query as a web search (quoted phrases,
	// "or", "-").
	PhraseFriendly bool
	// PreviewWords sizes the highlighted snippet of the Postgres backend.
	PreviewWords int
}

func (c Config) candidates() int {
	if c.CandidateLimit > 0 {
		return c.CandidateLimit
	}
	return candidateLimit
}

// LoadConfig reads [artifact_search], SEARCH_LEXICAL_BACKEND and SEARCH_SEMANTIC_ENABLED.
func LoadConfig() Config {
	cfg := appconfig.GetArtifactSearchConfig()
	return Config{
		Backend:         BackendFromEnv(),
		Dictionary:      sanitizeDictionary(cfg.Dictionary),
		LexicalWeight:   cfg.HybridLexicalWeight,
		SemanticWeight:  cfg.HybridSemanticWeight,
		SemanticEnabled: kbsearch.SemanticSearchEnabled(),
		MinLexicalScore: cfg.MinRank,
		PhraseFriendly:  cfg.PhraseFriendly,
		PreviewWords:    cfg.PreviewMaxWords,
	}
}

// BackendFromEnv reads SEARCH_LEXICAL_BACKEND (ParadeDB unless it names Postgres).
func BackendFromEnv() Backend {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SEARCH_LEXICAL_BACKEND"))) {
	case "postgres", "pg", "tsvector":
		return BackendPostgres
	default:
		return BackendParadeDB
	}
}

// sanitizeDictionary allows only dictionaries that are safe to render into SQL.
func sanitizeDictionary(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "english") {
		return "english"
	}
	return "simple"
}
