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
	prompt := "Translate the document review finding fields into the requested language. Return JSON with exactly these string keys: finding_type, title, description, suggestion. Preserve technical terms, line numbers, formulas, and identifiers."
	payload, err := t.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, "doc-review-finding-translation", prompt, t.modelName, string(input), "translate_doc_review_finding", "MID-CWB-DR-TRANSLATE"))
	if err != nil {
		return FindingTranslation{}, err
	}
	return findingTranslationFromMap(payload), nil
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
		return applyFindingTranslation(f, tr), nil
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
		return f, fmt.Errorf("translate finding %d to %s: %w", f.ID, language, err)
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
	if c.Translator == nil {
		translator, err := newLLMFindingTranslator()
		if err != nil {
			return nil, err
		}
		c.Translator = translator
	}

	out := make([]FindingItem, 0, len(findings))
	for _, f := range findings {
		localized, err := c.localizeFinding(ctx, language, f, metadataByFindingID[f.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, localized)
	}
	return out, nil
}
