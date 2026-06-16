package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newArtifactWikiContext(t *testing.T, artifactType, artifactID, lang string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/artifacts/wiki?artifact_type=" + artifactType + "&artifact_id=" + artifactID
	if lang != "" {
		target += "&lang=" + lang
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestGetArtifactWikiMetricCacheHit(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)

	recordDir := filepath.Join(artifactDir, "0", "5")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	articleJSON := `{"title":"Switching Frequency","lead":"A generated article."}`
	if err := os.WriteFile(filepath.Join(recordDir, "wikipage_metric_5_mtc_3.en.json"), []byte(articleJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := getArtifactWikiMetricPayloadFn
	getArtifactWikiMetricPayloadFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiMetricPayload, error) {
		return artifactWikiMetricPayload{
			RecordID:       5,
			ArtifactID:     "5_mtc_3",
			Record:         json.RawMessage(`{"metric_id":"5_mtc_3","metric_name":"Switching Frequency"}`),
			SourceDocument: json.RawMessage(`{"record_id":5,"title":"Std 20039","file_name":"std.txt","type":"txt"}`),
			Generated: artifactWikiGeneratedMeta{
				Model:         "cached-model",
				Lang:          "en",
				SchemaVersion: 1,
				SourceHash:    "sha256:test",
			},
		}, nil
	}
	defer func() { getArtifactWikiMetricPayloadFn = orig }()

	c, rec := newArtifactWikiContext(t, "metric", "5_mtc_3", "")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Status || resp.Fresh {
		t.Fatalf("status=%v fresh=%v, want true/false", resp.Status, resp.Fresh)
	}
	if resp.ArtifactType != "metric" || resp.ArtifactID != "5_mtc_3" {
		t.Fatalf("artifact identity = %s/%s", resp.ArtifactType, resp.ArtifactID)
	}
	if string(resp.Article) != articleJSON {
		t.Fatalf("article = %s, want %s", resp.Article, articleJSON)
	}
	if string(resp.Record) != `{"metric_id":"5_mtc_3","metric_name":"Switching Frequency"}` {
		t.Fatalf("record = %s", resp.Record)
	}
	if resp.Generated.Model != "cached-model" {
		t.Fatalf("generated.model = %q", resp.Generated.Model)
	}
}

func TestGetArtifactWikiMetricCacheMissGenerates(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	if err := os.MkdirAll(filepath.Join(artifactDir, "0", "5"), 0o755); err != nil {
		t.Fatal(err)
	}

	origPayload := getArtifactWikiMetricPayloadFn
	getArtifactWikiMetricPayloadFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiMetricPayload, error) {
		return artifactWikiMetricPayload{
			RecordID:   5,
			ArtifactID: "5_mtc_3",
			Record:     json.RawMessage(`{"metric_id":"5_mtc_3","metric_name":"Switching Frequency"}`),
			Generated: artifactWikiGeneratedMeta{
				Model:         "test-model",
				Lang:          "en",
				SchemaVersion: 1,
				SourceHash:    "sha256:test",
			},
		}, nil
	}
	defer func() { getArtifactWikiMetricPayloadFn = origPayload }()

	generatedArticle := json.RawMessage(`{"title":"Generated","lead":"Fresh article."}`)
	origArticle := buildArtifactWikiMetricArticleFn
	buildArtifactWikiMetricArticleFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (json.RawMessage, error) {
		return generatedArticle, nil
	}
	defer func() { buildArtifactWikiMetricArticleFn = origArticle }()

	c, rec := newArtifactWikiContext(t, "metric", "5_mtc_3", "")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Fresh {
		t.Fatalf("fresh = %v, want true", resp.Fresh)
	}
	if string(resp.Article) != string(generatedArticle) {
		t.Fatalf("article = %s, want %s", resp.Article, generatedArticle)
	}
	saved, err := os.ReadFile(filepath.Join(artifactDir, "0", "5", "wikipage_metric_5_mtc_3.en.json"))
	if err != nil {
		t.Fatalf("generated article not saved: %v", err)
	}
	if string(saved) != string(generatedArticle) {
		t.Fatalf("saved article = %s, want %s", saved, generatedArticle)
	}
}

func TestGetArtifactWikiRequiresArtifactType(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	c, rec := newArtifactWikiContext(t, "", "5_mtc_3", "")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetArtifactWikiRequiresArtifactID(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	c, rec := newArtifactWikiContext(t, "metric", "", "")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetArtifactWikiRejectsUnsupportedArtifactType(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	c, rec := newArtifactWikiContext(t, "mystery", "5_x_1", "")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetArtifactWikiCanReturnRecordWithoutArticle(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())

	orig := getArtifactWikiMetricPayloadFn
	getArtifactWikiMetricPayloadFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiMetricPayload, error) {
		return artifactWikiMetricPayload{
			RecordID:   5,
			ArtifactID: "5_mtc_3",
			Record:     json.RawMessage(`{"metric_id":"5_mtc_3","metric_name":"Switching Frequency"}`),
			Generated: artifactWikiGeneratedMeta{
				Model:         "test-model",
				Lang:          "en",
				SchemaVersion: 1,
				SourceHash:    "sha256:test",
			},
		}, nil
	}
	defer func() { getArtifactWikiMetricPayloadFn = orig }()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/artifacts/wiki?artifact_type=metric&artifact_id=5_mtc_3&include_article=0", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Article) != 0 {
		t.Fatalf("article = %s, want empty", resp.Article)
	}
	if string(resp.Record) != `{"metric_id":"5_mtc_3","metric_name":"Switching Frequency"}` {
		t.Fatalf("record = %s", resp.Record)
	}
}

func TestGetArtifactWikiTranslatesFromEnglishCacheWhenTargetLanguageMissing(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	t.Setenv("TRANSLATION_MODEL_NAME", "configured-translation-model")

	recordDir := filepath.Join(artifactDir, "0", "5")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	englishArticle := json.RawMessage(`{"title":"Switching Frequency","lead":"English article."}`)
	if err := os.WriteFile(filepath.Join(recordDir, "wikipage_metric_5_mtc_3.en.json"), englishArticle, 0o644); err != nil {
		t.Fatal(err)
	}

	origPayload := getArtifactWikiMetricPayloadFn
	getArtifactWikiMetricPayloadFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiMetricPayload, error) {
		return artifactWikiMetricPayload{
			RecordID:   5,
			ArtifactID: "5_mtc_3",
			Record:     json.RawMessage(`{"metric_id":"5_mtc_3"}`),
			Generated: artifactWikiGeneratedMeta{
				Model:         "test-model",
				Lang:          "zh-cn",
				SchemaVersion: 1,
				SourceHash:    "sha256:test",
			},
		}, nil
	}
	defer func() { getArtifactWikiMetricPayloadFn = origPayload }()

	translatedArticle := json.RawMessage(`{"title":"切换频率","lead":"中文页面。"}`)
	origTranslate := translateArtifactWikiMetricArticleFn
	translateArtifactWikiMetricArticleFn = func(ctx context.Context, logger ApiTypes.JimoLogger, article json.RawMessage, targetLang string) (json.RawMessage, error) {
		if string(article) != string(englishArticle) {
			t.Fatalf("translate source article = %s, want %s", article, englishArticle)
		}
		if targetLang != "zh-cn" {
			t.Fatalf("targetLang = %q, want zh-cn", targetLang)
		}
		return translatedArticle, nil
	}
	defer func() { translateArtifactWikiMetricArticleFn = origTranslate }()

	c, rec := newArtifactWikiContext(t, "metric", "5_mtc_3", "zh-cn")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.Article) != string(translatedArticle) {
		t.Fatalf("article = %s, want %s", resp.Article, translatedArticle)
	}
	saved, err := os.ReadFile(filepath.Join(recordDir, "wikipage_metric_5_mtc_3.zh-cn.json"))
	if err != nil {
		t.Fatalf("translated article not saved: %v", err)
	}
	if string(saved) != string(translatedArticle) {
		t.Fatalf("saved article = %s, want %s", saved, translatedArticle)
	}
}

func TestGetArtifactWikiGeneratesTargetLanguageDirectlyWhenTranslationModelMissing(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	t.Setenv("TRANSLATION_MODEL_NAME", "")

	recordDir := filepath.Join(artifactDir, "0", "5")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	englishArticle := json.RawMessage(`{"title":"Switching Frequency","lead":"English article."}`)
	if err := os.WriteFile(filepath.Join(recordDir, "wikipage_metric_5_mtc_3.en.json"), englishArticle, 0o644); err != nil {
		t.Fatal(err)
	}

	origPayload := getArtifactWikiMetricPayloadFn
	getArtifactWikiMetricPayloadFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiMetricPayload, error) {
		return artifactWikiMetricPayload{
			RecordID:   5,
			ArtifactID: "5_mtc_3",
			Record:     json.RawMessage(`{"metric_id":"5_mtc_3"}`),
			Generated: artifactWikiGeneratedMeta{
				Model:         "test-model",
				Lang:          "zh-cn",
				SchemaVersion: 1,
				SourceHash:    "sha256:test",
			},
		}, nil
	}
	defer func() { getArtifactWikiMetricPayloadFn = origPayload }()

	generatedChineseArticle := json.RawMessage(`{"title":"切换频率","lead":"中文页面。","generated":{"lang":"zh-cn"}}`)
	origArticle := buildArtifactWikiMetricArticleFn
	buildArtifactWikiMetricArticleFn = func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (json.RawMessage, error) {
		if lang != "zh-cn" {
			t.Fatalf("lang = %q, want zh-cn", lang)
		}
		return generatedChineseArticle, nil
	}
	defer func() { buildArtifactWikiMetricArticleFn = origArticle }()

	origTranslate := translateArtifactWikiMetricArticleFn
	translateArtifactWikiMetricArticleFn = func(ctx context.Context, logger ApiTypes.JimoLogger, article json.RawMessage, targetLang string) (json.RawMessage, error) {
		t.Fatalf("translate should not be called when translation model is missing")
		return nil, nil
	}
	defer func() { translateArtifactWikiMetricArticleFn = origTranslate }()

	c, rec := newArtifactWikiContext(t, "metric", "5_mtc_3", "zh-cn")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.Article) != string(generatedChineseArticle) {
		t.Fatalf("article = %s, want %s", resp.Article, generatedChineseArticle)
	}
	saved, err := os.ReadFile(filepath.Join(recordDir, "wikipage_metric_5_mtc_3.zh-cn.json"))
	if err != nil {
		t.Fatalf("directly generated article not saved: %v", err)
	}
	if string(saved) != string(generatedChineseArticle) {
		t.Fatalf("saved article = %s, want %s", saved, generatedChineseArticle)
	}
}

func TestGetArtifactWikiSupportsNonMetricProvider(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())

	origProvider := artifactWikiProviders["summary"]
	artifactWikiProviders["summary"] = artifactWikiProvider{
		loadPayload: func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
			return artifactWikiPayload{
				RecordID:       7,
				ArtifactID:     artifactID,
				Record:         json.RawMessage(`{"summary_id":"7_sum_1","summary_text":"Energy summary"}`),
				SourceDocument: json.RawMessage(`{"record_id":7,"title":"doc.pdf","file_name":"doc.pdf","type":"pdf"}`),
				Generated: artifactWikiGeneratedMeta{
					Lang:       "en",
					SourceHash: "sha256:test",
				},
			}, nil
		},
	}
	defer func() { artifactWikiProviders["summary"] = origProvider }()

	origBuild := buildGenericArtifactWikiArticleFn
	buildGenericArtifactWikiArticleFn = func(ctx context.Context, logger ApiTypes.JimoLogger, payload artifactWikiPayload, artifactType, lang string) (json.RawMessage, error) {
		return json.RawMessage(`{"title":"Energy summary","lead":"A summary page."}`), nil
	}
	defer func() { buildGenericArtifactWikiArticleFn = origBuild }()

	c, rec := newArtifactWikiContext(t, "summary", "7_sum_1", "en")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ArtifactType != "summary" || resp.ArtifactID != "7_sum_1" {
		t.Fatalf("artifact identity = %s/%s", resp.ArtifactType, resp.ArtifactID)
	}
	if string(resp.Record) != `{"summary_id":"7_sum_1","summary_text":"Energy summary"}` {
		t.Fatalf("record = %s", resp.Record)
	}
}

func TestGetArtifactWikiNonMetricCacheMissGenerates(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	if err := os.MkdirAll(filepath.Join(artifactDir, "0", "7"), 0o755); err != nil {
		t.Fatal(err)
	}

	origProvider := artifactWikiProviders["summary"]
	artifactWikiProviders["summary"] = artifactWikiProvider{
		loadPayload: func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
			return artifactWikiPayload{
				RecordID:       7,
				ArtifactID:     artifactID,
				Record:         json.RawMessage(`{"summary_id":"7_sum_1","summary_text":"Energy summary"}`),
				SourceDocument: json.RawMessage(`{"record_id":7,"title":"doc.pdf","file_name":"doc.pdf","type":"pdf"}`),
				Generated: artifactWikiGeneratedMeta{
					Lang:       "en",
					SourceHash: "sha256:test",
				},
			}, nil
		},
	}
	defer func() { artifactWikiProviders["summary"] = origProvider }()

	origBuild := buildGenericArtifactWikiArticleFn
	buildGenericArtifactWikiArticleFn = func(ctx context.Context, logger ApiTypes.JimoLogger, payload artifactWikiPayload, artifactType, lang string) (json.RawMessage, error) {
		if artifactType != "summary" {
			t.Fatalf("artifactType = %q, want summary", artifactType)
		}
		if lang != "en" {
			t.Fatalf("lang = %q, want en", lang)
		}
		return json.RawMessage(`{"title":"Energy summary","lead":"A generated summary page."}`), nil
	}
	defer func() { buildGenericArtifactWikiArticleFn = origBuild }()

	c, rec := newArtifactWikiContext(t, "summary", "7_sum_1", "en")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Fresh {
		t.Fatalf("fresh = %v, want true", resp.Fresh)
	}
	if string(resp.Article) != `{"title":"Energy summary","lead":"A generated summary page."}` {
		t.Fatalf("article = %s", resp.Article)
	}
	saved, err := os.ReadFile(filepath.Join(artifactDir, "0", "7", "wikipage_summary_7_sum_1.en.json"))
	if err != nil {
		t.Fatalf("generated article not saved: %v", err)
	}
	if string(saved) != `{"title":"Energy summary","lead":"A generated summary page."}` {
		t.Fatalf("saved article = %s", saved)
	}
}

func TestGetArtifactWikiNonMetricTranslatesFromEnglishCache(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	t.Setenv("TRANSLATION_MODEL_NAME", "configured-translation-model")

	recordDir := filepath.Join(artifactDir, "0", "7")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	englishArticle := json.RawMessage(`{"title":"Energy summary","lead":"English page."}`)
	if err := os.WriteFile(filepath.Join(recordDir, "wikipage_summary_7_sum_1.en.json"), englishArticle, 0o644); err != nil {
		t.Fatal(err)
	}

	origProvider := artifactWikiProviders["summary"]
	artifactWikiProviders["summary"] = artifactWikiProvider{
		loadPayload: func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
			return artifactWikiPayload{
				RecordID:       7,
				ArtifactID:     artifactID,
				Record:         json.RawMessage(`{"summary_id":"7_sum_1","summary_text":"Energy summary"}`),
				SourceDocument: json.RawMessage(`{"record_id":7,"title":"doc.pdf","file_name":"doc.pdf","type":"pdf"}`),
				Generated: artifactWikiGeneratedMeta{
					Lang:       "zh-cn",
					SourceHash: "sha256:test",
				},
			}, nil
		},
	}
	defer func() { artifactWikiProviders["summary"] = origProvider }()

	origTranslate := translateGenericArtifactWikiArticleFn
	translateGenericArtifactWikiArticleFn = func(ctx context.Context, logger ApiTypes.JimoLogger, article json.RawMessage, targetLang string) (json.RawMessage, error) {
		if string(article) != string(englishArticle) {
			t.Fatalf("translate source article = %s, want %s", article, englishArticle)
		}
		if targetLang != "zh-cn" {
			t.Fatalf("targetLang = %q, want zh-cn", targetLang)
		}
		return json.RawMessage(`{"title":"能源总结","lead":"中文页面。"}`), nil
	}
	defer func() { translateGenericArtifactWikiArticleFn = origTranslate }()

	c, rec := newArtifactWikiContext(t, "summary", "7_sum_1", "zh-cn")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.Article) != `{"title":"能源总结","lead":"中文页面。"}` {
		t.Fatalf("article = %s", resp.Article)
	}
	saved, err := os.ReadFile(filepath.Join(recordDir, "wikipage_summary_7_sum_1.zh-cn.json"))
	if err != nil {
		t.Fatalf("translated article not saved: %v", err)
	}
	if string(saved) != `{"title":"能源总结","lead":"中文页面。"}` {
		t.Fatalf("saved article = %s", saved)
	}
}

func TestGetArtifactWikiNonMetricGeneratesTargetLanguageDirectlyWhenTranslationModelMissing(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	t.Setenv("TRANSLATION_MODEL_NAME", "")
	if err := os.MkdirAll(filepath.Join(artifactDir, "0", "7"), 0o755); err != nil {
		t.Fatal(err)
	}

	origProvider := artifactWikiProviders["entity"]
	artifactWikiProviders["entity"] = artifactWikiProvider{
		loadPayload: func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
			return artifactWikiPayload{
				RecordID:       7,
				ArtifactID:     artifactID,
				Record:         json.RawMessage(`{"entity_id":"7_ent_1","entity":"Intelligent Software"}`),
				SourceDocument: json.RawMessage(`{"record_id":7,"title":"doc.pdf","file_name":"doc.pdf","type":"pdf"}`),
				Generated: artifactWikiGeneratedMeta{
					Lang:       "zh-cn",
					SourceHash: "sha256:test",
				},
			}, nil
		},
	}
	defer func() { artifactWikiProviders["entity"] = origProvider }()

	origBuild := buildGenericArtifactWikiArticleFn
	buildGenericArtifactWikiArticleFn = func(ctx context.Context, logger ApiTypes.JimoLogger, payload artifactWikiPayload, artifactType, lang string) (json.RawMessage, error) {
		if artifactType != "entity" {
			t.Fatalf("artifactType = %q, want entity", artifactType)
		}
		if lang != "zh-cn" {
			t.Fatalf("lang = %q, want zh-cn", lang)
		}
		return json.RawMessage(`{"title":"智能软件","lead":"中文页面。"}`), nil
	}
	defer func() { buildGenericArtifactWikiArticleFn = origBuild }()

	origTranslate := translateGenericArtifactWikiArticleFn
	translateGenericArtifactWikiArticleFn = func(ctx context.Context, logger ApiTypes.JimoLogger, article json.RawMessage, targetLang string) (json.RawMessage, error) {
		t.Fatalf("translate should not be called when translation model is missing")
		return nil, nil
	}
	defer func() { translateGenericArtifactWikiArticleFn = origTranslate }()

	c, rec := newArtifactWikiContext(t, "entity", "7_ent_1", "zh-cn")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.Article) != `{"title":"智能软件","lead":"中文页面。"}` {
		t.Fatalf("article = %s", resp.Article)
	}
}

func TestGetArtifactWikiNonMetricRefreshesStaleTargetLanguageCache(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	t.Setenv("TRANSLATION_MODEL_NAME", "configured-translation-model")

	recordDir := filepath.Join(artifactDir, "0", "7")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	englishArticle := json.RawMessage(`{
  "title":"Intelligent Software",
  "lead":"English article.",
  "definition":"Software with intelligent functions.",
  "background":"English background.",
  "generated":{"lang":"en"}
}`)
	if err := os.WriteFile(filepath.Join(recordDir, "wikipage_entity_7_ent_1.en.json"), englishArticle, 0o644); err != nil {
		t.Fatal(err)
	}
	staleZhArticle := json.RawMessage(`{
  "title":"Intelligent Software",
  "lead":"English article.",
  "definition":"Software with intelligent functions.",
  "background":"English background.",
  "generated":{"lang":"zh-cn"}
}`)
	if err := os.WriteFile(filepath.Join(recordDir, "wikipage_entity_7_ent_1.zh-cn.json"), staleZhArticle, 0o644); err != nil {
		t.Fatal(err)
	}

	origProvider := artifactWikiProviders["entity"]
	artifactWikiProviders["entity"] = artifactWikiProvider{
		loadPayload: func(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, artifactID, lang string) (artifactWikiPayload, error) {
			return artifactWikiPayload{
				RecordID:       7,
				ArtifactID:     artifactID,
				Record:         json.RawMessage(`{"entity_id":"7_ent_1","entity":"Intelligent Software"}`),
				SourceDocument: json.RawMessage(`{"record_id":7,"title":"doc.pdf","file_name":"doc.pdf","type":"pdf"}`),
				Generated: artifactWikiGeneratedMeta{
					Lang:       "zh-cn",
					SourceHash: "sha256:test",
				},
			}, nil
		},
	}
	defer func() { artifactWikiProviders["entity"] = origProvider }()

	origTranslate := translateGenericArtifactWikiArticleFn
	translateGenericArtifactWikiArticleFn = func(ctx context.Context, logger ApiTypes.JimoLogger, article json.RawMessage, targetLang string) (json.RawMessage, error) {
		if string(article) != string(englishArticle) {
			t.Fatalf("translate source article = %s, want %s", article, englishArticle)
		}
		return json.RawMessage(`{"title":"智能软件","lead":"中文页面。","definition":"具备智能功能的软件。","background":"中文背景。","generated":{"lang":"zh-cn"}}`), nil
	}
	defer func() { translateGenericArtifactWikiArticleFn = origTranslate }()

	c, rec := newArtifactWikiContext(t, "entity", "7_ent_1", "zh-cn")
	if err := GetArtifactWiki(c); err != nil {
		t.Fatalf("GetArtifactWiki err = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var resp artifactWikiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Fresh {
		t.Fatalf("fresh = %v, want true after stale cache refresh", resp.Fresh)
	}
	if string(resp.Article) == string(staleZhArticle) {
		t.Fatalf("article should have been refreshed")
	}
}
