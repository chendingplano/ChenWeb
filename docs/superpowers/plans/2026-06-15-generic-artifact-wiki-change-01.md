# Generic Artifact Wiki Change 01 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a generic search-detail wiki flow in ChenWeb so search results open a shared artifact wiki route keyed by `artifact_type + artifact_id`, with metrics implemented first, per-language caching/translation, and an immediate right-panel grounded inspector.

**Architecture:** Replace the current metric-only wiki route and response shape with a generic artifact wiki contract. The backend will introduce a generic handler that dispatches by `artifact_type`, reusing the existing metric compile/generate path through an adapter first. The frontend will add a generic artifact wiki page that always renders a grouped grounded-record inspector immediately while the left article panel loads, generates, or translates.

**Tech Stack:** Go 1.25, Echo, Svelte 5, TypeScript, Bun, existing `kb.search_artifacts` registry, existing metric wiki cache under `ARTIFACT_DIR`.

---

## Chunk 1: Contract And Backend Foundation

### Task 1: Lock the generic wiki API shape with failing Go tests

**Files:**
- Create: `server/api/kbhandler/artifact_wiki_handler_test.go`
- Modify: `server/api/kbhandler/metric_wiki_handler_test.go`
- Reference: `server/api/kbhandler/metric_wiki_generate_test.go`
- Reference: `KnowledgeStore/doc-repo/adrs/202606/2026061504-adr-search-detail-page.md`

- [ ] **Step 1: Write failing tests for the generic handler contract**

Add tests that cover:
- `GET /api/v1/kb/artifacts/wiki?artifact_type=metric&artifact_id=5_mtc_3`
- bad request for missing `artifact_type`
- bad request for missing `artifact_id`
- unsupported `artifact_type`
- cache hit returning `article`, `record`, `source_document`, and `generated`

Sketch:

```go
func TestGetArtifactWikiMetricCacheHit(t *testing.T) {
	// Arrange temp ARTIFACT_DIR and a cached metric article file.
	// Stub metric adapter lookup if needed.
	// Assert the generic envelope carries artifact_type/artifact_id and grounded record.
}
```

- [ ] **Step 2: Run the targeted Go tests to verify they fail**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetArtifactWiki|TestGetMetricWiki' -count=1
```

Expected: FAIL because the generic handler and generic response type do not exist yet.

- [ ] **Step 3: Implement the minimal generic backend types and handler**

Create a generic handler and shared response types, likely in new focused files:
- `server/api/kbhandler/artifact_wiki_handler.go`
- `server/api/kbhandler/artifact_wiki_types.go`

Implementation responsibilities:
- parse `artifact_type`, `artifact_id`, and `lang`
- default `lang` to `en`
- validate supported languages
- dispatch to a provider/adapter for `metric`
- return a generic envelope, not a metric-only payload

- [ ] **Step 4: Register the new route**

Modify `server/api/routes.go` to add:

```go
apiGroup.GET("/kb/artifacts/wiki", kbhandler.GetArtifactWiki)
```

Keep the existing metric wiki route alive until the frontend migration is complete.

- [ ] **Step 5: Re-run the targeted Go tests**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetArtifactWiki|TestGetMetricWiki' -count=1
```

Expected: PASS for the new generic contract tests, with legacy metric wiki tests still passing or intentionally adjusted.

- [ ] **Step 6: Commit**

```bash
git add server/api/routes.go server/api/kbhandler/artifact_wiki_handler.go server/api/kbhandler/artifact_wiki_types.go server/api/kbhandler/artifact_wiki_handler_test.go server/api/kbhandler/metric_wiki_handler_test.go
git commit -m "feat: add generic artifact wiki api contract"
```

### Task 2: Extract a metric provider behind the generic artifact contract

**Files:**
- Create: `server/api/kbhandler/artifact_wiki_metric_provider.go`
- Modify: `server/api/kbhandler/metric_wiki_compile.go`
- Modify: `server/api/kbhandler/metric_wiki_generate.go`
- Modify: `server/api/kbhandler/metric_wiki_handler.go`
- Test: `server/api/kbhandler/metric_wiki_generate_test.go`
- Test: `server/api/kbhandler/artifact_wiki_handler_test.go`

- [ ] **Step 1: Write failing tests for the metric provider output**

Add tests that verify the metric adapter can produce:
- generic article payload
- grounded full metric record
- source document metadata
- generated metadata with `lang`

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetArtifactWiki|TestAssembleMetricWikiPage' -count=1
```

Expected: FAIL because the metric provider does not yet map to the generic response shape.

- [ ] **Step 3: Implement the metric provider/adapter**

Refactor the metric wiki code so the generic handler can call a metric provider that:
- resolves `artifact_id` as the canonical metric id
- loads the full grounded `metricRecord`
- loads source document metadata
- returns the LLM article payload in a generic article field

Keep the metric-specific prompt and compilation logic. Do not duplicate the LLM generation path.

- [ ] **Step 4: Keep the legacy metric route delegating to shared logic**

Either:
- have `GetMetricWiki` call the new shared provider and translate the response back to the legacy metric shape, or
- leave it temporarily intact but powered by the same lower-level functions

This keeps the migration safe while the frontend changes land.

- [ ] **Step 5: Re-run the targeted Go tests**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetArtifactWiki|TestAssembleMetricWikiPage|TestGetMetricWiki' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/api/kbhandler/artifact_wiki_metric_provider.go server/api/kbhandler/metric_wiki_compile.go server/api/kbhandler/metric_wiki_generate.go server/api/kbhandler/metric_wiki_handler.go server/api/kbhandler/metric_wiki_generate_test.go server/api/kbhandler/artifact_wiki_handler_test.go
git commit -m "refactor: adapt metric wiki to generic artifact provider"
```

### Task 3: Add language-specific translation fallback from English cache

**Files:**
- Create: `server/api/kbhandler/artifact_wiki_translate.go`
- Modify: `server/api/kbhandler/artifact_wiki_handler.go`
- Modify: `server/api/kbhandler/metric_wiki_generate.go`
- Test: `server/api/kbhandler/artifact_wiki_handler_test.go`

- [ ] **Step 1: Write failing tests for translation/cache behavior**

Cover:
- target language cache hit
- target language cache miss with English cache present => translate from English cache
- no cache present => generate English, then translate for non-English request

- [ ] **Step 2: Run the targeted tests to verify they fail**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetArtifactWiki.*Lang|TestGetArtifactWiki.*Translate' -count=1
```

Expected: FAIL because no generic translation path exists yet.

- [ ] **Step 3: Implement translation/cache flow**

Implementation guidance:
- keep cache key at `artifact_type + artifact_id + lang`
- prefer English cache as translation source
- only fall back to fresh English generation when no English cache exists
- save translated target-language cache to disk before returning

If the existing metric prompt hardcodes English, keep that for source generation and isolate translation logic separately.

- [ ] **Step 4: Re-run the targeted tests**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetArtifactWiki.*Lang|TestGetArtifactWiki.*Translate' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/api/kbhandler/artifact_wiki_translate.go server/api/kbhandler/artifact_wiki_handler.go server/api/kbhandler/metric_wiki_generate.go server/api/kbhandler/artifact_wiki_handler_test.go
git commit -m "feat: add translated artifact wiki cache flow"
```

## Chunk 2: Frontend Route, Service, And Inspector

### Task 4: Add the generic artifact wiki client service and URL helpers

**Files:**
- Create: `web/src/lib/services/artifactWikiService.ts`
- Modify: `web/src/lib/services/kbArtifactSearch.ts`
- Modify: `web/src/lib/services/kbArtifactSearch.test.ts`
- Reference: `web/src/lib/services/metricWikiService.ts`

- [ ] **Step 1: Write failing frontend tests for URL helpers**

Add tests for helper functions such as:
- `buildArtifactWikiHref('metric', '5_mtc_3', 'en')`
- preserving `dark=0`
- preserving selected `lang`

- [ ] **Step 2: Run the targeted frontend tests to verify they fail**

Run:

```bash
cd web && bun test src/lib/services/kbArtifactSearch.test.ts
```

Expected: FAIL because generic wiki URL helpers do not exist yet.

- [ ] **Step 3: Implement the generic service**

Create:
- typed generic response model
- `getArtifactWiki(artifactType, artifactId, lang?)`
- URL builder for the `kb-artifact-wiki` route

Avoid baking metric-only assumptions into the service.

- [ ] **Step 4: Re-run the targeted frontend tests**

Run:

```bash
cd web && bun test src/lib/services/kbArtifactSearch.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/services/artifactWikiService.ts web/src/lib/services/kbArtifactSearch.ts web/src/lib/services/kbArtifactSearch.test.ts
git commit -m "feat: add generic artifact wiki client service"
```

### Task 5: Replace metric-only result links with generic artifact wiki links

**Files:**
- Modify: `web/src/lib/components/home3/kb-search-results-view.svelte`
- Modify: `web/src/routes/home3/knowledge/+page.svelte`
- Test: `web/src/lib/services/kbArtifactSearch.test.ts`

- [ ] **Step 1: Write a failing test or helper assertion for generic result href behavior**

If this component lacks direct component tests, move the result-link logic into a small helper module and test that helper instead.

- [ ] **Step 2: Run the relevant frontend tests to verify failure**

Run:

```bash
cd web && bun test src/lib/services/kbArtifactSearch.test.ts
```

Expected: FAIL because only metric results deep-link today.

- [ ] **Step 3: Implement the generic link behavior**

Changes:
- update search result href generation to use `artifact_type + artifact_id + lang`
- add `kb-artifact-wiki` to `KbSectionId`
- parse `artifact_type`, `artifact_id`, and `lang` from `page.url.searchParams`
- render the generic wiki section from `home3/knowledge/+page.svelte`

- [ ] **Step 4: Re-run the relevant frontend tests**

Run:

```bash
cd web && bun test src/lib/services/kbArtifactSearch.test.ts
cd web && bun run check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/components/home3/kb-search-results-view.svelte web/src/routes/home3/knowledge/+page.svelte web/src/lib/services/kbArtifactSearch.test.ts
git commit -m "feat: route search results to generic artifact wiki"
```

### Task 6: Build the generic two-panel wiki page with immediate right-panel loading

**Files:**
- Create: `web/src/lib/components/home3/artifact-record-inspector.svelte`
- Create: `web/src/lib/components/home3/artifact-record-groups.ts`
- Create: `web/src/lib/components/home3/artifact-wiki-page.svelte`
- Modify: `web/src/lib/components/home3/metric-wiki-view.svelte`
- Modify: `web/src/routes/home3/knowledge/+page.svelte`
- Reference: `web/src/lib/components/home3/editable-metadata-section.svelte`
- Reference: `web/src/lib/components/home3/metric-mgmt-view.svelte`

- [ ] **Step 1: Write failing tests for the grouping helpers**

Because Svelte component testing may not already be set up here, keep the logic testable by putting grouping/formatting into `artifact-record-groups.ts` with unit tests if needed.

Test expectations:
- metric record fields are grouped into readable sections
- JSON fields remain visible
- empty fields are either omitted or explicitly rendered according to the chosen helper contract

- [ ] **Step 2: Run the targeted frontend tests to verify they fail**

Run:

```bash
cd web && bun test src/lib/services/kbArtifactSearch.test.ts
```

Expected: FAIL once the new grouping helper tests are added.

- [ ] **Step 3: Implement the generic page and grouped inspector**

Requirements:
- left panel loads article via `getArtifactWiki`
- right panel renders from grounded `record` immediately
- the loading state must not block the right panel
- grouped inspector must show all metric fields, including JSON-backed fields such as `source_line_spans`, `metric_keywords`, `metric_keywords_en`, and `reasoning_tags`

Prefer a reusable layout component rather than stretching `metric-wiki-view.svelte` further.

- [ ] **Step 4: Re-run type and test checks**

Run:

```bash
cd web && bun test src/lib/services/kbArtifactSearch.test.ts
cd web && bun run check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/components/home3/artifact-record-inspector.svelte web/src/lib/components/home3/artifact-record-groups.ts web/src/lib/components/home3/artifact-wiki-page.svelte web/src/lib/components/home3/metric-wiki-view.svelte web/src/routes/home3/knowledge/+page.svelte
git commit -m "feat: add generic artifact wiki page and grouped inspector"
```

## Chunk 3: Compatibility, Verification, And Rollout Guardrails

### Task 7: Preserve or intentionally retire the legacy metric wiki path

**Files:**
- Modify: `web/src/lib/services/metricWikiService.ts`
- Modify: `server/api/kbhandler/metric_wiki_handler.go`
- Test: `server/api/kbhandler/metric_wiki_handler_test.go`

- [ ] **Step 1: Decide the compatibility posture in code**

Either:
- keep the legacy metric endpoint working and internally delegating to generic logic, or
- reduce it to a compatibility shim while the frontend fully migrates

Do not break existing tests silently.

- [ ] **Step 2: Add or adjust failing tests to lock the chosen behavior**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetMetricWiki' -count=1
```

Expected: FAIL if the refactor broke legacy assumptions.

- [ ] **Step 3: Implement the compatibility fix**

Make the legacy metric path either:
- preserve its existing JSON shape for callers, or
- explicitly redirect only if that is already acceptable in the codebase

- [ ] **Step 4: Re-run the legacy metric wiki tests**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetMetricWiki' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/services/metricWikiService.ts server/api/kbhandler/metric_wiki_handler.go server/api/kbhandler/metric_wiki_handler_test.go
git commit -m "fix: preserve metric wiki compatibility during generic rollout"
```

### Task 8: Run end-to-end verification for the metrics-first slice

**Files:**
- Verify only

- [ ] **Step 1: Run backend package tests**

Run:

```bash
go test ./server/api/kbhandler -count=1
```

Expected: PASS.

- [ ] **Step 2: Run focused frontend tests and type checks**

Run:

```bash
cd web && bun test src/lib/services/kbArtifactSearch.test.ts
cd web && bun run check
```

Expected: PASS.

- [ ] **Step 3: Build the server and frontend**

Run:

```bash
mise build-server
mise build-web
```

Expected: both builds succeed.

- [ ] **Step 4: Smoke-test the app manually**

Run the app:

```bash
mise dev
```

Manual checklist:
- open `/deep-wiki`
- submit a search with known metric results
- click a metric result and verify it opens `section=kb-artifact-wiki`
- verify the right panel appears immediately
- verify the left panel eventually shows cached/generated English content
- repeat with `lang=zh-cn` and verify translation/cache behavior

- [ ] **Step 5: Commit the verification-safe state**

```bash
git status
git add -A
git commit -m "test: verify generic artifact wiki metrics slice"
```

## Chunk 4: Follow-Through Notes For Post-Metrics Expansion

### Task 9: Stage the expansion points for other artifact types without implementing them yet

**Files:**
- Modify: `server/api/kbhandler/artifact_wiki_handler.go`
- Modify: `server/api/kbhandler/artifact_wiki_types.go`
- Modify: `KnowledgeStore/doc-repo/adrs/202606/2026061504-adr-search-detail-page.md`

- [ ] **Step 1: Add explicit TODO-style dispatch boundaries in code**

Keep the generic handler structured like:

```go
switch artifactType {
case "metric":
	return metricProvider(...)
default:
	return unsupportedArtifactType(...)
}
```

This keeps future onboarding of `summary`, `topic`, `scene_block`, `provision`, `product`, `inventory_item`, `entity`, `relation`, and `semantic_projection` straightforward.

- [ ] **Step 2: Verify unsupported types fail cleanly**

Run:

```bash
go test ./server/api/kbhandler -run 'TestGetArtifactWiki.*Unsupported' -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add server/api/kbhandler/artifact_wiki_handler.go server/api/kbhandler/artifact_wiki_types.go KnowledgeStore/doc-repo/adrs/202606/2026061504-adr-search-detail-page.md
git commit -m "chore: prepare generic artifact wiki for additional providers"
```

## File Structure Map

- `server/api/routes.go`
  Registers the new generic wiki endpoint.
- `server/api/kbhandler/artifact_wiki_handler.go`
  Owns request parsing, cache orchestration, provider dispatch, and response writing.
- `server/api/kbhandler/artifact_wiki_types.go`
  Defines the generic article/record/source/generated response contract.
- `server/api/kbhandler/artifact_wiki_metric_provider.go`
  Adapts metrics into the generic artifact wiki contract without duplicating generation logic.
- `server/api/kbhandler/artifact_wiki_translate.go`
  Encapsulates English-cache-first translation behavior.
- `server/api/kbhandler/artifact_wiki_handler_test.go`
  Covers generic route validation, cache behavior, translation behavior, and unsupported types.
- `server/api/kbhandler/metric_wiki_*`
  Legacy metric wiki implementation reused behind the generic provider, with compatibility preserved.
- `web/src/lib/services/artifactWikiService.ts`
  Frontend API client and route helpers for the generic wiki flow.
- `web/src/lib/components/home3/kb-search-results-view.svelte`
  Converts search clicks into generic artifact wiki navigation.
- `web/src/routes/home3/knowledge/+page.svelte`
  Adds the `kb-artifact-wiki` section and wires URL params into the generic page.
- `web/src/lib/components/home3/artifact-wiki-page.svelte`
  Generic two-panel artifact wiki page.
- `web/src/lib/components/home3/artifact-record-inspector.svelte`
  Curated grouped inspector for grounded artifact records.
- `web/src/lib/components/home3/artifact-record-groups.ts`
  Testable grouping/formatting logic for all record fields.

## Notes

- Do not introduce a database migration unless implementation proves the current cache metadata is insufficient.
- Keep the first slice metrics-only at the provider layer, not the route or contract layer.
- If `metric-wiki-view.svelte` becomes a compatibility burden, replace it with a thin wrapper over the generic page instead of extending the metric-specific layout further.
- After implementation, update docs that answer:
  - what knowledge changed
  - which docs/specs/tests changed
  - which docs are stale
  - what was intentionally left undocumented
