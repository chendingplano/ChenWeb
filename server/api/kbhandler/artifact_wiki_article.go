package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const genericArtifactWikiSchemaVersion = 1

var buildGenericArtifactWikiArticleFn = buildArtifactWikiArticle
var translateGenericArtifactWikiArticleFn = translateGenericArtifactWikiArticle

const genericArtifactWikiPrompt = `You are writing a concise wiki-style detail page for a searchable knowledge artifact.

You will receive:
- artifact_type
- artifact_id
- source_document metadata
- the grounded record JSON

Write an explanatory page that stays faithful to the record.
Do not invent facts.
Do not omit major non-empty fields from the record when they materially help explain the artifact.
Prefer grounded wording over general filler.

Return only a JSON object with these fields:
{
  "title": "artifact title",
  "lead": "1-3 sentence overview",
  "definition": "precise definition or statement, if available",
  "background": "supporting background or context",
  "how_used": "how this artifact is used, interpreted, or applied",
  "choosing_values": "guidance, implications, or handling notes when relevant"
}

"title" and "lead" must be non-empty. Other fields may be empty strings.`

func generateGenericArtifactWikiArticle(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (json.RawMessage, error) {
	return nil, fmt.Errorf("generic article generation requires payload context")
}

func buildArtifactWikiArticle(ctx context.Context, logger ApiTypes.JimoLogger, payload artifactWikiPayload, artifactType, lang string) (json.RawMessage, error) {
	if artifactType == "metric" {
		return nil, fmt.Errorf("metric article generation should use the metric-specific builder")
	}
	if normalizeArtifactWikiLang(lang) != "en" {
		return nil, fmt.Errorf("generic article builder only generates English source articles")
	}

	modelRef, cfg, err := loadWikiModelDef("WIKIPAGE_CREATION_MODEL_NAME")
	if err != nil {
		return nil, fmt.Errorf("load wiki model config: %w", err)
	}
	if strings.TrimSpace(modelRef) == "" {
		return nil, fmt.Errorf("missing env var WIKIPAGE_CREATION_MODEL_NAME")
	}
	client, err := defaultNewExtractMetricsClient(modelRef, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("create wiki LLM client: %w", err)
	}

	inputJSON, err := json.Marshal(map[string]any{
		"artifact_type":   artifactType,
		"artifact_id":     payload.ArtifactID,
		"record":          json.RawMessage(payload.Record),
		"source_document": json.RawMessage(payload.SourceDocument),
	})
	if err != nil {
		return nil, fmt.Errorf("encode generic artifact wiki input: %w", err)
	}

	out, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptName: "generic_artifact_wiki_prompt",
		PromptText: genericArtifactWikiPrompt + "\n\n" + metricWikiOutputLanguageInstruction("en"),
		ModelName:  cfg.ModelName,
		InputText:  string(inputJSON),
	})
	if err != nil {
		return nil, fmt.Errorf("generate generic artifact wiki article: %w", err)
	}

	page := genericArtifactWikiPage{
		Title:          firstNonEmptyVal(out, "title", "lead"),
		Lead:           firstNonEmptyVal(out, "lead", "definition"),
		Definition:     firstNonEmptyVal(out, "definition"),
		Background:     firstNonEmptyVal(out, "background"),
		HowUsed:        firstNonEmptyVal(out, "how_used"),
		ChoosingValues: firstNonEmptyVal(out, "choosing_values"),
		Generated: artifactWikiGeneratedMeta{
			Model:         cfg.ModelName,
			Lang:          "en",
			SchemaVersion: genericArtifactWikiSchemaVersion,
			SourceHash:    payload.Generated.SourceHash,
		},
	}
	data, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal generic artifact wiki page: %w", err)
	}
	return json.RawMessage(data), nil
}

func translateGenericArtifactWikiArticle(ctx context.Context, logger ApiTypes.JimoLogger, article json.RawMessage, targetLang string) (json.RawMessage, error) {
	targetLang = normalizeArtifactWikiLang(targetLang)
	if targetLang == "en" {
		return article, nil
	}

	var page genericArtifactWikiPage
	if err := json.Unmarshal(article, &page); err != nil {
		return nil, fmt.Errorf("decode generic artifact wiki page: %w", err)
	}

	modelRef, cfg, err := loadWikiModelDef("TRANSLATION_MODEL_NAME")
	if err != nil {
		return nil, fmt.Errorf("load translation model config: %w", err)
	}
	if strings.TrimSpace(modelRef) == "" {
		page.Generated.Lang = targetLang
		data, marshalErr := json.MarshalIndent(page, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal untranslated generic wiki page: %w", marshalErr)
		}
		return json.RawMessage(data), nil
	}

	client, err := defaultNewExtractMetricsClient(modelRef, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("create translation client: %w", err)
	}

	inputText, err := json.Marshal(map[string]any{
		"target_lang": targetLang,
		"page":        page,
	})
	if err != nil {
		return nil, fmt.Errorf("encode translation input: %w", err)
	}

	payload, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptName: "metric_wiki_translation_prompt",
		PromptText: metricWikiTranslationPrompt,
		ModelName:  cfg.ModelName,
		InputText:  string(inputText),
	})
	if err != nil {
		return nil, fmt.Errorf("translate generic wiki page: %w", err)
	}

	page.Title = firstNonEmpty(firstNonEmptyVal(payload, "title"), page.Title)
	page.Lead = firstNonEmpty(firstNonEmptyVal(payload, "lead"), page.Lead)
	page.Definition = firstNonEmpty(firstNonEmptyVal(payload, "definition"), page.Definition)
	page.Background = firstNonEmpty(firstNonEmptyVal(payload, "background"), page.Background)
	page.HowUsed = firstNonEmpty(firstNonEmptyVal(payload, "how_used"), page.HowUsed)
	page.ChoosingValues = firstNonEmpty(firstNonEmptyVal(payload, "choosing_values"), page.ChoosingValues)
	page.Generated.Lang = targetLang
	page.Generated.Model = cfg.ModelName

	data, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal translated generic wiki page: %w", err)
	}
	return json.RawMessage(data), nil
}
