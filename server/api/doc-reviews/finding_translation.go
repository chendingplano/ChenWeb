package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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
	prompt := "Translate the document review finding fields into the requested language. Translate all natural-language prose into the requested language; do not leave English unchanged except for standards identifiers, formulas, product names, code-like identifiers, and terms that are normally kept verbatim. If language is zh, use Simplified Chinese. Return JSON with exactly these string keys: finding_type, title, description, suggestion."
	if strings.TrimSpace(retryInstruction) != "" {
		prompt += " " + strings.TrimSpace(retryInstruction)
	}
	logger.Info("calling finding translation llm",
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
	logger.Info("finding translation llm response",
		"model_name", t.modelName,
		"prompt_name", findingTranslationPromptName,
		"language", language,
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

func likelyUntranslatedForLanguage(src FindingItem, tr FindingTranslation, language string) bool {
	language = supportedLanguageCode(language)
	if language == "" {
		return false
	}
	localized := applyFindingTranslation(src, tr)
	if !equivalentLocalizedContent(src, localized) {
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

	out := make([]FindingItem, 0, len(findings))
	translatedCount := 0
	for _, f := range findings {
		localized, err := c.localizeFinding(ctx, language, f, metadataByFindingID[f.ID])
		if err != nil {
			logger.Warn("doc review finding localization failed",
				"finding_id", f.ID,
				"language", language,
				"error", err,
				"prompt_name", findingTranslationPromptName,
				"model_name", llmTranslatorModelName(c.Translator),
			)
			return nil, err
		}
		if !equivalentLocalizedContent(f, localized) {
			translatedCount++
		}
		out = append(out, localized)
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
