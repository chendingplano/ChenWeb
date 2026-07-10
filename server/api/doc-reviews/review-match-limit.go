package docreviews

func maxMatchesToLLM() int {
	return envInt("MAX_MATCHES_TO_LLM", 3, 1)
}

func limitMatchesToLLM[T any](matches []T) []T {
	limit := maxMatchesToLLM()
	if len(matches) <= limit {
		return matches
	}
	return matches[:limit]
}
