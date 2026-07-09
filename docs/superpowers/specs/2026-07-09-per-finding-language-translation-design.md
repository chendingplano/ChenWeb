# Per-Finding Language Pulldown + On-Demand Translation

## Goal
Add a per-finding **Language** pulldown, placed immediately before the **Accept** button, on both doc-review findings views:
1. `doc-review-results-view.svelte` (the "Document Review" side panel).
2. `doc-review-report/[id]/+page.svelte` (the full report view, packages and severity layouts).

Switching a row's language displays that finding's title/description/suggestion in the selected language. If no translation exists yet for that language:
- `AUTO_TRANSLATE_FINDINGS=true` → translate automatically, no prompt.
- otherwise → ask the user to confirm before translating.

Supported languages are the list configured in `ChenWeb/config.toml::[frontend].supported_languages`.

**Guiding principle: reuse the existing translation implementation end-to-end. This is a new on-demand entry point into it, not new translation logic.** `llmFindingTranslator.TranslateFinding`, the four `prompt-doc-review-finding-*` prompts, and `FindingMetadataEnvelope`/`FindingLocalizedContent` storage are unchanged and fully reused.

## Current state (relevant pieces)
- `kb.doc_review_findings.metadata` (JSONB) stores per-language content as a flat map: `{"schema_version":1,"source_language":"en","canonical_language":"en","canonical_origin":"original","en":{"title":...,"description":...,"suggestion":...,"provenance":"canonical"}, "zh-cn": {...}}` (see `FindingMetadataEnvelope.MarshalJSON`/`UnmarshalJSON`, `server/api/doc-reviews/models.go:104-211`).
- Translation only currently happens in bulk, at finding-save time (`prepareFindingForStorage`, `finding_translation.go:506-671`), gated by the existing `DOC_REVIEW_TRANSLATION` env var (`"auto"` vs `"on-demand"`, `models.go`/`finding_translation.go:357-367`). There is no per-finding, on-demand translation endpoint today.
- `finding_translation.go:734-759` already has `(c *DocReviewController) localizeFinding` / `localizeFindings`, which look up a language in stored `metadata` and apply it (`translationFromMetadata`, `applyFindingTranslation`) — but never call the LLM; missing languages just fall back to unlocalized text.
- `llmFindingTranslator.TranslateFinding(ctx, language, finding FindingItem) (FindingLocalizedContent, error)` (`finding_translation.go:175`) is fully implemented and is exactly the call a new on-demand endpoint needs — it's just never invoked outside the bulk save path.
- Accept/Reject wiring for reference (same pattern the new pulldown follows): `PATCH /api/v1/doc-review/findings/:id` → `handler.go:258` → `controller.go:500 UpdateFinding` → `web/src/lib/services/docReviewService.ts:171 updateFinding`.
- `config.toml` already has `[frontend].supported_languages = ["en", "zh-cn", "ja", "ko"]` but no `default_language`. It's read by `kbhandler.loadKbFrontendConfig` (`kb_config_handler.go:68`, unexported) and served via `GET /api/v1/kb/config`, consumed today only by the unrelated KB-search language filter (`kb-search-results-view.svelte`).
- `doc-review-report/[id]/+page.svelte` already has a page-level language `<select>` (`selectedLanguage`, lines 65-66, 545-554) whose options come from `listLanguages()` → `GET /doc-review/languages` (env-var `DOC_REVIEW_REPORT_LANGUAGE`-driven — a separate, unrelated mechanism kept as-is). Its default currently hardcodes `'en'`.
- `doc-review-results-view.svelte` has no page-level language state today.

## Backend changes

### Config
`ChenWeb/config.toml`:
```toml
[frontend]
topic_types = [...]
supported_languages = ["en", "zh-cn", "ja", "ko"]
default_language = "en"
```

`server/api/kbhandler/kb_config_handler.go`:
- `kbFrontendConfig` gets `DefaultLanguage string \`json:"default_language"\``.
- `rawKbFrontendSection.Frontend` gets `DefaultLanguage string \`toml:"default_language"\``.
- `loadKbFrontendConfig` sets `DefaultLanguage` from the parsed value, falling back to `"en"` if empty — same pattern as the existing `supported_languages` fallback.
- Export the loader as `LoadKbFrontendConfig` (rename, update the one call site in `GetKbFrontendConfig`) so `doc-reviews` can call it too. No import cycle: `kbhandler` currently imports nothing under `server/api`.

### New endpoint
`POST /api/v1/doc-review/findings/:id/translate`

Request body: `{ "language": "zh-cn", "confirm": false }`

Response: `{ "status": true, "finding": <FindingItem>, "translated": bool, "needs_confirmation": bool }` (or `{"status": false, "error_msg": "..."}` on failure, matching existing endpoint conventions).

Route (`server/api/routes.go`, alongside the other finding-scoped routes at line ~549):
```go
apiGroup.POST("/doc-review/findings/:id/translate", docreviews.TranslateFinding)
```

Handler (`server/api/doc-reviews/handler.go`, new function alongside `UpdateFinding`):
- Parses `:id`, binds `{Language string; Confirm bool}`, calls `ctrl.TranslateFinding(ctx, id, language, confirm)`, returns the JSON envelope above.

Controller (`server/api/doc-reviews/controller.go`, new method `TranslateFinding`):
1. `language := supportedLanguageCode(req.Language)`; validate against `kbhandler.LoadKbFrontendConfig().SupportedLanguages` → 400 `"unsupported language"` if not present.
2. `SELECT id, pass, aspect, severity, finding_type, title, description, COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''), COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata,'{}'::jsonb)::text, COALESCE(artifact_id,'') FROM kb.doc_review_findings WHERE id = $1` (same column set as the existing findings-list query in `GetRequestWithFindings`) → 404 if no row.
3. `if tr, ok := translationFromMetadata(metadataBytes, language); ok` → return `applyFindingTranslation(finding, tr)`, `translated: false`, `needs_confirmation: false`. **No LLM call — this is the common/fast path** (language already translated, or user re-selecting a language they already viewed).
4. Otherwise, missing translation:
   - `autoTranslate := strings.EqualFold(strings.TrimSpace(os.Getenv("AUTO_TRANSLATE_FINDINGS")), "true")` (new env var, read the same way `DOC_REVIEW_TRANSLATION` is read elsewhere in `finding_translation.go`; independent of it).
   - If `!autoTranslate && !req.Confirm` → return the **original**, unmodified finding, `translated: false`, `needs_confirmation: true`. No LLM call.
   - Else (`autoTranslate` or `req.Confirm`):
     - `translator, err := newLLMFindingTranslator()` (existing constructor, reused as-is).
     - `content, err := translator.TranslateFinding(ctx, language, finding)` (existing method, reused as-is) — `finding` is the canonical-language row just loaded in step 2, matching how the bulk path always translates from canonical content.
     - On success, set `content.Provenance = "llm_translation"` if empty (matches bulk-path convention), then persist with a single-key JSONB merge to avoid clobbering other languages under concurrent requests:
       ```sql
       UPDATE kb.doc_review_findings
       SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object($1::text, $2::jsonb)
       WHERE id = $3
       ```
       (`$1` = language code, `$2` = `content` marshaled to JSON, `$3` = finding id).
     - Return `applyFindingTranslation(finding, content)`, `translated: true`, `needs_confirmation: false`.
     - On translator error → 500, `error_msg` from the error (matches existing error handling elsewhere in the package).

No changes to `prepareFindingForStorage`, `NormalizeFinding`, prompts, or the bulk save path — this is purely a new read/write entry point around the existing `metadata` column and existing `TranslateFinding` method.

## Frontend changes

### Shared language config (`web/src/lib/services/kbService.ts`)
Add:
```ts
export async function getKbFrontendConfig(): Promise<{
	topic_types: string[];
	supported_languages: string[];
	default_language: string;
	mandatory_processors: string[];
	required_processors: string[];
	max_doc_process_pipelines: number;
}> { ... }
```
wrapping `GET /api/v1/kb/config` (same envelope shape already handled inline in `kb-search-results-view.svelte`). `kb-search-results-view.svelte` is not touched — only the new call sites below use this helper.

### `docReviewService.ts`
Add:
```ts
export async function translateFinding(
	id: number,
	language: string,
	confirm = false
): Promise<{ finding: FindingItem; translated: boolean; needs_confirmation: boolean }> { ... }
```
`POST ${BASE}/findings/${id}/translate`, body `{ language, confirm }`, same error-handling convention as `updateFinding`.

### Per-row Language pulldown (both views)
Placement: inside the same actions container as Accept/Reject, immediately before the Accept button — `stopPropagation` on its container click like the existing buttons (so it doesn't trigger row expand/collapse in `doc-review-results-view.svelte`). Rendered unconditionally (not only when `review_status === 'pending'`), so translations remain viewable/switchable after a finding is accepted/rejected.

Options: `supported_languages` from `getKbFrontendConfig()`, fetched once on mount in each component.

Initial per-row value:
- `doc-review-results-view.svelte`: `default_language` from `getKbFrontendConfig()` (fallback `'en'`), since this view has no existing page-level language state.
- `+page.svelte`: the page's existing `selectedLanguage`, whose own fallback (currently hardcoded `'en'` in the `onMount` catch block and the "language not in list" branch, lines ~309-315) changes to `default_language` from `getKbFrontendConfig()`.

State added to each component:
- `findingLanguage: Record<number, string>` — currently-displayed language per finding id.
- `translating: Record<number, boolean>` — per-row in-flight flag (disables the pulldown, matches the existing `busyId` convention in `+page.svelte`).
- `pendingConfirm: { id: number; language: string } | null` — the one row (if any) currently showing the inline confirm prompt.

On pulldown change (`handleLanguageChange(finding, newLanguage)`):
1. `translating[finding.id] = true`; call `translateFinding(finding.id, newLanguage, false)`.
2. If `needs_confirmation` → set `pendingConfirm = { id: finding.id, language: newLanguage }`; the row's Accept/Reject (or status text) area is temporarily replaced with: `"No {newLanguage} translation exists. Translate now?"` + **Translate** / **Cancel** buttons.
   - **Cancel** → clear `pendingConfirm`, revert the pulldown's displayed value to `findingLanguage[finding.id]` (the previous selection).
   - **Translate** → call `translateFinding(finding.id, newLanguage, true)`, then proceed as step 3 below; on error, show a toast (existing `showToast`/`error` pattern per component) and revert the pulldown.
3. Otherwise → patch the row in place: `findings = findings.map(f => f.id === finding.id ? { ...f, title: resp.finding.title, description: resp.finding.description, suggestion: resp.finding.suggestion } : f)` (mirrors the existing `handleAcceptReject` update pattern), set `findingLanguage[finding.id] = newLanguage`, clear `pendingConfirm` if it was set for this row.
4. `translating[finding.id] = false` in all cases.

Only `title`, `description`, `suggestion` change with language — `evidence` and `location` are original source-document quotes and are never translated (matches `FindingLocalizedContent`'s fields).

In `+page.svelte`, changing the page-level `selectedLanguage` dropdown still triggers the existing `reloadFindingsForLanguage()` full refetch, which repopulates `findings` from the server and therefore resets every row's `findingLanguage` back to the (new) page-level language — per-row overrides are only a lightweight display layer on top of the currently-loaded data, not persisted client state.

## Out of scope
- No changes to the bulk/save-time translation path (`prepareFindingForStorage`, `DOC_REVIEW_TRANSLATION` env var, `NormalizeFinding`) or its prompts.
- No changes to `+page.svelte`'s existing page-level `selectedLanguage` reload mechanism (`reloadFindingsForLanguage`, `GET /doc-review/languages`) beyond its default-value fallback.
- No changes to `kb-search-results-view.svelte`'s language filter.
- No translation of `evidence`/`location` fields.
- No new confirm-modal component — the confirm prompt is an inline replacement within the row's existing actions area, consistent with this view's existing lack of modal usage (e.g. `onDelete` today has no confirmation step at all).
