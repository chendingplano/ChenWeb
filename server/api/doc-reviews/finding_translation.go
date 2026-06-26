package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// FindingTranslator translates the fields displayed in the review report.
type FindingTranslator interface {
	TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingTranslation, error)
}

type llmFindingTranslator struct {
	client          LLMJSONExtractor
	modelName       string
	promptText      string
	retryPromptText string
}

var errFindingTranslationUnavailable = errors.New("finding translation unavailable")

const findingTranslationPromptName = "doc-review-finding-translation"
const findingTranslationRetryPromptName = "doc-review-finding-translation-retry"

func promptFilePath(ref string) string {
	return filepath.Join("prompts", ref)
}

func newLLMFindingTranslator() (FindingTranslator, error) {
	modelRef := strings.TrimSpace(os.Getenv("TRANSLATION_MODEL_NAME"))
	if modelRef == "" {
		return nil, fmt.Errorf("%w: TRANSLATION_MODEL_NAME is not configured", errFindingTranslationUnavailable)
	}
	client, modelName, err := docprocessingBuildReviewerLLMClient(modelRef)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errFindingTranslationUnavailable, err)
	}

	promptPath := promptFilePath("prompt-doc-review-finding-translation-v1.md")
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		return nil, fmt.Errorf("%w: load base prompt: %v", errFindingTranslationUnavailable, err)
	}
	retryPromptPath := promptFilePath("prompt-doc-review-finding-translation-retry-v1.md")
	retryPromptBytes, err := os.ReadFile(retryPromptPath)
	if err != nil {
		return nil, fmt.Errorf("%w: load retry prompt: %v", errFindingTranslationUnavailable, err)
	}

	return &llmFindingTranslator{
		client:          client,
		modelName:       modelName,
		promptText:      strings.TrimSpace(string(promptBytes)),
		retryPromptText: strings.TrimSpace(string(retryPromptBytes)),
	}, nil
}

func (t *llmFindingTranslator) TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingTranslation, error) {
	return t.translateFindingAttempt(ctx, language, finding, findingTranslationPromptName, t.promptText)
}

func (t *llmFindingTranslator) translateFindingAttempt(ctx context.Context, language string, finding FindingItem, promptName string, promptText string) (FindingTranslation, error) {
	input, err := json.Marshal(map[string]string{
		"language":     language,
		"finding_type": finding.FindingType,
		"title":        finding.Title,
		"description":  finding.Description,
		"suggestion":   finding.Suggestion,
	})
	if err != nil {
		return FindingTranslation{}, err
	}
	prompt := promptText
	logger.Info("finding translation start",
		"model_name", t.modelName,
		"prompt_name", promptName,
		"language", language,
		"finding_id", finding.ID,
		"is_retry", promptName != findingTranslationPromptName,
		"content", string(input),
	)
	payload, err := t.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, promptName, prompt, t.modelName, string(input), "translate_doc_review_finding", "MID-CWB-DR-TRANSLATE"))
	if err != nil {
		return FindingTranslation{}, err
	}
	logger.Info("finding translation end  ",
		"finding_id", finding.ID,
		"is_retry", promptName != findingTranslationPromptName,
		"response", payload,
	)
	return findingTranslationFromMap(payload), nil
}

func translationDebugFields(translator FindingTranslator, promptName string) []any {
	fields := []any{"prompt_name", promptName}
	if llmTranslator, ok := translator.(*llmFindingTranslator); ok {
		fields = append(fields, "model_name", llmTranslator.modelName)
	}
	return fields
}

func supportedLanguageCode(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" || language == "en" || language == "en-us" {
		return ""
	}
	for _, r := range language {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return ""
		}
	}
	return language
}

func findingTranslationFromMap(m map[string]any) FindingTranslation {
	return FindingTranslation{
		FindingType: strings.TrimSpace(asString(m["finding_type"])),
		Title:       strings.TrimSpace(asString(m["title"])),
		Description: strings.TrimSpace(asString(m["description"])),
		Suggestion:  strings.TrimSpace(asString(m["suggestion"])),
	}
}

func translationFromMetadata(raw []byte, language string) (FindingTranslation, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return FindingTranslation{}, false
	}
	var metadata map[string]FindingTranslation
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return FindingTranslation{}, false
	}
	tr, ok := metadata[language]
	if !ok || (tr.FindingType == "" && tr.Title == "" && tr.Description == "" && tr.Suggestion == "") {
		return FindingTranslation{}, false
	}
	return tr, true
}

func applyFindingTranslation(f FindingItem, tr FindingTranslation) FindingItem {
	if tr.FindingType != "" {
		f.FindingType = tr.FindingType
	}
	if tr.Title != "" {
		f.Title = tr.Title
	}
	if tr.Description != "" {
		f.Description = tr.Description
	}
	if tr.Suggestion != "" {
		f.Suggestion = tr.Suggestion
	}
	return f
}

func equivalentLocalizedContent(src FindingItem, localized FindingItem) bool {
	// Compare only the natural-language prose fields (title, description, suggestion).
	// finding_type is metadata often in English snake_case and is excluded from this
	// check, consistent with findingInTargetLanguage and translationInTargetLanguage.
	return strings.TrimSpace(localized.Title) == strings.TrimSpace(src.Title) &&
		strings.TrimSpace(localized.Description) == strings.TrimSpace(src.Description) &&
		strings.TrimSpace(localized.Suggestion) == strings.TrimSpace(src.Suggestion)
}

// containsChineseChars reports whether s contains any CJK Unified Ideograph (Han character).
func containsChineseChars(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// findingInTargetLanguage reports whether the natural-language prose fields of
// the finding (title, description, suggestion) are already written in the target
// language. finding_type is metadata (often English snake_case) and is excluded
// from this check. When this returns true the LLM call can be skipped entirely;
// the finding is used as its own translation.
func findingInTargetLanguage(f FindingItem, language string) bool {
	language = supportedLanguageCode(language)
	if language == "" {
		return false
	}
	switch language {
	case "zh":
		return containsChineseChars(f.Title) &&
			containsChineseChars(f.Description) &&
			containsChineseChars(f.Suggestion)
	default:
		return false
	}
}

// translationInTargetLanguage reports whether the translated prose fields
// contain characters from the target language. A translation whose output
// still lacks target-language characters is likely stale English from a
// model failure, even if its text differs from the source.
func translationInTargetLanguage(tr FindingTranslation, language string) bool {
	switch supportedLanguageCode(language) {
	case "zh":
		// Every non-empty prose field must contain target-language characters.
		// If the LLM returned a partial translation (e.g. Chinese title but
		// English description/suggestion), the entry is treated as stale and
		// will be retranslated.
		if tr.Title != "" && !containsChineseChars(tr.Title) {
			return false
		}
		if tr.Description != "" && !containsChineseChars(tr.Description) {
			return false
		}
		if tr.Suggestion != "" && !containsChineseChars(tr.Suggestion) {
			return false
		}
		// At least one non-empty prose field must exist to be valid.
		return tr.Title != "" || tr.Description != "" || tr.Suggestion != ""
	default:
		return false
	}
}

func likelyUntranslatedForLanguage(src FindingItem, tr FindingTranslation, language string) bool {
	language = supportedLanguageCode(language)
	if language == "" {
		return false
	}
	localized := applyFindingTranslation(src, tr)
	if !equivalentLocalizedContent(src, localized) {
		// The cached/generated content differs from the source. Normally this
		// means the text was translated successfully. But if the result still
		// doesn't contain target-language characters, it is likely stale
		// English output from a model failure — treat it as untranslated so
		// the system retries.
		if !translationInTargetLanguage(tr, language) {
			return true
		}
		return false
	}
	// If the source content is already in the target language, identical
	// content means it was correctly left as-is, not that translation failed.
	if findingInTargetLanguage(src, language) {
		return false
	}
	// For now we only offer non-English UI languages configured by operators.
	// If all user-visible fields are byte-for-byte identical to the English
	// source, treat the cache/generation as stale and force an explicit retry.
	return true
}

func saveFindingTranslation(ctx context.Context, db *sql.DB, findingID int64, language string, tr FindingTranslation) error {
	if db == nil {
		return nil
	}
	body, err := json.Marshal(tr)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		UPDATE kb.doc_review_findings
		SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), ARRAY[$2], $3::jsonb, true)
		WHERE id = $1`,
		findingID, language, string(body),
	)
	if err != nil {
		return fmt.Errorf("save finding translation %d/%s: %w", findingID, language, err)
	}
	return nil
}

func (c *DocReviewController) localizeFinding(ctx context.Context, language string, f FindingItem, metadata []byte) (FindingItem, error) {
	language = supportedLanguageCode(language)
	if language == "" {
		return f, nil
	}
	if tr, ok := translationFromMetadata(metadata, language); ok {
		if likelyUntranslatedForLanguage(f, tr, language) {
			fields := []any{
				"finding_id", f.ID,
				"language", language,
				"src_finding_type", f.FindingType,
				"src_title", f.Title,
				"src_description", f.Description,
				"src_suggestion", f.Suggestion,
				"cached_finding_type", tr.FindingType,
				"cached_title", tr.Title,
				"cached_description", tr.Description,
				"cached_suggestion", tr.Suggestion,
			}
			logger.Info("cached finding translation appears untranslated; retrying",
				append(fields, translationDebugFields(c.Translator, findingTranslationPromptName)...)...,
			)
		} else {
			logger.Info("localizeFinding: using cached translation",
				"finding_id", f.ID,
				"language", language,
				"cached_finding_type", tr.FindingType,
				"cached_title", tr.Title,
				"cached_description", tr.Description,
				"cached_suggestion", tr.Suggestion,
			)
			return applyFindingTranslation(f, tr), nil
		}
	}
	logger.Info("localizeFinding: no valid cached translation, proceeding",
		"finding_id", f.ID,
		"language", language,
	)
	// If the finding's prose fields are already in the target language, skip
	// the LLM call entirely. Save the source content as the cached translation
	// so subsequent requests for this language hit the cache.
	if findingInTargetLanguage(f, language) {
		tr := FindingTranslation{
			FindingType: f.FindingType,
			Title:       f.Title,
			Description: f.Description,
			Suggestion:  f.Suggestion,
		}
		if err := saveFindingTranslation(context.WithoutCancel(ctx), c.DB, f.ID, language, tr); err != nil {
			logger.Warn("failed to save self-translation for already-localized finding",
				"finding_id", f.ID, "language", language, "error", err)
		}
		logger.Info("localizeFinding: source already in target language, skipping LLM",
			"finding_id", f.ID,
			"language", language,
			"src_finding_type", f.FindingType,
			"src_title", f.Title,
			"src_description", f.Description,
			"src_suggestion", f.Suggestion,
		)
		return f, nil
	}
	logger.Info("localizeFinding: proceeding to LLM translation",
		"finding_id", f.ID,
		"language", language,
	)

	translator := c.Translator
	if translator == nil {
		var err error
		translator, err = newLLMFindingTranslator()
		if err != nil {
			return f, err
		}
	}
	tr, err := translator.TranslateFinding(ctx, language, f)
	if err != nil {
		return f, fmt.Errorf("translate finding %d to %s (model=%s prompt=%s): %w", f.ID, language, llmTranslatorModelName(translator), findingTranslationRetryPromptName, err)
	}
	if likelyUntranslatedForLanguage(f, tr, language) {
		fields := []any{
			"finding_id", f.ID,
			"language", language,
			"src_finding_type", f.FindingType,
			"src_title", f.Title,
			"src_description", f.Description,
			"src_suggestion", f.Suggestion,
			"llm_finding_type", tr.FindingType,
			"llm_title", tr.Title,
			"llm_description", tr.Description,
			"llm_suggestion", tr.Suggestion,
		}
		logger.Warn("finding translation attempt returned untranslated content; retrying with stricter prompt",
			append(fields, translationDebugFields(translator, findingTranslationPromptName)...)...,
		)
		if llmTranslator, ok := translator.(*llmFindingTranslator); ok {
			tr, err = llmTranslator.translateFindingAttempt(ctx, language, f, findingTranslationRetryPromptName, llmTranslator.retryPromptText)
			if err != nil {
				return f, fmt.Errorf("translate finding %d to %s retry (model=%s prompt=%s): %w", f.ID, language, llmTranslatorModelName(translator), findingTranslationPromptName, err)
			}
			if likelyUntranslatedForLanguage(f, tr, language) {
				return f, fmt.Errorf("translate finding %d to %s: model returned untranslated content after retry (model=%s prompt=%s)", f.ID, language, llmTranslatorModelName(translator), findingTranslationRetryPromptName)
			}
		} else {
			return f, fmt.Errorf("translate finding %d to %s: model returned untranslated content (model=%s prompt=%s)", f.ID, language, llmTranslatorModelName(translator), findingTranslationPromptName)
		}
	}
	if err := saveFindingTranslation(context.WithoutCancel(ctx), c.DB, f.ID, language, tr); err != nil {
		logger.Warn("finding translation save failed", "finding_id", f.ID, "language", language, "error", err)
	} else {
		logger.Info("finding translation saved", "finding_id", f.ID, "language", language)
	}
	return applyFindingTranslation(f, tr), nil
}

func (c *DocReviewController) localizeFindings(ctx context.Context, language string, findings []FindingItem, metadataByFindingID map[int64][]byte) ([]FindingItem, error) {
	language = supportedLanguageCode(language)
	if language == "" {
		return findings, nil
	}
	logger.Info("localizing doc review findings", "language", language, "finding_count", len(findings))
	if c.Translator == nil {
		translator, err := newLLMFindingTranslator()
		if err != nil {
			return nil, err
		}
		c.Translator = translator
	}

	// Translate findings concurrently. On first error, cancel remaining in-flight
	// translations and return the error.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		idx     int
		finding FindingItem
		err     error
	}

	out := make([]FindingItem, len(findings))
	ch := make(chan result, len(findings))
	var wg sync.WaitGroup

	for i, f := range findings {
		wg.Add(1)
		go func(idx int, finding FindingItem) {
			defer wg.Done()
			localized, err := c.localizeFinding(ctx, language, finding, metadataByFindingID[finding.ID])
			if err != nil {
				cancel()
				ch <- result{idx: idx, err: err}
				return
			}
			ch <- result{idx: idx, finding: localized}
		}(i, f)
	}

	wg.Wait()
	close(ch)

	var firstErr error
	for r := range ch {
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
		if r.err == nil {
			out[r.idx] = r.finding
		}
	}

	if firstErr != nil {
		return nil, fmt.Errorf("translate findings to %s: %w", language, firstErr)
	}

	translatedCount := 0
	for i := range out {
		if !equivalentLocalizedContent(findings[i], out[i]) {
			translatedCount++
		}
	}
	logger.Info("localized doc review findings", "language", language, "finding_count", len(findings), "translated_count", translatedCount)
	return out, nil
}

func llmTranslatorModelName(translator FindingTranslator) string {
	if llmTranslator, ok := translator.(*llmFindingTranslator); ok {
		return llmTranslator.modelName
	}
	return ""
}
