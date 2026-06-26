package docreviews

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type fakeFindingTranslator struct {
	calls int
	out   FindingTranslation
	seq   []FindingTranslation
	err   error
}

func (f *fakeFindingTranslator) TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingTranslation, error) {
	f.calls++
	if len(f.seq) > 0 {
		next := f.seq[0]
		f.seq = f.seq[1:]
		return next, f.err
	}
	return f.out, f.err
}

func TestTranslationFromMetadata(t *testing.T) {
	raw := []byte(`{"zh":{"finding_type":"类型","title":"标题","description":"描述","suggestion":"建议"}}`)
	tr, ok := translationFromMetadata(raw, "zh")
	if !ok {
		t.Fatal("translationFromMetadata ok=false, want true")
	}
	if tr.Title != "标题" || tr.Suggestion != "建议" {
		t.Fatalf("translation=%#v", tr)
	}
}

func TestLocalizeFindingTranslatesAndSavesMissingMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	translator := &fakeFindingTranslator{out: FindingTranslation{
		FindingType: "技术准确性",
		Title:       "范围符号错误",
		Description: "描述中文",
		Suggestion:  "建议中文",
	}}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.doc_review_findings
		SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), ARRAY[$2], $3::jsonb, true)
		WHERE id = $1`)).
		WithArgs(int64(42), "zh", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	f := FindingItem{ID: 42, FindingType: "technical_accuracy", Title: "Bad range", Description: "English", Suggestion: "Fix it"}
	got, err := ctrl.localizeFinding(context.Background(), "zh", f, []byte(`{}`))
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}

	if translator.calls != 1 {
		t.Fatalf("translator calls=%d, want 1", translator.calls)
	}
	if got.Title != "范围符号错误" || got.Description != "描述中文" {
		t.Fatalf("localized finding=%#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestLocalizeFindingUsesCachedMetadata(t *testing.T) {
	translator := &fakeFindingTranslator{}
	ctrl := &DocReviewController{Translator: translator}
	body, _ := json.Marshal(map[string]FindingTranslation{
		"zh": {Title: "缓存标题", Description: "缓存描述"},
	})

	f := FindingItem{ID: 7, Title: "Original", Description: "Original desc"}
	got, err := ctrl.localizeFinding(context.Background(), "zh", f, body)
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}

	if translator.calls != 0 {
		t.Fatalf("translator calls=%d, want 0", translator.calls)
	}
	if got.Title != "缓存标题" || got.Description != "缓存描述" {
		t.Fatalf("localized finding=%#v", got)
	}
}

func TestLocalizeFindingRetranslatesCachedUntranslatedMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	translator := &fakeFindingTranslator{out: FindingTranslation{
		Title:       "中文标题",
		Description: "中文描述",
		Suggestion:  "中文建议",
	}}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.doc_review_findings
		SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), ARRAY[$2], $3::jsonb, true)
		WHERE id = $1`)).
		WithArgs(int64(9), "zh", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	f := FindingItem{ID: 9, Title: "English title", Description: "English desc", Suggestion: "English suggestion"}
	body, _ := json.Marshal(map[string]FindingTranslation{
		"zh": {
			Title:       "English title",
			Description: "English desc",
			Suggestion:  "English suggestion",
		},
	})

	got, err := ctrl.localizeFinding(context.Background(), "zh", f, body)
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}
	if translator.calls != 1 {
		t.Fatalf("translator calls=%d, want 1", translator.calls)
	}
	if got.Title != "中文标题" || got.Description != "中文描述" || got.Suggestion != "中文建议" {
		t.Fatalf("localized finding=%#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestLocalizeFindingErrorsWhenTranslatorReturnsUntranslatedContent(t *testing.T) {
	translator := &fakeFindingTranslator{out: FindingTranslation{
		Title:       "English title",
		Description: "English desc",
		Suggestion:  "English suggestion",
	}}
	ctrl := &DocReviewController{Translator: translator}
	f := FindingItem{ID: 10, Title: "English title", Description: "English desc", Suggestion: "English suggestion"}

	_, err := ctrl.localizeFinding(context.Background(), "zh", f, []byte(`{}`))
	if err == nil {
		t.Fatal("localizeFinding error=nil, want error")
	}
}

func TestLocalizeFindingRetriesWhenFirstTranslationIsUntranslated(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	llmTranslator := &llmFindingTranslator{
		client: &fakeJSONExtractor{seq: []map[string]any{
			{
				"title":       "English title",
				"description": "English desc",
				"suggestion":  "English suggestion",
			},
			{
				"title":       "中文标题",
				"description": "中文描述",
				"suggestion":  "中文建议",
			},
		}},
		modelName: "test-model",
	}
	ctrl := &DocReviewController{DB: db, Translator: llmTranslator}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.doc_review_findings
		SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), ARRAY[$2], $3::jsonb, true)
		WHERE id = $1`)).
		WithArgs(int64(11), "zh", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	f := FindingItem{ID: 11, Title: "English title", Description: "English desc", Suggestion: "English suggestion"}
	got, err := ctrl.localizeFinding(context.Background(), "zh", f, []byte(`{}`))
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}
	if got.Title != "中文标题" || got.Description != "中文描述" || got.Suggestion != "中文建议" {
		t.Fatalf("localized finding=%#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestLocalizeFindingsErrorsWhenTranslationModelMissing(t *testing.T) {
	t.Setenv("TRANSLATION_MODEL_NAME", "")

	ctrl := &DocReviewController{}
	findings := []FindingItem{{ID: 42, Title: "Bad range", Description: "English"}}

	_, err := ctrl.localizeFindings(context.Background(), "zh", findings, map[int64][]byte{42: []byte(`{}`)})
	if err == nil {
		t.Fatal("localizeFindings error=nil, want error")
	}
	if !errors.Is(err, errFindingTranslationUnavailable) {
		t.Fatalf("errors.Is(err, errFindingTranslationUnavailable)=false; err=%v", err)
	}
}

func TestContainsChineseChars(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"你好世界", true},
		{"hello world", false},
		{"mixed 中文 text", true},
		{"", false},
		{"12345", false},
		{"abc123", false},
		{"。？！", false}, // CJK punctuation, not Han
	}
	for _, tc := range cases {
		got := containsChineseChars(tc.input)
		if got != tc.want {
			t.Errorf("containsChineseChars(%q)=%v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestFindingInTargetLanguage_zh(t *testing.T) {
	cases := []struct {
		name    string
		finding FindingItem
		lang    string
		want    bool
	}{
		{
			name:    "all Chinese prose fields",
			finding: FindingItem{Title: "中文标题", Description: "中文描述", Suggestion: "中文建议"},
			lang:    "zh",
			want:    true,
		},
		{
			name:    "English prose fields",
			finding: FindingItem{Title: "English title", Description: "English desc", Suggestion: "English suggestion"},
			lang:    "zh",
			want:    false,
		},
		{
			name:    "mixed — Chinese title only, description English",
			finding: FindingItem{Title: "中文", Description: "English desc", Suggestion: "English suggestion"},
			lang:    "zh",
			want:    false, // not ALL three prose fields
		},
		{
			name:    "empty fields",
			finding: FindingItem{},
			lang:    "zh",
			want:    false,
		},
		{
			name:    "English with en language (no-op)",
			finding: FindingItem{Title: "Hello", Description: "World"},
			lang:    "en",
			want:    false,
		},
		{
			name:    "finding_type alone in Chinese is not enough",
			finding: FindingItem{FindingType: "中文类型", Title: "English", Description: "English", Suggestion: "English"},
			lang:    "zh",
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findingInTargetLanguage(tc.finding, tc.lang)
			if got != tc.want {
				t.Errorf("findingInTargetLanguage=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestLocalizeFindingSkipsWhenAlreadyInTargetLanguage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	translator := &fakeFindingTranslator{}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	// Expect one DB save of the self-translation.
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.doc_review_findings
		SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), ARRAY[$2], $3::jsonb, true)
		WHERE id = $1`)).
		WithArgs(int64(1), "zh", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	f := FindingItem{
		ID: 1, FindingType: "technical_error",
		Title: "中文标题", Description: "中文描述", Suggestion: "中文建议",
	}
	got, err := ctrl.localizeFinding(context.Background(), "zh", f, []byte(`{}`))
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}
	if translator.calls != 0 {
		t.Fatalf("translator.calls=%d, want 0 (LLM should not be called)", translator.calls)
	}
	if got.Title != "中文标题" || got.Description != "中文描述" {
		t.Fatalf("localized finding=%#v, want original Chinese fields", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCachedChineseTranslationIsNotFlaggedAsUntranslated(t *testing.T) {
	translator := &fakeFindingTranslator{}
	ctrl := &DocReviewController{Translator: translator}

	// Chinese finding with a cached self-translation (identical to source).
	body := `{"zh":{"finding_type":"technical_error","title":"中文标题","description":"中文描述","suggestion":"中文建议"}}`
	f := FindingItem{
		ID: 2, FindingType: "technical_error",
		Title: "中文标题", Description: "中文描述", Suggestion: "中文建议",
	}

	got, err := ctrl.localizeFinding(context.Background(), "zh", f, []byte(body))
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}
	if translator.calls != 0 {
		t.Fatalf("translator calls=%d, want 0 (cache hit)", translator.calls)
	}
	if got.Title != "中文标题" {
		t.Fatalf("got.Title=%q, want 中文标题", got.Title)
	}
}

func TestTranslationInTargetLanguage_zh(t *testing.T) {
	cases := []struct {
		name string
		tr   FindingTranslation
		lang string
		want bool
	}{
		{"all Chinese fields", FindingTranslation{Title: "中文", Description: "中文描述", Suggestion: "中文建议"}, "zh", true},
		{"all English fields", FindingTranslation{Title: "English", Description: "English desc", Suggestion: "English suggestion"}, "zh", false},
		{"some Chinese fields but missing in title and suggestion", FindingTranslation{Title: "English", Description: "包含中文的描述", Suggestion: "English suggestion"}, "zh", false},
		{"empty fields", FindingTranslation{}, "zh", false},
		{"Chinese with en language", FindingTranslation{Title: "中文", Description: "中文"}, "en", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translationInTargetLanguage(tc.tr, tc.lang)
			if got != tc.want {
				t.Errorf("translationInTargetLanguage=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestLikelyUntranslatedForLanguage_DetectsStaleEnglishCache(t *testing.T) {
	// Source has mixed English+Chinese phrases. Cached "zh" entry is in
	// English and differs from source — should be flagged as untranslated.
	src := FindingItem{
		Title:       "Assumes boundaries are self-evident",
		Description: "The system relies on '预期应用' but does not define a method.",
		Suggestion:  "Add criteria for boundary cases.",
	}
	cached := FindingTranslation{
		Title:       "Assumes boundaries not self-evident",
		Description: "The classification system relies on expected application.",
		Suggestion:  "Define boundary criteria.",
	}
	if !likelyUntranslatedForLanguage(src, cached, "zh") {
		t.Error("likelyUntranslatedForLanguage=false, want true (stale English cache not detected)")
	}
}

func TestLikelyUntranslatedForLanguage_ValidChineseCacheNotFlagged(t *testing.T) {
	src := FindingItem{
		Title:       "English title",
		Description: "English description",
		Suggestion:  "English suggestion",
	}
	cached := FindingTranslation{
		Title:       "中文标题",
		Description: "中文描述",
		Suggestion:  "中文建议",
	}
	if likelyUntranslatedForLanguage(src, cached, "zh") {
		t.Error("likelyUntranslatedForLanguage=true, want false (valid Chinese cache should not be flagged)")
	}
}

func TestLocalizeFindingRetranslatesStaleEnglishCacheOnMixedFinding(t *testing.T) {
	// Simulates finding 1624: English title + Chinese phrases in description,
	// with a cached zh entry that is in English (from a failed LLM attempt).
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	translator := &fakeFindingTranslator{out: FindingTranslation{
		Title:       "假设边界是自明的",
		Description: "分类系统依赖'预期应用'但未定义方法。",
		Suggestion:  "为边界情况添加判定标准。",
	}}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.doc_review_findings
		SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), ARRAY[$2], $3::jsonb, true)
		WHERE id = $1`)).
		WithArgs(int64(1624), "zh", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Cached "zh" entry with English content that differs from source.
	cachedBody := `{"zh":{"finding_type":"undocumented_assumption","title":"Assumes boundaries not self-evident","description":"The classification system relies on expected application.","suggestion":"Define boundary criteria."}}`

	f := FindingItem{
		ID: 1624, FindingType: "undocumented_assumption",
		Title:       "Assumes boundaries are self-evident",
		Description: "The system relies on '预期应用' but does not define a method.",
		Suggestion:  "Add criteria for boundary cases.",
	}

	got, err := ctrl.localizeFinding(context.Background(), "zh", f, []byte(cachedBody))
	if err != nil {
		t.Fatalf("localizeFinding: %v", err)
	}
	if translator.calls != 1 {
		t.Fatalf("translator calls=%d, want 1 (should retranslate stale English cache)", translator.calls)
	}
	if got.Title != "假设边界是自明的" {
		t.Fatalf("got.Title=%q, want 假设边界是自明的", got.Title)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
