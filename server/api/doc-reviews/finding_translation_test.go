package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type fakeFindingTranslator struct {
	normalizeCalls int
	normalizeOut   FindingNormalization
	normalizeErr   error

	translateCalls []string
	translateOut   map[string]FindingLocalizedContent
	translateErr   error
}

func (f *fakeFindingTranslator) NormalizeFinding(ctx context.Context, canonicalLanguage string, finding FindingItem) (FindingNormalization, error) {
	f.normalizeCalls++
	return f.normalizeOut, f.normalizeErr
}

func (f *fakeFindingTranslator) TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingLocalizedContent, error) {
	f.translateCalls = append(f.translateCalls, language)
	if out, ok := f.translateOut[language]; ok {
		return out, f.translateErr
	}
	return FindingLocalizedContent{}, f.translateErr
}

type concurrencyFindingTranslator struct {
	normalizeOut FindingNormalization
	translateOut map[string]FindingLocalizedContent
	delay        time.Duration

	current int32
	maxSeen int32
}

func (f *concurrencyFindingTranslator) NormalizeFinding(ctx context.Context, canonicalLanguage string, finding FindingItem) (FindingNormalization, error) {
	return f.normalizeOut, nil
}

func (f *concurrencyFindingTranslator) TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingLocalizedContent, error) {
	cur := atomic.AddInt32(&f.current, 1)
	defer atomic.AddInt32(&f.current, -1)
	for {
		max := atomic.LoadInt32(&f.maxSeen)
		if cur <= max {
			break
		}
		if atomic.CompareAndSwapInt32(&f.maxSeen, max, cur) {
			break
		}
	}
	time.Sleep(f.delay)
	if out, ok := f.translateOut[language]; ok {
		return out, nil
	}
	return FindingLocalizedContent{}, nil
}

func TestTranslationFromMetadataReadsI18NEnvelope(t *testing.T) {
	raw := []byte(`{"i18n":{"schema_version":1,"translations":{"zh":{"title":"标题","description":"描述","suggestion":"建议","provenance":"original_extraction"}}}}`)
	tr, ok := translationFromMetadata(raw, "zh")
	if !ok {
		t.Fatal("translationFromMetadata ok=false, want true")
	}
	if tr.Title != "标题" || tr.Description != "描述" || tr.Suggestion != "建议" {
		t.Fatalf("translation=%#v", tr)
	}
}

func TestTranslationFromMetadataReadsLegacyShape(t *testing.T) {
	raw := []byte(`{"zh":{"title":"标题","description":"描述","suggestion":"建议"}}`)
	tr, ok := translationFromMetadata(raw, "zh")
	if !ok {
		t.Fatal("translationFromMetadata ok=false, want true")
	}
	if tr.Title != "标题" || tr.Description != "描述" || tr.Suggestion != "建议" {
		t.Fatalf("translation=%#v", tr)
	}
}

func TestFindingNormalizationFromMapAcceptsNestedCanonicalObject(t *testing.T) {
	got := findingNormalizationFromMap(map[string]any{
		"source_language":    "en",
		"canonical_language": "en",
		"canonical_origin":   "original",
		"canonical": map[string]any{
			"title":       "Missing spaces in standard references",
			"description": "In line 64, there are missing spaces between consecutive standard numbers and text.",
			"suggestion":  "Insert spaces between consecutive standard references.",
		},
	})
	if got.Canonical.Title != "Missing spaces in standard references" {
		t.Fatalf("canonical title=%q", got.Canonical.Title)
	}
	if got.Canonical.Description == "" || got.Canonical.Suggestion == "" {
		t.Fatalf("canonical=%#v", got.Canonical)
	}
}

func TestFindingNormalizationFromMapFallsBackToTopLevelProseFields(t *testing.T) {
	got := findingNormalizationFromMap(map[string]any{
		"source_language":    "en",
		"canonical_language": "en",
		"canonical_origin":   "original",
		"title":              "Missing spaces in standard references",
		"description":        "In line 64, there are missing spaces between consecutive standard numbers and text.",
		"suggestion":         "Insert spaces between consecutive standard references.",
	})
	if got.Canonical.Title != "Missing spaces in standard references" {
		t.Fatalf("canonical title=%q", got.Canonical.Title)
	}
	if got.Canonical.Description == "" || got.Canonical.Suggestion == "" {
		t.Fatalf("canonical=%#v", got.Canonical)
	}
}

func TestFindingNormalizationFromMapAcceptsNestedFindingObject(t *testing.T) {
	got := findingNormalizationFromMap(map[string]any{
		"source_language":    "zh",
		"canonical_language": "en",
		"canonical_origin":   "translated",
		"finding": map[string]any{
			"title":       "Non-standard term '冰冻'",
			"description": "The term 'frozen' is used for laboratory equipment where 'freezing' is the standard and more precise term.",
			"suggestion":  "冷冻干燥机, 冷冻切片机",
		},
	})
	if got.Canonical.Title != "Non-standard term '冰冻'" {
		t.Fatalf("canonical title=%q", got.Canonical.Title)
	}
	if got.Canonical.Description == "" || got.Canonical.Suggestion != "冷冻干燥机, 冷冻切片机" {
		t.Fatalf("canonical=%#v", got.Canonical)
	}
}

func TestPrepareFindingForStorageCanonicalizesEnglishAndPreservesChineseSource(t *testing.T) {
	translator := &fakeFindingTranslator{
		normalizeOut: FindingNormalization{
			SourceLanguage:           "zh",
			SourceLanguageConfidence: 0.99,
			CanonicalLanguage:        "en",
			CanonicalOrigin:          "translated",
			Canonical: FindingLocalizedContent{
				Title:       "Undefined acceptance criteria",
				Description: "The requirement states a condition but does not define acceptance criteria.",
				Suggestion:  "Add measurable acceptance criteria.",
			},
			SourceTranslation: FindingLocalizedContent{
				Title:       "未定义验收标准",
				Description: "该要求陈述了条件，但没有定义验收标准。",
				Suggestion:  "补充可衡量的验收标准。",
				Provenance:  "original_extraction",
			},
		},
	}

	finding := ReviewFinding{
		Pass:        "P1",
		Aspect:      "grammar_spelling",
		Severity:    "medium",
		FindingType: "grammar",
		Title:       "未定义验收标准",
		Description: "该要求陈述了条件，但没有定义验收标准。",
		Evidence:    "验收应充分。",
		Location:    "42",
		Suggestion:  "补充可衡量的验收标准。",
		Confidence:  0.97,
	}

	prepared, err := prepareFindingForStorage(context.Background(), translator, []string{"en", "zh"}, finding)
	if err != nil {
		t.Fatalf("prepareFindingForStorage: %v", err)
	}
	if translator.normalizeCalls != 1 {
		t.Fatalf("normalizeCalls=%d, want 1", translator.normalizeCalls)
	}
	if len(translator.translateCalls) != 0 {
		t.Fatalf("translateCalls=%v, want none because source zh should be reused", translator.translateCalls)
	}
	if prepared.Canonical.Title != "Undefined acceptance criteria" {
		t.Fatalf("canonical title=%q", prepared.Canonical.Title)
	}
	if prepared.Canonical.Evidence != "验收应充分。" {
		t.Fatalf("evidence=%q, want original evidence unchanged", prepared.Canonical.Evidence)
	}
	if prepared.Metadata.I18N.SourceLanguage != "zh" {
		t.Fatalf("source_language=%q", prepared.Metadata.I18N.SourceLanguage)
	}
	if got := prepared.Metadata.I18N.Translations["zh"].Title; got != "未定义验收标准" {
		t.Fatalf("zh title=%q", got)
	}
	if got := prepared.Metadata.I18N.Translations["en"].Title; got != "Undefined acceptance criteria" {
		t.Fatalf("en title=%q", got)
	}
}

func TestPrepareFindingForStorageAutoTranslatesConfiguredDisplayLanguages(t *testing.T) {
	translator := &fakeFindingTranslator{
		normalizeOut: FindingNormalization{
			SourceLanguage:           "en",
			SourceLanguageConfidence: 0.95,
			CanonicalLanguage:        "en",
			CanonicalOrigin:          "original",
			Canonical: FindingLocalizedContent{
				Title:       "Subject-verb disagreement",
				Description: "The singular subject uses a plural verb.",
				Suggestion:  "Change 'are' to 'is'.",
			},
		},
		translateOut: map[string]FindingLocalizedContent{
			"zh": {
				Title:       "主谓不一致",
				Description: "单数主语使用了复数谓语。",
				Suggestion:  "将“are”改为“is”。",
				Provenance:  "llm_translation",
			},
		},
	}

	prepared, err := prepareFindingForStorage(context.Background(), translator, []string{"en", "zh"}, ReviewFinding{
		FindingType: "grammar",
		Title:       "Subject-verb disagreement",
		Description: "The singular subject uses a plural verb.",
		Suggestion:  "Change 'are' to 'is'.",
		Evidence:    "The system are ready.",
	})
	if err != nil {
		t.Fatalf("prepareFindingForStorage: %v", err)
	}
	if len(translator.translateCalls) != 1 || translator.translateCalls[0] != "zh" {
		t.Fatalf("translateCalls=%v, want [zh]", translator.translateCalls)
	}
	if got := prepared.Metadata.I18N.Translations["zh"].Title; got != "主谓不一致" {
		t.Fatalf("zh title=%q", got)
	}
	if prepared.Canonical.Evidence != "The system are ready." {
		t.Fatalf("evidence=%q, want original evidence unchanged", prepared.Canonical.Evidence)
	}
}

func TestPrepareFindingForStorageTranslatesConfiguredLanguagesInParallel(t *testing.T) {
	t.Setenv("MAX_DOC_REVIEWER_TASKS", "2")
	translator := &concurrencyFindingTranslator{
		normalizeOut: FindingNormalization{
			SourceLanguage:           "en",
			SourceLanguageConfidence: 0.95,
			CanonicalLanguage:        "en",
			CanonicalOrigin:          "original",
			Canonical: FindingLocalizedContent{
				Title:       "Subject-verb disagreement",
				Description: "The singular subject uses a plural verb.",
				Suggestion:  "Change 'are' to 'is'.",
			},
		},
		translateOut: map[string]FindingLocalizedContent{
			"zh": {Title: "主谓不一致", Description: "单数主语使用了复数谓语。", Suggestion: "将“are”改为“is”。"},
			"ja": {Title: "主語と動詞が一致していません", Description: "単数主語に複数形の動詞が使われています。", Suggestion: "「are」を「is」に変更します。"},
			"fr": {Title: "Desaccord sujet-verbe", Description: "Le sujet singulier utilise un verbe pluriel.", Suggestion: "Remplacez « are » par « is »."},
		},
		delay: 40 * time.Millisecond,
	}

	prepared, err := prepareFindingForStorage(context.Background(), translator, []string{"en", "zh", "ja", "fr"}, ReviewFinding{
		FindingType: "grammar",
		Title:       "Subject-verb disagreement",
		Description: "The singular subject uses a plural verb.",
		Suggestion:  "Change 'are' to 'is'.",
	})
	if err != nil {
		t.Fatalf("prepareFindingForStorage: %v", err)
	}
	if prepared.Metadata.I18N.Translations["zh"].Title == "" || prepared.Metadata.I18N.Translations["ja"].Title == "" || prepared.Metadata.I18N.Translations["fr"].Title == "" {
		t.Fatalf("translations=%#v", prepared.Metadata.I18N.Translations)
	}
	if got := atomic.LoadInt32(&translator.maxSeen); got < 2 {
		t.Fatalf("max concurrent translations=%d, want at least 2", got)
	}
	if got := atomic.LoadInt32(&translator.maxSeen); got > 2 {
		t.Fatalf("max concurrent translations=%d, want at most 2", got)
	}
}

func TestLocalizeFindingUsesStoredTranslationWithoutLLM(t *testing.T) {
	ctrl := &DocReviewController{}
	body := []byte(`{"i18n":{"schema_version":1,"translations":{"zh":{"title":"缓存标题","description":"缓存描述","suggestion":"缓存建议","provenance":"llm_translation"}}}}`)

	f := FindingItem{ID: 7, Title: "Original", Description: "Original desc", Suggestion: "Original suggestion"}
	got, err := ctrl.localizeFinding(context.Background(), "zh", f, body)
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}
	if got.Title != "缓存标题" || got.Description != "缓存描述" || got.Suggestion != "缓存建议" {
		t.Fatalf("localized finding=%#v", got)
	}
}

func TestLocalizeFindingFallsBackToCanonicalEnglishWhenTranslationMissing(t *testing.T) {
	ctrl := &DocReviewController{}
	f := FindingItem{ID: 8, Title: "Canonical title", Description: "Canonical desc", Suggestion: "Canonical suggestion"}
	got, err := ctrl.localizeFinding(context.Background(), "zh", f, []byte(`{"i18n":{"schema_version":1,"translations":{"en":{"title":"Canonical title","description":"Canonical desc","suggestion":"Canonical suggestion","provenance":"canonical"}}}}`))
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}
	if got.Title != "Canonical title" || got.Description != "Canonical desc" || got.Suggestion != "Canonical suggestion" {
		t.Fatalf("localized finding=%#v", got)
	}
}

func TestSaveFindingsStoresCanonicalEnglishAndI18NMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	translator := &fakeFindingTranslator{
		normalizeOut: FindingNormalization{
			SourceLanguage:           "zh",
			SourceLanguageConfidence: 0.98,
			CanonicalLanguage:        "en",
			CanonicalOrigin:          "translated",
			Canonical: FindingLocalizedContent{
				Title:       "Undefined scope term",
				Description: "The term is used before it is defined.",
				Suggestion:  "Define the term when it first appears.",
			},
			SourceTranslation: FindingLocalizedContent{
				Title:       "范围术语未定义",
				Description: "该术语在定义前已被使用。",
				Suggestion:  "首次出现时定义该术语。",
				Provenance:  "original_extraction",
			},
		},
	}
	store := ReviewFindingsSQLStore{
		DB:         db,
		Translator: translator,
		Languages:  []string{"en", "zh"},
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.doc_review_findings
    (input_record_id, run_id, pass, aspect, severity, finding_type,
     title, description, evidence, location, suggestion, confidence, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`)).
		WithArgs(
			int64(100), int64(200), "P1", "grammar_spelling", "medium", "grammar",
			"Undefined scope term", "The term is used before it is defined.", "范围见下文", "44", "Define the term when it first appears.", 0.88, sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	_, err = store.SaveFindings(context.Background(), 100, 200, []ReviewFinding{{
		Pass:        "P1",
		Aspect:      "grammar_spelling",
		Severity:    "medium",
		FindingType: "grammar",
		Title:       "范围术语未定义",
		Description: "该术语在定义前已被使用。",
		Evidence:    "范围见下文",
		Location:    "44",
		Suggestion:  "首次出现时定义该术语。",
		Confidence:  0.88,
	}})
	if err != nil {
		t.Fatalf("SaveFindings: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestFindingMetadataJSONRoundTrip(t *testing.T) {
	meta := FindingMetadataEnvelope{
		I18N: FindingI18NMetadata{
			SchemaVersion:            1,
			SourceLanguage:           "zh",
			SourceLanguageConfidence: 0.99,
			CanonicalLanguage:        "en",
			CanonicalOrigin:          "translated",
			Translations: map[string]FindingLocalizedContent{
				"en": {Title: "English title", Description: "English desc", Suggestion: "English suggestion", Provenance: "canonical"},
				"zh": {Title: "中文标题", Description: "中文描述", Suggestion: "中文建议", Provenance: "original_extraction"},
			},
		},
	}

	body, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var roundTrip FindingMetadataEnvelope
	if err := json.Unmarshal(body, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if roundTrip.I18N.Translations["zh"].Title != "中文标题" {
		t.Fatalf("roundTrip zh title=%q", roundTrip.I18N.Translations["zh"].Title)
	}
}

func TestNewLLMFindingTranslatorLoadsPromptsFromPromptDir(t *testing.T) {
	t.Setenv("TRANSLATION_MODEL_NAME", "test-model")

	oldBuilder := docprocessingBuildReviewerLLMClient
	docprocessingBuildReviewerLLMClient = func(modelRef string) (LLMJSONExtractor, string, error) {
		return &fakeJSONExtractor{}, "test-model", nil
	}
	defer func() {
		docprocessingBuildReviewerLLMClient = oldBuilder
	}()

	promptDir := t.TempDir()
	t.Setenv("PROMPT_DIR", promptDir)
	for _, name := range []string{
		"prompt-doc-review-finding-normalize-v1.md",
		"prompt-doc-review-finding-normalize-retry-v1.md",
		"prompt-doc-review-finding-localize-v1.md",
		"prompt-doc-review-finding-localize-retry-v1.md",
	} {
		if err := os.WriteFile(filepath.Join(promptDir, name), []byte("test prompt"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}

	got, err := newLLMFindingTranslator()
	if err != nil {
		t.Fatalf("newLLMFindingTranslator: %v", err)
	}
	if got == nil {
		t.Fatal("newLLMFindingTranslator returned nil translator")
	}
}

func TestNormalizeFindingErrorIncludesOffendingFieldContent(t *testing.T) {
	translator := &llmFindingTranslator{
		client: &fakeJSONExtractor{out: map[string]any{
			"source_language":       "zh",
			"canonical_language":    "en",
			"canonical_origin":      "translated",
			"canonical_title":       "中文标题",
			"canonical_description": "仍然是中文描述",
			"canonical_suggestion":  "仍然是中文建议",
			"source_translation": map[string]any{
				"title":       "原始中文标题",
				"description": "原始中文描述",
				"suggestion":  "原始中文建议",
				"provenance":  "original_extraction",
			},
		}},
		modelName:                "test-model",
		normalizePromptText:      "test prompt",
		normalizeRetryPromptText: "retry prompt",
	}

	_, err := translator.NormalizeFinding(context.Background(), "en", FindingItem{
		Title:       "原始标题",
		Description: "原始描述",
		Suggestion:  "原始建议",
	})
	if err == nil {
		t.Fatal("NormalizeFinding error=nil, want error")
	}
	if errors.Is(err, errFindingTranslationUnavailable) {
		t.Fatalf("unexpected availability error: %v", err)
	}
	for _, want := range []string{
		`source_title="原始标题"`,
		`source_description="原始描述"`,
		`source_suggestion="原始建议"`,
		`canonical_title="中文标题"`,
		`canonical_description="仍然是中文描述"`,
		`canonical_suggestion="仍然是中文建议"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("NormalizeFinding error %q does not contain %q", err.Error(), want)
		}
	}
}

func TestNormalizationInCanonicalLanguage_AllowsEnglishWithQuotedChineseExample(t *testing.T) {
	n := FindingNormalization{
		SourceLanguage:    "en",
		CanonicalLanguage: "en",
		Canonical: FindingLocalizedContent{
			Title:       "Missing line breaks and spaces between standard references",
			Description: "Multiple GB/T standard references are concatenated without proper spacing or line breaks, making it difficult to read and identify individual standards.",
			Suggestion:  "Split into separate lines with proper spacing, e.g., 'GB/T2423.24 环境试验 第2部分：试验方法 试验Sa：模拟地面上的太阳辐射及其试验导则\\nGB/T2423.25 人造气氛腐蚀试验 盐雾试验'.",
		},
	}
	if !normalizationInCanonicalLanguage(n, "en") {
		t.Fatal("normalizationInCanonicalLanguage=false, want true for English prose with quoted Chinese example content")
	}
}

func TestNormalizationInCanonicalLanguage_AllowsChineseLiteralSuggestionWhenTitleAndDescriptionAreEnglish(t *testing.T) {
	n := FindingNormalization{
		SourceLanguage:    "zh",
		CanonicalLanguage: "en",
		Canonical: FindingLocalizedContent{
			Title:       "Non-standard term '冰冻'",
			Description: "The term 'frozen' is used for laboratory equipment where 'freezing' is the standard and more precise term.",
			Suggestion:  "冷冻干燥机, 冷冻切片机",
		},
	}
	if !normalizationInCanonicalLanguage(n, "en") {
		t.Fatal("normalizationInCanonicalLanguage=false, want true when English explanation accompanies a source-language literal fix suggestion")
	}
}

func TestNormalizationInCanonicalLanguage_RejectsChineseWithOnlyShortLatinTokens(t *testing.T) {
	n := FindingNormalization{
		SourceLanguage:    "zh",
		CanonicalLanguage: "en",
		Canonical: FindingLocalizedContent{
			Title:       "GB/T 标准未分行",
			Description: "多个 GB/T 标准连在一起。",
			Suggestion:  "按 GB/T2423.24 和 GB/T2423.25 分行。",
		},
	}
	if normalizationInCanonicalLanguage(n, "en") {
		t.Fatal("normalizationInCanonicalLanguage=true, want false for non-English prose with only short Latin tokens")
	}
}
