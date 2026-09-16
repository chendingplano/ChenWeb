# Past Reviews Sorting and Filtering Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add server-backed sorting, OR keyword filtering, and case-insensitive product-name filtering to the Product Review Past reviews list.

**Architecture:** Extend the existing profile-list endpoint with validated sort/filter query parameters and a tenant-scoped keyword vocabulary in the same response. The Svelte intake component owns toolbar state, debounces name changes, and reloads profiles while preserving existing card selection and re-run flows. The backend performs filtering and ordering before applying the existing bounded limit.

**Tech Stack:** Go, Echo, PostgreSQL JSONB, sqlmock, Svelte 5, TypeScript, Node test runner, Paraglide i18n.

---

## File map

- Modify `server/api/product-reviews/handler.go`: parse and normalize list query parameters and include keyword vocabulary in the response.
- Modify `server/api/product-reviews/store.go`: add safe sort mapping, filtered profile query, and distinct keyword vocabulary query.
- Modify `server/api/product-reviews/models.go`: add list query/result types only if existing types cannot express the new response fields.
- Modify `server/api/product-reviews/handler_test.go`: test query parsing/defaults and keyword normalization.
- Modify `server/api/product-reviews/store_test.go`: test query shape, arguments, filter predicates, ordering, and vocabulary results.
- Modify `web/src/lib/services/productMetricReviewService.ts`: serialize sort, keywords, name, and limit; type the response vocabulary.
- Modify `web/src/lib/services/productMetricReviewService.test.ts`: test query serialization and response decoding.
- Modify `web/src/lib/components/home3/product-review-intake-view.svelte`: add the layout-A toolbar, keyword picker/chips, debounced name filter, loading/race protection, and filtered empty state.
- Modify `web/messages/en.json` and `web/messages/zh-cn.json`: add labels, sort options, filter placeholders, and empty-state strings.
- Regenerate Paraglide output using the repository’s existing web message generation command; do not hand-edit generated output.
- Modify the next Product Metric Reviewer manual revision under `KnowledgeStore/doc-repo/user-manuals/` to document the controls.

## Chunk 1: Backend query contract

### Task 1: Add pure query parsing and normalization tests

**Files:**
- Test: `server/api/product-reviews/handler_test.go`
- Modify: `server/api/product-reviews/handler.go`

- [ ] Step 1: Add table-driven tests for absent/invalid sort fallback, all six valid sort values, whitespace trimming, empty keyword removal, duplicate keyword removal, comma-separated keyword values, and name trimming.
- [ ] Step 2: Run `cd server && go test ./api/product-reviews -run 'Test(Parse|List).*Filter|Test.*Sort' -count=1`; verify the new tests fail because the helpers/fields do not exist.
- [ ] Step 3: Implement small pure helpers that parse the query into a struct, whitelist sort values, normalize keywords, and omit empty values. Keep `limit` parsing compatible with the existing helper.
- [ ] Step 4: Run the focused handler tests again; expect PASS.

### Task 2: Implement filtered/sorted profile listing

**Files:**
- Modify: `server/api/product-reviews/store.go`
- Test: `server/api/product-reviews/store_test.go`

- [ ] Step 1: Add sqlmock tests for each sort expression, `ILIKE` name matching, keyword OR matching through JSONB element extraction, tenant scoping, filter-before-limit ordering, and stable id tie-breaking.
- [ ] Step 2: Add a sqlmock test for the distinct tenant keyword vocabulary, including empty keyword exclusion and deterministic alphabetical ordering.
- [ ] Step 3: Run `cd server && go test ./api/product-reviews -run 'Test(ListProfiles|ListProfileKeywords)' -count=1`; verify failures identify the missing query behavior.
- [ ] Step 4: Change `Store.ListProfiles` to accept the parsed list query, construct only the `ORDER BY` clause from a fixed map, and pass all user values as SQL parameters. Use `EXISTS (SELECT 1 FROM jsonb_array_elements_text(COALESCE(p.keywords, '[]'::jsonb)) ...)` with OR semantics.
- [ ] Step 5: Add `Store.ListProfileKeywords` (or an equivalent single store helper) to return the tenant’s distinct non-empty keywords ordered alphabetically.
- [ ] Step 6: Update `ListProductProfiles` to call the filtered profile query and vocabulary query and return both in the JSON envelope. Preserve the existing default/max limit behavior and non-blocking intake semantics for initial list failures.
- [ ] Step 7: Run `cd server && go test ./api/product-reviews -count=1`; expect PASS.

## Chunk 2: Frontend transport and UI

### Task 3: Extend the service client

**Files:**
- Modify: `web/src/lib/services/productMetricReviewService.ts`
- Test: `web/src/lib/services/productMetricReviewService.test.ts`

- [ ] Step 1: Add a failing test that calls `listProfiles({ sort, keywords, name, limit })` and asserts URL encoding, repeated keyword parameters, and omission of empty filters.
- [ ] Step 2: Run `cd web && bun test web/src/lib/services/productMetricReviewService.test.ts` (or the repository’s equivalent focused command); verify it fails with the current signature.
- [ ] Step 3: Update `ProfileSummary` list response typing with `keywords` vocabulary and change `listProfiles` to accept an options object while preserving a compatibility overload only if existing callers require it.
- [ ] Step 4: Implement deterministic query serialization with `URLSearchParams`; use one `keywords` parameter per selected keyword and encode the name filter.
- [ ] Step 5: Run the focused service tests; expect PASS.

### Task 4: Add toolbar state and reload behavior

**Files:**
- Modify: `web/src/lib/components/home3/product-review-intake-view.svelte`

- [ ] Step 1: Add component tests or focused interaction coverage for default sort, selecting/removing keywords, immediate sort/keyword reload, debounced name reload, stale-response suppression, and no-match empty state.
- [ ] Step 2: Run the focused component tests before implementation and confirm the new behavior is absent.
- [ ] Step 3: Add state for sort, selected keywords, keyword vocabulary, name filter, list request sequence/token, and debounce timer. Initialize the default to time descending.
- [ ] Step 4: Replace the one-shot `loadProfiles()` call with a parameterized loader that requests the current filters, stores returned profiles/vocabulary, and ignores responses whose request token is no longer current.
- [ ] Step 5: Wire sort and keyword changes to immediate loading. Wire name changes to a short debounce and clear the timer on component teardown if necessary.
- [ ] Step 6: Render layout A below the Past reviews heading: sort select, keyword multi-select/popover with removable chips, and name input. Keep controls keyboard accessible and compatible with both light/dark themes.
- [ ] Step 7: Distinguish no profiles from no filtered matches, preserve the selected-card and nested View results click behavior, and ensure a selected profile that disappears due to filtering does not cause an invalid re-run.
- [ ] Step 8: Run the focused frontend tests and manually inspect the control at narrow and wide widths.

### Task 5: Add localized strings

**Files:**
- Modify: `web/messages/en.json`
- Modify: `web/messages/zh-cn.json`
- Regenerate: repository Paraglide generated files via existing command

- [ ] Step 1: Add strings for Sort, Filter by keywords, Filter by name, all six options, keyword picker empty/selected states, clear/remove actions, and filtered no-match state in both locales.
- [ ] Step 2: Run the project’s message compilation command and verify generated imports used by the component exist.
- [ ] Step 3: Run `cd web && bun run check`; fix only errors introduced by this change.

## Chunk 3: Verification and documentation

### Task 6: Update user documentation

**Files:**
- Create: next versioned `KnowledgeStore/doc-repo/user-manuals/product-metric-reviewer-v*.md` according to the existing versioning convention, or modify the in-progress current revision if that is the repository convention at implementation time.

- [ ] Step 1: Document the six sort choices, that sorting applies to the whole returned list, OR behavior for multiple keywords, and case-insensitive substring name matching.
- [ ] Step 2: Add troubleshooting guidance for an empty filtered result and preserve older manual revisions unchanged.

### Task 7: Run verification and review the diff

**Files:**
- No additional source files.

- [ ] Step 1: Run `cd server && go test ./api/product-reviews ./...` and `go vet ./...`.
- [ ] Step 2: Run `cd web && bun test`, `bun run check`, and `bun run build`.
- [ ] Step 3: Run the app with `mise dev` and manually verify default ordering, each sort mode, multi-keyword OR behavior, name substring matching, clearing filters, no-match state, card selection, and View results.
- [ ] Step 4: Inspect `git diff` and `jj status`; confirm no generated or unrelated files changed unexpectedly.
- [ ] Step 5: Commit the implementation and documentation through `jj`, then run `jj log --no-graph -r 'ancestors(@, 4)'` to confirm the expected linear history.

