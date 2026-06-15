package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	toml "github.com/pelletier/go-toml/v2"
)

const metricWikiSchemaVersion = 1

// metricWikiPage is the cached page document. Grounded structured fields
// (infobox, in_this_corpus, title) are filled server-side from the compiled
// context; the prose sections are written by the LLM. This keeps measured
// values authoritative and confines model creativity to explanatory text.
type metricWikiPage struct {
	MetricID       string              `json:"metric_id"`
	Title          string              `json:"title"`
	Lead           string              `json:"lead"`
	Infobox        metricWikiInfobox   `json:"infobox"`
	Definition     string              `json:"definition,omitempty"`
	Background     string              `json:"background,omitempty"`
	HowUsed        string              `json:"how_used,omitempty"`
	ChoosingValues string              `json:"choosing_values,omitempty"`
	InThisCorpus   metricWikiCorpus    `json:"in_this_corpus"`
	RelatedMetrics []string            `json:"related_metrics,omitempty"`
	Generated      metricWikiGenerated `json:"generated"`
}

type metricWikiInfobox struct {
	Value             string   `json:"value,omitempty"`
	Unit              string   `json:"unit,omitempty"`
	RangeType         string   `json:"range_type,omitempty"`
	ThresholdOrTarget string   `json:"threshold_or_target,omitempty"`
	MeasurementFreq   string   `json:"measurement_frequency,omitempty"`
	Subject           string   `json:"subject,omitempty"`
	Confidence        *float64 `json:"confidence,omitempty"`
}

type metricWikiCorpus struct {
	SourceDocument metricWikiDocMeta `json:"source_document"`
	SourceExcerpt  string            `json:"source_excerpt,omitempty"`
	ChunkSummary   string            `json:"chunk_summary,omitempty"`
}

type metricWikiGenerated struct {
	Model         string `json:"model"`
	Lang          string `json:"lang"`
	SchemaVersion int    `json:"schema_version"`
	SourceHash    string `json:"source_hash"`
}

// metricWikiProse is the LLM-authored portion of the page.
type metricWikiProse struct {
	Lead           string
	Definition     string
	Background     string
	HowUsed        string
	ChoosingValues string
	RelatedMetrics []string
}

// buildMetricWikiPageFn is the seam used by the handler's cache-miss path. It is
// a variable so tests can substitute a fake that skips DB/LLM access.
var buildMetricWikiPageFn = buildMetricWikiPage

// buildMetricWikiPage compiles the metric's context and generates its page JSON.
func buildMetricWikiPage(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, recordID int64, metricID, lang string) (json.RawMessage, error) {
	mctx, err := compileMetricWikiContext(db, recordID, metricID)
	if err != nil {
		return nil, fmt.Errorf("compile metric wiki context: %w", err)
	}
	return generateMetricWikiPage(ctx, logger, mctx, lang)
}

// generateMetricWikiPage calls the configured LLM to write the prose sections,
// assembles them with the grounded fields, and returns the marshaled page JSON
// in the requested language.
func generateMetricWikiPage(ctx context.Context, logger ApiTypes.JimoLogger, mctx metricWikiContext, lang string) (json.RawMessage, error) {
	if lang == "" {
		lang = "en"
	}
	prompt := metricWikiPromptText(lang)
	inputText, err := metricWikiFactsJSON(mctx)
	if err != nil {
		return nil, fmt.Errorf("encode metric facts: %w", err)
	}

	prose, modelName, err := runMetricWikiProse(ctx, logger, prompt, inputText)
	if err != nil {
		return nil, err
	}

	page := assembleMetricWikiPage(mctx, prose, modelName, lang)
	data, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal metric wiki page: %w", err)
	}
	return json.RawMessage(data), nil
}

// runMetricWikiProse tries the primary model, then the fallback, returning the
// first valid prose result and the model name that produced it.
func runMetricWikiProse(ctx context.Context, logger ApiTypes.JimoLogger, prompt, inputText string) (metricWikiProse, string, error) {
	type modelAttempt struct {
		envKey   string
		required bool
	}
	attempts := []modelAttempt{
		{"WIKIPAGE_CREATION_MODEL_NAME", true},
		{"WIKIPAGE_CREATION_FALLBACK", false},
	}

	var lastErr error
	sawModel := false
	for _, a := range attempts {
		modelRef, cfg, err := loadWikiModelDef(a.envKey)
		if err != nil {
			lastErr = err
			if logger != nil {
				logger.Error("load wiki model config failed", "env", a.envKey, "model_ref", modelRef, "err", err)
			}
			continue
		}
		if modelRef == "" {
			if a.required {
				return metricWikiProse{}, "", fmt.Errorf("missing env var %s", a.envKey)
			}
			continue
		}
		sawModel = true

		client, err := defaultNewExtractMetricsClient(cfg, logger)
		if err != nil {
			lastErr = err
			if logger != nil {
				logger.Error("create wiki LLM client failed", "env", a.envKey, "model_name", cfg.ModelName, "err", err)
			}
			continue
		}
		if logger != nil {
			logger.Info("generating metric wiki prose", "env", a.envKey, "model_name", cfg.ModelName)
		}
		payload, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
			PromptText: prompt,
			ModelName:  cfg.ModelName,
			InputText:  inputText,
		})
		if err != nil {
			lastErr = err
			if logger != nil {
				logger.Error("wiki LLM call failed", "env", a.envKey, "err", err)
			}
			continue
		}
		prose := parseMetricWikiProse(payload)
		if strings.TrimSpace(prose.Lead) == "" {
			lastErr = fmt.Errorf("model %q returned no usable lead", cfg.ModelName)
			if logger != nil {
				logger.Error("wiki LLM output unusable", "env", a.envKey)
			}
			continue
		}
		return prose, cfg.ModelName, nil
	}

	if !sawModel && lastErr == nil {
		return metricWikiProse{}, "", fmt.Errorf("no wiki generation model configured")
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all wiki generation models failed")
	}
	return metricWikiProse{}, "", lastErr
}

// parseMetricWikiProse extracts the prose fields from the LLM payload, tolerating
// missing keys.
func parseMetricWikiProse(payload map[string]any) metricWikiProse {
	return metricWikiProse{
		Lead:           stringVal(payload, "lead"),
		Definition:     stringVal(payload, "definition"),
		Background:     stringVal(payload, "background"),
		HowUsed:        firstNonEmptyVal(payload, "how_used", "usage"),
		ChoosingValues: firstNonEmptyVal(payload, "choosing_values", "value_guidance"),
		RelatedMetrics: stringSliceVal(payload, "related_metrics"),
	}
}

// assembleMetricWikiPage merges grounded fields with LLM prose.
func assembleMetricWikiPage(mctx metricWikiContext, prose metricWikiProse, modelName, lang string) metricWikiPage {
	m := mctx.Metric
	page := metricWikiPage{
		MetricID:       mctx.MetricID,
		Title:          metricDisplayTitle(m),
		Lead:           prose.Lead,
		Definition:     firstNonEmpty(prose.Definition, ptrStr(m.FormulaOrDefinition)),
		Background:     prose.Background,
		HowUsed:        prose.HowUsed,
		ChoosingValues: prose.ChoosingValues,
		RelatedMetrics: prose.RelatedMetrics,
		Infobox: metricWikiInfobox{
			Value:             ptrStr(m.MetricValue),
			Unit:              firstNonEmpty(ptrStr(m.MetricUnit), ptrStr(m.MetricUnitEn)),
			RangeType:         ptrStr(m.ValueRangeType),
			ThresholdOrTarget: ptrStr(m.ThresholdOrTarget),
			MeasurementFreq:   ptrStr(m.MeasurementFreq),
			Subject:           firstNonEmpty(ptrStr(m.MetricSubject), ptrStr(m.MetricSubjectEn)),
			Confidence:        m.Confidence,
		},
		InThisCorpus: metricWikiCorpus{
			SourceDocument: mctx.Document,
			SourceExcerpt:  firstNonEmpty(ptrStr(m.MetricContext), ptrStr(m.MetricDesc)),
		},
		Generated: metricWikiGenerated{
			Model:         modelName,
			Lang:          lang,
			SchemaVersion: metricWikiSchemaVersion,
			SourceHash:    metricWikiSourceHash(mctx),
		},
	}
	return page
}

// saveMetricWikiPage writes the page atomically (temp file + rename) so a
// concurrent reader never observes a partial file.
func saveMetricWikiPage(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wikipage-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// metricWikiDefaultPrompt is the built-in generation prompt. It is used unless
// WIKIPAGE_CREATION_PROMPT names a prompt file under prompts/.
const metricWikiDefaultPrompt = `You are writing an encyclopedia-style wiki page for a single metric extracted from a knowledge base.

You are given a JSON object describing everything the corpus knows about the metric (its name, value, unit, subject, description, context, and source document). The page's measured values and source facts are filled in elsewhere; your job is ONLY to write the explanatory prose.

Write in English. Ground every statement in the provided facts. You may add general background, typical usage, and guidance on choosing values from well-established domain knowledge, but DO NOT guess, invent specific numbers, or state anything you are not confident about. If you are unsure about a section, leave it as an empty string rather than guessing.

Return ONLY a single JSON object with these string fields (no markdown, no extra keys):
{
  "lead": "1-2 sentence summary of what this metric is, grounded in the facts.",
  "definition": "A precise definition or formula, if derivable from the facts; else empty.",
  "background": "General background on the concept (clearly general knowledge, not corpus-specific).",
  "how_used": "Where and when this metric is typically applied.",
  "choosing_values": "Guidance on selecting or interpreting values, if applicable; else empty.",
  "related_metrics": ["names of closely related metrics or concepts"]
}

"lead" must be non-empty. All other fields may be empty when the facts do not support them.`

// metricWikiPromptText returns the generation prompt: a file named by
// WIKIPAGE_CREATION_PROMPT under prompts/ when set and readable, else the
// built-in default.
func metricWikiPromptText(lang string) string {
	base := metricWikiDefaultPrompt
	if ref := strings.TrimSpace(os.Getenv("WIKIPAGE_CREATION_PROMPT")); ref != "" {
		if bs, err := os.ReadFile(filepath.Join("prompts", ref)); err == nil {
			if text := strings.TrimSpace(string(bs)); text != "" {
				base = text
			}
		}
	}
	return strings.TrimSpace(base) + "\n\n" + metricWikiOutputLanguageInstruction(lang)
}

func metricWikiOutputLanguageInstruction(lang string) string {
	switch strings.TrimSpace(strings.ToLower(lang)) {
	case "", "en":
		return "Output language requirement: write all human-readable prose fields in English."
	case "zh-cn":
		return "Output language requirement: write all human-readable prose fields in Simplified Chinese (zh-CN). Do not default to English."
	default:
		return fmt.Sprintf("Output language requirement: write all human-readable prose fields in %s.", lang)
	}
}

// metricWikiFactsJSON serializes the grounded facts the model is allowed to use.
func metricWikiFactsJSON(mctx metricWikiContext) (string, error) {
	data, err := json.MarshalIndent(mctx, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// loadWikiModelDef resolves a model definition from the .models.toml file named
// by the given env var. An unset env var returns an empty modelRef with no error
// so optional (fallback) models are skipped cleanly.
func loadWikiModelDef(modelRefEnv string) (string, ApiTypes.LLMModelDef, error) {
	modelRef := strings.TrimSpace(os.Getenv(modelRefEnv))
	if modelRef == "" {
		return "", ApiTypes.LLMModelDef{}, nil
	}
	modelPath, err := resolveModelsFilePathForKBHandler("MODEL_DEF_FILE")
	if err != nil {
		return modelRef, ApiTypes.LLMModelDef{}, err
	}
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRef, ApiTypes.LLMModelDef{}, fmt.Errorf("read %s failed: %w", modelPath, err)
	}
	parsed := ApiTypes.LLMModelsFile{}
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return modelRef, ApiTypes.LLMModelDef{}, fmt.Errorf("parse %s failed: %w", modelPath, err)
	}
	modelDef, ok := parsed[modelRef]
	if !ok {
		return modelRef, ApiTypes.LLMModelDef{}, fmt.Errorf("model %q not found in %s", modelRef, modelPath)
	}
	modelDef.ModelName = strings.TrimSpace(modelDef.ModelName)
	modelDef.APIKey = strings.TrimSpace(modelDef.APIKey)
	modelDef.BaseURL = strings.TrimSpace(modelDef.BaseURL)
	modelDef.ThinkingType = strings.TrimSpace(modelDef.ThinkingType)
	switch {
	case modelDef.ModelName == "":
		return modelRef, ApiTypes.LLMModelDef{}, fmt.Errorf("model %q missing model_name", modelRef)
	case modelDef.APIKey == "":
		return modelRef, ApiTypes.LLMModelDef{}, fmt.Errorf("model %q missing api_key", modelRef)
	case modelDef.BaseURL == "":
		return modelRef, ApiTypes.LLMModelDef{}, fmt.Errorf("model %q missing base_url", modelRef)
	case modelDef.TimeoutSec <= 0:
		return modelRef, ApiTypes.LLMModelDef{}, fmt.Errorf("model %q missing or invalid timeout_sec", modelRef)
	}
	return modelRef, modelDef, nil
}

// --- small helpers ---

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}


func stringSliceVal(m map[string]any, key string) []string {
	arr := anySlice(m, key)
	if len(arr) == 0 {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s := strings.TrimSpace(anyAsString(v)); s != "" {
			out = append(out, s)
		}
	}
	return out
}
