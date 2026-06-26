package docreviews

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type fakeFindingTranslator struct {
	calls int
	out   FindingTranslation
}

func (f *fakeFindingTranslator) TranslateFinding(ctx context.Context, language string, finding FindingItem) (FindingTranslation, error) {
	f.calls++
	return f.out, nil
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
	got := ctrl.localizeFinding(context.Background(), "zh", f, []byte(`{}`))

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
	got := ctrl.localizeFinding(context.Background(), "zh", f, body)

	if translator.calls != 0 {
		t.Fatalf("translator calls=%d, want 0", translator.calls)
	}
	if got.Title != "缓存标题" || got.Description != "缓存描述" {
		t.Fatalf("localized finding=%#v", got)
	}
}
