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
