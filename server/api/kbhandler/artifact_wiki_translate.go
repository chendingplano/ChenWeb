package kbhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

var translateArtifactWikiMetricArticleFn = translateArtifactWikiMetricArticle

const metricWikiTranslationPrompt = `You are translating a metric wiki page JSON object.

Preserve the JSON object structure and keys exactly.
Translate only human-readable prose fields into the requested target language.
Do not modify ids, field names, units, numeric values, booleans, or null values.
Do not summarize, shorten, omit, or simplify the content.
If a source prose field is non-empty, the translated field must also be non-empty and must preserve the same meaning and level of detail.
Return valid JSON matching the input shape.`

func translateArtifactWikiMetricArticle(ctx context.Context, logger ApiTypes.JimoLogger, article json.RawMessage, targetLang string) (json.RawMessage, error) {
	targetLang = normalizeArtifactWikiLang(targetLang)
	if targetLang == "en" {
		return article, nil
	}

	var page metricWikiPage
	if err := json.Unmarshal(article, &page); err != nil {
		return nil, fmt.Errorf("decode metric wiki page: %w", err)
	}
	modelRef, cfg, err := loadWikiModelDef("TRANSLATION_MODEL_NAME")
	if err != nil {
		return nil, fmt.Errorf("load translation model config: %w", err)
	}
	if strings.TrimSpace(modelRef) == "" {
		page.Generated.Lang = targetLang
		data, marshalErr := json.MarshalIndent(page, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal untranslated metric wiki page: %w", marshalErr)
		}
		return json.RawMessage(data), nil
	}

	client, err := defaultNewExtractMetricsClient(modelRef, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("create translation client: %w", err)
	}

	input := map[string]any{"target_lang": targetLang, "page": page}
	inputText, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode translation input: %w", err)
	}

	payload, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptName: llmclients.EnsurePromptName("metric_wiki_translation_prompt", "translate_metric_wiki", "MID-CWB-TRANSLATE-METRIC-WIKI", cfg.ModelName),
		PromptText: metricWikiTranslationPrompt,
		ModelName:  cfg.ModelName,
		InputText:  string(inputText),
		CallReason: "translate_metric_wiki",
		CallLoc:    "MID-CWB-TRANSLATE-METRIC-WIKI",
	})
	if err != nil {
		return nil, fmt.Errorf("translate metric wiki page: %w", err)
	}

	page.Title = firstNonEmptyVal(payload, "title")
	page.Lead = firstNonEmptyVal(payload, "lead")
	page.Definition = firstNonEmptyVal(payload, "definition")
	page.Background = firstNonEmptyVal(payload, "background")
	page.HowUsed = firstNonEmptyVal(payload, "how_used")
	page.ChoosingValues = firstNonEmptyVal(payload, "choosing_values")
	if related := stringSliceVal(payload, "related_metrics"); len(related) > 0 {
		page.RelatedMetrics = related
	}
	if sourceExcerpt := firstNonEmptyVal(payload, "source_excerpt"); sourceExcerpt != "" {
		page.InThisCorpus.SourceExcerpt = sourceExcerpt
	}
	if chunkSummary := firstNonEmptyVal(payload, "chunk_summary"); chunkSummary != "" {
		page.InThisCorpus.ChunkSummary = chunkSummary
	}
	page.Generated.Lang = targetLang
	page.Generated.Model = cfg.ModelName

	data, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal translated metric wiki page: %w", err)
	}
	return json.RawMessage(data), nil
}
