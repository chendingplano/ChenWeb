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
}

// LoadConfig reads [artifact_search], SEARCH_LEXICAL_BACKEND and SEARCH_SEMANTIC_ENABLED.
func LoadConfig() Config {
	cfg := appconfig.GetArtifactSearchConfig()
	return Config{
		Backend:         backendFromEnv(),
		Dictionary:      sanitizeDictionary(cfg.Dictionary),
		LexicalWeight:   cfg.HybridLexicalWeight,
		SemanticWeight:  cfg.HybridSemanticWeight,
		SemanticEnabled: kbsearch.SemanticSearchEnabled(),
	}
}

func backendFromEnv() Backend {
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
