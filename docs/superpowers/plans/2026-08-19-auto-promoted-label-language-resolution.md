# Auto-Promoted Label Language Resolution Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make auto-promoted ontology labels language-aware and usable by the governed name resolver like human-approved content.

**Architecture:** A pure Unicode-script classifier in the keywords package returns `zh`, `en`, or `und` for each auto-promoted label. The auto-promotion transaction uses that classifier for preferred labels and aliases. The name resolver’s governed-content predicates include both usable lifecycle statuses, so exact lookup and preferred-name display see auto-promoted rows.

**Tech Stack:** Go 1.25, `unicode`, `database/sql`, `go-sqlmock`, PostgreSQL-backed ontology stores.

---

## File structure

- Create: `server/api/ontology/keywords/label_language.go` — pure script-to-language resolver.
- Create: `server/api/ontology/keywords/label_language_test.go` — table-driven classification tests.
- Modify: `server/api/ontology/keywords/alignment.go` — use resolved language for each auto-promoted label.
- Modify: `server/api/ontology/keywords/alignment_test.go` — prove the transaction writes resolved preferred and alias languages.
- Modify: `server/api/ontology/names/resolver.go` — include auto-promoted terms and labels in usable governed lookups and display selection.
- Modify: `server/api/ontology/names/resolver_test.go` — prove auto-promoted labels resolve under language filters and provide preferred display names.
- Modify: `KnowledgeStore/doc-repo/user-manuals/ontology-labels-guide-v1.0-en.md` — replace the temporary known-limitation wording with the implemented behavior.

## Chunk 1: Language resolution and auto-promotion writes

### Task 1: Specify the script classifier with failing tests

**Files:**

- Create: `server/api/ontology/keywords/label_language_test.go`

- [ ] **Step 1: Write failing table-driven tests**

Cover `每户配备分类垃圾容器的数量` → `zh`, `Display luminance` → `en`, mixed `显示 luminance` → `zh`, and blank, `250`, `---`, and Arabic-script input → `und`.

- [ ] **Step 2: Run the focused test to verify RED**

Run: `go test ./server/api/ontology/keywords -run TestAutoPromotedLabelLanguage -count=1`

Expected: FAIL because the resolver does not exist.

- [ ] **Step 3: Implement the smallest pure resolver**

Create `autoPromotedLabelLanguage(label string) string`. Scan runes with `unicode.Is(unicode.Han, r)` and `unicode.Is(unicode.Latin, r)`: Han wins and returns `zh`; otherwise Latin returns `en`; otherwise return `und`.

- [ ] **Step 4: Run the focused test to verify GREEN**

Run: `go test ./server/api/ontology/keywords -run TestAutoPromotedLabelLanguage -count=1`

Expected: PASS.

### Task 2: Use the resolver in the transaction

**Files:**

- Modify: `server/api/ontology/keywords/alignment.go:355-376`
- Modify: `server/api/ontology/keywords/alignment_test.go:474-581`

- [ ] **Step 1: Write a failing auto-promotion transaction test**

Add transaction cases proving a Chinese preferred label and Chinese alias are both inserted with `zh`, English preferred and alternate labels are inserted with `en`, and a Chinese preferred label with a Latin alias persists `zh` and `en` independently.

- [ ] **Step 2: Run the focused test to verify RED**

Run: `go test ./server/api/ontology/keywords -run TestAlignmentsStoreEnsureAcceptedOrCreateAutoCreatesTerm -count=1`

Expected: FAIL because production code still passes `und` for each label.

- [ ] **Step 3: Replace fixed `und` assignments**

Set `Lang` from `autoPromotedLabelLanguage` for the canonical label and each non-empty alias. Do not change transaction boundaries, statuses, term identifiers, or alignment writes.

- [ ] **Step 4: Run focused keyword tests to verify GREEN**

Run: `go test ./server/api/ontology/keywords -run 'Test(AutoPromotedLabelLanguage|AlignmentsStoreEnsureAcceptedOrCreate)' -count=1`

Expected: PASS.

## Chunk 2: Runtime visibility and documentation

### Task 3: Make the governed name resolver treat auto-promoted rows as usable

**Files:**

- Modify: `server/api/ontology/names/resolver.go:364-404`
- Modify: `server/api/ontology/names/resolver_test.go`

- [ ] **Step 1: Write failing resolver tests**

Add sqlmock-backed tests where both the ontology term and label have `auto-promoted` status. Assert `zh`, `en`, and `und` labels can be exact-matched when their requested language matches, and that `termPrefName` returns an auto-promoted preferred label only when its joined term also has a usable status.

- [ ] **Step 2: Run the focused test to verify RED**

Run: `go test ./server/api/ontology/names -run 'Test.*AutoPromoted' -count=1`

Expected: FAIL because the SQL predicates require `included_in_release`.

- [ ] **Step 3: Widen only the usable-status predicates**

Change the term-label exact lookup and preferred-name lookup to join terms and accept `status IN ('included_in_release', 'auto-promoted')` for both terms and labels. Preserve all kind, module, language, normalization, ambiguity, and ranking behavior. The preferred-name join also prevents an orphaned label from being displayed as a usable term name.

- [ ] **Step 4: Run the focused resolver tests to verify GREEN**

Run: `go test ./server/api/ontology/names -run 'Test.*AutoPromoted' -count=1`

Expected: PASS.

### Task 4: Update the manual and verify the completed change

**Files:**

- Modify: `KnowledgeStore/doc-repo/user-manuals/ontology-labels-guide-v1.0-en.md`

- [ ] **Step 1: Revise the language section**

Replace the statement that auto-promotion currently writes `und` for every label with the implemented deterministic Han/Latin/fallback rule. Update the related field cross-reference and common-pitfall wording, preserve the orphaned-data warning, and update the change log and modification time.

- [ ] **Step 2: Run all affected tests and formatting checks**

Run:

```bash
gofmt -w server/api/ontology/keywords/label_language.go server/api/ontology/keywords/label_language_test.go server/api/ontology/keywords/alignment.go server/api/ontology/keywords/alignment_test.go server/api/ontology/names/resolver.go server/api/ontology/names/resolver_test.go
go test ./server/api/ontology/keywords ./server/api/ontology/names ./server/api/doc-processing
git diff --check
```

Expected: all Go tests pass and `git diff --check` produces no output.

- [ ] **Step 3: Commit intentionally by repository**

Commit ChenWeb code and plan changes with Jujutsu. Commit the KnowledgeStore manual change separately with Jujutsu. Do not include unrelated `server/tmp`, diary, or ontology-terms manual changes.
