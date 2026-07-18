package docreviews

import (
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// maxMatchesToLLM caps how many matched artifacts are passed to each review
// LLM call. [doc-reviewer].max_artifacts_passed_to_llm in config.toml /
// config.local.toml takes precedence; when unset (<=0) it falls back to the
// MAX_MATCHES_TO_LLM env var, then the built-in default of 3.
func maxMatchesToLLM() int {
	if v := appconfig.GetDocReviewerMaxArtifactsPassedToLLM(); v > 0 {
		return v
	}
	return envInt("MAX_MATCHES_TO_LLM", 3, 1)
}

func limitMatchesToLLM[T any](matches []T) []T {
	limit := maxMatchesToLLM()
	if len(matches) <= limit {
		return matches
	}
	return matches[:limit]
}
