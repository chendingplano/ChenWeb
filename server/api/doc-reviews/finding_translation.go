package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// FindingTranslator normalizes findings into canonical English and localizes
// the canonical prose into configured display languages.
type FindingTranslator interface {
	NormalizeFinding(ctx context.Context, canonicalLanguage string, finding FindingItem) (FindingNormalization, error)
	TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingLocalizedContent, error)
}

type llmFindingTranslator struct {
	client                     LLMJSONExtractor
	modelName                  string
	normalizePromptText        string
	normalizeRetryPromptText   string
	translationPromptText      string
	translationRetryPromptText string
}

var errFindingTranslationUnavailable = errors.New("finding translation unavailable")

const findingNormalizationPromptName = "doc-review-finding-normalize"
const findingNormalizationRetryPromptName = "doc-review-finding-normalize-retry"
const findingTranslationPromptName = "doc-review-finding-localize"
const findingTranslationRetryPromptName = "doc-review-finding-localize-retry"

func newLLMFindingTranslator() (FindingTranslator, error) {
	modelRef := strings.TrimSpace(os.Getenv("TRANSLATION_MODEL_NAME"))
	if modelRef == "" {
		return nil, fmt.Errorf("%w: TRANSLATION_MODEL_NAME is not configured", errFindingTranslationUnavailable)
	}
	client, modelName, err := docprocessingBuildReviewerLLMClient(modelRef)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errFindingTranslationUnavailable, err)
	}

	loadPrompt := func(name string) (string, error) {
		body, _, _, err := loadPromptByRef(name)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(body), nil
	}

	normalizePromptText, err := loadPrompt("prompt-doc-review-finding-normalize-v1.md")
	if err != nil {
		return nil, fmt.Errorf("%w: load normalization prompt: %v", errFindingTranslationUnavailable, err)
	}
	normalizeRetryPromptText, err := loadPrompt("prompt-doc-review-finding-normalize-retry-v1.md")
	if err != nil {
		return nil, fmt.Errorf("%w: load normalization retry prompt: %v", errFindingTranslationUnavailable, err)
	}
	translationPromptText, err := loadPrompt("prompt-doc-review-finding-localize-v1.md")
	if err != nil {
		return nil, fmt.Errorf("%w: load localization prompt: %v", errFindingTranslationUnavailable, err)
	}
	translationRetryPromptText, err := loadPrompt("prompt-doc-review-finding-localize-retry-v1.md")
	if err != nil {
		return nil, fmt.Errorf("%w: load localization retry prompt: %v", errFindingTranslationUnavailable, err)
	}

	return &llmFindingTranslator{
		client:                     client,
		modelName:                  modelName,
		normalizePromptText:        normalizePromptText,
		normalizeRetryPromptText:   normalizeRetryPromptText,
		translationPromptText:      translationPromptText,
		translationRetryPromptText: translationRetryPromptText,
	}, nil
}

func (t *llmFindingTranslator) NormalizeFinding(ctx context.Context, canonicalLanguage string, finding FindingItem) (FindingNormalization, error) {
	return t.normalizeFindingAttempt(ctx, canonicalLanguage, finding, findingNormalizationPromptName, t.normalizePromptText)
}

func (t *llmFindingTranslator) normalizeFindingAttempt(ctx context.Context, canonicalLanguage string, finding FindingItem, promptName string, promptText string) (FindingNormalization, error) {
	input, err := json.Marshal(map[string]any{
		"canonical_language": canonicalLanguage,
		"finding": map[string]any{
			"severity":     finding.Severity,
			"finding_type": finding.FindingType,
			"title":        finding.Title,
			"description":  finding.Description,
			"evidence":     finding.Evidence,
			"location":     finding.Location,
			"suggestion":   finding.Suggestion,
			"confidence":   finding.Confidence,
		},
	})
	if err != nil {
		return FindingNormalization{}, err
	}
	logger.Info("doc-review finding normalization start",
		"model_name", t.modelName,
		"prompt_name", promptName,
		"canonical_language", canonicalLanguage,
		"input_json", string(input),
	)
	payload, err := t.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, promptName, promptText, t.modelName, string(input), "normalize_doc_review_finding", "MID-CWB-DR-NORMALIZE"))
	if err != nil {
		return FindingNormalization{}, err
	}
	logger.Info("doc-review finding normalization response",
		"model_name", t.modelName,
		"prompt_name", promptName,
		"canonical_language", canonicalLanguage,
		"response_json", compactJSONForLog(payload),
	)
	result := findingNormalizationFromMap(payload)
	if !normalizationInCanonicalLanguage(result, canonicalLanguage) && promptName == findingNormalizationPromptName {
		return t.normalizeFindingAttempt(ctx, canonicalLanguage, finding, findingNormalizationRetryPromptName, t.normalizeRetryPromptText)
	}
	if !normalizationInCanonicalLanguage(result, canonicalLanguage) {
		return FindingNormalization{}, fmt.Errorf(
			"normalization output not in canonical language %q; source_title=%q source_description=%q source_suggestion=%q canonical_title=%q canonical_description=%q canonical_suggestion=%q detected_source_language=%q canonical_language=%q",
			canonicalLanguage,
			compactDiagnosticText(finding.Title),
			compactDiagnosticText(finding.Description),
			compactDiagnosticText(finding.Suggestion),
			compactDiagnosticText(result.Canonical.Title),
			compactDiagnosticText(result.Canonical.Description),
			compactDiagnosticText(result.Canonical.Suggestion),
			strings.TrimSpace(result.SourceLanguage),
			strings.TrimSpace(result.CanonicalLanguage),
		)
	}
	return result, nil
}

func (t *llmFindingTranslator) TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingLocalizedContent, error) {
	return t.translateFindingAttempt(ctx, language, finding, findingTranslationPromptName, t.translationPromptText)
}

func (t *llmFindingTranslator) translateFindingAttempt(ctx context.Context, language string, finding FindingItem, promptName string, promptText string) (FindingLocalizedContent, error) {
	input, err := json.Marshal(map[string]string{
		"target_language":    language,
		"canonical_language": "en",
		"finding_type":       finding.FindingType,
		"title":              finding.Title,
		"description":        finding.Description,
		"suggestion":         finding.Suggestion,
		"evidence":           finding.Evidence,
	})
	if err != nil {
		return FindingLocalizedContent{}, err
	}
	logger.Info("doc-review finding localization start",
		"model_name", t.modelName,
		"prompt_name", promptName,
		"target_language", language,
		"input_json", string(input),
	)
	payload, err := t.client.ExtractJSON(ctx, newDocReviewLLMJSONInput(ctx, promptName, promptText, t.modelName, string(input), "localize_doc_review_finding", "MID-CWB-DR-LOCALIZE"))
	if err != nil {
		return FindingLocalizedContent{}, err
	}
	logger.Info("doc-review finding localization response",
		"model_name", t.modelName,
		"prompt_name", promptName,
		"target_language", language,
		"response_json", compactJSONForLog(payload),
	)
	tr := findingLocalizedContentFromMap(payload)
	if !translationInTargetLanguage(tr, language) && promptName == findingTranslationPromptName {
		return t.translateFindingAttempt(ctx, language, finding, findingTranslationRetryPromptName, t.translationRetryPromptText)
	}
	if !translationInTargetLanguage(tr, language) {
		return FindingLocalizedContent{}, fmt.Errorf("localized output not in target language %q", language)
	}
	if tr.Provenance == "" {
		tr.Provenance = "llm_translation"
	}
	return tr, nil
}

func findingNormalizationFromMap(m map[string]any) FindingNormalization {
	sourceMap := mapFromAny(m["source_translation"])
	canonicalMap := mapFromAny(m["canonical"])
	findingMap := mapFromAny(m["finding"])
	canonicalTitle := strings.TrimSpace(asString(m["canonical_title"]))
	if canonicalTitle == "" {
		canonicalTitle = strings.TrimSpace(asString(canonicalMap["title"]))
	}
	if canonicalTitle == "" {
		canonicalTitle = strings.TrimSpace(asString(findingMap["title"]))
	}
	if canonicalTitle == "" {
		canonicalTitle = strings.TrimSpace(asString(m["title"]))
	}
	canonicalDescription := strings.TrimSpace(asString(m["canonical_description"]))
	if canonicalDescription == "" {
		canonicalDescription = strings.TrimSpace(asString(canonicalMap["description"]))
	}
	if canonicalDescription == "" {
		canonicalDescription = strings.TrimSpace(asString(findingMap["description"]))
	}
	if canonicalDescription == "" {
		canonicalDescription = strings.TrimSpace(asString(m["description"]))
	}
	canonicalSuggestion := strings.TrimSpace(asString(m["canonical_suggestion"]))
	if canonicalSuggestion == "" {
		canonicalSuggestion = strings.TrimSpace(asString(canonicalMap["suggestion"]))
	}
	if canonicalSuggestion == "" {
		canonicalSuggestion = strings.TrimSpace(asString(findingMap["suggestion"]))
	}
	if canonicalSuggestion == "" {
		canonicalSuggestion = strings.TrimSpace(asString(m["suggestion"]))
	}
	return FindingNormalization{
		SourceLanguage:           strings.TrimSpace(asString(m["source_language"])),
		SourceLanguageConfidence: asFloat64Generic(m["source_language_confidence"]),
		CanonicalLanguage:        strings.TrimSpace(asString(m["canonical_language"])),
		CanonicalOrigin:          strings.TrimSpace(asString(m["canonical_origin"])),
		Canonical: FindingLocalizedContent{
			Title:       canonicalTitle,
			Description: canonicalDescription,
			Suggestion:  canonicalSuggestion,
			Provenance:  "canonical",
		},
		SourceTranslation: FindingLocalizedContent{
			Title:       strings.TrimSpace(asString(sourceMap["title"])),
			Description: strings.TrimSpace(asString(sourceMap["description"])),
			Suggestion:  strings.TrimSpace(asString(sourceMap["suggestion"])),
			Provenance:  strings.TrimSpace(asString(sourceMap["provenance"])),
		},
	}
}

func mapFromAny(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

func asFloat64Generic(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
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

func compactDiagnosticText(s string) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	const maxLen = 180
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func compactJSONForLog(v any) string {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<marshal_error:%v>", err)
	}
	return compactDiagnosticText(string(body))
}

func normalizeConfiguredLanguages(languages []string) []string {
	seen := map[string]bool{"en": true}
	out := []string{"en"}
	for _, language := range languages {
		if lang := supportedLanguageCode(language); lang != "" && !seen[lang] {
			seen[lang] = true
			out = append(out, lang)
		}
	}
	return out
}

func findingLocalizedContentFromMap(m map[string]any) FindingLocalizedContent {
	return FindingLocalizedContent{
		Title:       strings.TrimSpace(asString(m["title"])),
		Description: strings.TrimSpace(asString(m["description"])),
		Suggestion:  strings.TrimSpace(asString(m["suggestion"])),
		Provenance:  strings.TrimSpace(asString(m["provenance"])),
	}
}

func translationFromMetadata(raw []byte, language string) (FindingTranslation, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return FindingTranslation{}, false
	}
	var env FindingMetadataEnvelope
	if err := json.Unmarshal(raw, &env); err == nil {
		if tr, ok := env.I18N.Translations[language]; ok && (tr.Title != "" || tr.Description != "" || tr.Suggestion != "") {
			return tr, true
		}
	}

	var legacy map[string]FindingTranslation
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return FindingTranslation{}, false
	}
	tr, ok := legacy[language]
	if !ok || (tr.Title == "" && tr.Description == "" && tr.Suggestion == "") {
		return FindingTranslation{}, false
	}
	return tr, true
}

func applyFindingTranslation(f FindingItem, tr FindingTranslation) FindingItem {
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

func containsChineseChars(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func containsLatinLetters(s string) bool {
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return true
		}
	}
	return false
}

func containsEnglishWord(s string, minLen int) bool {
	runLen := 0
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			runLen++
			if runLen >= minLen {
				return true
			}
			continue
		}
		runLen = 0
	}
	return false
}

func translationInTargetLanguage(tr FindingLocalizedContent, language string) bool {
	switch supportedLanguageCode(language) {
	case "zh":
		if tr.Title != "" && !containsChineseChars(tr.Title) {
			return false
		}
		if tr.Description != "" && !containsChineseChars(tr.Description) {
			return false
		}
		if tr.Suggestion != "" && !containsChineseChars(tr.Suggestion) {
			return false
		}
		return tr.Title != "" || tr.Description != "" || tr.Suggestion != ""
	default:
		if language == "" || strings.EqualFold(language, "en") || strings.EqualFold(language, "en-us") {
			if tr.Title != "" && !containsEnglishWord(tr.Title, 3) {
				return false
			}
			if tr.Description != "" && !containsEnglishWord(tr.Description, 3) {
				return false
			}
			if tr.Suggestion != "" && !containsEnglishWord(tr.Suggestion, 3) {
				return false
			}
			return tr.Title != "" || tr.Description != "" || tr.Suggestion != ""
		}
		return tr.Title != "" || tr.Description != "" || tr.Suggestion != ""
	}
}

func englishCanonicalFieldOK(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	return containsEnglishWord(s, 3)
}

func normalizationInCanonicalLanguage(n FindingNormalization, canonicalLanguage string) bool {
	if strings.TrimSpace(n.CanonicalLanguage) == "" {
		n.CanonicalLanguage = canonicalLanguage
	}
	if !strings.EqualFold(n.CanonicalLanguage, canonicalLanguage) {
		return false
	}
	if strings.EqualFold(canonicalLanguage, "en") || strings.EqualFold(canonicalLanguage, "en-us") {
		// Title/description carry the explanatory prose and should be canonical English.
		// Suggestion may legitimately be a literal replacement string in the source language.
		return englishCanonicalFieldOK(n.Canonical.Title) &&
			englishCanonicalFieldOK(n.Canonical.Description)
	}
	return translationInTargetLanguage(n.Canonical, canonicalLanguage)
}

type preparedFindingForStorage struct {
	Canonical ReviewFinding
	Metadata  FindingMetadataEnvelope
}

func prepareFindingForStorage(ctx context.Context, translator FindingTranslator, languages []string, finding ReviewFinding) (preparedFindingForStorage, error) {
	if translator == nil {
		var err error
		translator, err = newLLMFindingTranslator()
		if err != nil {
			return preparedFindingForStorage{}, err
		}
	}

	base := FindingItem{
		Pass:        finding.Pass,
		Aspect:      finding.Aspect,
		Severity:    finding.Severity,
		FindingType: finding.FindingType,
		Title:       finding.Title,
		Description: finding.Description,
		Evidence:    finding.Evidence,
		Location:    finding.Location,
		Suggestion:  finding.Suggestion,
		Confidence:  finding.Confidence,
	}
	normalized, err := translator.NormalizeFinding(ctx, "en", base)
	if err != nil {
		return preparedFindingForStorage{}, fmt.Errorf("normalize finding: %w", err)
	}
	if normalized.CanonicalOrigin == "" {
		normalized.CanonicalOrigin = "translated"
	}
	if normalized.CanonicalLanguage == "" {
		normalized.CanonicalLanguage = "en"
	}
	if normalized.SourceLanguage == "" {
		normalized.SourceLanguage = "und"
	}
	if normalized.Canonical.Provenance == "" {
		normalized.Canonical.Provenance = "canonical"
	}

	translations := map[string]FindingLocalizedContent{
		"en": {
			Title:       normalized.Canonical.Title,
			Description: normalized.Canonical.Description,
			Suggestion:  normalized.Canonical.Suggestion,
			Provenance:  "canonical",
		},
	}
	sourceLanguage := strings.ToLower(strings.TrimSpace(normalized.SourceLanguage))
	if sourceLanguage != "" && sourceLanguage != "en" &&
		(normalized.SourceTranslation.Title != "" || normalized.SourceTranslation.Description != "" || normalized.SourceTranslation.Suggestion != "") {
		if normalized.SourceTranslation.Provenance == "" {
			normalized.SourceTranslation.Provenance = "original_extraction"
		}
		translations[sourceLanguage] = normalized.SourceTranslation
	}

	canonicalFinding := FindingItem{
		Pass:        finding.Pass,
		Aspect:      finding.Aspect,
		Severity:    finding.Severity,
		FindingType: finding.FindingType,
		Title:       normalized.Canonical.Title,
		Description: normalized.Canonical.Description,
		Evidence:    finding.Evidence,
		Location:    finding.Location,
		Suggestion:  normalized.Canonical.Suggestion,
		Confidence:  finding.Confidence,
	}
	var pendingLanguages []string
	for _, language := range normalizeConfiguredLanguages(languages) {
		if language == "en" {
			continue
		}
		if _, ok := translations[language]; ok {
			continue
		}
		pendingLanguages = append(pendingLanguages, language)
	}
	if len(pendingLanguages) > 0 {
		type translationResult struct {
			language  string
			localized FindingLocalizedContent
		}
		maxTasks := maxDocReviewerTasks(len(pendingLanguages))
		results, err := runConcurrent(ctx, maxTasks, len(pendingLanguages), func(workerCtx context.Context, i int) (translationResult, error) {
			language := pendingLanguages[i]
			localized, err := translator.TranslateFinding(workerCtx, language, canonicalFinding)
			if err != nil {
				return translationResult{}, fmt.Errorf("translate finding to %s: %w", language, err)
			}
			if localized.Provenance == "" {
				localized.Provenance = "llm_translation"
			}
			return translationResult{language: language, localized: localized}, nil
		})
		if err != nil {
			return preparedFindingForStorage{}, err
		}
		for _, result := range results {
			translations[result.language] = result.localized
		}
	}

	return preparedFindingForStorage{
		Canonical: ReviewFinding{
			Pass:        canonicalFinding.Pass,
			Aspect:      canonicalFinding.Aspect,
			Severity:    canonicalFinding.Severity,
			FindingType: canonicalFinding.FindingType,
			Title:       canonicalFinding.Title,
			Description: canonicalFinding.Description,
			Evidence:    canonicalFinding.Evidence,
			Location:    canonicalFinding.Location,
			Suggestion:  canonicalFinding.Suggestion,
			Confidence:  canonicalFinding.Confidence,
		},
		Metadata: FindingMetadataEnvelope{
			I18N: FindingI18NMetadata{
				SchemaVersion:            1,
				SourceLanguage:           normalized.SourceLanguage,
				SourceLanguageConfidence: normalized.SourceLanguageConfidence,
				CanonicalLanguage:        normalized.CanonicalLanguage,
				CanonicalOrigin:          normalized.CanonicalOrigin,
				Translations:             translations,
			},
		},
	}, nil
}

func (c *DocReviewController) localizeFinding(ctx context.Context, language string, f FindingItem, metadata []byte) (FindingItem, error) {
	language = supportedLanguageCode(language)
	if language == "" {
		return f, nil
	}
	if tr, ok := translationFromMetadata(metadata, language); ok {
		return applyFindingTranslation(f, tr), nil
	}
	return f, nil
}

func (c *DocReviewController) localizeFindings(ctx context.Context, language string, findings []FindingItem, metadataByFindingID map[int64][]byte) ([]FindingItem, error) {
	language = supportedLanguageCode(language)
	if language == "" {
		return findings, nil
	}
	out := make([]FindingItem, 0, len(findings))
	for _, finding := range findings {
		localized, err := c.localizeFinding(ctx, language, finding, metadataByFindingID[finding.ID])
		if err != nil {
			return nil, err
		}
		out = append(out, localized)
	}
	return out, nil
}
