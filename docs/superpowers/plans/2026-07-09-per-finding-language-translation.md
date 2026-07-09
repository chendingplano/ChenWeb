# Per-Finding Language Pulldown + On-Demand Translation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a per-finding **Language** pulldown (placed before the **Accept** button) to both doc-review findings views, backed by a new on-demand translation endpoint that reuses the existing `llmFindingTranslator` — translating automatically when `AUTO_TRANSLATE_FINDINGS=true`, otherwise prompting the user to confirm.

**Architecture:** A new `POST /api/v1/doc-review/findings/:id/translate` endpoint checks `kb.doc_review_findings.metadata` for an existing translation first (no LLM call if found); if missing, it either calls the existing `llmFindingTranslator.TranslateFinding` immediately (auto mode / confirmed) or reports back that confirmation is needed (no LLM call). The frontend adds a per-row `<select>` in both views that calls this endpoint and patches the row's title/description/suggestion in place.

**Tech Stack:** Go 1.25 / Echo / `database/sql` (Postgres, `lib/pq`-style `$n` placeholders) / `sqlmock` for tests; SvelteKit 5 (runes) frontend, no frontend test runner in this repo (verify manually via `mise dev`).

## Global Constraints
- Module path is `github.com/chendingplano/deepdoc` (not `ChenWeb`) — use this import prefix for all new Go imports.
- Reuse the existing translation implementation exactly: `llmFindingTranslator.TranslateFinding`, its prompts, and `FindingMetadataEnvelope`/`FindingLocalizedContent` storage are unchanged. This task only adds a new on-demand entry point into them — no new translation logic.
- `AUTO_TRANSLATE_FINDINGS` is a new env var, independent of the existing `DOC_REVIEW_TRANSLATION`.
- `config.toml`'s `[frontend].default_language` is a **list** (`["zh-cn"]`), not a scalar — treat it as such end-to-end (Go `[]string`, TS `string[]`), using the first element as "the" default.
- Only `title`, `description`, `suggestion` are translated — never `evidence`/`location`.
- Follow existing code style exactly per file (tabs in `+page.svelte`, 4-space indent in `doc-review-results-view.svelte`, no refactor of unrelated code).
- Spec: `docs/superpowers/specs/2026-07-09-per-finding-language-translation-design.md`

---

## Task 1: Backend config — `default_language` in `kbhandler`

**Files:**
- Modify: `ChenWeb/server/api/kbhandler/kb_config_handler.go`
- Test: `ChenWeb/server/api/kbhandler/kb_config_handler_test.go` (new file)

**Interfaces:**
- Produces: `kbhandler.LoadKbFrontendConfig() (kbFrontendConfig, error)` (renamed/exported from `loadKbFrontendConfig`), where `kbFrontendConfig` gains `DefaultLanguage []string` (JSON key `default_language`). Task 2 depends on calling `kbhandler.LoadKbFrontendConfig()` and reading `.SupportedLanguages`.

- [x] **Step 1: Write the failing test**

Create `/Users/cding/Workspace/ChenWeb/server/api/kbhandler/kb_config_handler_test.go`:

```go
package kbhandler

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}

func TestLoadKbFrontendConfigReadsDefaultLanguageList(t *testing.T) {
	path := writeTestConfig(t, `
[frontend]
topic_types = ["fact"]
supported_languages = ["en", "zh-cn", "ja"]
default_language = ["zh-cn"]
`)
	t.Setenv("KB_CONFIG_FILE", path)

	cfg, err := LoadKbFrontendConfig()
	if err != nil {
		t.Fatalf("LoadKbFrontendConfig: %v", err)
	}
	if len(cfg.DefaultLanguage) != 1 || cfg.DefaultLanguage[0] != "zh-cn" {
		t.Fatalf("DefaultLanguage = %#v, want [zh-cn]", cfg.DefaultLanguage)
	}
	if len(cfg.SupportedLanguages) != 3 || cfg.SupportedLanguages[2] != "ja" {
		t.Fatalf("SupportedLanguages = %#v", cfg.SupportedLanguages)
	}
}

func TestLoadKbFrontendConfigDefaultsDefaultLanguageToEnWhenAbsent(t *testing.T) {
	path := writeTestConfig(t, `
[frontend]
supported_languages = ["en", "zh-cn"]
`)
	t.Setenv("KB_CONFIG_FILE", path)

	cfg, err := LoadKbFrontendConfig()
	if err != nil {
		t.Fatalf("LoadKbFrontendConfig: %v", err)
	}
	if len(cfg.DefaultLanguage) != 1 || cfg.DefaultLanguage[0] != "en" {
		t.Fatalf("DefaultLanguage = %#v, want [en]", cfg.DefaultLanguage)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/kbhandler/... -run TestLoadKbFrontendConfig -v`
Expected: FAIL — `LoadKbFrontendConfig` undefined (only unexported `loadKbFrontendConfig` exists) and `DefaultLanguage` field undefined.

- [x] **Step 3: Implement**

In `kb_config_handler.go`, update the struct (around line 16-22):

```go
type kbFrontendConfig struct {
	TopicTypes             []string `json:"topic_types"`
	SupportedLanguages     []string `json:"supported_languages"`
	DefaultLanguage        []string `json:"default_language"`
	MandatoryProcessors    []string `json:"mandatory_processors"`
	RequiredProcessors     []string `json:"required_processors"`
	MaxDocProcessPipelines int      `json:"max_doc_process_pipelines"`
}
```

Update `rawKbFrontendSection` (around line 58-66):

```go
type rawKbFrontendSection struct {
	Frontend struct {
		TopicTypes         []string `toml:"topic_types"`
		SupportedLanguages []string `toml:"supported_languages"`
		DefaultLanguage    []string `toml:"default_language"`
	} `toml:"frontend"`
	DocProcessing struct {
		RequiredProcessors []string `toml:"required_processors"`
	} `toml:"doc-processing"`
}
```

Rename `loadKbFrontendConfig` to `LoadKbFrontendConfig` (exported) and add the default-language fallback, around line 68-97:

```go
// LoadKbFrontendConfig reads the [frontend] and [doc-processing] sections from
// config.toml. Exported so other packages (e.g. doc-reviews, for on-demand
// finding translation) can reuse the same config source.
func LoadKbFrontendConfig() (kbFrontendConfig, error) {
	path := resolveKbConfigFilePath()
	body, err := os.ReadFile(path)
	if err != nil {
		return kbFrontendConfig{}, err
	}
	var raw rawKbFrontendSection
	if err := toml.Unmarshal(body, &raw); err != nil {
		return kbFrontendConfig{}, err
	}
	types := raw.Frontend.TopicTypes
	if types == nil {
		types = []string{}
	}
	supportedLanguages := raw.Frontend.SupportedLanguages
	if len(supportedLanguages) == 0 {
		supportedLanguages = defaultSupportedLanguages()
	}
	defaultLanguage := raw.Frontend.DefaultLanguage
	if len(defaultLanguage) == 0 {
		defaultLanguage = defaultLanguageList()
	}
	reqProcs := raw.DocProcessing.RequiredProcessors
	if reqProcs == nil {
		reqProcs = []string{}
	}
	return kbFrontendConfig{
		TopicTypes:             types,
		SupportedLanguages:     supportedLanguages,
		DefaultLanguage:        defaultLanguage,
		MandatoryProcessors:    mandatoryProcessorIDs,
		RequiredProcessors:     reqProcs,
		MaxDocProcessPipelines: maxDocProcessPipelinesFromEnv(),
	}, nil
}

func defaultLanguageList() []string {
	return []string{"en"}
}
```

Update the one call site in `GetKbFrontendConfig` (around line 36-56) — rename the call and add the fallback field:

```go
func GetKbFrontendConfig(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_CFG_001")
	defer rc.Close()

	cfg, err := LoadKbFrontendConfig()
	if err != nil {
		rc.GetLogger().Warn("load kb frontend config failed", "err", err)
		return c.JSON(http.StatusOK, kbFrontendConfigResponse{
			Status: true,
			Config: kbFrontendConfig{
				TopicTypes:             []string{},
				SupportedLanguages:     defaultSupportedLanguages(),
				DefaultLanguage:        defaultLanguageList(),
				MandatoryProcessors:    mandatoryProcessorIDs,
				RequiredProcessors:     []string{},
				MaxDocProcessPipelines: maxDocProcessPipelinesFromEnv(),
			},
		})
	}

	return c.JSON(http.StatusOK, kbFrontendConfigResponse{Status: true, Config: cfg})
}
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/kbhandler/... -run TestLoadKbFrontendConfig -v`
Expected: PASS (both tests)

- [x] **Step 5: Run the full kbhandler package test suite to check nothing broke**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/kbhandler/... -v 2>&1 | tail -40`
Expected: PASS (or same pre-existing failures unrelated to this change — check before assuming a regression)

- [x] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "Export LoadKbFrontendConfig and add default_language to [frontend] config"
```

---

## Task 2: Backend — `DocReviewController.TranslateFinding`

**Files:**
- Modify: `ChenWeb/server/api/doc-reviews/finding_translation.go`
- Test: `ChenWeb/server/api/doc-reviews/finding_translation_test.go`

**Interfaces:**
- Consumes: `kbhandler.LoadKbFrontendConfig()` (Task 1), `newLLMFindingTranslator()`, `translationFromMetadata(raw []byte, language string) (FindingTranslation, bool)`, `applyFindingTranslation(f FindingItem, tr FindingTranslation) FindingItem`, `applyFindingMetadata(f *FindingItem, data []byte)` (all pre-existing in this package), `fakeFindingTranslator` (pre-existing test helper in `finding_translation_test.go`).
- Produces: `type TranslateFindingResult struct { Finding FindingItem; Translated bool; NeedsConfirmation bool }` and `func (c *DocReviewController) TranslateFinding(ctx context.Context, findingID int64, language string, confirm bool) (TranslateFindingResult, error)`. Task 3's HTTP handler calls this directly.

- [ ] **Step 1: Write the failing tests**

Append to `/Users/cding/Workspace/ChenWeb/server/api/doc-reviews/finding_translation_test.go`:

```go
func writeSupportedLanguagesConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
[frontend]
supported_languages = ["en", "zh-cn", "ja"]
default_language = ["zh-cn"]
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	t.Setenv("KB_CONFIG_FILE", path)
}

func findingRowColumns() []string {
	return []string{
		"id", "pass", "aspect", "severity", "finding_type", "title", "description",
		"evidence", "location", "suggestion", "confidence", "review_status", "metadata", "artifact_id",
	}
}

func TestTranslateFindingReturnsCachedTranslationWithoutLLMCall(t *testing.T) {
	writeSupportedLanguagesConfig(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	metadata := `{"schema_version":1,"canonical_language":"en","en":{"title":"Canonical title","description":"Canonical desc","suggestion":"Canonical sug","provenance":"canonical"},"zh-cn":{"title":"中文标题","description":"中文描述","suggestion":"中文建议","provenance":"llm_translation"}}`
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, pass, aspect, severity, finding_type, title, description,
	       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
	       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text,
	       COALESCE(artifact_id,'')
	FROM kb.doc_review_findings WHERE id = $1`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(findingRowColumns()).
			AddRow(int64(42), "P5", "provisions", "medium", "issue", "Canonical title", "Canonical desc",
				"", "115", "Canonical sug", 0.95, "pending", metadata, ""))

	translator := &fakeFindingTranslator{}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	result, err := ctrl.TranslateFinding(context.Background(), 42, "zh-cn", false)
	if err != nil {
		t.Fatalf("TranslateFinding: %v", err)
	}
	if result.Translated {
		t.Fatalf("Translated = true, want false (cached path)")
	}
	if result.NeedsConfirmation {
		t.Fatalf("NeedsConfirmation = true, want false")
	}
	if result.Finding.Title != "中文标题" {
		t.Fatalf("Finding.Title = %q, want 中文标题", result.Finding.Title)
	}
	if len(translator.translateCalls) != 0 {
		t.Fatalf("translateCalls = %v, want none (no LLM call expected)", translator.translateCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTranslateFindingAutoTranslatesWhenEnvVarSet(t *testing.T) {
	writeSupportedLanguagesConfig(t)
	t.Setenv("AUTO_TRANSLATE_FINDINGS", "true")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	metadata := `{"schema_version":1,"canonical_language":"en","en":{"title":"Canonical title","description":"Canonical desc","suggestion":"Canonical sug","provenance":"canonical"}}`
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, pass, aspect, severity, finding_type, title, description,
	       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
	       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text,
	       COALESCE(artifact_id,'')
	FROM kb.doc_review_findings WHERE id = $1`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(findingRowColumns()).
			AddRow(int64(42), "P5", "provisions", "medium", "issue", "Canonical title", "Canonical desc",
				"", "115", "Canonical sug", 0.95, "pending", metadata, ""))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.doc_review_findings
	SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object($1::text, $2::jsonb)
	WHERE id = $3`)).
		WithArgs("zh-cn", sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	translator := &fakeFindingTranslator{
		translateOut: map[string]FindingLocalizedContent{
			"zh-cn": {Title: "中文标题", Description: "中文描述", Suggestion: "中文建议"},
		},
	}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	result, err := ctrl.TranslateFinding(context.Background(), 42, "zh-cn", false)
	if err != nil {
		t.Fatalf("TranslateFinding: %v", err)
	}
	if !result.Translated {
		t.Fatalf("Translated = false, want true")
	}
	if result.NeedsConfirmation {
		t.Fatalf("NeedsConfirmation = true, want false")
	}
	if result.Finding.Title != "中文标题" {
		t.Fatalf("Finding.Title = %q, want 中文标题", result.Finding.Title)
	}
	if len(translator.translateCalls) != 1 || translator.translateCalls[0] != "zh-cn" {
		t.Fatalf("translateCalls = %v, want [zh-cn]", translator.translateCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTranslateFindingNeedsConfirmationWhenAutoOffAndNotConfirmed(t *testing.T) {
	writeSupportedLanguagesConfig(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	metadata := `{"schema_version":1,"canonical_language":"en","en":{"title":"Canonical title","description":"Canonical desc","suggestion":"Canonical sug","provenance":"canonical"}}`
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, pass, aspect, severity, finding_type, title, description,
	       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
	       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text,
	       COALESCE(artifact_id,'')
	FROM kb.doc_review_findings WHERE id = $1`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(findingRowColumns()).
			AddRow(int64(42), "P5", "provisions", "medium", "issue", "Canonical title", "Canonical desc",
				"", "115", "Canonical sug", 0.95, "pending", metadata, ""))

	translator := &fakeFindingTranslator{}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	result, err := ctrl.TranslateFinding(context.Background(), 42, "zh-cn", false)
	if err != nil {
		t.Fatalf("TranslateFinding: %v", err)
	}
	if result.Translated {
		t.Fatalf("Translated = true, want false")
	}
	if !result.NeedsConfirmation {
		t.Fatalf("NeedsConfirmation = false, want true")
	}
	if result.Finding.Title != "Canonical title" {
		t.Fatalf("Finding.Title = %q, want unchanged Canonical title", result.Finding.Title)
	}
	if len(translator.translateCalls) != 0 {
		t.Fatalf("translateCalls = %v, want none (no LLM call expected)", translator.translateCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v (an UPDATE would violate this - none was expected)", err)
	}
}

func TestTranslateFindingTranslatesWhenConfirmed(t *testing.T) {
	writeSupportedLanguagesConfig(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	metadata := `{"schema_version":1,"canonical_language":"en","en":{"title":"Canonical title","description":"Canonical desc","suggestion":"Canonical sug","provenance":"canonical"}}`
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, pass, aspect, severity, finding_type, title, description,
	       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
	       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text,
	       COALESCE(artifact_id,'')
	FROM kb.doc_review_findings WHERE id = $1`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows(findingRowColumns()).
			AddRow(int64(42), "P5", "provisions", "medium", "issue", "Canonical title", "Canonical desc",
				"", "115", "Canonical sug", 0.95, "pending", metadata, ""))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.doc_review_findings
	SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object($1::text, $2::jsonb)
	WHERE id = $3`)).
		WithArgs("zh-cn", sqlmock.AnyArg(), int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	translator := &fakeFindingTranslator{
		translateOut: map[string]FindingLocalizedContent{
			"zh-cn": {Title: "中文标题", Description: "中文描述", Suggestion: "中文建议"},
		},
	}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	result, err := ctrl.TranslateFinding(context.Background(), 42, "zh-cn", true)
	if err != nil {
		t.Fatalf("TranslateFinding: %v", err)
	}
	if !result.Translated {
		t.Fatalf("Translated = false, want true")
	}
	if len(translator.translateCalls) != 1 || translator.translateCalls[0] != "zh-cn" {
		t.Fatalf("translateCalls = %v, want [zh-cn]", translator.translateCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestTranslateFindingRejectsUnsupportedLanguage(t *testing.T) {
	writeSupportedLanguagesConfig(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	translator := &fakeFindingTranslator{}
	ctrl := &DocReviewController{DB: db, Translator: translator}

	_, err = ctrl.TranslateFinding(context.Background(), 42, "fr", false)
	if err == nil {
		t.Fatalf("TranslateFinding: expected error for unsupported language, got nil")
	}
	if len(translator.translateCalls) != 0 {
		t.Fatalf("translateCalls = %v, want none", translator.translateCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v (no DB query expected before language validation)", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/doc-reviews/... -run TestTranslateFinding -v`
Expected: FAIL to compile — `TranslateFinding` method missing.

- [ ] **Step 3: Implement**

`finding_translation_test.go` already imports both `os` and `path/filepath` (used by existing tests) — the new `writeSupportedLanguagesConfig` helper needs no import changes in the test file.

In `/Users/cding/Workspace/ChenWeb/server/api/doc-reviews/finding_translation.go`, update the import block at the top:

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/chendingplano/deepdoc/server/api/kbhandler"
	"github.com/chendingplano/shared/go/api/ApiUtils"
)
```

Append to the end of `finding_translation.go` (after the existing `localizeFindings` function, currently ending around line 759):

```go
// TranslateFindingResult is returned by TranslateFinding.
type TranslateFindingResult struct {
	Finding           FindingItem `json:"finding"`
	Translated        bool        `json:"translated"`
	NeedsConfirmation bool        `json:"needs_confirmation"`
}

// TranslateFinding returns a finding localized into language, translating on
// demand via the LLM when no stored translation exists yet. This reuses the
// existing llmFindingTranslator / FindingMetadataEnvelope machinery end to
// end - it is only a new on-demand entry point into it, not new translation
// logic.
//
// If a translation for language is already stored in metadata, it is applied
// directly with no LLM call (Translated=false, NeedsConfirmation=false).
// If missing: when AUTO_TRANSLATE_FINDINGS=true or confirm=true, the LLM is
// called and the result persisted (Translated=true). Otherwise, the original
// finding is returned unchanged with NeedsConfirmation=true and no LLM call,
// letting the caller prompt the user before retrying with confirm=true.
func (c *DocReviewController) TranslateFinding(ctx context.Context, findingID int64, language string, confirm bool) (TranslateFindingResult, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return TranslateFindingResult{}, fmt.Errorf("language is required")
	}
	if !containsLanguage(configuredSupportedLanguages(), language) {
		return TranslateFindingResult{}, fmt.Errorf("unsupported language: %q", language)
	}

	finding, metadata, err := c.loadFindingWithMetadata(ctx, findingID)
	if err != nil {
		return TranslateFindingResult{}, err
	}

	if tr, ok := translationFromMetadata(metadata, language); ok {
		return TranslateFindingResult{Finding: applyFindingTranslation(finding, tr)}, nil
	}

	autoTranslate := strings.EqualFold(strings.TrimSpace(os.Getenv("AUTO_TRANSLATE_FINDINGS")), "true")
	if !autoTranslate && !confirm {
		return TranslateFindingResult{Finding: finding, NeedsConfirmation: true}, nil
	}

	translator := c.Translator
	if translator == nil {
		translator, err = newLLMFindingTranslator()
		if err != nil {
			return TranslateFindingResult{}, err
		}
	}
	content, err := translator.TranslateFinding(ctx, language, finding)
	if err != nil {
		return TranslateFindingResult{}, fmt.Errorf("translate finding %d to %s: %w", findingID, language, err)
	}
	if content.Provenance == "" {
		content.Provenance = "llm_translation"
	}

	contentJSON, err := json.Marshal(content)
	if err != nil {
		return TranslateFindingResult{}, fmt.Errorf("marshal translation for finding %d: %w", findingID, err)
	}
	res, err := c.DB.ExecContext(ctx, `UPDATE kb.doc_review_findings
	SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object($1::text, $2::jsonb)
	WHERE id = $3`,
		language, string(contentJSON), findingID,
	)
	if err != nil {
		return TranslateFindingResult{}, fmt.Errorf("persist translation for finding %d: %w", findingID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return TranslateFindingResult{}, fmt.Errorf("finding %d not found", findingID)
	}
	logger.Info("finding translated on demand", "finding_id", findingID, "language", language)

	return TranslateFindingResult{Finding: applyFindingTranslation(finding, content), Translated: true}, nil
}

// loadFindingWithMetadata loads one finding row (the same columns
// GetRequestWithFindings selects) plus its raw metadata JSON.
func (c *DocReviewController) loadFindingWithMetadata(ctx context.Context, id int64) (FindingItem, []byte, error) {
	var f FindingItem
	var metadata string
	err := c.DB.QueryRowContext(ctx, `SELECT id, pass, aspect, severity, finding_type, title, description,
	       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
	       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text,
	       COALESCE(artifact_id,'')
	FROM kb.doc_review_findings WHERE id = $1`, id,
	).Scan(&f.ID, &f.Pass, &f.Aspect, &f.Severity, &f.FindingType,
		&f.Title, &f.Description, &f.Evidence, &f.Location, &f.Suggestion,
		&f.Confidence, &f.ReviewStatus, &metadata, &f.ArtifactID)
	if err != nil {
		if err == sql.ErrNoRows {
			return FindingItem{}, nil, fmt.Errorf("finding %d not found", id)
		}
		return FindingItem{}, nil, fmt.Errorf("load finding %d: %w", id, err)
	}
	applyFindingMetadata(&f, []byte(metadata))
	return f, []byte(metadata), nil
}

// configuredSupportedLanguages reads config.toml's [frontend].supported_languages
// via kbhandler, falling back to a small default set if the config cannot be
// loaded (keeps this endpoint working even if config.toml is unreadable).
func configuredSupportedLanguages() []string {
	cfg, err := kbhandler.LoadKbFrontendConfig()
	if err != nil || len(cfg.SupportedLanguages) == 0 {
		return []string{"en", "zh-cn"}
	}
	return cfg.SupportedLanguages
}

func containsLanguage(list []string, language string) bool {
	for _, l := range list {
		if strings.EqualFold(strings.TrimSpace(l), language) {
			return true
		}
	}
	return false
}
```

Note: `kbhandler.LoadKbFrontendConfig()` returns the unexported type `kbFrontendConfig`; that's fine here because `configuredSupportedLanguages` captures it with `:=` and only reads the exported `.SupportedLanguages` field — it never names the type.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/doc-reviews/... -run TestTranslateFinding -v`
Expected: PASS (all 5 tests)

- [ ] **Step 5: Run the full doc-reviews package test suite to check nothing broke**

Run: `cd /Users/cding/Workspace/ChenWeb && go test ./server/api/doc-reviews/... -v 2>&1 | tail -60`
Expected: PASS (or same pre-existing failures unrelated to this change)

- [ ] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "Add DocReviewController.TranslateFinding for on-demand per-finding translation"
```

---

## Task 3: Backend — HTTP handler + route

**Files:**
- Modify: `ChenWeb/server/api/doc-reviews/handler.go`
- Modify: `ChenWeb/server/api/routes.go`

**Interfaces:**
- Consumes: `(c *DocReviewController) TranslateFinding(ctx, findingID int64, language string, confirm bool) (TranslateFindingResult, error)` (Task 2), `parseID(c echo.Context, name string) (int64, error)` (pre-existing in `handler.go`).
- Produces: `func TranslateFinding(c echo.Context) error` (echo handler), registered at `POST /api/v1/doc-review/findings/:id/translate`. Task 5/6 (frontend) call this route.

- [ ] **Step 1: Implement the handler**

In `/Users/cding/Workspace/ChenWeb/server/api/doc-reviews/handler.go`, add immediately after `UpdateFinding` (after line 275):

```go
// TranslateFinding returns a finding localized into the requested language,
// translating on demand via the LLM when needed. See DocReviewController.TranslateFinding.
func TranslateFinding(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	var body struct {
		Language string `json:"language"`
		Confirm  bool   `json:"confirm"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid body"})
	}
	ctrl := NewDocReviewController()
	result, err := ctrl.TranslateFinding(c.Request().Context(), id, body.Language, body.Confirm)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status":              true,
		"finding":             result.Finding,
		"translated":          result.Translated,
		"needs_confirmation":  result.NeedsConfirmation,
	})
}
```

In `/Users/cding/Workspace/ChenWeb/server/api/routes.go`, add the route immediately after the existing `PATCH /doc-review/findings/:id` line (around line 548):

```go
	apiGroup.PATCH("/doc-review/findings/:id", docreviews.UpdateFinding)
	apiGroup.POST("/doc-review/findings/:id/translate", docreviews.TranslateFinding)
	apiGroup.GET("/doc-review/findings/:id/lines", docreviews.GetFindingLines)
```

- [ ] **Step 2: Build to verify it compiles**

Run: `cd /Users/cding/Workspace/ChenWeb && go build -buildvcs=false -o ./.cache/server.exe ./server/cmd/deepdoc/.`
Expected: builds with no errors (no test for routing wiring itself — Task 2's controller tests already cover the logic; this step just confirms the handler/route compile and match the controller's signature).

- [ ] **Step 3: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "Wire up POST /doc-review/findings/:id/translate endpoint"
```

---

## Task 4: Frontend — service layer

**Files:**
- Modify: `ChenWeb/web/src/lib/services/kbService.ts`
- Modify: `ChenWeb/web/src/lib/services/docReviewService.ts`

**Interfaces:**
- Produces: `KbFrontendConfig.default_language: string[]` (extended type), `translateFinding(id: number, language: string, confirm?: boolean): Promise<{finding: FindingItem; translated: boolean; needs_confirmation: boolean}>`. Tasks 5 and 6 import both.

- [ ] **Step 1: Extend `KbFrontendConfig`**

In `/Users/cding/Workspace/ChenWeb/web/src/lib/services/kbService.ts`, update the type (around line 1304-1310):

```ts
export type KbFrontendConfig = {
	topic_types: string[];
	supported_languages: string[];
	default_language: string[];
	mandatory_processors: string[];
	required_processors: string[];
	max_doc_process_pipelines: number;
};
```

(`getKbFrontendConfig()` itself — line 1312-1318 — needs no change; it already returns the whole `config` object from the API response, so the new field flows through automatically.)

- [ ] **Step 2: Add `translateFinding`**

In `/Users/cding/Workspace/ChenWeb/web/src/lib/services/docReviewService.ts`, add immediately after `updateFinding` (after line 184):

```ts
export type TranslateFindingResult = {
	finding: FindingItem;
	translated: boolean;
	needs_confirmation: boolean;
};

export async function translateFinding(
	id: number,
	language: string,
	confirm = false
): Promise<TranslateFindingResult> {
	const res = await fetch(`${BASE}/findings/${id}/translate`, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ language, confirm })
	});
	const data = await res.json();
	if (!data.status) throw new Error(data.error_msg || 'Failed to translate finding');
	return {
		finding: data.finding,
		translated: !!data.translated,
		needs_confirmation: !!data.needs_confirmation
	};
}
```

- [ ] **Step 3: Type-check**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun run check 2>&1 | tail -40`
Expected: no new type errors attributable to these two files.

- [ ] **Step 4: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "Add translateFinding service call and default_language to KbFrontendConfig"
```

---

## Task 5: Frontend — `doc-review-results-view.svelte` per-row Language pulldown

**Files:**
- Modify: `ChenWeb/web/src/lib/components/home3/doc-review-results-view.svelte`

**Interfaces:**
- Consumes: `getKbFrontendConfig()` (`$lib/services/kbService`), `translateFinding(id, language, confirm)` (Task 4), pre-existing `updateFinding`, `FindingItem` type.

This view uses 4-space indentation — match it exactly.

- [ ] **Step 1: Add imports and state**

Update the import block (lines 1-4):

```svelte
    import { onMount, onDestroy } from 'svelte';
    import { getRequest, restartRequest, updateFinding, stopRequest, translateFinding } from '$lib/services/docReviewService';
    import type { RequestStatus, FindingItem, AspectStatus, ReviewPackageInfo } from '$lib/services/docReviewService';
    import { getKbFrontendConfig } from '$lib/services/kbService';
```

Add new state, right after the existing `let packages = $state<ReviewPackageInfo[]>([]);` line (in the "State" block, ~line 36):

```svelte
    let packages = $state<ReviewPackageInfo[]>([]);
    let supportedLanguages = $state<string[]>(['en']);
    let defaultLanguage = $state('en');
    let findingLanguage = $state<Record<number, string>>({});
    let translating = $state<Record<number, boolean>>({});
    let pendingConfirm = $state<{ id: number; language: string } | null>(null);
```

- [ ] **Step 2: Fetch supported languages on mount**

Update `onMount` (lines 291-296):

```svelte
    onMount(async () => {
        await pollStatus();
        if (isActive) {
            startPolling();
        }
        try {
            const config = await getKbFrontendConfig();
            supportedLanguages = config.supported_languages?.length ? config.supported_languages : ['en'];
            defaultLanguage = config.default_language?.[0] ?? 'en';
        } catch {
            supportedLanguages = ['en'];
            defaultLanguage = 'en';
        }
    });
```

- [ ] **Step 3: Add the translate handlers**

Add immediately after `handleAcceptReject` (after line 309):

```svelte
    function applyTranslatedFinding(id: number, translated: FindingItem, language: string) {
        findings = findings.map(f =>
            f.id === id
                ? { ...f, title: translated.title, description: translated.description, suggestion: translated.suggestion }
                : f
        );
        findingLanguage = { ...findingLanguage, [id]: language };
    }

    async function handleLanguageChange(finding: FindingItem, newLanguage: string) {
        const previous = findingLanguage[finding.id] ?? defaultLanguage;
        translating = { ...translating, [finding.id]: true };
        try {
            const resp = await translateFinding(finding.id, newLanguage, false);
            if (resp.needs_confirmation) {
                pendingConfirm = { id: finding.id, language: newLanguage };
                return;
            }
            applyTranslatedFinding(finding.id, resp.finding, newLanguage);
        } catch (e: any) {
            error = e.message;
            findingLanguage = { ...findingLanguage, [finding.id]: previous };
        } finally {
            translating = { ...translating, [finding.id]: false };
        }
    }

    async function confirmTranslate(finding: FindingItem) {
        if (!pendingConfirm || pendingConfirm.id !== finding.id) return;
        const language = pendingConfirm.language;
        translating = { ...translating, [finding.id]: true };
        try {
            const resp = await translateFinding(finding.id, language, true);
            applyTranslatedFinding(finding.id, resp.finding, language);
        } catch (e: any) {
            error = e.message;
        } finally {
            translating = { ...translating, [finding.id]: false };
            pendingConfirm = null;
        }
    }

    function cancelTranslate() {
        pendingConfirm = null;
    }
```

- [ ] **Step 4: Insert the pulldown and confirm UI in the row markup**

Replace the actions `<div>` (lines 641-650):

```svelte
                            <div style="display: flex; gap: 0.25rem; align-items: center;">
                                {#if finding.review_status === 'pending'}
                                    <button onclick={(e) => { e.stopPropagation(); handleAcceptReject(finding.id, 'accepted'); }}
                                        style="padding: 0.25rem 0.5rem; background: {successBg}; color: #22c55e; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Accept</button>
                                    <button onclick={(e) => { e.stopPropagation(); handleAcceptReject(finding.id, 'rejected'); }}
                                        style="padding: 0.25rem 0.5rem; background: rgba(239,68,68,0.1); color: #ef4444; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Reject</button>
                                {:else}
                                    <span style="font-size: 0.75rem; color: {textMuted}; text-transform: capitalize;">{finding.review_status}</span>
                                {/if}
                            </div>
```

with:

```svelte
                            <div onclick={(e) => e.stopPropagation()} style="display: flex; gap: 0.25rem; align-items: center;">
                                {#if pendingConfirm?.id === finding.id}
                                    <span style="font-size: 0.75rem; color: {textMuted};">Translate to {pendingConfirm.language}?</span>
                                    <button onclick={() => confirmTranslate(finding)}
                                        style="padding: 0.25rem 0.5rem; background: {successBg}; color: #22c55e; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Translate</button>
                                    <button onclick={cancelTranslate}
                                        style="padding: 0.25rem 0.5rem; background: {inputBg}; color: {textMuted}; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Cancel</button>
                                {:else}
                                    <select
                                        value={findingLanguage[finding.id] ?? defaultLanguage}
                                        disabled={translating[finding.id]}
                                        onchange={(e) => handleLanguageChange(finding, (e.target as HTMLSelectElement).value)}
                                        style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 4px; padding: 0.2rem 0.4rem; color: {textPrimary}; font-size: 0.75rem;">
                                        {#each supportedLanguages as lang (lang)}
                                            <option value={lang}>{lang}</option>
                                        {/each}
                                    </select>
                                    {#if finding.review_status === 'pending'}
                                        <button onclick={() => handleAcceptReject(finding.id, 'accepted')}
                                            style="padding: 0.25rem 0.5rem; background: {successBg}; color: #22c55e; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Accept</button>
                                        <button onclick={() => handleAcceptReject(finding.id, 'rejected')}
                                            style="padding: 0.25rem 0.5rem; background: rgba(239,68,68,0.1); color: #ef4444; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Reject</button>
                                    {:else}
                                        <span style="font-size: 0.75rem; color: {textMuted}; text-transform: capitalize;">{finding.review_status}</span>
                                    {/if}
                                {/if}
                            </div>
```

(Moving `e.stopPropagation()` onto the wrapping div replaces the per-button `stopPropagation` calls that were inline before — same effect, now shared by the select and both confirm buttons too.)

- [ ] **Step 5: Manual verification**

Run: `cd /Users/cding/Workspace/ChenWeb && mise dev`

In a browser, open the Document Review side panel for a completed review (Home → Applications → Document Review), and for one finding:
1. Confirm the Language `<select>` appears immediately before Accept/Reject, populated with `config.toml`'s `supported_languages`.
2. Pick a language with no existing translation, with `AUTO_TRANSLATE_FINDINGS` unset/not `"true"` in the server's env — confirm the inline "Translate to X?" prompt appears in place of Accept/Reject, with no title/description change yet.
3. Click **Translate** — confirm the row's title/description/suggestion update in place, the prompt disappears, and Accept/Reject reappear.
4. Switch back to the original language, then back to the translated one again — confirm it updates instantly with no confirm prompt (cached path, no LLM latency).
5. Restart the server with `AUTO_TRANSLATE_FINDINGS=true` and pick a third, never-before-translated language — confirm it translates immediately with no prompt.

Stop the dev server when done.

- [ ] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "Add per-finding Language pulldown to doc-review-results-view.svelte"
```

---

## Task 6: Frontend — `+page.svelte` per-row Language pulldown (both packages and severity views)

**Files:**
- Modify: `ChenWeb/web/src/routes/home3/doc-review-report/[id]/+page.svelte`

**Interfaces:**
- Consumes: `getKbFrontendConfig()` (`$lib/services/kbService`), `translateFinding(id, language, confirm)` (Task 4).

This file uses tab indentation — match it exactly.

- [ ] **Step 1: Add imports and state**

Update the import block (lines 1-16):

```svelte
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import {
		getReport,
		getRequest,
		listLanguages,
		updateFinding,
		translateFinding,
		regenerateReport,
		generateCorrectionReport,
		type FindingItem,
		type ReviewPackageInfo
	} from '$lib/services/docReviewService';
	import { getKbFrontendConfig } from '$lib/services/kbService';
	import DocStructureView from '$lib/components/home3/doc-structure-view.svelte';
	import EditToolDialog from '$lib/components/home3/edit-tool-dialog.svelte';
	import LlmAutoFixDialog from '$lib/components/home3/llm-auto-fix-dialog.svelte';
```

Add new state right after `let selectedLanguage = $state('en');` (line 66):

```svelte
	let selectedLanguage = $state('en');
	let supportedLanguages = $state<string[]>(['en']);
	let defaultLanguage = $state('en');
	let findingLanguage = $state<Record<number, string>>({});
	let translatingId = $state<number | null>(null);
	let pendingConfirm = $state<{ id: number; language: string } | null>(null);
```

- [ ] **Step 2: Fetch config and use it as the default-language fallback**

Replace the language-loading block inside `load()` (lines 305-317):

```svelte
			try {
				const config = await getKbFrontendConfig();
				supportedLanguages = config.supported_languages?.length ? config.supported_languages : ['en'];
				defaultLanguage = config.default_language?.[0] ?? 'en';
			} catch {
				supportedLanguages = ['en'];
				defaultLanguage = 'en';
			}
			try {
				const configuredLanguages = await listLanguages();
				languages = configuredLanguages.length > 0 ? configuredLanguages : ['en'];
				if (!languages.includes(selectedLanguage)) {
					selectedLanguage = languages.includes(defaultLanguage) ? defaultLanguage : (languages[0] ?? defaultLanguage);
					localStorage.setItem(LANG_STORAGE_KEY, selectedLanguage);
				}
			} catch {
				languages = ['en'];
				selectedLanguage = defaultLanguage;
				localStorage.setItem(LANG_STORAGE_KEY, defaultLanguage);
			}
```

(`selectedLanguage`'s own default is only used as a fallback when nothing is in `localStorage` yet or the stored value isn't in the loaded `languages` list — the `onMount` block at line 488-490 that applies `localStorage.getItem(LANG_STORAGE_KEY)` first is unchanged.)

- [ ] **Step 3: Add the translate handlers**

Add immediately after `setFindingStatus` (after line 391):

```svelte
	function applyTranslatedFinding(id: number, translated: FindingItem, language: string) {
		findings = findings.map((f) =>
			f.id === id
				? { ...f, title: translated.title, description: translated.description, suggestion: translated.suggestion }
				: f
		);
		findingLanguage = { ...findingLanguage, [id]: language };
	}

	async function handleFindingLanguageChange(f: FindingItem, newLanguage: string) {
		const previous = findingLanguage[f.id] ?? defaultLanguage;
		translatingId = f.id;
		try {
			const resp = await translateFinding(f.id, newLanguage, false);
			if (resp.needs_confirmation) {
				pendingConfirm = { id: f.id, language: newLanguage };
				return;
			}
			applyTranslatedFinding(f.id, resp.finding, newLanguage);
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Translation failed');
			findingLanguage = { ...findingLanguage, [f.id]: previous };
		} finally {
			translatingId = null;
		}
	}

	async function confirmFindingTranslate(f: FindingItem) {
		if (!pendingConfirm || pendingConfirm.id !== f.id) return;
		const language = pendingConfirm.language;
		translatingId = f.id;
		try {
			const resp = await translateFinding(f.id, language, true);
			applyTranslatedFinding(f.id, resp.finding, language);
		} catch (e) {
			showToast('error', e instanceof Error ? e.message : 'Translation failed');
		} finally {
			translatingId = null;
			pendingConfirm = null;
		}
	}

	function cancelFindingTranslate() {
		pendingConfirm = null;
	}
```

- [ ] **Step 4: Insert the pulldown into both row layouts**

The `.finding-actions` block is duplicated verbatim in the packages view (lines 693-726) and the severity view (lines 789-822). In **both** places, insert the language control immediately before the Accept `<button>`:

```svelte
											<div class="finding-actions">
												<button
													class="act"
													disabled={busyId === f.id}
													onclick={() => onAutoFix(f)}
													title="Fix the offending line(s) automatically with the configured model"
												>
													LLM Auto Fix
												</button>
												<button
													class="act"
													disabled={busyId === f.id}
													onclick={() => (editFindingId = f.id)}
													title="Open the find/replace editor for the offending line(s)"
												>
													Edit Tool
												</button>
												<button
													class="act danger"
													disabled={busyId === f.id}
													onclick={() => onDelete(f)}
													title="Remove this finding from the report"
												>
													Delete
												</button>
												{#if pendingConfirm?.id === f.id}
													<span class="finding-loc">Translate to {pendingConfirm.language}?</span>
													<button class="act" onclick={() => confirmFindingTranslate(f)}>Translate</button>
													<button class="act" onclick={cancelFindingTranslate}>Cancel</button>
												{:else}
													<label class="language-picker">
														<select
															value={findingLanguage[f.id] ?? defaultLanguage}
															disabled={translatingId === f.id}
															onchange={(e) => handleFindingLanguageChange(f, (e.target as HTMLSelectElement).value)}
														>
															{#each supportedLanguages as lang (lang)}
																<option value={lang}>{lang}</option>
															{/each}
														</select>
													</label>
												{/if}
												<button
													class="act"
													disabled={busyId === f.id}
													onclick={() => onAccept(f)}
													title="Keep as is — take no action"
												>
													Accept
												</button>
											</div>
```

Apply this exact same replacement to both the packages-view block (lines 693-726) and the severity-view block (lines 789-822) — they are currently byte-identical, so the same inserted snippet goes in both.

- [ ] **Step 5: Manual verification**

Run: `cd /Users/cding/Workspace/ChenWeb && mise dev`

In a browser, open a doc-review report page (`/home3/doc-review-report/<id>`):
1. Confirm the per-row Language `<select>` appears before Accept in both "packages" view mode and "severity" view mode, for the same finding.
2. Confirm the page-level Language `<select>` at the top still works as before (bulk reviewer-group localization via `reloadFindingsForLanguage`) and that switching it resets any per-row override you'd made (per the spec's documented interaction).
3. Repeat the untranslated → confirm → translated flow, and the cached-instant flow, as in Task 5 Step 5.

Stop the dev server when done.

- [ ] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "Add per-finding Language pulldown to doc-review-report page (packages + severity views)"
```

---

## Self-Review Notes

- **Spec coverage:** Task 1 covers the config section; Task 2 covers the controller/endpoint logic (all 5 cases from the spec's testing plan: cached hit, auto-translate, needs-confirmation, confirmed-translate, unsupported-language); Task 3 covers the HTTP/route wiring; Task 4 covers the frontend service layer; Tasks 5 and 6 cover both UI views including the page-level/per-row interaction documented in the spec.
- **Type consistency:** `TranslateFindingResult` (Go, Task 2) fields (`Finding`, `Translated`, `NeedsConfirmation`) map 1:1 to the JSON keys (`finding`, `translated`, `needs_confirmation`) used by the handler (Task 3) and the TS `TranslateFindingResult` type (Task 4) consumed identically in Tasks 5 and 6.
- **No placeholders:** every step has concrete code; manual-verification steps (5 in Tasks 5/6) are explicit checklists rather than "test the UI" placeholders, since this repo has no frontend test runner.

## Recovery Note (2026-07-09)

This plan file was briefly lost from disk when a `jj restore config.toml docs/` command (run outside this session, while Task 1's implementer subagent was working in this same non-isolated working copy) reverted the `docs/` directory to its pre-plan state. Rewritten verbatim from conversation context after Task 1 completed and committed successfully (commit `unxw 7f8f`, unaffected since it wasn't under `docs/`). Task 1's checkboxes above are marked done to reflect this.

---

## Task 7: Post-verification fixes to `doc-review-results-view.svelte`

**Origin:** live manual verification (by the user, on their own running dev instance) of Tasks 5/6 surfaced three real gaps, all confined to `doc-review-results-view.svelte`:
1. The per-row pulldown cosmetically shows `defaultLanguage` (e.g. `zh-cn`) as selected, but nothing ever checks/applies that language against the loaded (canonical/English) content — so the dropdown and the displayed text disagree.
2. When a translate call fails (e.g. this environment has no `TRANSLATION_MODEL_NAME`/prompt env vars configured, so any real LLM call errors), the failure is only shown in a global top-of-panel `{#if error}` banner shared with unrelated actions (Stop, Accept/Reject) — easy to miss when scrolled down a long findings list, reads as "the button does nothing."
3. This panel has no page-level Language selector at all (only `+page.svelte` does) — the user wants one here too, that seeds the default for all rows while still letting any individual row be overridden via its own pulldown.

**Files:**
- Modify: `ChenWeb/web/src/lib/components/home3/doc-review-results-view.svelte`

**Interfaces:**
- Consumes: existing `handleLanguageChange`, `confirmTranslate`, `applyTranslatedFinding`, `defaultLanguage`, `supportedLanguages`, `findingLanguage`, `pendingConfirm`, `translating` state (all from Task 5). No backend or service-layer changes.

- [ ] **Step 1: Add a per-row translate-error state, and switch translate failures from the global banner to it**

Add new state next to the existing Task 5 state (near `let pendingConfirm = $state<{ id: number; language: string } | null>(null);`):

```svelte
    let translateError = $state<Record<number, string>>({});
```

In `handleLanguageChange`, replace the `catch` block:

```svelte
        } catch (e: any) {
            error = e.message;
            findingLanguage = { ...findingLanguage, [finding.id]: previous };
        } finally {
```

with:

```svelte
        } catch (e: any) {
            translateError = { ...translateError, [finding.id]: e.message };
            findingLanguage = { ...findingLanguage, [finding.id]: previous };
        } finally {
```

In `confirmTranslate`, replace the `catch` block:

```svelte
        } catch (e: any) {
            error = e.message;
        } finally {
```

with:

```svelte
        } catch (e: any) {
            translateError = { ...translateError, [finding.id]: e.message };
        } finally {
```

In `applyTranslatedFinding`, clear any prior error for that row on success — add as the first line of the function:

```svelte
    function applyTranslatedFinding(id: number, translated: FindingItem, language: string) {
        translateError = { ...translateError, [id]: '' };
        findings = findings.map(f =>
```

(An empty string is falsy in the `{#if translateError[finding.id]}` check below, so this correctly hides the message once a translation succeeds — no need to delete the key.)

The global `error` variable and its existing top-of-panel banner (`{#if error}` block) are untouched — they still cover Stop/Accept/Reject failures as before; translate failures no longer write to `error`.

- [ ] **Step 2: Render the per-row error inline in the expanded body**

Find the expanded-body block (currently):

```svelte
                        {#if expandedFindings.has(finding.id)}
                            <div style="padding: 0 1rem 0.75rem; border-top: 1px solid {borderColor};">
                                <p style="color: {textSecondary}; font-size: 0.85rem; margin-top: 0.5rem;">{finding.description}</p>
```

Insert an error line right after the opening `<div>`, before the description paragraph:

```svelte
                        {#if expandedFindings.has(finding.id)}
                            <div style="padding: 0 1rem 0.75rem; border-top: 1px solid {borderColor};">
                                {#if translateError[finding.id]}
                                    <p style="margin-top: 0.5rem; color: #ef4444; font-size: 0.8rem;">Translation failed: {translateError[finding.id]}</p>
                                {/if}
                                <p style="color: {textSecondary}; font-size: 0.85rem; margin-top: 0.5rem;">{finding.description}</p>
```

- [ ] **Step 3: Auto-check the default language on first expand**

Replace `toggleFinding`:

```svelte
    function toggleFinding(id: number) {
        const next = new Set(expandedFindings);
        if (next.has(id)) next.delete(id); else next.add(id);
        expandedFindings = next;
    }
```

with:

```svelte
    function toggleFinding(id: number) {
        const next = new Set(expandedFindings);
        if (next.has(id)) {
            next.delete(id);
        } else {
            next.add(id);
            if (findingLanguage[id] === undefined) {
                const finding = findings.find(f => f.id === id);
                if (finding) handleLanguageChange(finding, defaultLanguage);
            }
        }
        expandedFindings = next;
    }
```

This mirrors exactly what selecting `defaultLanguage` in the row's own pulldown would do (cache hit → instant apply; missing + `AUTO_TRANSLATE_FINDINGS=true` → auto-translate; missing + not auto → inline "Translate to X?" prompt) — it just fires it automatically the first time a row is expanded, instead of requiring the user to also touch the per-row pulldown. `findingLanguage[id] === undefined` guards it to run at most once per row per page load (once set — to any language, including via this auto-check — it won't re-fire on subsequent collapse/expand).

- [ ] **Step 4: Add a page-level Language selector that seeds the per-row default**

Add a handler, near the other translate handlers (after `cancelTranslate`):

```svelte
    function handlePageLanguageChange(newLanguage: string) {
        defaultLanguage = newLanguage;
        findingLanguage = {};
        pendingConfirm = null;
        translateError = {};
        for (const id of expandedFindings) {
            const finding = findings.find(f => f.id === id);
            if (finding) handleLanguageChange(finding, newLanguage);
        }
    }
```

(Mirrors `+page.svelte`'s `reloadFindingsForLanguage`: switching the page-level language clears every row's per-row override — collapsed rows lazily pick up the new default via Step 3 when the user later expands them; already-expanded rows are re-checked immediately since they're visible right now.)

In the header block, insert the selector between the Request badge and the status span:

```svelte
            <span style="color: {textMuted}; font-size: 0.8rem; padding: 0.15rem 0.5rem; border: 1px solid {borderColor}; border-radius: 5px; font-family: monospace;">Request #{requestId}</span>
            <span style="margin-left: auto; font-size: 0.8rem; font-weight: 600; padding: 0.2rem 0.6rem; border-radius: 6px; background: {accentTint}; color: {accent}; text-transform: capitalize;">{viewStatus}</span>
```

becomes:

```svelte
            <span style="color: {textMuted}; font-size: 0.8rem; padding: 0.15rem 0.5rem; border: 1px solid {borderColor}; border-radius: 5px; font-family: monospace;">Request #{requestId}</span>
            <label style="display: inline-flex; align-items: center; gap: 0.4rem; font-size: 0.8rem; color: {textMuted};">
                <span>Language</span>
                <select
                    value={defaultLanguage}
                    onchange={(e) => handlePageLanguageChange((e.target as HTMLSelectElement).value)}
                    style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 6px; padding: 0.25rem 0.5rem; color: {textPrimary}; font-size: 0.8rem;">
                    {#each supportedLanguages as lang (lang)}
                        <option value={lang}>{lang}</option>
                    {/each}
                </select>
            </label>
            <span style="margin-left: auto; font-size: 0.8rem; font-weight: 600; padding: 0.2rem 0.6rem; border-radius: 6px; background: {accentTint}; color: {accent}; text-transform: capitalize;">{viewStatus}</span>
```

(`margin-left: auto` stays on the `viewStatus` span, so it and the Stop button remain pushed to the right; the new selector sits inline right after the Request badge, on the left.)

- [ ] **Step 5: Type-check**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun run check 2>&1 | tail -40`
Expected: no new errors in this file (pre-existing errors/warnings elsewhere are not this task's concern).

- [ ] **Step 6: Commit**

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit -m "Fix live-verification findings: auto-check default language on expand, page-level selector, inline translate errors"
```

- [ ] **Step 7: Manual verification (by the controller/user, not the implementer)**

Reload the Document Review panel and confirm:
1. Expanding a finding row (that has no stored translation for the current default language, with a working LLM/translation config) triggers a visible "Translate to X?" prompt automatically, without touching the pulldown.
2. If the translate call fails, a red "Translation failed: ..." message appears inside the expanded row itself, not just (or instead of) the top banner.
3. The new page-level Language selector changes the default for rows not yet individually overridden, and re-checks any currently-expanded row immediately.
4. Individual rows can still be set to a different language than the page-level default via their own pulldown, without that being clobbered until the next page-level change.

---

## Task 8: Replace inline confirm prompt with a modal dialog + visible in-progress indicator

**Origin:** second round of live user verification (Task 7's changes tested live). The inline "Translate to X? Translate/Cancel" text embedded in the row header (added in Task 5, kept by Task 7's auto-check) is too easy to miss, especially once auto-triggered by expanding a row — user feedback: "Most users often overlook the message. Change to a dialog to prompt the user to translate instead." Also requested: a visible in-progress indicator during translation, since an LLM call may take a while.

**Files:**
- Modify: `ChenWeb/web/src/lib/components/home3/doc-review-results-view.svelte`

**Interfaces:**
- Consumes: existing `pendingConfirm`, `translating`, `findings`, `cancelTranslate`, `confirmTranslate`, `handleLanguageChange` state/functions (from Tasks 5 and 7), and the existing inline-modal convention already used twice in this same file for the JSON-viewer and Markdown-viewer modals (`{#if jsonModalOpen}...{/if}` / `{#if mdModalOpen}...{/if}` near the end of the file, before `<style>`) — reuse that exact backdrop/dialog visual pattern (`position: fixed; inset: 0; z-index: 1000; background: rgba(0,0,0,0.6); backdrop-filter: blur(2px);` backdrop, centered card) rather than inventing a new one or a separate component file.

- [ ] **Step 1: Simplify the row's actions area — remove the inline confirm branch, add a visible spinner while translating**

Replace the actions `<div>` (the block added/modified across Tasks 5 and 7):

```svelte
                            <div onclick={(e) => e.stopPropagation()} style="display: flex; gap: 0.25rem; align-items: center;">
                                {#if pendingConfirm?.id === finding.id}
                                    <span style="font-size: 0.75rem; color: {textMuted};">Translate to {pendingConfirm.language}?</span>
                                    <button onclick={() => confirmTranslate(finding)}
                                        style="padding: 0.25rem 0.5rem; background: {successBg}; color: #22c55e; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Translate</button>
                                    <button onclick={cancelTranslate}
                                        style="padding: 0.25rem 0.5rem; background: {inputBg}; color: {textMuted}; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Cancel</button>
                                {:else}
                                    <select
                                        value={findingLanguage[finding.id] ?? defaultLanguage}
                                        disabled={translating[finding.id]}
                                        onchange={(e) => handleLanguageChange(finding, (e.target as HTMLSelectElement).value)}
                                        style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 4px; padding: 0.2rem 0.4rem; color: {textPrimary}; font-size: 0.75rem;">
                                        {#each supportedLanguages as lang (lang)}
                                            <option value={lang}>{lang}</option>
                                        {/each}
                                    </select>
                                    {#if finding.review_status === 'pending'}
                                        <button onclick={() => handleAcceptReject(finding.id, 'accepted')}
                                            style="padding: 0.25rem 0.5rem; background: {successBg}; color: #22c55e; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Accept</button>
                                        <button onclick={() => handleAcceptReject(finding.id, 'rejected')}
                                            style="padding: 0.25rem 0.5rem; background: rgba(239,68,68,0.1); color: #ef4444; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Reject</button>
                                    {:else}
                                        <span style="font-size: 0.75rem; color: {textMuted}; text-transform: capitalize;">{finding.review_status}</span>
                                    {/if}
                                {/if}
                            </div>
```

with:

```svelte
                            <div onclick={(e) => e.stopPropagation()} style="display: flex; gap: 0.25rem; align-items: center;">
                                <select
                                    value={findingLanguage[finding.id] ?? defaultLanguage}
                                    disabled={translating[finding.id]}
                                    onchange={(e) => handleLanguageChange(finding, (e.target as HTMLSelectElement).value)}
                                    style="background: {inputBg}; border: 1px solid {borderColor}; border-radius: 4px; padding: 0.2rem 0.4rem; color: {textPrimary}; font-size: 0.75rem;">
                                    {#each supportedLanguages as lang (lang)}
                                        <option value={lang}>{lang}</option>
                                    {/each}
                                </select>
                                {#if translating[finding.id]}
                                    <LoaderIcon size={14} style="animation: spin 1s linear infinite; color: {accent};" />
                                {/if}
                                {#if finding.review_status === 'pending'}
                                    <button onclick={() => handleAcceptReject(finding.id, 'accepted')}
                                        style="padding: 0.25rem 0.5rem; background: {successBg}; color: #22c55e; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Accept</button>
                                    <button onclick={() => handleAcceptReject(finding.id, 'rejected')}
                                        style="padding: 0.25rem 0.5rem; background: rgba(239,68,68,0.1); color: #ef4444; border: none; border-radius: 4px; cursor: pointer; font-size: 0.75rem;">Reject</button>
                                {:else}
                                    <span style="font-size: 0.75rem; color: {textMuted}; text-transform: capitalize;">{finding.review_status}</span>
                                {/if}
                            </div>
```

(`LoaderIcon` is already imported at the top of this file — reused from the existing JSON/Markdown modal loading states and the panel's own "Loading review request" state. No new import needed.)

- [ ] **Step 2: Add the translate-confirm modal**

Insert a new modal block, following the exact same structural pattern as the existing `{#if mdModalOpen}...{/if}` block in this file (backdrop + centered card), immediately before the file's closing `<style>` block:

```svelte
<!-- Translate Confirm Modal -->
{#if pendingConfirm}
    {@const pendingFinding = findings.find(f => f.id === pendingConfirm.id)}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
        onclick={(e) => { if (e.target === e.currentTarget && !translating[pendingConfirm.id]) cancelTranslate(); }}
        onkeydown={(e) => { if (e.key === 'Escape' && !translating[pendingConfirm.id]) cancelTranslate(); }}
        role="dialog"
        aria-modal="true"
        tabindex="-1"
        style="position: fixed; inset: 0; z-index: 1000; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.6); backdrop-filter: blur(2px);"
    >
        <div style="background: {darkMode ? '#161B27' : '#FFFFFF'}; border: 1px solid {borderColor}; border-radius: 14px; width: min(90vw, 420px); box-shadow: 0 24px 64px rgba(0,0,0,0.5); padding: 1.5rem;">
            <div style="font-weight: 600; font-size: 1rem; color: {textPrimary}; margin-bottom: 0.5rem;">Translate finding</div>
            {#if pendingFinding}
                <div style="font-size: 0.85rem; color: {textSecondary}; margin-bottom: 1rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;">{pendingFinding.title}</div>
            {/if}
            {#if translating[pendingConfirm.id]}
                <div style="display: flex; align-items: center; gap: 0.6rem; color: {textSecondary}; font-size: 0.9rem; padding: 0.5rem 0;">
                    <LoaderIcon size={18} style="animation: spin 1s linear infinite; color: {accent};" />
                    Translating to {pendingConfirm.language}…
                </div>
            {:else}
                <div style="font-size: 0.9rem; color: {textPrimary}; margin-bottom: 1.25rem;">
                    No {pendingConfirm.language} translation exists yet. Translate this finding now?
                </div>
                <div style="display: flex; justify-content: flex-end; gap: 0.5rem;">
                    <button onclick={cancelTranslate}
                        style="padding: 0.4rem 0.9rem; background: transparent; color: {textMuted}; border: 1px solid {borderColor}; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">Cancel</button>
                    <button onclick={() => pendingFinding && confirmTranslate(pendingFinding)}
                        style="padding: 0.4rem 0.9rem; background: {successBg}; color: #22c55e; border: none; border-radius: 8px; cursor: pointer; font-size: 0.85rem;">Translate</button>
                </div>
            {/if}
        </div>
    </div>
{/if}
```

Notes on behavior (no code changes needed beyond the above — these fall out of existing Task 5/7 logic):
- The modal appears automatically for *any* path that sets `pendingConfirm` — the per-row pulldown, the page-level selector, and the Task 7 auto-check-on-expand all already funnel through `handleLanguageChange`, which is what sets `pendingConfirm`. No new triggering logic is needed.
- While `translating[pendingConfirm.id]` is true (set by `confirmTranslate` before its `await`, cleared in its `finally`), the modal shows the spinner state and the backdrop/Escape dismiss is disabled (`!translating[pendingConfirm.id]` guard) so the in-flight request can't be silently abandoned mid-flight by an accidental outside click.
- On completion (success or error), `confirmTranslate`'s `finally` block already sets `pendingConfirm = null`, which closes the modal automatically — no new close logic needed. On error, `translateError[id]` is already set (Task 7) and renders inline in the row's expanded body as before.

- [ ] **Step 3: Type-check**

Run: `cd /Users/cding/Workspace/ChenWeb/web && bun run check 2>&1 | tail -40`
Expected: no new errors in this file.

- [ ] **Step 4: Commit**

Scope the commit to only this file (there may be other unrelated dirty files in the shared working copy):

```bash
cd /Users/cding/Workspace/ChenWeb
jj commit web/src/lib/components/home3/doc-review-results-view.svelte -m "Replace inline translate-confirm prompt with a modal dialog and add an in-progress spinner"
```

- [ ] **Step 5: Manual verification (by the controller/user, not the implementer)**

Reload the Document Review panel and confirm:
1. Triggering a translate-confirmation (via expanding an untranslated row, or via the pulldown/page-level selector) now shows a modal dialog centered on screen with a dimmed backdrop, not inline text in the row.
2. Clicking **Translate** in the modal shows a spinner + "Translating to X…" while the request is in flight, and the modal closes automatically when it completes (success or failure).
3. Clicking **Cancel**, or clicking the backdrop, closes the modal without translating (only while not actively translating).
4. On failure, the modal still closes and the existing inline "Translation failed: ..." message (Task 7) appears in the row's expanded body.
