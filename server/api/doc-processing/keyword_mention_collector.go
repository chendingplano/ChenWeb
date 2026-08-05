package docprocessing

import (
	"context"
	"database/sql"
	"strings"
	"unicode"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

// englishStopwords is a set of common English stopwords that the mention
// collector skips. Not full NLP — just enough to reduce noise in observe mode.
var englishStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true, "has": true,
	"had": true, "having": true, "do": true, "does": true, "did": true, "doing": true,
	"will": true, "would": true, "shall": true, "should": true, "may": true,
	"might": true, "must": true, "can": true, "could": true, "and": true,
	"or": true, "not": true, "no": true, "but": true, "if": true, "then": true,
	"else": true, "when": true, "where": true, "how": true, "what": true,
	"which": true, "who": true, "whom": true, "this": true, "that": true,
	"these": true, "those": true, "it": true, "its": true, "for": true,
	"with": true, "without": true, "at": true, "by": true, "from": true,
	"to": true, "of": true, "in": true, "on": true, "about": true,
}

// KeywordMentionCollector extracts candidate keyword mentions from document
// text in observe mode. It is a standalone function, not yet wired into the
// Phase C pipeline. Track B ships observe-only with no downstream consumers.
type KeywordMentionCollector struct {
	DB            *sql.DB
	KeywordFamily *keywords.KeywordFamily
}

// CollectFromText extracts keyword candidates from text and resolves each
// through the keyword family. In observe/on modes: writes mentions,
// surfaces/keys for matched concepts, and unresolved backlog for misses.
// In off mode: no-op.
func (mc *KeywordMentionCollector) CollectFromText(ctx context.Context, artifactRef, text, ksID string) error {
	if mc.DB == nil || mc.KeywordFamily == nil {
		return nil
	}
	// K7: gate on "not off", not on observe-only — collection must stay on
	// when the resolver mode flips to "on".
	if keywords.ResolverMode() == "off" {
		return nil
	}

	tokens := extractKeywordCandidates(text)
	seen := make(map[string]bool)
	scope := ksID
	if scope == "" {
		scope = "_"
	}

	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true

		_, err := mc.KeywordFamily.ResolveSurface(ctx, token, scope, artifactRef, "")
		if err != nil {
			// Best-effort: log but don't fail the batch.
			continue
		}
	}
	return nil
}

// extractKeywordCandidates tokenizes text and returns candidate keyword tokens.
// Splits on whitespace/punctuation, keeps tokens 2-50 chars, skips stopwords.
// Supports ASCII, CJK, and mixed scripts.
func extractKeywordCandidates(text string) []string {
	var tokens []string
	var current []rune

	flush := func() {
		if len(current) >= 2 && len(current) <= 50 {
			token := strings.ToLower(string(current))
			if !englishStopwords[token] {
				tokens = append(tokens, string(current))
			}
		}
		current = nil
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}
