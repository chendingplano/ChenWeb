package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode"
)

// FindingTranslator translates the fields displayed in the review report.
type FindingTranslator interface {
	TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingTranslation, error)
}

type llmFindingTranslator struct {
	client    LLMJSONExtractor
	modelName string
}

var errFindingTranslationUnavailable = errors.New("finding translation unavailable")

const findingTranslationPromptName = "doc-review-finding-translation"

func newLLMFindingTranslator() (FindingTranslator, error) {
	modelRef := strings.TrimSpace(os.Getenv("TRANSLATION_MODEL_NAME"))
	if modelRef == "" {
		return nil, fmt.Errorf("%w: TRANSLATION_MODEL_NAME is not configured", errFindingTranslationUnavailable)
	}
	client, modelName, err := docprocessingBuildReviewerLLMClient(modelRef)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errFindingTranslationUnavailable, err)
	}
	return &llmFindingTranslator{client: client, modelName: modelName}, nil
}

func (t *llmFindingTranslator) TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingTranslation, error) {
	return t.translateFindingAttempt(ctx, language, finding, "")
}

func (t *llmFindingTranslator) translateFindingAttempt(ctx context.Context, language string, finding FindingItem, retryInstruction string) (FindingTranslation, error) {
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
	prompt := "Translate the document review finding fields INTO the target language specified by the \"language\" field in the JSON input below. The \"language\" field IS the target language for translation — do NOT treat it as the source language. Translate all natural-language prose into the target language. Translate finding_type too; it is user-visible and must be translated even if it looks like snake_case or a code-like label. Preserve standards identifiers, formulas, product names, and explicit literal identifiers that appear inside the prose. If the target language is zh, use Simplified Chinese. If any field's content is already in the target language, output it unchanged — do not translate it away from the target language. Return JSON with exactly these string keys: finding_type, title, description, suggestion."
	if strings.TrimSpace(retryInstruction) != "" {
		prompt += " " + strings.TrimSpace(retryInstruction)
	}
	logger.Info("finding translation start",
		"model_name", t.modelName,
		"prompt_name", findingTranslationPromptName,
		"language", language,
		"finding_id", finding.ID,
		"is_retry", strings.TrimSpace(retryInstruction) != "",
		"content", string(input),
	)
	payload, err := t.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, findingTranslationPromptName, prompt, t.modelName, string(input), "translate_doc_review_finding", "MID-CWB-DR-TRANSLATE"))
	if err != nil {
		return FindingTranslation{}, err
	}
	logger.Info("finding translation end  ",
		"finding_id", finding.ID,
		"is_retry", strings.TrimSpace(retryInstruction) != "",
		"response", payload,
	)
	return findingTranslationFromMap(payload), nil
}

func translationDebugFields(translator FindingTranslator) []any {
	fields := []any{"prompt_name", findingTranslationPromptName}
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
	return strings.TrimSpace(localized.FindingType) == strings.TrimSpace(src.FindingType) &&
		strings.TrimSpace(localized.Title) == strings.TrimSpace(src.Title) &&
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
		return containsChineseChars(tr.Title) ||
			containsChineseChars(tr.Description) ||
			containsChineseChars(tr.Suggestion)
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
				"title", f.Title,
			}
			logger.Warn("cached finding translation appears untranslated; retrying",
				append(fields, translationDebugFields(c.Translator)...)...,
			)
		} else {
			return applyFindingTranslation(f, tr), nil
		}
	}
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
		if err := saveFindingTranslation(ctx, c.DB, f.ID, language, tr); err != nil {
			logger.Warn("failed to save self-translation for already-localized finding",
				"finding_id", f.ID, "language", language, "error", err)
		}
		return f, nil
	}

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
		return f, fmt.Errorf("translate finding %d to %s (model=%s prompt=%s): %w", f.ID, language, llmTranslatorModelName(translator), findingTranslationPromptName, err)
	}
	if likelyUntranslatedForLanguage(f, tr, language) {
		fields := []any{
			"finding_id", f.ID,
			"language", language,
			"title", f.Title,
		}
		logger.Warn("finding translation attempt returned untranslated content; retrying with stricter prompt",
			append(fields, translationDebugFields(translator)...)...,
		)
		if llmTranslator, ok := translator.(*llmFindingTranslator); ok {
			retryInstruction := "Your previous output was invalid because the translated prose remained in English. Re-translate now. For zh, title, description, and suggestion must be written in Simplified Chinese whenever they contain natural-language prose. Do not echo the source English sentence."
			tr, err = llmTranslator.translateFindingAttempt(ctx, language, f, retryInstruction)
			if err != nil {
				return f, fmt.Errorf("translate finding %d to %s retry (model=%s prompt=%s): %w", f.ID, language, llmTranslatorModelName(translator), findingTranslationPromptName, err)
			}
			if likelyUntranslatedForLanguage(f, tr, language) {
				return f, fmt.Errorf("translate finding %d to %s: model returned untranslated content after retry (model=%s prompt=%s)", f.ID, language, llmTranslatorModelName(translator), findingTranslationPromptName)
			}
		} else {
			return f, fmt.Errorf("translate finding %d to %s: model returned untranslated content (model=%s prompt=%s)", f.ID, language, llmTranslatorModelName(translator), findingTranslationPromptName)
		}
	}
	if err := saveFindingTranslation(ctx, c.DB, f.ID, language, tr); err != nil {
		logger.Warn("finding translation save failed", "finding_id", f.ID, "language", language, "error", err)
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
