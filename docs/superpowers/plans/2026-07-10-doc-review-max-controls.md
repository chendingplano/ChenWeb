# Document Review Maximum Controls Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add request review depth, depth-specific per-reviewer soft output limits, and a separate cap on related artifacts sent to LLMs.

**Architecture:** Persist review depth on the immutable request, resolve depth-indexed TOML limits into each `ReviewerConfig`, and enforce them through one synchronized scheduler gate shared by the standard and artifact scheduling paths. Preserve existing retrieval breadth while slicing ranked matches only at LLM payload construction.

**Tech Stack:** Go 1.25, PostgreSQL/Goose SQL migrations, Echo, TOML, Svelte 5/TypeScript, jj.

**Design:** `docs/superpowers/specs/2026-07-10-doc-review-max-controls-design.md`

---

## File Map

**Create**

- `project_migrations/20260710000001_add_doc_review_request_depth.sql` — request-depth schema change and constraint.
- `server/api/doc-reviews/review-output-limits.go` — deterministic finding classification and synchronized work gate.
- `server/api/doc-reviews/review-output-limits_test.go` — gate, overshoot, skipped-index, progress, and cancellation tests.
- `server/api/doc-reviews/review-match-limit.go` — environment-backed LLM-only match slicing.
- `server/api/doc-reviews/review-match-limit_test.go` — exact environment parsing and generic ranking-preserving slice tests.

**Modify: backend contracts/config**

- `server/api/doc-reviews/models.go` — request input/status depth fields.
- `server/api/doc-reviews/controller.go` — validate, persist, load, and propagate depth.
- `server/api/doc-reviews/controller_test.go` and `status_dr15_test.go` — database/API lifecycle tests.
- `server/api/doc-reviews/review-config.go` — global and reviewer arrays, validation, and depth resolution.
- `server/api/doc-reviews/review-config_test.go` — defaults, inheritance, and invalid config tests through an uncached parser.
- `server/api/doc-reviews/review-document.go` — processor/config fields, resolved limits, remove hard retrieval cap, standard scheduler integration.
- `server/api/doc-reviews/review_document_selection_test.go` and `review-metrics_test.go` — propagation and retrieval-cap regression tests.

**Modify: match payloads**

- `server/api/doc-reviews/review-metrics.go` / `_test.go`
- `server/api/doc-reviews/review-provisions.go` / `_test.go`
- `server/api/doc-reviews/review-entities.go` / `_test.go`
- `server/api/doc-reviews/review-inventory-items.go` / `_test.go`

**Modify: scheduling**

- `server/api/doc-reviews/review-artifact-window.go` / `_test.go` — seed/remainder phase queues and exact skipped indexes.
- Every reviewer file returned by `rg -l 'runReviewerConcurrent\(' server/api/doc-reviews -g '*.go'` — pass `ReviewerConfig`, reviewer identity, logger, and record ID into the shared scheduler.
- `server/api/doc-reviews/review-metrics-completeness.go` — adopt the artifact scheduler's new signature.

**Modify: frontend/docs**

- `web/src/lib/services/docReviewService.ts` — depth wire types.
- `web/src/lib/components/home3/document-review-view.svelte` — depth selection, summary, and serialization.
- `doc-review.local.toml` — retain intentional `[10, 20, 30]` testing overrides and clarify comments.
- `KnowledgeStore/doc-repo/adrs/202607/2026071001-adr-doc-review-max-controls.md` — implementation status and operational documentation.

---

## Chunk 1: Configuration, Request Contract, and Match Payloads

### Task 1: Depth-indexed TOML limits

**Files:**
- Modify: `server/api/doc-reviews/review-config.go`
- Create: `server/api/doc-reviews/review-config_test.go`
- Modify: `doc-review.local.toml`

- [ ] **Step 1: Write failing config tests**

Extract an uncached `parseDocReviewConfig(raw []byte) (*DocReviewConfig, error)` used by `loadDocReviewConfig`, then add table-driven tests with complete assertions:

```go
func TestResolveOutputLimits_Defaults(t *testing.T) {
    cfg, err := parseDocReviewConfig(nil)
    if err != nil { t.Fatal(err) }
    gotFindings, gotAnalyses := cfg.ResolveOutputLimits("grammar_spelling", 1)
    if gotFindings != 100 || gotAnalyses != 100 {
        t.Fatalf("limits = %d/%d, want 100/100", gotFindings, gotAnalyses)
    }
}

func TestResolveOutputLimits_PrecedenceAndPartialInheritance(t *testing.T) {
    cfg, err := parseDocReviewConfig([]byte(`
max_findings = [10,20,30]
max_analyses = [11,21,31]
[reviewers.grammar_spelling]
max_findings = [1,2,3]
`))
    if err != nil { t.Fatal(err) }
    gotFindings, gotAnalyses := cfg.ResolveOutputLimits("grammar_spelling", 2)
    if gotFindings != 2 || gotAnalyses != 21 {
        t.Fatalf("limits = %d/%d, want 2/21", gotFindings, gotAnalyses)
    }
}

func TestValidateOutputLimitsRejectsMalformedArrays(t *testing.T) {
    cases := []string{
        `max_findings=[1,2]`,
        `max_analyses=[1,0,3]`,
        "[reviewers.grammar_spelling]\nmax_findings=[1,-1,3]",
        "[reviewers.grammar_spelling]\nmax_analyses=[1,2,3,4]",
    }
    for _, raw := range cases {
        if _, err := parseDocReviewConfig([]byte(raw)); err == nil {
            t.Errorf("parseDocReviewConfig(%q) succeeded, want error", raw)
        }
    }
}
```

Because these tests call the new uncached parser directly, they do not mutate `docReviewCfgOnce`. Add this uncached file-path test for the loader itself:

```go
func TestLoadDocReviewConfigValidatesOutputLimits(t *testing.T) {
    path := filepath.Join(t.TempDir(), "doc-review.local.toml")
    if err := os.WriteFile(path, []byte(`max_findings=[1,2]`), 0o600); err != nil { t.Fatal(err) }
    t.Setenv("DOC_REVIEW_CONFIG_FILE", path)
    if _, err := loadDocReviewConfig(); err == nil || !strings.Contains(err.Error(), "max_findings") {
        t.Fatalf("loadDocReviewConfig error=%v, want max_findings validation error", err)
    }
}
```

- [ ] **Step 2: Run RED**

Run: `go test ./server/api/doc-reviews -run 'TestResolveOutputLimits|TestValidateOutputLimits|TestLoadDocReviewConfigValidatesOutputLimits' -count=1`

Expected: FAIL because limit fields/resolution do not exist.

- [ ] **Step 3: Implement minimal config support**

Add constants and fields:

```go
var defaultReviewOutputLimits = [3]int{100, 200, 300}

type ReviewAspectConfig struct {
    MaxFindings []int `toml:"max_findings"`
    MaxAnalyses []int `toml:"max_analyses"`
}

type DocReviewConfig struct {
    MaxFindings []int `toml:"max_findings"`
    MaxAnalyses []int `toml:"max_analyses"`
}
```

Implement `validateOutputLimitArray(scope, name string, values []int) error`, call it after TOML parsing for root and each reviewer, and implement `ResolveOutputLimits(aspect string, depth int) (int, int)` with built-in → root → reviewer precedence and depth normalized to 1 for defensive internal calls.

- [ ] **Step 4: Run GREEN**

Run: `go test ./server/api/doc-reviews -run 'TestResolveOutputLimits|TestValidateOutputLimits|TestLoadDocReviewConfigValidatesOutputLimits' -count=1`

Expected: PASS.

- [ ] **Step 5: Clarify local testing override**

Keep root values `[10, 20, 30]` and add a comment stating built-in defaults are `[100, 200, 300]` and the local values are deliberate test overrides.

- [ ] **Step 6: Commit**

Run: `jj commit -m 'Add depth-indexed doc review output config' server/api/doc-reviews/review-config.go server/api/doc-reviews/review-config_test.go doc-review.local.toml`

### Task 2: Review-depth schema and request lifecycle

**Files:**
- Create: `project_migrations/20260710000001_add_doc_review_request_depth.sql`
- Modify: `server/api/doc-reviews/models.go`
- Modify: `server/api/doc-reviews/controller.go`
- Modify: `server/api/doc-reviews/controller_test.go`
- Modify: `server/api/doc-reviews/status_dr15_test.go`

- [ ] **Step 1: Write failing request-depth tests**

Use the existing `connectTestDB`, `ensureTables`, `insertTestInput`, `cleanupInputs`, and `cleanupRequests` helpers. Add this complete default/validation/load test structure:

```go
func TestAcceptRequestDefaultsReviewDepthToOne(t *testing.T) {
    db := connectTestDB(t); defer db.Close(); ensureTables(t, db)
    inputID := insertTestInput(t, db, "Review depth default"); defer cleanupInputs(t, db, inputID)
    c := &DocReviewController{DB: db}
    result, err := c.AcceptRequest(context.Background(), SubmitRequestInput{
        InputRecordID: inputID, Tier: "custom", Aspects: []string{"grammar_spelling"}, RequesterName: "test",
    })
    if err != nil { t.Fatal(err) }
    defer cleanupRequests(t, db, result.RequestID)
    req, err := c.GetRequest(context.Background(), result.RequestID)
    if err != nil { t.Fatal(err) }
    if req.ReviewDepth != 1 { t.Fatalf("review_depth=%d, want 1", req.ReviewDepth) }
}

func TestAcceptRequestPersistsAndLoadsReviewDepthThree(t *testing.T) {
    db := connectTestDB(t); defer db.Close(); ensureTables(t, db)
    inputID := insertTestInput(t, db, "Review depth three"); defer cleanupInputs(t, db, inputID)
    c := &DocReviewController{DB: db}
    result, err := c.AcceptRequest(context.Background(), SubmitRequestInput{
        InputRecordID: inputID, Tier: "custom", Aspects: []string{"grammar_spelling"},
        RequesterName: "test", ReviewDepth: 3,
    })
    if err != nil { t.Fatal(err) }
    defer cleanupRequests(t, db, result.RequestID)
    req, err := c.GetRequest(context.Background(), result.RequestID)
    if err != nil { t.Fatal(err) }
    if req.ReviewDepth != 3 { t.Fatalf("review_depth=%d, want 3", req.ReviewDepth) }
}

func TestAcceptRequestRejectsInvalidReviewDepth(t *testing.T) {
    db := connectTestDB(t); defer db.Close(); ensureTables(t, db)
    inputID := insertTestInput(t, db, "Invalid review depth"); defer cleanupInputs(t, db, inputID)
    c := &DocReviewController{DB: db}
    ctx := context.Background()
    for _, depth := range []int{-1, 4} {
        _, err := c.AcceptRequest(ctx, SubmitRequestInput{
            InputRecordID: inputID, Tier: "custom", Aspects: []string{"grammar_spelling"},
            RequesterName: "test", ReviewDepth: depth,
        })
        re, ok := err.(*RequestError)
        if !ok || re.Status != http.StatusUnprocessableEntity {
            t.Fatalf("depth %d error=%v, want 422 RequestError", depth, err)
        }
    }
}
```

Augment existing `TestController_RestartRequest_CreatesNewRunForCompletedRequest`: submit with `ReviewDepth: 3`; after `RestartRequest`, call `c.loadRequest(ctx, result.RequestID, runID)` and assert `req.ReviewDepth == 3`. Add `TestController_RecoverStalledReviewsRetainsReviewDepth` using the same setup, leaving the request accepted/pending, calling `RecoverStalledReviews`, selecting the returned entry for that request ID, then calling `loadRequest` with its new run ID and asserting depth 3. Before that recovery test, mark any unrelated accepted/running test requests stopped so its assertions are isolated; cleanup remains scoped to its own request.

- [ ] **Step 2: Run RED**

Run: `go test ./server/api/doc-reviews -run 'Test.*ReviewDepth|TestController_.*RestartRequest|TestController_.*RecoverStalled' -count=1`

Expected: FAIL because models and SQL lack `review_depth`.

- [ ] **Step 3: Add the Goose migration**

```sql
-- +goose Up
ALTER TABLE kb.doc_review_requests
    ADD COLUMN IF NOT EXISTS review_depth INT NOT NULL DEFAULT 1;
ALTER TABLE kb.doc_review_requests
    DROP CONSTRAINT IF EXISTS chk_doc_review_requests_review_depth;
ALTER TABLE kb.doc_review_requests
    ADD CONSTRAINT chk_doc_review_requests_review_depth
    CHECK (review_depth BETWEEN 1 AND 3);

-- +goose Down
ALTER TABLE kb.doc_review_requests
    DROP CONSTRAINT IF EXISTS chk_doc_review_requests_review_depth;
ALTER TABLE kb.doc_review_requests
    DROP COLUMN IF EXISTS review_depth;
```

Confirm `server/cmd/config/config.go` already resolves `project_migrations` and calls `sharedgoose.RunProjectMigrations`; do not alter startup wiring unless that check fails.

Exact check:

Run: `rg -n 'projectRel = "project_migrations"|RunProjectMigrations' server/cmd/config/config.go`

Expected: one default-directory assignment and one startup `RunProjectMigrations` call.

- [ ] **Step 4: Implement request model and controller behavior**

Add the field shown below to `SubmitRequestInput` and `RequestStatus`:

```go
ReviewDepth int `json:"review_depth"`
```

At the start of `AcceptRequest`, normalize 0 to 1 and reject values outside 1..3 using `RequestError{Status: http.StatusUnprocessableEntity}`. Add the column/value to INSERT. Add the column to `loadRequest` SELECT/Scan. In `RunReview`, set `processor.ReviewDepth = req.ReviewDepth`. Restart and recovery continue through `loadRequest`; do not duplicate depth on run rows.

Update `ensureTables`' `kb.doc_review_requests` DDL with `review_depth INT NOT NULL DEFAULT 1` and add a following idempotent `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` statement so an already-created local test table is upgraded.

- [ ] **Step 5: Run GREEN**

Run: `go test ./server/api/doc-reviews -run 'Test.*ReviewDepth|TestController_.*RestartRequest|TestController_.*RecoverStalled' -count=1`

Expected: PASS.

- [ ] **Step 6: Verify migration shape**

Run: `rg -n 'goose Up|review_depth|CHECK|goose Down' project_migrations/20260710000001_add_doc_review_request_depth.sql`

Expected: Up adds the constrained column and Down removes constraint then column.

- [ ] **Step 7: Commit**

Run: `jj commit -m 'Persist document review depth' project_migrations/20260710000001_add_doc_review_request_depth.sql server/api/doc-reviews/models.go server/api/doc-reviews/controller.go server/api/doc-reviews/controller_test.go server/api/doc-reviews/status_dr15_test.go`

### Task 3: Separate retrieval from LLM match limits

**Files:**
- Modify: `server/api/doc-reviews/review-document.go`
- Create: `server/api/doc-reviews/review-match-limit.go`
- Create: `server/api/doc-reviews/review-match-limit_test.go`
- Modify: `server/api/doc-reviews/review-metrics.go`, `review-metrics_test.go`
- Modify: `server/api/doc-reviews/review-provisions.go`, `review-provisions_test.go`
- Modify: `server/api/doc-reviews/review-entities.go`, `review-entities_test.go`
- Modify: `server/api/doc-reviews/review-inventory-items.go`, `review-inventory-items_test.go`
- Modify: `server/api/doc-reviews/review_document_selection_test.go`

- [ ] **Step 1: Write failing payload tests**

In `review-match-limit_test.go`, test `limitMatchesToLLM([]int{1,2,3,4,5})` returns `[1,2,3]` without reordering. Table-test `maxMatchesToLLM()` with `t.Setenv`: unset/blank/malformed/overflow → 3, zero/negative → 1, positive → itself.

Augment the existing production-path tests `TestReviewMetric_PayloadAndFindingTagging`, `TestReviewProvision_PayloadAndFindingTagging`, `TestReviewEntity_PayloadAndFindingTagging`, and `TestReviewInventoryItem_PayloadAndFindingTagging`. Give each test five concrete matches with IDs `m1` through `m5`, call the real `reviewMetric`/`reviewProvision`/`reviewEntity`/`reviewItem` method with the existing capturing `fakeJSONExtractor`, unmarshal `fake.inputTexts[0]`, and assert its `matching_*` array has length 3 with the first and third IDs `m1` and `m3`. For example, add to the metrics test after the production call:

```go
t.Setenv("MAX_MATCHES_TO_LLM", "3")
ms := []matchedMetric{
    {view: metricView{MetricID:"m1"}}, {view: metricView{MetricID:"m2"}},
    {view: metricView{MetricID:"m3"}}, {view: metricView{MetricID:"m4"}},
    {view: metricView{MetricID:"m5"}},
}
var payload struct {
    Matches []struct {
        Metric struct { MetricID string `json:"metric_id"` } `json:"metric"`
    } `json:"matching_metrics"`
}
if err := json.Unmarshal([]byte(fake.inputTexts[0]), &payload); err != nil { t.Fatal(err) }
if len(payload.Matches) != 3 || payload.Matches[0].Metric.MetricID != "m1" || payload.Matches[2].Metric.MetricID != "m3" {
    t.Fatalf("matching_metrics=%v, want ranked m1,m2,m3", payload.Matches)
}
```

Repeat the exact serialized-request assertion with payload arrays `matching_provisions`, `matching_entities`, and `matching_items`, decoding IDs `prov_id`, `entity_id`, and `inventory_item_id`. Rename the augmented tests only if needed; otherwise keep their existing production-path names. These tests must fail until the real payload construction slices matches.

- [ ] **Step 2: Write failing retrieval regression test**

Set each reviewer-specific `*_REVIEW_MAX_MATCHES=25`, build reviewers, and assert `maxMatches == 25` rather than 10.

- [ ] **Step 3: Run RED**

Run: `go test ./server/api/doc-reviews -run 'Test.*MatchesToLLM|TestReview(Metric|Provision|Entity|Inventory).*Payload|TestBuildReviewers.*RetrievalLimit' -count=1`

Expected: FAIL because payloads are unsliced and retrieval is capped at 10.

- [ ] **Step 4: Implement one shared slice helper**

Add the helper only in the new focused `server/api/doc-reviews/review-match-limit.go`:

```go
func maxMatchesToLLM() int { return envInt("MAX_MATCHES_TO_LLM", 3, 1) }

func limitMatchesToLLM[T any](matches []T) []T {
    limit := maxMatchesToLLM()
    if len(matches) <= limit { return matches }
    return matches[:limit]
}
```

Use it only in the four LLM `payloadObj` constructions, for example `"matching_metrics": matchedMetricsPayload(limitMatchesToLLM(ms))`. Do not slice any `matched*Payload(ms)` used for persisted review logs or other bookkeeping. Preserve the existing `matches` completion-log field as the retrieved count and add `matches_sent_to_llm: len(limitMatchesToLLM(ms))`. Remove the hard `>10` clamp and logger parameter added solely for that clamp from `buildReviewers`; preserve reviewer-specific retrieval defaults.

- [ ] **Step 5: Run GREEN and focused regressions**

Run: `go test ./server/api/doc-reviews -run 'Test.*MatchesToLLM|TestBuildReviewers.*(RetrievalLimit|MaxTasks)|TestReview(Metric|Provision|Entity|Inventory)' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

Run: `jj commit -m 'Limit artifact matches sent to review LLMs' server/api/doc-reviews/review-document.go server/api/doc-reviews/review-match-limit.go server/api/doc-reviews/review-match-limit_test.go server/api/doc-reviews/review-{metrics,provisions,entities,inventory-items}.go server/api/doc-reviews/review-{metrics,provisions,entities,inventory-items}_test.go server/api/doc-reviews/review_document_selection_test.go`

---

## Chunk 2: Scheduler Enforcement, Frontend, and Documentation

### Task 4: Synchronized output work gate

**Files:**
- Create: `server/api/doc-reviews/review-output-limits.go`
- Create: `server/api/doc-reviews/review-output-limits_test.go`
- Modify: `server/api/doc-reviews/review-document.go`
- Modify: `server/api/doc-reviews/review_document_selection_test.go`

- [ ] **Step 1: Write failing unit tests for classification and claims**

Tests must prove:

- `"analysis"`, `" Analysis "`, and case variants count as analyses;
- blank, missing, unknown, and known non-analysis types count as findings;
- `claimNext` atomically checks counts, removes the next original index from the gate-owned ordered phase queue, and records it;
- reaching either limit rejects subsequent claims;
- already claimed tasks may complete and overshoot;
- `unclaimedIndexes(total)` returns exact sorted original indexes;
- `replaceQueue` installs the remainder phase only after seed reservation and before remainder workers start.

- [ ] **Step 2: Run RED**

Run: `go test ./server/api/doc-reviews -run 'TestReviewWorkGate|TestClassifyReviewOutput' -count=1`

Expected: FAIL because the gate does not exist.

- [ ] **Step 3: Implement the minimal gate**

Implement a mutex-protected type with:

```go
type reviewWorkGate struct {
    mu sync.Mutex
    maxFindings, maxAnalyses int
    findings, analyses int
    queue []int
    claimed map[int]struct{}
}

func newReviewWorkGate(maxFindings, maxAnalyses int, queue []int) *reviewWorkGate
func (g *reviewWorkGate) claimNext() (int, bool)
func (g *reviewWorkGate) replaceQueue(queue []int)
func (g *reviewWorkGate) complete(findings []ReviewFinding)
func (g *reviewWorkGate) reached() bool
func (g *reviewWorkGate) unclaimedIndexes(total int) []int
func (g *reviewWorkGate) snapshot() reviewOutputSnapshot
```

`claimNext` performs reached-check, ordered queue removal, and claimed-set insertion under one lock. `replaceQueue` copies its input and is called only at a phase boundary with no active claimers. Classification uses `strings.EqualFold(strings.TrimSpace(...), "analysis")`.

- [ ] **Step 4: Add fields and limit propagation**

First add failing `TestBuildReviewers_PropagatesDepthSpecificOutputLimits` in `review_document_selection_test.go`: configure one enabled grammar runner, set `ReviewDepth: 2`, use effective config values 20/21, call `buildReviewers`, and assert `cfg.MaxFindings == 20`, `cfg.MaxAnalyses == 21`, and `cfg.ReviewDepth == 2`.

Then add `MaxFindings`, `MaxAnalyses`, and `ReviewDepth` to `ReviewerConfig`/`ReviewProcessor`. Resolve limits for every runner in `buildReviewers` through one helper using the request depth. Reviewer identity, logger, and record ID remain scheduler arguments.

- [ ] **Step 5: Run GREEN**

Run: `go test ./server/api/doc-reviews -run 'TestReviewWorkGate|TestClassifyReviewOutput|TestBuildReviewers.*OutputLimits' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

Run: `jj commit -m 'Add document review output work gate' server/api/doc-reviews/review-output-limits.go server/api/doc-reviews/review-output-limits_test.go server/api/doc-reviews/review-document.go server/api/doc-reviews/review_document_selection_test.go`

### Task 5: Enforce limits in standard and artifact schedulers

**Files:**
- Modify: `server/api/doc-reviews/review-document.go`
- Modify: `server/api/doc-reviews/review-artifact-window.go`
- Modify: `server/api/doc-reviews/review-artifact-window_test.go`
- Create: `server/api/doc-reviews/review-output-scheduler_test.go`
- Modify: `server/api/doc-reviews/review-assumptions.go`, `review-clarity.go`, `review-completeness.go`, `review-conciseness.go`, `review-correctness.go`, `review-cross-reference-correctness.go`, `review-currency.go`, `review-diagrams.go`, `review-entities.go`, `review-error-handling.go`, `review-evidence-rationale.go`, `review-examples.go`, `review-formatting-consistency.go`, `review-grammar-spelling.go`, `review-heading-hierarchy.go`, `review-internal-contradictions.go`, `review-internal-policy.go`, `review-legal-compliance.go`, `review-limitations.go`, `review-localization.go`, `review-logical-flow.go`, `review-modularity.go`, `review-navigability.go`, `review-performance.go`, `review-prerequisites.go`, `review-readability.go`, `review-regulatory-compliance.go`, `review-relevance.go`, `review-requirement-traceability.go`, `review-section-balance.go`, `review-security.go`, `review-standards-compliance.go`, `review-technical-accuracy.go`, `review-terminology-consistency.go`, `review-testable-claims.go`, and `review-tone-voice.go`
- Modify: `server/api/doc-reviews/review-{metrics,provisions,inventory-items,metrics-completeness}.go`

- [ ] **Step 1: Write failing standard scheduler tests**

In `review-output-scheduler_test.go`, use four per-index release channels and a buffered `started` channel. Run four units with `maxTasks=2`, depth 2, max findings 1, max analyses 10, record ID 42, reviewer `grammar_spelling`, and a `captureLogger`. Wait for two starts, release one to return a non-analysis finding, then release the other. Assert only two task functions ran, final progress has `CompletedUnits=4`, and the warning's alternating key/value arguments contain:

```go
map[string]any{
    "reviewer":"grammar_spelling", "review_depth":2,
    "max_findings":1, "max_analyses":10,
    "findings":2, "analyses":0, "skipped_units":2,
    "record_id":int64(42),
}
```

Also assert `skipped_unit_indexes` equals the two unstarted original indexes. Add a second controlled-channel test that cancels the context, releases blocked work, and asserts `errors.Is(err, ErrPipelineStopped)`, plus `findLogEntry(logger.entries, "warn", outputLimitWarningMessage) == nil`; caller cancellation must never be logged as budget withholding. Limit withholding in the first test returns nil.

- [ ] **Step 2: Write failing artifact phase test**

Create four units whose window grouping yields original seed indexes 0 and 2 and remainder indexes 1 and 3. Seed 0 closes an explicit completion signal and returns one finding immediately while seed 2 blocks on a release channel. Set `LLM_CALL_STAGGER` to a deterministic test value, wait for seed 0's completion signal, assert neither remainder starts, release seed 2, and assert skipped indexes `[1,3]`, a final progress snapshot with `CompletedUnits=4`, and every structured field listed in Step 1 with the artifact reviewer identity and record ID.

- [ ] **Step 3: Run RED**

Run: `go test ./server/api/doc-reviews -run 'TestRunReviewerConcurrent.*Limit|TestRunArtifactUnits.*Limit' -count=1`

Expected: FAIL because schedulers pre-launch work and do not gate claims.

- [ ] **Step 4: Implement standard incremental claiming**

Replace delegation to the generic pre-existing concurrency helper with at most `maxTasks` workers. Each worker obtains the next natural input index through the shared gate, executes it, records its completed results, and repeats. After workers finish, mark unclaimed indexes complete in progress without invoking their functions. Return `ErrPipelineStopped` only for caller cancellation. Emit one aggregate warning when indexes were skipped.

- [ ] **Step 5: Implement artifact phase queues**

Keep seed partitioning. Reserve eligible seed indexes through the gate before launching seeds. After stagger, launch at most `maxTasks` remainder workers, each claiming one original remainder index at a time. Derive skipped indexes from all original indexes minus the gate's claimed set. Do not create goroutines for unclaimed units. After workers finish, advance the artifact progress tracker once for every unclaimed index so its final snapshot reports all units complete. If caller cancellation caused the stop, return `ErrPipelineStopped` and suppress the budget-withholding warning.

- [ ] **Step 6: Update call sites mechanically**

Change scheduler signatures to receive the effective `ReviewerConfig`, reviewer name, logger, and record ID. Update every exact standard caller listed above and all four artifact-scheduler callers. Use the reviewer `Name()` or fixed aspect string consistently; do not add reviewer-specific limit logic. Run `rg -n 'runReviewerConcurrent\(|runArtifactUnitsWindowGrouped\(' server/api/doc-reviews -g '*.go'` to inspect every updated call, then compile the package.

- [ ] **Step 7: Run GREEN and package tests**

Run: `gofmt -w server/api/doc-reviews/*.go`

Expected: all Go changes from Tasks 1 through 5 are formatted before the final backend commit in Step 8.

Run: `go test ./server/api/doc-reviews -run 'TestRunReviewerConcurrent|TestRunArtifactUnits|TestReviewWorkGate' -count=1`

Expected: PASS.

Run: `go test ./server/api/doc-reviews/... -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

Run: `jj commit -m 'Enforce soft document review output limits' server/api/doc-reviews`

### Task 6: Frontend review-depth control

**Files:**
- Modify: `web/src/lib/services/docReviewService.ts`
- Modify: `web/src/lib/components/home3/document-review-view.svelte`

- [ ] **Step 1: Establish RED with type check**

Add `reviewDepth = $state<1 | 2 | 3>(1)`, render the control and summary, and reference `review_depth` in the request before changing `SubmitInput`. Run the frontend check to observe the missing-property/type failure.

Run: `cd web && bun run check`.

Expected: FAIL because service types do not expose `review_depth`.

- [ ] **Step 2: Implement wire types and UI**

Add `review_depth: 1 | 2 | 3` to `SubmitInput` and `review_depth: number` to `RequestStatus`. In Step 2 render three accessible radio/button options labeled Depth 1, Depth 2, and Depth 3. Include the chosen value in Step 4 summary and `handleSubmit` payload. Reset it to 1 in `resetForm`.

- [ ] **Step 3: Run GREEN**

Run: `cd web && bun run check`.

Expected: PASS with no new warnings.

- [ ] **Step 4: Commit**

Run: `jj commit -m 'Add review depth to document review form' web/src/lib/services/docReviewService.ts web/src/lib/components/home3/document-review-view.svelte`

### Task 7: Full verification and ADR completion

**Files:**
- Modify: `/Users/cding/Workspace/KnowledgeStore/doc-repo/adrs/202607/2026071001-adr-doc-review-max-controls.md`

- [ ] **Step 1: Run focused verification**

Run: `go test ./server/api/doc-reviews/... -count=1`

Expected: PASS.

Run: `go test ./server/cmd/config/... -count=1`

Expected: PASS and migration-directory wiring remains valid.

- [ ] **Step 2: Run project build and frontend checks**

Run: `go build ./...`

Expected: PASS.

Run: `cd web && bun run check`

Expected: PASS with no new errors.

Run: `mise build-web`

Expected: PASS.

- [ ] **Step 3: Validate migration if a local database URL is available**

If both `goose` and `DATABASE_URL` are available, run exactly:

```bash
goose -dir project_migrations postgres "$DATABASE_URL" status
goose -dir project_migrations postgres "$DATABASE_URL" up
goose -dir project_migrations postgres "$DATABASE_URL" down
goose -dir project_migrations postgres "$DATABASE_URL" up
```

Expected: status succeeds; up applies or reports current; down removes the latest depth migration; final up reapplies it. If either prerequisite is absent, record that rollback execution was intentionally skipped and rely on SQL shape plus startup-wiring tests; do not invent credentials.

- [ ] **Step 4: Update the ADR**

Set status to Implemented and fill Database Migrations, Data Formats, Environment Variables, Code Changes, Operational Behaviors, Consequences, Tests, and Documentation Impact. State built-in `[100,200,300]` defaults versus local `[10,20,30]` testing overrides, per-reviewer scope, soft overshoot, exact cancellation logging, and `MAX_MATCHES_TO_LLM=3` behavior.

- [ ] **Step 5: Answer the workspace documentation protocol in the ADR**

Record:

- knowledge changed: request depth and output scheduling semantics;
- affected docs/specs/ADRs/tests;
- updated docs: ADR plus design/plan;
- stale docs: none found, or list any discovered during verification;
- intentionally undocumented: internal mutex/goroutine implementation details beyond operational semantics.

- [ ] **Step 6: Confirm ChenWeb is already committed, then commit the KnowledgeStore ADR separately**

Run in `ChenWeb`: `jj status`.

Expected: no changes because Tasks 1 through 6 committed every exact ChenWeb file they changed. If formatting produced a change, include it in the owning task's prior commit with `jj squash` rather than creating an unspecified catch-all commit.

Run in `KnowledgeStore`: `jj status`, inspect `jj diff -- doc-repo/adrs/202607/2026071001-adr-doc-review-max-controls.md`, then `jj commit -m 'Document implemented review maximum controls' doc-repo/adrs/202607/2026071001-adr-doc-review-max-controls.md`.

- [ ] **Step 7: Final clean-state verification**

Run: `jj -R ChenWeb status && jj -R KnowledgeStore status`

Expected: both working copies have no changes.
