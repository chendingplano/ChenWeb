# Doc Review Report: Per-Artifact Sections Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** In the Typst doc-review report, group `metrics`/`provisions`/`inventory_items` findings by the artifact under review (`artifact_id`), and — for provisions, the only reviewer with a comparison-analyses table today — render each provision's `kb.doc_review_provision_analyses` rows together with that provision's findings in the same section, including provisions that have analyses but zero findings.

**Architecture:** All three reviewers already write `kb.doc_review_findings.artifact_id` (ADR 2026070603). `report.go`'s `Build()` gains an `ArtifactID` field on `ReportFinding` so the value survives into the persisted report skeleton. `typst_report.go` loads `kb.doc_review_provision_analyses` once per report (keyed by `prov_id`) and, when rendering `provisions`/`metrics`/`inventory_items` aspect sections, groups findings by `ArtifactID` into per-artifact Typst blocks instead of one flat list; for `provisions`, artifact groups are unioned with any `prov_id` that has analyses but no finding, and the `provisions` aspect section (and its pass, if otherwise absent) is forced to render whenever analyses exist for this run. The Typst template gains one new helper, `artifact-group`, and `aspect-section` gains an `artifact-groups` parameter alongside its existing flat `findings` parameter, so unrelated aspects (grammar_spelling, etc.) are completely unaffected.

**Tech Stack:** Go 1.25 (`database/sql`, `DATA-DOG/go-sqlmock` for tests), Typst template language.

## Global Constraints

- Prompts (not touched by this plan) live under `prompts/`, prefixed `prompt-`, per `ChenWeb/CLAUDE.md` — not relevant here since no prompt changes are needed.
- Surgical changes only: don't touch `metrics`/`inventory_items` reviewer code, don't add analyses tables for them (out of scope — that's ADR 2026070604, a separate proposal not yet implemented).
- Every changed line must trace to this plan's goal — no drive-by refactors of unrelated code in the touched files.
- `go build ./...` and `go test ./server/api/doc-reviews/...` must stay clean after every task.
- Scope is the Typst report only (`typst_report.go`, `report.go`, `template-document-report.typ`). The HTML (`renderHTML`) and Markdown (`renderMarkdown`) renderers in `report.go` are NOT touched by this plan — they are secondary viewers per ADR 2026062203's Overview ("Use the Typst template... to create the report"), and adding per-artifact grouping there is unscoped follow-on work.

---

### Task 1: `ReportFinding.ArtifactID` — carry the artifact ID into the report skeleton

**Files:**
- Modify: `server/api/doc-reviews/report.go:75-89` (`ReportFinding` struct), `server/api/doc-reviews/report.go:172-185` (`Build()`'s finding-to-`ReportFinding` conversion)
- Test: `server/api/doc-reviews/report_test.go`

**Interfaces:**
- Produces: `ReportFinding.ArtifactID string` (json tag `artifact_id,omitempty`), populated from `FindingItem.ArtifactID` (already exists, `models.go:83`) inside `Build()`. Later tasks read this field to group findings.

- [ ] **Step 1: Write the failing test**

Add to `server/api/doc-reviews/report_test.go` (new test function, anywhere after the existing tests — e.g. after `TestReportBuild_MetaFields`):

```go
func TestReportBuild_PropagatesArtifactID(t *testing.T) {
	gen := newTestReportGenerator()

	req := &RequestStatus{
		ID: 1, InputRecordID: 416, Tier: "must_review",
		Status:      "completed",
		CreateTime:  "2026-06-21T12:00:00Z",
		LatestRunID: 416,
	}

	findings := []FindingItem{{
		ID:          1,
		Pass:        "P5",
		Aspect:      "provisions",
		Severity:    "medium",
		FindingType: "conflict",
		Title:       "Conflict",
		Description: "Description",
		ArtifactID:  "1001_prv_3",
	}}

	report, err := gen.Build(context.Background(), req, findings)
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("Findings len=%d, want 1", len(report.Findings))
	}
	if got := report.Findings[0].ArtifactID; got != "1001_prv_3" {
		t.Errorf("Findings[0].ArtifactID = %q, want %q", got, "1001_prv_3")
	}
	pg := report.FindingsByPass["P5"]
	if len(pg.Findings) != 1 || pg.Findings[0].ArtifactID != "1001_prv_3" {
		t.Errorf("FindingsByPass[P5].Findings[0].ArtifactID = %+v, want ArtifactID=1001_prv_3", pg.Findings)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run TestReportBuild_PropagatesArtifactID -v`
Expected: FAIL — `report.Findings[0].ArtifactID` is `""` (field doesn't exist yet, or is always zero value), not `"1001_prv_3"`.

- [ ] **Step 3: Write minimal implementation**

In `server/api/doc-reviews/report.go`, add the field to `ReportFinding` (after `Confidence`):

```go
// ReportFinding is a single finding within a report.
type ReportFinding struct {
	ID          int64           `json:"id,omitempty"`
	Pass        string          `json:"pass"`
	Aspect      string          `json:"aspect"`
	Severity    string          `json:"severity"`
	FindingType string          `json:"finding_type"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Evidence    string          `json:"evidence,omitempty"`
	Location    string          `json:"location,omitempty"`
	Suggestion  string          `json:"suggestion,omitempty"`
	Confidence  float64         `json:"confidence"`
	// ArtifactID identifies the artifact-under-review (metric_id / prov_id /
	// inventory_item_id) this finding is about, for the per-artifact
	// reviewers (ADR 2026070603). Empty for entities/text-chunk findings.
	ArtifactID string          `json:"artifact_id,omitempty"`
	Sources    []SourceContext `json:"sources,omitempty"` // source blocks with before/after context
	Related    []SourceContext `json:"related_sources,omitempty"`
}
```

In `Build()`, add the field when constructing `rf` (the `rf := ReportFinding{...}` block):

```go
			rf := ReportFinding{
				ID:   f.ID,
				Pass: f.Pass, Aspect: f.Aspect, Severity: f.Severity,
				FindingType: f.FindingType, Title: f.Title, Description: cleanReportDescription(f.Description, len(related) > 0),
				Evidence: f.Evidence, Location: f.Location, Suggestion: f.Suggestion,
				Confidence: f.Confidence,
				ArtifactID: f.ArtifactID,
				Related:    related,
			}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run TestReportBuild_PropagatesArtifactID -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd ChenWeb
git add server/api/doc-reviews/report.go server/api/doc-reviews/report_test.go
git commit -m "doc-review report: carry artifact_id into ReportFinding"
```

---

### Task 2: `loadProvisionAnalysesByRun` — read back `kb.doc_review_provision_analyses`

**Files:**
- Modify: `server/api/doc-reviews/review-provisions.go` (add function near `saveProvisionAnalyses`, after line 347)
- Test: `server/api/doc-reviews/review-provisions_test.go`

**Interfaces:**
- Consumes: `ProvisionAnalysis` struct (already exists, `review-provisions.go:274-279`: `RelatedArtifactID string`, `RelatedRecordID int64`, `Relationship string`, `Summary string`).
- Produces: `loadProvisionAnalysesByRun(ctx context.Context, db *sql.DB, req *RequestStatus) (map[string][]ProvisionAnalysis, error)` — keyed by `prov_id`. Returns `(nil, nil)` when `db == nil || req == nil || req.InputRecordID == 0 || req.LatestRunID == 0` (mirrors `loadReportFindingsWithMetadata`'s guard in `typst_report.go:164`). Later tasks call this from `typst_report.go`.

- [ ] **Step 1: Write the failing test**

Add to `server/api/doc-reviews/review-provisions_test.go` (after `TestReviewProvision_SavesAnalyses`):

```go
func TestLoadProvisionAnalysesByRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	query := regexp.QuoteMeta(`
		SELECT prov_id, COALESCE(related_artifact_id,''), COALESCE(related_record_id,0), relationship, summary
		FROM kb.doc_review_provision_analyses
		WHERE input_record_id = $1 AND run_id = $2
		ORDER BY id ASC`)
	mock.ExpectQuery(query).
		WithArgs(int64(88), int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{
			"prov_id", "related_artifact_id", "related_record_id", "relationship", "summary",
		}).AddRow("1001_prv_3", "2002_prv_9", int64(2002), "same_subject", "Identical text, no conflict.").
			AddRow("1001_prv_3", "3003_prv_1", int64(3003), "unrelated", "Different subject entirely."))

	req := &RequestStatus{InputRecordID: 88, LatestRunID: 99}
	got, err := loadProvisionAnalysesByRun(context.Background(), db, req)
	if err != nil {
		t.Fatalf("loadProvisionAnalysesByRun: %v", err)
	}
	entries := got["1001_prv_3"]
	if len(entries) != 2 {
		t.Fatalf("entries len=%d, want 2 (%+v)", len(entries), got)
	}
	if entries[0].RelatedArtifactID != "2002_prv_9" || entries[0].Relationship != "same_subject" {
		t.Errorf("entries[0]=%+v, want related=2002_prv_9 relationship=same_subject", entries[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestLoadProvisionAnalysesByRun_NilDB(t *testing.T) {
	got, err := loadProvisionAnalysesByRun(context.Background(), nil, &RequestStatus{InputRecordID: 88, LatestRunID: 99})
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v, want (nil, nil) for nil db", got, err)
	}
}
```

Check the test file's imports already include `regexp` and `"github.com/DATA-DOG/go-sqlmock"` (used by `TestReviewProvision_SavesAnalyses`); add `"regexp"` to the import block if it's not already there.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run TestLoadProvisionAnalysesByRun -v`
Expected: FAIL with `undefined: loadProvisionAnalysesByRun`

- [ ] **Step 3: Write minimal implementation**

Add to `server/api/doc-reviews/review-provisions.go`, after `saveProvisionAnalyses` (after line 347):

```go
// loadProvisionAnalysesByRun returns this run's comparison analyses grouped by
// prov_id, for the report's per-artifact sections (ADR 2026062203 §1.2).
// Returns (nil, nil) when db or req is unavailable, mirroring
// loadReportFindingsWithMetadata's guard in typst_report.go.
func loadProvisionAnalysesByRun(ctx context.Context, db *sql.DB, req *RequestStatus) (map[string][]ProvisionAnalysis, error) {
	if db == nil || req == nil || req.InputRecordID == 0 || req.LatestRunID == 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT prov_id, COALESCE(related_artifact_id,''), COALESCE(related_record_id,0), relationship, summary
		FROM kb.doc_review_provision_analyses
		WHERE input_record_id = $1 AND run_id = $2
		ORDER BY id ASC`, req.InputRecordID, req.LatestRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]ProvisionAnalysis{}
	for rows.Next() {
		var provID string
		var a ProvisionAnalysis
		if err := rows.Scan(&provID, &a.RelatedArtifactID, &a.RelatedRecordID, &a.Relationship, &a.Summary); err != nil {
			return nil, err
		}
		out[provID] = append(out[provID], a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run 'TestLoadProvisionAnalysesByRun|TestReviewProvision_SavesAnalyses' -v`
Expected: PASS (both old and new tests)

- [ ] **Step 5: Commit**

```bash
cd ChenWeb
git add server/api/doc-reviews/review-provisions.go server/api/doc-reviews/review-provisions_test.go
git commit -m "doc-review: add loadProvisionAnalysesByRun for report rendering"
```

---

### Task 3: Extract `buildFindingBlock` — pure refactor, no behavior change

**Files:**
- Modify: `server/api/doc-reviews/typst_report.go:328-394` (inside `buildTypstSource`)
- Test: `server/api/doc-reviews/typst_report_test.go`

**Interfaces:**
- Produces: `buildFindingBlock(f ReportFinding, fid string) string` — returns the exact `review-finding(...)` Typst block text that was previously built inline. Task 4 calls this from the new per-artifact grouping path; the existing flat path is rewired to call it too, so there is exactly one place that formats a finding block.

- [ ] **Step 1: Write the failing test**

Add to `server/api/doc-reviews/typst_report_test.go` (after `TestBuildTypstSourceUsesDatabaseFindingID`):

```go
func TestBuildFindingBlockRendersSourcesAndCorrection(t *testing.T) {
	f := ReportFinding{
		ID:          42,
		Title:       "Conflict",
		Description: "Description",
		Suggestion:  "Suggestion",
		Sources:     []SourceContext{{Source: "115: source line"}},
		Related:     []SourceContext{{Before: "85: before", Source: "87: matched metric", After: "88: after"}},
	}
	block := buildFindingBlock(f, "42")
	if !strings.Contains(block, `id: "42"`) {
		t.Fatalf("block missing id: %s", block)
	}
	if !strings.Contains(block, "errors: [Conflict]") {
		t.Fatalf("block missing errors content: %s", block)
	}
	if !strings.Contains(block, "related-sources: (") || !strings.Contains(block, "87: matched metric") {
		t.Fatalf("block missing related-sources: %s", block)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run TestBuildFindingBlockRendersSourcesAndCorrection -v`
Expected: FAIL with `undefined: buildFindingBlock`

- [ ] **Step 3: Write minimal implementation**

In `server/api/doc-reviews/typst_report.go`, extract the inline block-building code (currently lines 335-388, inside the `for _, f := range af` loop) into a standalone function, placed just before `buildTypstSource` (i.e. right after the imports/package-level `passLabelMap`, or immediately above `buildTypstSource` — insert before line 256's `// buildTypstSource returns...` comment):

```go
// buildFindingBlock renders one review-finding(...) Typst call for f, using
// fid as its displayed id (either the DB finding id or an "F-NN" ordinal
// fallback — see findingDisplayID).
func buildFindingBlock(f ReportFinding, fid string) string {
	var blockB strings.Builder
	fmt.Fprintf(&blockB, "      review-finding(\n")
	fmt.Fprintf(&blockB, "        id: \"%s\",\n", typStr(fid))

	// Emit sources array — one dict per source location group.
	fmt.Fprintf(&blockB, "        sources: (")
	if len(f.Sources) > 0 {
		fmt.Fprintf(&blockB, "\n")
		for _, sc := range f.Sources {
			fmt.Fprintf(&blockB, "          (\n")
			if sc.Before != "" {
				fmt.Fprintf(&blockB, "            before: [%s],\n", typLines(sc.Before))
			} else {
				fmt.Fprintf(&blockB, "            before: none,\n")
			}
			fmt.Fprintf(&blockB, "            source: [%s],\n", typLines(sc.Source))
			if sc.After != "" {
				fmt.Fprintf(&blockB, "            after: [%s],\n", typLines(sc.After))
			} else {
				fmt.Fprintf(&blockB, "            after: none,\n")
			}
			fmt.Fprintf(&blockB, "          ),\n")
		}
		fmt.Fprintf(&blockB, "        ")
	}
	fmt.Fprintf(&blockB, "),\n")

	fmt.Fprintf(&blockB, "        related-sources: (")
	if len(f.Related) > 0 {
		fmt.Fprintf(&blockB, "\n")
		for _, sc := range f.Related {
			fmt.Fprintf(&blockB, "          (\n")
			if sc.Before != "" {
				fmt.Fprintf(&blockB, "            before: [%s],\n", typLines(sc.Before))
			} else {
				fmt.Fprintf(&blockB, "            before: none,\n")
			}
			fmt.Fprintf(&blockB, "            source: [%s],\n", typLines(sc.Source))
			if sc.After != "" {
				fmt.Fprintf(&blockB, "            after: [%s],\n", typLines(sc.After))
			} else {
				fmt.Fprintf(&blockB, "            after: none,\n")
			}
			fmt.Fprintf(&blockB, "          ),\n")
		}
		fmt.Fprintf(&blockB, "        ")
	}
	fmt.Fprintf(&blockB, "),\n")

	fmt.Fprintf(&blockB, "        errors: [%s],\n", typContent(f.Title))
	fmt.Fprintf(&blockB, "        explanation: [%s],\n", typContent(f.Description))
	fmt.Fprintf(&blockB, "        correction: [%s],\n", typContent(f.Suggestion))
	fmt.Fprintf(&blockB, "      ),")
	return blockB.String()
}

// findingDisplayID returns the DB finding id as a string, or an "F-NN"
// ordinal fallback when id is 0 (legacy report JSON with no finding row id).
func findingDisplayID(id int64, ordinal int) string {
	if id == 0 {
		return fmt.Sprintf("F-%02d", ordinal)
	}
	return fmt.Sprintf("%d", id)
}
```

Then replace the loop body inside `buildTypstSource` (the `for _, f := range af { findingIdx++; ...; findingBlocks = append(findingBlocks, block) }` block, originally lines 328-394) with:

```go
			for _, f := range af {
				findingIdx++
				fid := findingDisplayID(f.ID, findingIdx)
				findingBlocks = append(findingBlocks, buildFindingBlock(f, fid))

				if f.Severity == "high" {
					aspectHighCount++
				}
			}
```

(This is the same loop, just calling the two new functions instead of the inline `blockB` code. `var findingBlocks []string` and `var aspectHighCount int` declarations above the loop are unchanged.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run 'TestBuildFindingBlockRendersSourcesAndCorrection|TestBuildTypstSourceUsesDatabaseFindingID' -v`
Expected: PASS for both — `TestBuildTypstSourceUsesDatabaseFindingID` confirms the refactor didn't change `buildTypstSource`'s output for the existing flat-list path.

- [ ] **Step 5: Commit**

```bash
cd ChenWeb
git add server/api/doc-reviews/typst_report.go server/api/doc-reviews/typst_report_test.go
git commit -m "doc-review typst report: extract buildFindingBlock helper (no behavior change)"
```

---

### Task 4: Per-artifact grouping in `buildTypstSource` (findings-only aspects: metrics, inventory_items)

**Files:**
- Modify: `server/api/doc-reviews/typst_report.go` (imports; `buildTypstSource`'s aspect loop)
- Test: `server/api/doc-reviews/typst_report_test.go`

**Interfaces:**
- Consumes: `ReportFinding.ArtifactID` (Task 1), `buildFindingBlock`/`findingDisplayID` (Task 3).
- Produces: `isArtifactAnchoredAspect(aspect string) bool`; `buildArtifactGroupsArg(af []ReportFinding, provisionAnalyses map[string][]ProvisionAnalysis, aspect string, findingIdx *int) string` (Typst `artifact-group(...)` array literal, one entry per distinct `ArtifactID` in `af`, in first-seen order). `buildTypstSource` gains a new parameter `provisionAnalyses map[string][]ProvisionAnalysis` (Task 2's return type) — this task only wires the parameter through and uses it for the `provisions` case in the *next* task; here `metrics`/`inventory_items` are proven to route through the new artifact-grouping path with `provisionAnalyses` always empty for those two aspects (they have no analyses table).

- [ ] **Step 1: Write the failing test**

Add to `server/api/doc-reviews/typst_report_test.go`:

```go
func TestBuildTypstSourceGroupsMetricsFindingsByArtifactID(t *testing.T) {
	req := &RequestStatus{RequesterName: "Reviewer"}
	skeleton := &ReportSkeleton{
		Meta: ReportMeta{ReportID: "rpt_88", DocumentTitle: "Document title", GeneratedAt: "2026-07-06T12:00:00Z"},
		FindingsByPass: map[string]PassGroup{
			"P5": {
				Label: "Technical & Compliance",
				Findings: []ReportFinding{
					{ID: 1, Pass: "P5", Aspect: "metrics", Severity: "high", Title: "Conflict A", ArtifactID: "1001_m_7"},
					{ID: 2, Pass: "P5", Aspect: "metrics", Severity: "low", Title: "Conflict B", ArtifactID: "1001_m_7"},
					{ID: 3, Pass: "P5", Aspect: "metrics", Severity: "medium", Title: "Conflict C", ArtifactID: "1001_m_9"},
				},
			},
		},
		PassOrder: []string{"P5"},
	}

	src := buildTypstSource(skeleton, req, "en", "/tmp/template.typ", nil)

	if !strings.Contains(src, `artifact-group(`) {
		t.Fatalf("typst source missing artifact-group(...) calls: %s", src)
	}
	if !strings.Contains(src, `title: "1001_m_7"`) || !strings.Contains(src, `title: "1001_m_9"`) {
		t.Fatalf("typst source missing per-artifact titles: %s", src)
	}
	// Both findings for 1001_m_7 must land inside the SAME artifact-group call.
	groupStart := strings.Index(src, `title: "1001_m_7"`)
	nextGroup := strings.Index(src[groupStart+1:], "artifact-group(")
	segment := src[groupStart:]
	if nextGroup >= 0 {
		segment = src[groupStart : groupStart+1+nextGroup]
	}
	if !strings.Contains(segment, "Conflict A") || !strings.Contains(segment, "Conflict B") {
		t.Fatalf("findings for the same artifact were not grouped together: %s", segment)
	}
}

func TestBuildTypstSourceLeavesNonArtifactAspectsFlat(t *testing.T) {
	req := &RequestStatus{RequesterName: "Reviewer"}
	skeleton := &ReportSkeleton{
		Meta: ReportMeta{ReportID: "rpt_88", DocumentTitle: "Document title", GeneratedAt: "2026-07-06T12:00:00Z"},
		FindingsByPass: map[string]PassGroup{
			"P1": {
				Label: "Language & Style",
				Findings: []ReportFinding{
					{ID: 1, Pass: "P1", Aspect: "grammar_spelling", Severity: "high", Title: "Typo"},
				},
			},
		},
		PassOrder: []string{"P1"},
	}

	src := buildTypstSource(skeleton, req, "en", "/tmp/template.typ", nil)
	if strings.Contains(src, "artifact-group(") {
		t.Fatalf("non-artifact-anchored aspect should not use artifact-group: %s", src)
	}
	if !strings.Contains(src, `findings: (`) {
		t.Fatalf("expected flat findings list: %s", src)
	}
}
```

Update the existing `TestBuildTypstSourceUsesDatabaseFindingID` call site to pass the new parameter (it's testing the `metrics` aspect, which after this task routes through `artifact-group` too — since its finding has no `ArtifactID` set, it will render as one group with an empty title):

```go
	src := buildTypstSource(skeleton, req, "en", "/tmp/template.typ", nil)
```

(This replaces the existing `src := buildTypstSource(skeleton, req, "en", "/tmp/template.typ")` call — just adding the trailing `nil` argument. The test's existing assertions on `id: "42"` and `related-sources:` still hold since `buildFindingBlock`'s output format is unchanged.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run 'TestBuildTypstSourceGroupsMetricsFindingsByArtifactID|TestBuildTypstSourceLeavesNonArtifactAspectsFlat|TestBuildTypstSourceUsesDatabaseFindingID' -v`
Expected: FAIL — compile error (`buildTypstSource` takes 4 args, not 5) until Step 3 is done.

- [ ] **Step 3: Write minimal implementation**

Add `"sort"` to `typst_report.go`'s import block (after `"regexp"`):

```go
import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)
```

Add these two functions right after `buildFindingBlock`/`findingDisplayID` (from Task 3):

```go
// isArtifactAnchoredAspect reports whether aspect belongs to one of the
// per-artifact reviewers (ADR 2026070603), whose findings should be grouped
// by ArtifactID rather than rendered as a flat list.
func isArtifactAnchoredAspect(aspect string) bool {
	switch aspect {
	case "metrics", "provisions", "inventory_items":
		return true
	default:
		return false
	}
}

// buildArtifactGroupsArg renders the Typst artifact-group(...) array for one
// aspect-section: af is grouped by ArtifactID in first-seen order. For the
// "provisions" aspect, artifact IDs that have analyses (provisionAnalyses)
// but no finding are unioned in (sorted, appended after the finding-derived
// IDs) so "no conflict" comparisons are still visible (ADR 2026070602 / ADR
// 2026062203 §1.2). findingIdx is the shared ordinal counter also used by the
// flat-findings path, threaded by pointer so numbering stays continuous.
func buildArtifactGroupsArg(af []ReportFinding, provisionAnalyses map[string][]ProvisionAnalysis, aspect string, findingIdx *int) string {
	artifactFindingMap := map[string][]ReportFinding{}
	var artifactIDs []string
	seen := map[string]bool{}
	for _, f := range af {
		if !seen[f.ArtifactID] {
			seen[f.ArtifactID] = true
			artifactIDs = append(artifactIDs, f.ArtifactID)
		}
		artifactFindingMap[f.ArtifactID] = append(artifactFindingMap[f.ArtifactID], f)
	}

	if aspect == "provisions" && len(provisionAnalyses) > 0 {
		analysisIDs := make([]string, 0, len(provisionAnalyses))
		for id := range provisionAnalyses {
			analysisIDs = append(analysisIDs, id)
		}
		sort.Strings(analysisIDs)
		for _, id := range analysisIDs {
			if !seen[id] {
				seen[id] = true
				artifactIDs = append(artifactIDs, id)
			}
		}
	}

	var groups []string
	for _, artifactID := range artifactIDs {
		var findingBlocks []string
		for _, f := range artifactFindingMap[artifactID] {
			*findingIdx++
			findingBlocks = append(findingBlocks, buildFindingBlock(f, findingDisplayID(f.ID, *findingIdx)))
		}
		var findingsArg string
		if len(findingBlocks) > 0 {
			findingsArg = "\n" + strings.Join(findingBlocks, "\n") + "\n      "
		}

		var analysesArg string
		if entries := provisionAnalyses[artifactID]; aspect == "provisions" && len(entries) > 0 {
			var analysisBlocks []string
			for _, a := range entries {
				analysisBlocks = append(analysisBlocks, fmt.Sprintf(
					"        (related: \"%s\", relationship: \"%s\", summary: [%s]),",
					typStr(a.RelatedArtifactID), typStr(a.Relationship), typContent(a.Summary),
				))
			}
			analysesArg = "\n" + strings.Join(analysisBlocks, "\n") + "\n      "
		}

		title := artifactID
		if title == "" {
			title = "(unidentified artifact)"
		}
		groups = append(groups, fmt.Sprintf(
			"      artifact-group(\n"+
				"        title: \"%s\",\n"+
				"        analyses: (%s),\n"+
				"        findings: (%s),\n"+
				"      ),",
			typStr(title), analysesArg, findingsArg,
		))
	}
	if len(groups) == 0 {
		return ""
	}
	return "\n" + strings.Join(groups, "\n") + "\n    "
}
```

Now change `buildTypstSource`'s signature and wire the two paths together. Replace:

```go
func buildTypstSource(skeleton *ReportSkeleton, req *RequestStatus, lang, absTemplatePath string) string {
```

with:

```go
func buildTypstSource(skeleton *ReportSkeleton, req *RequestStatus, lang, absTemplatePath string, provisionAnalyses map[string][]ProvisionAnalysis) string {
```

Then replace the per-aspect block (originally the loop building `assessment`/`problems`/`guidelines`/`section` — the code right after Task 3's `for _, f := range af { ... }` loop) so it branches on `isArtifactAnchoredAspect`:

```go
			assessment := buildAssessment(len(af), aspectHighCount)
			problems := buildProblems([]string{aspect})
			guidelines := buildGuidelines(af)

			var findingsArg, artifactGroupsArg string
			if isArtifactAnchoredAspect(aspect) {
				artifactGroupsArg = buildArtifactGroupsArg(af, provisionAnalyses, aspect, &findingIdx)
			} else if len(findingBlocks) > 0 {
				findingsArg = "\n" + strings.Join(findingBlocks, "\n") + "\n    "
			}

			section := fmt.Sprintf(
				"    aspect-section(\n"+
					"      title: \"%s\",\n"+
					"      findings: (%s),\n"+
					"      artifact-groups: (%s),\n"+
					"      assessment: [%s],\n"+
					"      problems: [%s],\n"+
					"      guidelines: [%s],\n"+
					"    ),",
				typStr(aspect),
				findingsArg,
				artifactGroupsArg,
				typContent(assessment),
				typContent(problems),
				typContent(guidelines),
			)
			aspectLines = append(aspectLines, section)
```

Important: this still computes `findingBlocks` in the earlier `for _, f := range af` loop (Task 3's loop) even when the aspect is artifact-anchored — that's harmless (the variable is simply unused in that branch), but to avoid "declared and not used" confusion and duplicate finding-numbering (the flat loop's `findingIdx++` would double-count against `buildArtifactGroupsArg`'s own `findingIdx` increments), change Task 3's loop so it *skips* incrementing `findingIdx`/building blocks when the aspect is artifact-anchored:

```go
			var findingBlocks []string
			var aspectHighCount int
			artifactAnchored := isArtifactAnchoredAspect(aspect)

			for _, f := range af {
				if f.Severity == "high" {
					aspectHighCount++
				}
				if artifactAnchored {
					continue
				}
				findingIdx++
				fid := findingDisplayID(f.ID, findingIdx)
				findingBlocks = append(findingBlocks, buildFindingBlock(f, fid))
			}
```

Finally, update the two remaining call sites of `buildTypstSource`:

1. In `buildTypstSource`'s own caller inside this file — actually `buildTypstSource` is called from `GenerateTypstReport` (line 67: `src := buildTypstSource(variant.skeleton, req, variant.language, absTemplatePath)`). Leave this call as-is for now — Task 5 updates it to pass real `provisionAnalyses`. For this task only, since `typstVariant` doesn't carry analyses yet, temporarily pass `nil`:

```go
		src := buildTypstSource(variant.skeleton, req, variant.language, absTemplatePath, nil)
```

(Task 5 replaces this `nil` with the real threaded value.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run 'TestBuildTypstSourceGroupsMetricsFindingsByArtifactID|TestBuildTypstSourceLeavesNonArtifactAspectsFlat|TestBuildTypstSourceUsesDatabaseFindingID' -v`
Expected: PASS for all three.

Also run the full package to catch any other call site: `cd ChenWeb && go build ./... && go test ./server/api/doc-reviews/...` — expect it to fail only on the two `buildTypstVariants`-based tests (`TestBuildTypstVariantsUsesLocalizedMetadata`, `TestBuildTypstVariantsUsesSingleConfiguredLanguage`) if `buildTypstVariants` itself doesn't compile yet; those are fixed in Task 5. If they already pass unchanged, leave them — Task 5 will touch them regardless.

- [ ] **Step 5: Commit**

```bash
cd ChenWeb
git add server/api/doc-reviews/typst_report.go server/api/doc-reviews/typst_report_test.go
git commit -m "doc-review typst report: group metrics/provisions/inventory_items findings by artifact_id"
```

---

### Task 5: Thread `provisionAnalyses` through `buildTypstVariants` / `GenerateTypstReport`

**Files:**
- Modify: `server/api/doc-reviews/typst_report.go` (`typstVariant` struct, `buildTypstVariants`, `GenerateTypstReport`)
- Test: `server/api/doc-reviews/typst_report_test.go` (update the two existing `buildTypstVariants` tests)

**Interfaces:**
- Consumes: `loadProvisionAnalysesByRun` (Task 2).
- Produces: `typstVariant.provisionAnalyses map[string][]ProvisionAnalysis` — populated once per report (not per language, since analyses aren't localized) and passed into `buildTypstSource`.

- [ ] **Step 1: Write the failing test**

Modify `TestBuildTypstVariantsUsesLocalizedMetadata` in `server/api/doc-reviews/typst_report_test.go` (around line 150-251): after the existing `mock.ExpectQuery(findingsQuery)...` block, add an expectation for the new analyses query returning zero rows, and assert `variants[0].provisionAnalyses` is empty:

```go
	analysesQuery := regexp.QuoteMeta(`
		SELECT prov_id, COALESCE(related_artifact_id,''), COALESCE(related_record_id,0), relationship, summary
		FROM kb.doc_review_provision_analyses
		WHERE input_record_id = $1 AND run_id = $2
		ORDER BY id ASC`)
	mock.ExpectQuery(analysesQuery).
		WithArgs(int64(88), int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{
			"prov_id", "related_artifact_id", "related_record_id", "relationship", "summary",
		}))
```

Insert this block right after the existing `mock.ExpectQuery(findingsQuery)....AddRow(...))` call and before `req := &RequestStatus{...}`. Then add this assertion right before the final `if err := mock.ExpectationsWereMet(); err != nil {` line:

```go
	if len(variants[0].provisionAnalyses) != 0 {
		t.Fatalf("provisionAnalyses = %+v, want empty", variants[0].provisionAnalyses)
	}
```

Apply the identical two edits to `TestBuildTypstVariantsUsesSingleConfiguredLanguage` (around line 253-311): insert the same `analysesQuery` expectation after its `mock.ExpectQuery(findingsQuery)...` block, and add the same `provisionAnalyses` length assertion before its final `mock.ExpectationsWereMet()` check.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run 'TestBuildTypstVariantsUsesLocalizedMetadata|TestBuildTypstVariantsUsesSingleConfiguredLanguage' -v`
Expected: FAIL — either a compile error (`variants[0].provisionAnalyses` undefined) or an unmet sqlmock expectation (the analyses query is never issued).

- [ ] **Step 3: Write minimal implementation**

In `server/api/doc-reviews/typst_report.go`, add the field to `typstVariant`:

```go
type typstVariant struct {
	language          string
	suffix            string
	skeleton          *ReportSkeleton
	provisionAnalyses map[string][]ProvisionAnalysis
}
```

Update `buildTypstVariants`:

```go
func buildTypstVariants(ctx context.Context, req *RequestStatus, base *ReportSkeleton) ([]typstVariant, error) {
	findings, metadataByFindingID, err := loadReportFindingsWithMetadata(ctx, ApiTypes.ProjectDBHandle, req)
	if err != nil {
		return nil, fmt.Errorf("load localized findings: %w", err)
	}
	provisionAnalyses, err := loadProvisionAnalysesByRun(ctx, ApiTypes.ProjectDBHandle, req)
	if err != nil {
		return nil, fmt.Errorf("load provision analyses: %w", err)
	}
	languages := docReviewReportLanguagesFromEnv()
	var variants []typstVariant
	for _, language := range languages {
		variants = append(variants, typstVariant{
			language:          language,
			suffix:            reportLanguageSuffix(language),
			skeleton:          buildLocalizedTypstSkeleton(ctx, req, base, findings, metadataByFindingID, language),
			provisionAnalyses: provisionAnalyses,
		})
	}
	return variants, nil
}
```

Update the call site in `GenerateTypstReport` (from Task 4's temporary `nil`):

```go
		src := buildTypstSource(variant.skeleton, req, variant.language, absTemplatePath, variant.provisionAnalyses)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -v`
Expected: PASS across the whole `doc-reviews` package (this also re-verifies Tasks 1-4's tests still pass together).

- [ ] **Step 5: Commit**

```bash
cd ChenWeb
git add server/api/doc-reviews/typst_report.go server/api/doc-reviews/typst_report_test.go
git commit -m "doc-review typst report: thread provisionAnalyses into report generation"
```

---

### Task 6: Force the `provisions` section to render when analyses exist but findings don't

**Files:**
- Modify: `server/api/doc-reviews/typst_report.go` (`buildTypstSource`'s pass/aspect loop setup)
- Test: `server/api/doc-reviews/typst_report_test.go`

**Interfaces:**
- Consumes: `buildArtifactGroupsArg`'s existing union logic (Task 4) — this task supplies the missing piece: making sure `"provisions"` enters `aspectSlugs` (and, if needed, that pass `"P5"` enters the pass loop at all) even when zero `provisions` findings exist this run.
- Produces: no new exported symbols; behavior-only change inside `buildTypstSource`.

- [ ] **Step 1: Write the failing test**

Add to `server/api/doc-reviews/typst_report_test.go`:

```go
func TestBuildTypstSourceRendersProvisionsAnalysesWithNoFindings(t *testing.T) {
	req := &RequestStatus{RequesterName: "Reviewer"}
	// Zero findings anywhere — simulates a rerun of only the provisions
	// reviewer where every provision compared clean (no conflicts).
	skeleton := &ReportSkeleton{
		Meta:           ReportMeta{ReportID: "rpt_88", DocumentTitle: "Document title", GeneratedAt: "2026-07-06T12:00:00Z"},
		FindingsByPass: map[string]PassGroup{},
		PassOrder:      nil,
	}
	provisionAnalyses := map[string][]ProvisionAnalysis{
		"1001_prv_3": {{RelatedArtifactID: "2002_prv_9", Relationship: "same_subject", Summary: "Identical, no conflict."}},
	}

	src := buildTypstSource(skeleton, req, "en", "/tmp/template.typ", provisionAnalyses)

	if !strings.Contains(src, `title: "provisions"`) {
		t.Fatalf("expected a provisions aspect-section even with zero findings: %s", src)
	}
	if !strings.Contains(src, `title: "1001_prv_3"`) {
		t.Fatalf("expected an artifact-group for the analysis-only provision: %s", src)
	}
	if !strings.Contains(src, "Identical, no conflict.") {
		t.Fatalf("expected the analysis summary to render: %s", src)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run TestBuildTypstSourceRendersProvisionsAnalysesWithNoFindings -v`
Expected: FAIL — `src` contains neither `title: "provisions"` nor `title: "1001_prv_3"` (the pass loop never runs since `PassOrder` is empty).

- [ ] **Step 3: Write minimal implementation**

In `buildTypstSource`, locate the start of the package/aspect section building (the code that currently reads `for _, p := range skeleton.PassOrder {` — right after the `// ── Package-level sections...` comment and `var aspectLines []string` / `findingIdx := 0` declarations). Add a `provisionsPass` constant near the top of the file (alongside `passLabelMap`):

```go
// provisionsPass is the pass the provisions reviewer is configured under
// (doc-review.local.toml [reviewers.provisions]; ADR 2026063003). Used to
// force the provisions aspect-section to render when it has analyses but
// zero findings this run (ADR 2026070602 / ADR 2026062203 §1.2).
const provisionsPass = "P5"
```

Then, immediately before `for _, p := range skeleton.PassOrder {`, build an extended pass list and use it in place of `skeleton.PassOrder` for this loop only:

```go
	passOrder := skeleton.PassOrder
	if len(provisionAnalyses) > 0 && !passOrderContains(passOrder, provisionsPass) {
		passOrder = append(append([]string{}, passOrder...), provisionsPass)
	}

	var aspectLines []string
	findingIdx := 0
	for _, p := range passOrder {
```

(Replace the loop's range expression from `skeleton.PassOrder` to `passOrder`; leave everything else in the loop body as Task 4 left it.)

Add the small helper near `isArtifactAnchoredAspect`:

```go
func passOrderContains(passOrder []string, target string) bool {
	for _, p := range passOrder {
		if p == target {
			return true
		}
	}
	return false
}
```

Finally, inside the loop body, right after the existing `aspectSlugs`/`aspectFindingMap` construction (the loop over `pg.Findings` that populates them — from the original code, this is the block right after `label, ok := passLabelMap[p]; if !ok { label = p }`), add the synthetic-provisions injection:

```go
			// ── Group findings by aspect within this package ──
			aspectFindingMap := map[string][]ReportFinding{}
			var aspectSlugs []string
			aspectSeen := map[string]bool{}
			for _, f := range pg.Findings {
				if !aspectSeen[f.Aspect] {
					aspectSeen[f.Aspect] = true
					aspectSlugs = append(aspectSlugs, f.Aspect)
				}
				aspectFindingMap[f.Aspect] = append(aspectFindingMap[f.Aspect], f)
			}
			if p == provisionsPass && len(provisionAnalyses) > 0 && !aspectSeen["provisions"] {
				aspectSeen["provisions"] = true
				aspectSlugs = append(aspectSlugs, "provisions")
			}
```

`pg` for the synthetic pass is the zero-value `PassGroup{}` (Go's map access returns the zero value for a missing key — `skeleton.FindingsByPass["P5"]` when `"P5"` was never in the original map), so `pg.Findings` is `nil` and the rest of the loop (label lookup, `aspectLines = append(..., heading...)`) works unchanged; `af := aspectFindingMap["provisions"]` will be `nil`, which `buildArtifactGroupsArg` already handles (Task 4: `for _, f := range af` over a nil slice is a no-op, and the union-with-analyses branch still adds the analysis-only IDs).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -v`
Expected: PASS across the whole package, including the new test and all prior tasks' tests (confirms the `provisionAnalyses == nil`/empty case — the overwhelming majority of aspects and all pre-ADR-2026070602 runs — is unaffected, since `passOrderContains`/the injection only fires when `len(provisionAnalyses) > 0`).

- [ ] **Step 5: Commit**

```bash
cd ChenWeb
git add server/api/doc-reviews/typst_report.go server/api/doc-reviews/typst_report_test.go
git commit -m "doc-review typst report: render provisions section even with zero findings when analyses exist"
```

---

### Task 7: Typst template — `artifact-group` helper + `aspect-section` `artifact-groups` param

**Files:**
- Modify: `docs/doc-templates/template-document-report.typ`
- Test: `server/api/doc-reviews/typst_report_test.go` (compile check, skipped if the `typst` binary isn't installed — same pattern as `TestTypstReportCompilesWithDelimiterHeavyContent`)

**Interfaces:**
- Consumes: nothing Go-side; this is the Typst-side counterpart of the `artifact-group(...)`/`artifact-groups: (...)` calls Task 4/6 already emit as source text.
- Produces: `#let artifact-group(title:, analyses:, findings:)`, and `#let aspect-section(..., artifact-groups: (), ...)` — both callable from the generated `.typ` source.

- [ ] **Step 1: Write the failing test**

Add to `server/api/doc-reviews/typst_report_test.go`:

```go
func TestTypstTemplateCompilesArtifactGroupSection(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst binary not available; skipping compile check")
	}

	templatePath, err := filepath.Abs("../../docs/doc-templates/template-document-report.typ")
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}

	req := &RequestStatus{RequesterName: "Reviewer"}
	skeleton := &ReportSkeleton{
		Meta: ReportMeta{ReportID: "rpt_88", DocumentTitle: "Document title", GeneratedAt: "2026-07-06T12:00:00Z"},
		FindingsByPass: map[string]PassGroup{
			"P5": {
				Label: "Technical & Compliance",
				Findings: []ReportFinding{
					{ID: 1, Pass: "P5", Aspect: "provisions", Severity: "medium", Title: "Conflict",
						Description: "Description", Suggestion: "Suggestion", ArtifactID: "1001_prv_3",
						Sources: []SourceContext{{Source: "20: source line"}}},
				},
			},
		},
		PassOrder: []string{"P5"},
	}
	provisionAnalyses := map[string][]ProvisionAnalysis{
		"1001_prv_3": {{RelatedArtifactID: "2002_prv_9", Relationship: "same_subject", Summary: "Also checked this candidate."}},
	}

	src := buildTypstSource(skeleton, req, "en", templatePath, provisionAnalyses)

	dir := t.TempDir()
	typPath := filepath.Join(dir, "check.typ")
	if err := os.WriteFile(typPath, []byte(src), 0o644); err != nil {
		t.Fatalf("write typ: %v", err)
	}
	out, err := exec.Command("typst", "compile", "--root", "/", typPath, filepath.Join(dir, "check.pdf")).CombinedOutput()
	if err != nil {
		t.Fatalf("typst compile failed: %v\n%s\n--- source ---\n%s", err, out, src)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -run TestTypstTemplateCompilesArtifactGroupSection -v`
Expected: FAIL with a Typst compile error — `artifact-group` is not defined by the template, and `aspect-section` doesn't accept `artifact-groups:`. (If the `typst` binary isn't installed in this environment, the test SKIPs instead — that's expected too; run it wherever `typst` is available before considering this task done.)

- [ ] **Step 3: Write minimal implementation**

In `docs/doc-templates/template-document-report.typ`, add a new `artifact-group` function right after `review-finding` and before the `// ── aspect-section:` comment (i.e., insert after line 178's closing `}` of `review-finding`):

```typst
// ── artifact-group: findings + comparison analyses for one artifact ─
// Parameters:
//   title    – artifact identifier (metric_id / prov_id / inventory_item_id)
//   analyses – array of dicts: (related: string, relationship: string, summary: content).
//              Comparison records for this artifact, retained independent of
//              whether a finding was raised (ADR 2026070602). Empty when the
//              reviewer has no analyses table yet (metrics, inventory_items).
//   findings – array of content blocks produced by review-finding(...)
#let artifact-group(
  title: "",
  analyses: (),
  findings: (),
) = {
  heading(level: 3, title)

  if analyses.len() > 0 {
    text(weight: "semibold", size: 9pt, fill: clr-muted, "Comparison Analyses")
    v(3pt)
    for a in analyses {
      block(
        width: 100%,
        fill: clr-source-bg,
        radius: 4pt,
        inset: (x: 10pt, y: 8pt),
        {
          text(weight: "semibold", size: 8.5pt, fill: clr-muted,
            "vs. " + a.related + "  ·  " + a.relationship)
          v(3pt)
          text(size: 9pt, a.summary)
        },
      )
      v(6pt)
    }
    v(4pt)
  }

  if findings.len() > 0 {
    for f in findings {
      v(8pt)
      f
    }
  } else {
    v(4pt)
    text(style: "italic", fill: clr-muted, size: 9pt, "No findings for this artifact.")
  }
}
```

Then modify `aspect-section` to accept and prefer `artifact-groups`:

```typst
#let aspect-section(
  title: "",
  findings: (),
  artifact-groups: (),
  assessment: [],
  problems: [],
  guidelines: [],
) = {
  heading(level: 2, title)

  if artifact-groups.len() > 0 {
    for g in artifact-groups {
      v(8pt)
      g
    }
  } else if findings.len() > 0 {
    for f in findings {
      v(8pt)
      f
    }
  } else {
    v(4pt)
    text(style: "italic", fill: clr-muted, size: 9pt, "No findings for this aspect.")
  }

  v(12pt)
  heading(level: 4, "Overall Assessment")
  assessment
  v(8pt)

  heading(level: 4, "Main Problem Analysis")
  problems
  v(8pt)

  heading(level: 4, "Guidelines and Recommendations")
  guidelines
}
```

(Only the function signature and the `findings.len() > 0` branch structure change; the `assessment`/`problems`/`guidelines` rendering below is untouched.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-reviews/... -v`
Expected: PASS across the whole package. If `typst` isn't installed locally, `TestTypstTemplateCompilesArtifactGroupSection` and `TestTypstReportCompilesWithDelimiterHeavyContent` both SKIP (not fail) — that's the existing, accepted pattern in this file.

- [ ] **Step 5: Commit**

```bash
cd ChenWeb
git add docs/doc-templates/template-document-report.typ server/api/doc-reviews/typst_report_test.go
git commit -m "doc-review typst template: add artifact-group helper for per-artifact sections"
```

---

## Self-Review

**Spec coverage** (against ADR 2026062203 §1.2, as scoped by the user's decisions this session):
- "Findings grouped per-artifact for metrics/provisions/inventory_items, one section per artifact" → Task 4 (`buildArtifactGroupsArg`, `isArtifactAnchoredAspect`).
- "Reviewer parts stay separate, not mixed" → already true (existing `aspect-section` per aspect); untouched.
- "Compile analysis and findings of the same artifact into the same section" → Task 4 (analyses embedded inside each `artifact-group` call alongside that artifact's findings).
- "Union: provisions with analyses but no findings still get a section" (this session's clarification) → Task 6.
- "`artifact_id` propagates from DB into the report" → Task 1.
- "Read back `kb.doc_review_provision_analyses`" → Task 2.
- Typst template support for the new structure → Task 7.
- Metrics/inventory-items get artifact grouping but no analyses (no table yet, per this session's earlier scope decision) → Task 4's `buildArtifactGroupsArg` only unions analyses when `aspect == "provisions"`; metrics/inventory_items groups are pure finding-derived, `analyses: ()` always empty for them.

**Out of scope, confirmed with user:** ADR 2026070604 (metrics/inventory-items own analyses tables) — not implemented by this plan. HTML/Markdown renderers — not touched.

**Placeholder scan:** no TBD/TODO markers; every step has complete code and exact commands.

**Type consistency:** `ProvisionAnalysis` (existing type, `review-provisions.go:274`) used consistently as `map[string][]ProvisionAnalysis` across `loadProvisionAnalysesByRun` (Task 2), `typstVariant.provisionAnalyses` (Task 5), `buildTypstSource`'s new parameter (Task 4), and `buildArtifactGroupsArg` (Task 4) — same type everywhere, no renaming drift.
