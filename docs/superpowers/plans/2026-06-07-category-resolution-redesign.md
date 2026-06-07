# Category Resolution Redesign Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace per-resolver in-memory snapshot + cosine matching with a process-wide hashmap and alias-based resolution, eliminating wasted LLM calls on non-English keys and across concurrent pipelines.

**Architecture:** A package-level `categoryIndex` singleton (sync.RWMutex, `map[categoryType]map[normalizedKey]int64`) replaces the per-resolver snapshot. Resolution is two-tier: exact hashmap lookup → LLM create. The rawKey (e.g., a Chinese term) is absorbed as an alias after LLM creation, so future occurrences hit tier 1. No embedding or cosine is used in the resolution path.

**Tech Stack:** Go 1.25, PostgreSQL, go-sqlmock (tests), goose (migration)

**Spec:** `KnowledgeStore/Capsules/coding-capsules/categories/category-resolution-redesign-2026-06-07.md`

---

## Chunk 1: Migration and categoryIndex data structure

### Task 1: DB migration — `kb.category_alias_conflicts`

**Files:**
- Create: `ChenWeb/project_migrations/20260607000002_add_kb_category_alias_conflicts.sql`

- [ ] **Step 1: Write the migration file**

```sql
-- +goose Up
CREATE TABLE kb.category_alias_conflicts (
    alias         text NOT NULL,
    category_type text NOT NULL,
    category_ids  int8[] NOT NULL,
    detected_at   timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT category_alias_conflicts_pkey PRIMARY KEY (category_type, alias)
);

-- +goose Down
DROP TABLE IF EXISTS kb.category_alias_conflicts;
```

- [ ] **Step 2: Verify the migration runs**

```bash
cd ChenWeb
go run ./cmd/server/... --migrate-only 2>&1 | grep -E "category_alias_conflicts|error|Error" | head -20
```

Expected: migration applied without error (or "already applied" if run before).

- [ ] **Step 3: Commit**

```bash
git add project_migrations/20260607000002_add_kb_category_alias_conflicts.sql
git commit -m "feat: add kb.category_alias_conflicts table for alias conflict tracking"
```

---

### Task 2: `categoryIndex` — process-wide in-memory index

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_registry.go` (add after `artifactCategoryRecord` type, around line 175)

- [ ] **Step 1: Write the failing test for `categoryIndex.lookup` and `put`**

Add to `ChenWeb/server/api/doc-processing/artifact_category_registry_test.go`:

```go
func TestCategoryIndexLookupMiss(t *testing.T) {
    idx := newCategoryIndex()
    if _, ok := idx.lookup("metric", "latency"); ok {
        t.Fatal("expected miss on empty index")
    }
}

func TestCategoryIndexPutAndLookup(t *testing.T) {
    idx := newCategoryIndex()
    idx.put("metric", "latency", 7, 0)
    id, ok := idx.lookup("metric", "latency")
    if !ok || id != 7 {
        t.Fatalf("lookup = (%d, %v), want (7, true)", id, ok)
    }
}

func TestCategoryIndexPutConflictKeepsHigherSeenCount(t *testing.T) {
    idx := newCategoryIndex()
    idx.put("metric", "rt", 1, 5)  // seen_count=5
    conflict := idx.put("metric", "rt", 2, 10) // seen_count=10, wins
    if !conflict {
        t.Fatal("expected conflict=true")
    }
    id, _ := idx.lookup("metric", "rt")
    if id != 2 {
        t.Fatalf("id = %d, want 2 (higher seen_count wins)", id)
    }
}

func TestCategoryIndexPutConflictKeepsExistingIfHigherSeenCount(t *testing.T) {
    idx := newCategoryIndex()
    idx.put("metric", "rt", 1, 10) // seen_count=10
    conflict := idx.put("metric", "rt", 2, 5) // seen_count=5, loses
    if !conflict {
        t.Fatal("expected conflict=true")
    }
    id, _ := idx.lookup("metric", "rt")
    if id != 1 {
        t.Fatalf("id = %d, want 1 (higher seen_count wins)", id)
    }
}

func TestCategoryIndexPutAllSetsMultipleKeys(t *testing.T) {
    idx := newCategoryIndex()
    idx.putAll("metric", []string{"latency", "response time", "rt"}, 7)
    for _, k := range []string{"latency", "response time", "rt"} {
        if id, ok := idx.lookup("metric", k); !ok || id != 7 {
            t.Errorf("lookup(%q) = (%d, %v), want (7, true)", k, id, ok)
        }
    }
}

func TestCategoryIndexIsLoadedAndMarkLoaded(t *testing.T) {
    idx := newCategoryIndex()
    if idx.isLoaded("metric") {
        t.Fatal("expected not loaded")
    }
    idx.markLoaded("metric")
    if !idx.isLoaded("metric") {
        t.Fatal("expected loaded after markLoaded")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd ChenWeb && go test ./server/api/doc-processing/... -run "TestCategoryIndex" -v 2>&1 | tail -20
```

Expected: FAIL — `newCategoryIndex` undefined.

- [ ] **Step 3: Implement `categoryIndex`**

Add to `artifact_category_registry.go`, after the `artifactCategoryRecord` struct (around line 175):

```go
// categoryIndex is a process-wide in-memory lookup table mapping (categoryType,
// normalizedKey) → category_id. It is the primary resolution mechanism: exact match
// on the key plus all known aliases, acronyms, and display names. All writes are
// protected by a RWMutex; reads use RLock.
type categoryIndex struct {
    mu     sync.RWMutex
    loaded map[string]bool
    byType map[string]map[string]int64      // [categoryType][normalizedKey] → categoryID
    seenBy map[string]map[string]int64      // [categoryType][normalizedKey] → seen_count (for conflict resolution)
}

func newCategoryIndex() *categoryIndex {
    return &categoryIndex{
        loaded: map[string]bool{},
        byType: map[string]map[string]int64{},
        seenBy: map[string]map[string]int64{},
    }
}

// globalCategoryIndex is the process-wide singleton used by all categoryResolver
// instances. Tests inject a fresh categoryIndex per test to avoid cross-test pollution.
var globalCategoryIndex = newCategoryIndex()

// lookup returns the category_id for normKey within categoryType, or false if absent.
func (ci *categoryIndex) lookup(categoryType, normKey string) (int64, bool) {
    ci.mu.RLock()
    defer ci.mu.RUnlock()
    m := ci.byType[categoryType]
    if m == nil {
        return 0, false
    }
    id, ok := m[normKey]
    return id, ok
}

// isLoaded reports whether the index has been populated from the DB for categoryType.
func (ci *categoryIndex) isLoaded(categoryType string) bool {
    ci.mu.RLock()
    defer ci.mu.RUnlock()
    return ci.loaded[categoryType]
}

// markLoaded marks categoryType as fully loaded from the DB.
func (ci *categoryIndex) markLoaded(categoryType string) {
    ci.mu.Lock()
    defer ci.mu.Unlock()
    ci.loaded[categoryType] = true
}

// put adds normKey→id to the index. seenCount is used to resolve conflicts: the entry
// with the higher seen_count wins. Returns true if a conflict was detected (a different
// id already occupied this key).
func (ci *categoryIndex) put(categoryType, normKey string, id int64, seenCount int64) (conflict bool) {
    if normKey == "" {
        return false
    }
    ci.mu.Lock()
    defer ci.mu.Unlock()
    m := ci.byType[categoryType]
    if m == nil {
        m = map[string]int64{}
        ci.byType[categoryType] = m
        ci.seenBy[categoryType] = map[string]int64{}
    }
    sc := ci.seenBy[categoryType]
    if existing, ok := m[normKey]; ok && existing != id {
        conflict = true
        if seenCount <= sc[normKey] {
            return // existing entry has higher or equal seen_count; do not overwrite
        }
    }
    m[normKey] = id
    sc[normKey] = seenCount
    return
}

// putAll calls put for each key in keys, ignoring empties. seenCount 0 is used
// (appropriate for newly created categories which have seen_count=1 in the DB).
func (ci *categoryIndex) putAll(categoryType string, keys []string, id int64) {
    for _, k := range keys {
        ci.put(categoryType, k, id, 0)
    }
}
```

Add `"sync"` to the import block if not already present.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ChenWeb && go test ./server/api/doc-processing/... -run "TestCategoryIndex" -v 2>&1 | tail -20
```

Expected: all 6 `TestCategoryIndex*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server/api/doc-processing/artifact_category_registry.go \
        server/api/doc-processing/artifact_category_registry_test.go
git commit -m "feat: add process-wide categoryIndex for category resolution"
```

---

### Task 3: Update `loadActiveCategories` + add `logAliasConflict`

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_registry.go`

- [ ] **Step 1: Write the failing test for `loadIntoIndex`**

Add to `artifact_category_registry_db_test.go`:

```go
func TestLoadIntoIndexPopulatesFromMatchKeys(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New: %v", err)
    }
    defer db.Close()

    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("metric").
        WillReturnRows(
            sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}).
                AddRow(int64(1), "latency", "approved", "", []byte(`["latency","rt"]`), int64(10)).
                AddRow(int64(2), "throughput", "approved", "", []byte(`["throughput"]`), int64(5)),
        )

    reg := artifactCategoryRegistry{DB: db}
    idx := newCategoryIndex()
    conflicts, err := reg.loadIntoIndex(context.Background(), "metric", idx)
    if err != nil {
        t.Fatalf("loadIntoIndex: %v", err)
    }
    if len(conflicts) != 0 {
        t.Fatalf("unexpected conflicts: %v", conflicts)
    }
    if id, ok := idx.lookup("metric", "latency"); !ok || id != 1 {
        t.Errorf("latency → %d (%v), want 1", id, ok)
    }
    if id, ok := idx.lookup("metric", "rt"); !ok || id != 1 {
        t.Errorf("rt → %d (%v), want 1", id, ok)
    }
    if id, ok := idx.lookup("metric", "throughput"); !ok || id != 2 {
        t.Errorf("throughput → %d (%v), want 2", id, ok)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("unmet sql expectations: %v", err)
    }
}

func TestLoadIntoIndexDetectsConflict(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New: %v", err)
    }
    defer db.Close()

    // Both rows share "rt" in match_keys; row 1 has higher seen_count and wins.
    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("metric").
        WillReturnRows(
            sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}).
                AddRow(int64(1), "response time", "approved", "", []byte(`["response time","rt"]`), int64(20)).
                AddRow(int64(2), "reaction time", "approved", "", []byte(`["reaction time","rt"]`), int64(3)),
        )

    reg := artifactCategoryRegistry{DB: db}
    idx := newCategoryIndex()
    conflicts, err := reg.loadIntoIndex(context.Background(), "metric", idx)
    if err != nil {
        t.Fatalf("loadIntoIndex: %v", err)
    }
    if len(conflicts) != 1 || conflicts[0].Alias != "rt" {
        t.Fatalf("conflicts = %v, want [{alias:rt ...}]", conflicts)
    }
    id, _ := idx.lookup("metric", "rt")
    if id != 1 {
        t.Fatalf("rt → %d, want 1 (higher seen_count)", id)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("unmet sql expectations: %v", err)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd ChenWeb && go test ./server/api/doc-processing/... -run "TestLoadIntoIndex" -v 2>&1 | tail -10
```

Expected: FAIL — `loadIntoIndex` undefined.

- [ ] **Step 3: Add `aliasConflict` type and `loadIntoIndex` method**

In `artifact_category_registry.go`, replace the existing `loadActiveCategories` method (lines 22-48) with:

```go
// aliasConflict records a key that maps to two or more different category IDs.
type aliasConflict struct {
    Alias    string
    IDs      []int64
}

// loadIntoIndex loads all approved ∪ pending_review rows for categoryType from the DB,
// populates idx with their match_keys, and returns any alias conflicts found.
// Records are processed in descending seen_count order so the higher-seen_count entry
// wins on conflict.
func (r artifactCategoryRegistry) loadIntoIndex(ctx context.Context, categoryType string, idx *categoryIndex) ([]aliasConflict, error) {
    const q = `
SELECT category_id, category_key, status, COALESCE(canonical_of, ''), match_keys, seen_count
FROM kb.artifact_categories
WHERE category_type = $1 AND status IN ('approved', 'pending_review')
ORDER BY seen_count DESC`
    rows, err := r.DB.QueryContext(ctx, q, categoryType)
    if err != nil {
        return nil, fmt.Errorf("(MID_26060701) load categories into index: %w", err)
    }
    defer rows.Close()

    conflictMap := map[string][]int64{}
    for rows.Next() {
        var (
            rec       artifactCategoryRecord
            matchKeys []byte
            seenCount int64
        )
        if err := rows.Scan(&rec.CategoryID, &rec.CategoryKey, &rec.Status, &rec.CanonicalOf, &matchKeys, &seenCount); err != nil {
            return nil, err
        }
        rec.CategoryType = categoryType
        _ = json.Unmarshal(matchKeys, &rec.MatchKeys)

        for _, mk := range rec.MatchKeys {
            if conflict := idx.put(categoryType, mk, rec.CategoryID, seenCount); conflict {
                existing, _ := idx.lookup(categoryType, mk)
                ids := []int64{existing, rec.CategoryID}
                if existing == rec.CategoryID {
                    // higher seen_count won; flip to show the loser
                    ids = []int64{rec.CategoryID, existing}
                }
                conflictMap[mk] = ids
            }
        }
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }
    idx.markLoaded(categoryType)

    var out []aliasConflict
    for alias, ids := range conflictMap {
        out = append(out, aliasConflict{Alias: alias, IDs: ids})
    }
    return out, nil
}

// loadActiveCategories returns the approved ∪ pending_review categories as a slice,
// used by the snapshot-style path (retained for inventory registry compatibility).
func (r artifactCategoryRegistry) loadActiveCategories(ctx context.Context, categoryType string) ([]artifactCategoryRecord, error) {
    const q = `
SELECT category_id, category_key, status, COALESCE(canonical_of, ''), match_keys, seen_count
FROM kb.artifact_categories
WHERE category_type = $1 AND status IN ('approved', 'pending_review')`
    rows, err := r.DB.QueryContext(ctx, q, categoryType)
    if err != nil {
        return nil, fmt.Errorf("(MID_26060410) load active categories: %w", err)
    }
    defer rows.Close()
    var out []artifactCategoryRecord
    for rows.Next() {
        var (
            rec       artifactCategoryRecord
            matchKeys []byte
            seenCount int64
        )
        if err := rows.Scan(&rec.CategoryID, &rec.CategoryKey, &rec.Status, &rec.CanonicalOf, &matchKeys, &seenCount); err != nil {
            return nil, err
        }
        rec.CategoryType = categoryType
        _ = json.Unmarshal(matchKeys, &rec.MatchKeys)
        out = append(out, rec)
    }
    return out, rows.Err()
}
```

- [ ] **Step 4: Add `logAliasConflict` method**

Add to `artifact_category_registry.go` after `absorbAlias`:

```go
// logAliasConflict upserts an alias conflict record. categoryIDs is the full set of
// category IDs that share alias within categoryType.
func (r artifactCategoryRegistry) logAliasConflict(ctx context.Context, categoryType, alias string, categoryIDs []int64) error {
    const stmt = `
INSERT INTO kb.category_alias_conflicts (alias, category_type, category_ids, detected_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (category_type, alias) DO UPDATE
    SET category_ids = EXCLUDED.category_ids,
        detected_at  = NOW()`
    if _, err := r.DB.ExecContext(ctx, stmt, alias, categoryType, pq.Array(categoryIDs)); err != nil {
        return fmt.Errorf("(MID_26060702) log alias conflict %q: %w", alias, err)
    }
    return nil
}
```

Add `"github.com/lib/pq"` to the import block (check if already imported; if not, add it).

- [ ] **Step 5: Run tests**

```bash
cd ChenWeb && go test ./server/api/doc-processing/... -run "TestLoadIntoIndex" -v 2>&1 | tail -10
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/api/doc-processing/artifact_category_registry.go \
        server/api/doc-processing/artifact_category_registry_db_test.go
git commit -m "feat: add loadIntoIndex, logAliasConflict; categoryIndex data structure"
```

---

## Chunk 2: Resolver rewrite

### Task 4: Rewrite `categoryResolver` — swap snapshot for injected index

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_resolver.go`

The resolver struct changes from holding its own snapshot to holding a `*categoryIndex`.
The `ensureLoaded` method is replaced by `ensureIndexLoaded` which delegates to `reg.loadIntoIndex`.

- [ ] **Step 1: Update the resolver struct and constructor**

Replace the `categoryResolver` struct and `newCategoryResolver` function (lines 29-58):

```go
// categoryResolver implements the Identify Artifact Categories procedure: it resolves a
// raw category key to a kb.artifact_categories.category_id. It looks up the key in the
// process-wide category index (exact/alias match) before creating a new category via the
// LLM. The index field is the process-wide globalCategoryIndex in production; tests
// inject a fresh categoryIndex per test for isolation.
type categoryResolver struct {
    reg     artifactCategoryRegistry
    creator categoryCreator
    index   *categoryIndex
}

func newCategoryResolver(reg artifactCategoryRegistry, creator categoryCreator) *categoryResolver {
    return &categoryResolver{reg: reg, creator: creator, index: globalCategoryIndex}
}
```

- [ ] **Step 2: Replace `ensureLoaded` with `ensureIndexLoaded`**

Replace the `ensureLoaded` method (lines 330-346) with:

```go
// ensureIndexLoaded populates the process-wide index for categoryType from the DB if it
// has not been loaded yet. Any alias conflicts detected during load are written to
// kb.category_alias_conflicts (best-effort; log-only on failure).
func (cr *categoryResolver) ensureIndexLoaded(ctx context.Context, categoryType string) error {
    if cr.index.isLoaded(categoryType) {
        return nil
    }
    conflicts, err := cr.reg.loadIntoIndex(ctx, categoryType, cr.index)
    if err != nil {
        return err
    }
    for _, c := range conflicts {
        if lerr := cr.reg.logAliasConflict(ctx, categoryType, c.Alias, c.IDs); lerr != nil {
            // best-effort: log but do not fail resolution
            _ = lerr
        }
    }
    return nil
}
```

- [ ] **Step 3: Rewrite `Resolve`**

Replace the `Resolve` method (lines 63-92):

```go
// Resolve returns the category_id for rawKey under categoryType, creating the category
// via the LLM when no existing one matches. evidence is optional context (the triggering
// artifact) passed to the LLM for disambiguation on a miss.
func (cr *categoryResolver) Resolve(ctx context.Context, rawKey, categoryType string, evidence map[string]any) (int64, error) {
    normKey := normalizeCategoryKey(rawKey)
    if normKey == "" {
        return 0, fmt.Errorf("(MID_26060420) empty category key")
    }
    if err := cr.ensureIndexLoaded(ctx, categoryType); err != nil {
        return 0, err
    }

    if id, ok := cr.index.lookup(categoryType, normKey); ok {
        if err := cr.reg.absorbAlias(ctx, id, normKey); err != nil {
            return 0, err
        }
        return id, nil
    }

    if cr.creator == nil {
        return 0, fmt.Errorf("(MID_26060422) no creator configured; cannot create category %q", normKey)
    }
    return cr.createAndMint(ctx, categoryType, normKey, evidence)
}
```

- [ ] **Step 4: Rewrite `createAndMint`**

Replace the `createAndMint` method (lines 238-272):

```go
// createAndMint creates a category via the LLM and persists it. It coalesces concurrent
// creates of the same (categoryType, normKey) across the process via categoryCreateGroup
// so only one LLM call is made per novel key. The shared work runs on a detached context
// so one caller cancelling does not abort a create that other pipelines are awaiting.
// After a successful create, both the canonical key and the original normKey are added to
// the process-wide index — normKey acts as the translation cache for non-English input.
func (cr *categoryResolver) createAndMint(ctx context.Context, categoryType, normKey string, evidence map[string]any) (int64, error) {
    ch := categoryCreateGroup.DoChan(categoryType+"\x00"+normKey, func() (any, error) {
        bg := context.WithoutCancel(ctx)
        created, err := cr.creator.CreateCategory(bg, normKey, categoryType, evidence)
        if err != nil {
            return nil, fmt.Errorf("(MID_26060421) create category %q: %w", normKey, err)
        }
        id, err := cr.reg.mintCategory(bg, created, categoryType, nil)
        if err != nil {
            return nil, err
        }
        // Index the canonical key and all LLM-returned surface forms.
        cr.index.put(categoryType, normalizeCategoryKey(created.CategoryKey), id, 1)
        cr.index.putAll(categoryType, normalizeCategoryKeys(created.DisplayNames), id)
        cr.index.putAll(categoryType, normalizeCategoryKeys(created.Aliases), id)
        cr.index.putAll(categoryType, normalizeCategoryKeys(created.Acronyms), id)
        // Index the original rawKey as an alias so non-English keys resolve next time.
        cr.index.put(categoryType, normKey, id, 1)
        if aerr := cr.reg.absorbAlias(bg, id, normKey); aerr != nil {
            // best-effort: the DB write failing does not break resolution for this run
            _ = aerr
        }
        return id, nil
    })

    select {
    case <-ctx.Done():
        return 0, ctx.Err()
    case res := <-ch:
        if res.Err != nil {
            return 0, res.Err
        }
        return res.Val.(int64), nil
    }
}
```

- [ ] **Step 5: Add `normalizeCategoryKeys` helper**

Add to `artifact_category_registry.go` near `normalizeCategoryKey`:

```go
// normalizeCategoryKeys normalizes a slice of raw keys, dropping empties.
func normalizeCategoryKeys(raw []string) []string {
    out := make([]string, 0, len(raw))
    for _, s := range raw {
        if k := normalizeCategoryKey(s); k != "" {
            out = append(out, k)
        }
    }
    return out
}
```

- [ ] **Step 6: Build to check for compile errors**

```bash
cd ChenWeb && go build ./server/api/doc-processing/... 2>&1 | head -30
```

Expected: compile errors about removed fields/methods — these will be fixed in subsequent tasks.

- [ ] **Step 7: Commit partial (even if broken) to preserve the work**

```bash
git add server/api/doc-processing/artifact_category_resolver.go \
        server/api/doc-processing/artifact_category_registry.go
git commit -m "wip: rewrite categoryResolver to use process-wide categoryIndex"
```

---

### Task 5: Rewrite `ResolveBatch` — remove clustering and embedding phases

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_resolver.go`

- [ ] **Step 1: Replace `ResolveBatch`**

Replace the entire `ResolveBatch` method (lines 110-231):

```go
// ResolveBatch resolves many raw keys of one categoryType in a single pass. It
// normalizes and dedups, looks each up in the process-wide index, and concurrently
// creates any that are still missing — bounded by maxConcurrency and coalesced
// process-wide by categoryCreateGroup. Returns a map from normalized key to
// category_id; keys that failed to resolve are absent from ids and present in errs.
func (cr *categoryResolver) ResolveBatch(ctx context.Context, categoryType string, reqs []categoryRequest, maxConcurrency int) (map[string]int64, map[string]error) {
    ids := make(map[string]int64)
    errs := make(map[string]error)
    if len(reqs) == 0 {
        return ids, errs
    }
    if maxConcurrency < 1 {
        maxConcurrency = 1
    }

    // Phase 0 — normalize + dedup, preserving first-seen order and evidence.
    type distinctKey struct {
        norm     string
        evidence map[string]any
    }
    var distinct []distinctKey
    seen := map[string]struct{}{}
    for _, r := range reqs {
        nk := normalizeCategoryKey(r.RawKey)
        if nk == "" {
            continue
        }
        if _, ok := seen[nk]; ok {
            continue
        }
        seen[nk] = struct{}{}
        distinct = append(distinct, distinctKey{norm: nk, evidence: r.Evidence})
    }
    if len(distinct) == 0 {
        return ids, errs
    }

    if err := cr.ensureIndexLoaded(ctx, categoryType); err != nil {
        for _, d := range distinct {
            errs[d.norm] = err
        }
        return ids, errs
    }

    // Phase 1 — index lookup; collect misses.
    type missEntry struct {
        norm     string
        evidence map[string]any
    }
    var misses []missEntry
    for _, d := range distinct {
        if id, ok := cr.index.lookup(categoryType, d.norm); ok {
            if err := cr.reg.absorbAlias(ctx, id, d.norm); err != nil {
                errs[d.norm] = err
                continue
            }
            ids[d.norm] = id
            continue
        }
        misses = append(misses, missEntry{norm: d.norm, evidence: d.evidence})
    }
    if len(misses) == 0 {
        return ids, errs
    }
    if cr.creator == nil {
        for _, m := range misses {
            errs[m.norm] = fmt.Errorf("(MID_26060523) no creator configured; cannot create category %q", m.norm)
        }
        return ids, errs
    }

    // Phase 2 — create each miss concurrently, bounded by maxConcurrency.
    var (
        mu  sync.Mutex
        wg  sync.WaitGroup
        sem = make(chan struct{}, maxConcurrency)
    )
    for _, m := range misses {
        wg.Add(1)
        sem <- struct{}{}
        go func() {
            defer wg.Done()
            defer func() { <-sem }()
            id, err := cr.createAndMint(ctx, categoryType, m.norm, m.evidence)
            mu.Lock()
            defer mu.Unlock()
            if err != nil {
                errs[m.norm] = err
                return
            }
            ids[m.norm] = id
        }()
    }
    wg.Wait()
    return ids, errs
}
```

- [ ] **Step 2: Build**

```bash
cd ChenWeb && go build ./server/api/doc-processing/... 2>&1 | head -30
```

Expected: remaining compile errors relate to deleted fields/interfaces in wiring and test files.

---

## Chunk 3: Cleanup, wiring, and tests

### Task 6: Delete dead code

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_resolver.go`
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_registry.go`

- [ ] **Step 1: Delete from `artifact_category_resolver.go`**

Delete the following functions entirely:
- `embedKeys` (lines 276-301)
- `clusterCategoryMisses` (lines 308-328)
- `addToSnapshot` (lines 350-367)
- The `categoryEmbedder` interface (lines 25-27)

- [ ] **Step 2: Remove cosine tier from `matchCategoryInSnapshot` in `artifact_category_registry.go`**

In `matchCategoryInSnapshot` (lines 183-208), delete the entire cosine block (the `if len(queryEmbedding) >= categoryEmbeddingMinDims` section), leaving only the tier-1/2 exact match loop:

```go
func matchCategoryInSnapshot(normKey string, snapshot []artifactCategoryRecord) (artifactCategoryRecord, bool) {
    for _, rec := range snapshot {
        for _, mk := range rec.MatchKeys {
            if mk == normKey {
                return rec, true
            }
        }
    }
    return artifactCategoryRecord{}, false
}
```

Update the signature (remove `queryEmbedding []float64` and `cosineThreshold float64` parameters). This function is still used by `inventory_category_registry.go` and its tests.

- [ ] **Step 3: Delete `categoryCosine` and `categoryEmbeddingMinDims`**

Delete from `artifact_category_registry.go`:
- `const categoryEmbeddingMinDims = 2` (line 163)
- `func categoryCosine(a, b []float64) float64` (lines 230-244)

- [ ] **Step 4: Remove `Embedding` field from `artifactCategoryRecord`**

In `artifactCategoryRecord` (lines 167-175), remove the `Embedding []float64` field. Also remove `embedding []byte` scan variable and `json.Unmarshal(embedding, &rec.Embedding)` from both `loadIntoIndex` and `loadActiveCategories` query scans (the `embedding` column is no longer fetched for category resolution).

- [ ] **Step 5: Build**

```bash
cd ChenWeb && go build ./server/api/doc-processing/... 2>&1 | head -30
```

Expected: remaining errors in wiring and test files.

---

### Task 7: Update wiring

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_wiring.go`

- [ ] **Step 1: Remove `searchCategoryEmbedder` and update `newMetricCategoryResolver`**

Delete the `searchCategoryEmbedder` struct, `newSearchCategoryEmbedder`, and `EmbedCategory` method (lines 63-87).

Replace `newMetricCategoryResolver` (lines 48-61):

```go
func newMetricCategoryResolver(db *sql.DB, logger ApiTypes.JimoLogger) *categoryResolver {
    var creator categoryCreator
    if c, err := newLLMCategoryCreator(db, logger); err != nil {
        if logger != nil {
            logger.Warn("category resolver: LLM creator unavailable; new categories cannot be created",
                "env", "CREATE_ARTIFACT_CATEGORY_PROMPT/CREATE_ARTIFACT_CATEGORY_MODEL_NAME", "error", err.Error())
        }
    } else {
        creator = c
    }
    return newCategoryResolver(artifactCategoryRegistry{DB: db}, creator)
}
```

Remove unused imports (`kbsearch`, `llmclients`, `strconv`) that were only used by the embedder. Keep what remains.

- [ ] **Step 2: Remove `categoryMatchMinCosine` and `categoryResolveMaxConcurrency` if unused**

Check if `categoryResolveMaxConcurrency` is still used by `ResolveBatch` callers. If not, delete both helpers. If `categoryResolveMaxConcurrency` is still called from `artifact_indexing.go`, keep it.

```bash
grep -n "categoryMatchMinCosine\|categoryResolveMaxConcurrency" /Users/cding/Workspace/ChenWeb/server/api/doc-processing/artifact_indexing.go
```

- [ ] **Step 3: Build**

```bash
cd ChenWeb && go build ./server/api/doc-processing/... 2>&1 | head -30
```

Expected: compile errors only in test files now.

---

### Task 8: Update tests

**Files:**
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_resolver_test.go`
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_batch_test.go`
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_registry_test.go`
- Modify: `ChenWeb/server/api/doc-processing/artifact_category_wiring_test.go` (if affected)

- [ ] **Step 1: Rewrite `artifact_category_resolver_test.go`**

The `fakeCategoryEmbedder` and `programmableCategoryEmbedder` are gone. Tests now inject a fresh `categoryIndex` directly. Replace the entire file:

```go
package docprocessing

import (
    "context"
    "testing"

    "github.com/DATA-DOG/go-sqlmock"
)

type fakeCategoryCreator struct {
    called int
    out    createdCategory
    err    error
}

func (f *fakeCategoryCreator) CreateCategory(_ context.Context, _ string, _ string, _ map[string]any) (createdCategory, error) {
    f.called++
    return f.out, f.err
}

func newResolverWithFreshIndex(db interface{ QueryContext(...) (*sql.Rows, error); ExecContext(...) (sql.Result, error) }, creator categoryCreator) *categoryResolver {
    // Use a fresh index per test — avoids cross-test state via globalCategoryIndex.
    cr := newCategoryResolver(artifactCategoryRegistry{DB: db.(*sql.DB)}, creator)
    cr.index = newCategoryIndex()
    return cr
}

func newLoadRows(id int64, key, status, matchKeys string) *sqlmock.Rows {
    return sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}).
        AddRow(id, key, status, "", []byte(matchKeys), int64(1))
}

func TestResolverReturnsExistingMatchWithoutCreating(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("metric").
        WillReturnRows(newLoadRows(7, "latency", "approved", `["latency"]`))
    mock.ExpectExec("UPDATE kb\\.artifact_categories").
        WithArgs(int64(7), "latency").
        WillReturnResult(sqlmock.NewResult(0, 1))

    creator := &fakeCategoryCreator{}
    cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: newCategoryIndex()}
    id, err := cr.Resolve(context.Background(), "Latency", "metric", nil)
    if err != nil {
        t.Fatalf("Resolve error: %v", err)
    }
    if id != 7 {
        t.Fatalf("Resolve id = %d, want 7", id)
    }
    if creator.called != 0 {
        t.Fatalf("creator called %d times, want 0", creator.called)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("unmet sql expectations: %v", err)
    }
}

func TestResolverCreatesOnMiss(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("metric").
        WillReturnRows(sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}))
    mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
        WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(42)))
    // absorbAlias for the normKey
    mock.ExpectExec("UPDATE kb\\.artifact_categories").
        WillReturnResult(sqlmock.NewResult(0, 1))

    creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "throughput"}}
    cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: newCategoryIndex()}
    id, err := cr.Resolve(context.Background(), "throughput", "metric", nil)
    if err != nil {
        t.Fatalf("Resolve error: %v", err)
    }
    if id != 42 {
        t.Fatalf("Resolve id = %d, want 42", id)
    }
    if creator.called != 1 {
        t.Fatalf("creator called %d times, want 1", creator.called)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("unmet sql expectations: %v", err)
    }
}

func TestResolverErrorsWhenNoCreatorOnMiss(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("metric").
        WillReturnRows(sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}))

    cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: nil, index: newCategoryIndex()}
    if _, err := cr.Resolve(context.Background(), "throughput", "metric", nil); err == nil {
        t.Fatal("expected error when no creator is configured and category is missing")
    }
}

func TestResolverCachesIndexAcrossCalls(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    // Only ONE DB load expected even though Resolve is called twice for the same type.
    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("metric").
        WillReturnRows(newLoadRows(7, "latency", "approved", `["latency"]`))
    mock.ExpectExec("UPDATE kb\\.artifact_categories").
        WithArgs(int64(7), "latency").
        WillReturnResult(sqlmock.NewResult(0, 1))
    mock.ExpectExec("UPDATE kb\\.artifact_categories").
        WithArgs(int64(7), "latency").
        WillReturnResult(sqlmock.NewResult(0, 1))

    cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: &fakeCategoryCreator{}, index: newCategoryIndex()}
    if _, err := cr.Resolve(context.Background(), "latency", "metric", nil); err != nil {
        t.Fatalf("Resolve#1 error: %v", err)
    }
    if _, err := cr.Resolve(context.Background(), "latency", "metric", nil); err != nil {
        t.Fatalf("Resolve#2 error: %v", err)
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("unmet sql expectations: %v", err)
    }
}

// TestResolverAbsorbsNonEnglishKeyAsAlias verifies that after creating a category via a
// Chinese rawKey, the Chinese key is indexed so the next call is a direct lookup (no
// second LLM call).
func TestResolverAbsorbsNonEnglishKeyAsAlias(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    // First call: load (empty), create, absorb alias
    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("inventory_item").
        WillReturnRows(sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"}))
    mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
        WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(10)))
    mock.ExpectExec("UPDATE kb\\.artifact_categories"). // absorbAlias for 调制解调器
        WillReturnResult(sqlmock.NewResult(0, 1))

    // Second call: index already loaded and Chinese key already in index → just absorbAlias
    mock.ExpectExec("UPDATE kb\\.artifact_categories").
        WillReturnResult(sqlmock.NewResult(0, 1))

    creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "modem"}}
    idx := newCategoryIndex()
    cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: idx}

    id1, err := cr.Resolve(context.Background(), "调制解调器", "inventory_item", nil)
    if err != nil || id1 != 10 {
        t.Fatalf("first Resolve: id=%d err=%v", id1, err)
    }
    if creator.called != 1 {
        t.Fatalf("creator called %d times after first resolve, want 1", creator.called)
    }

    id2, err := cr.Resolve(context.Background(), "调制解调器", "inventory_item", nil)
    if err != nil || id2 != 10 {
        t.Fatalf("second Resolve: id=%d err=%v", id2, err)
    }
    if creator.called != 1 {
        t.Fatalf("creator called %d times after second resolve, want still 1 (index hit)", creator.called)
    }

    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("unmet sql expectations: %v", err)
    }
}
```

- [ ] **Step 2: Rewrite `artifact_category_batch_test.go`**

Remove `TestClusterCategoryMisses` and `TestResolveBatchClustersSynonymsIntoOneCreate` (both depend on deleted clustering logic). Rewrite the remaining two tests to use the new index-based API, and add a new test for the concurrent-dedup behavior:

```go
package docprocessing

import (
    "context"
    "testing"

    "github.com/DATA-DOG/go-sqlmock"
)

func emptyCategoryLoadRows() *sqlmock.Rows {
    return sqlmock.NewRows([]string{"category_id", "category_key", "status", "canonical_of", "match_keys", "seen_count"})
}

func TestResolveBatchDedupCreatesOnce(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("metric").
        WillReturnRows(emptyCategoryLoadRows())
    mock.ExpectQuery("INSERT INTO kb\\.artifact_categories").
        WillReturnRows(sqlmock.NewRows([]string{"category_id"}).AddRow(int64(42)))
    mock.ExpectExec("UPDATE kb\\.artifact_categories"). // absorbAlias
        WillReturnResult(sqlmock.NewResult(0, 1))

    creator := &fakeCategoryCreator{out: createdCategory{CategoryKey: "throughput"}}
    cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: newCategoryIndex()}

    reqs := []categoryRequest{
        {RawKey: "Throughput"},
        {RawKey: "throughput"},
        {RawKey: "  THROUGHPUT "},
    }
    ids, errs := cr.ResolveBatch(context.Background(), "metric", reqs, 4)
    if len(errs) != 0 {
        t.Fatalf("unexpected errs: %v", errs)
    }
    if creator.called != 1 {
        t.Fatalf("creator called %d times, want 1", creator.called)
    }
    if ids["throughput"] != 42 {
        t.Fatalf("ids[throughput] = %d, want 42", ids["throughput"])
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("unmet sql expectations: %v", err)
    }
}

func TestResolveBatchMatchesExistingWithoutCreate(t *testing.T) {
    db, mock, err := sqlmock.New()
    if err != nil {
        t.Fatalf("sqlmock.New failed: %v", err)
    }
    defer db.Close()

    mock.ExpectQuery("SELECT .* FROM kb\\.artifact_categories").
        WithArgs("metric").
        WillReturnRows(newLoadRows(7, "latency", "approved", `["latency"]`))
    mock.ExpectExec("UPDATE kb\\.artifact_categories").
        WithArgs(int64(7), "latency").
        WillReturnResult(sqlmock.NewResult(0, 1))

    creator := &fakeCategoryCreator{}
    cr := &categoryResolver{reg: artifactCategoryRegistry{DB: db}, creator: creator, index: newCategoryIndex()}

    ids, errs := cr.ResolveBatch(context.Background(), "metric", []categoryRequest{{RawKey: "Latency"}}, 4)
    if len(errs) != 0 {
        t.Fatalf("unexpected errs: %v", errs)
    }
    if creator.called != 0 {
        t.Fatalf("creator called %d times, want 0", creator.called)
    }
    if ids["latency"] != 7 {
        t.Fatalf("ids[latency] = %d, want 7", ids["latency"])
    }
    if err := mock.ExpectationsWereMet(); err != nil {
        t.Fatalf("unmet sql expectations: %v", err)
    }
}
```

- [ ] **Step 3: Update `artifact_category_registry_test.go`** — remove cosine tests

Delete `TestMatchCategoryInSnapshotCosineAboveThreshold` and `TestMatchCategoryInSnapshotCosineBelowThreshold`. Update `TestMatchCategoryInSnapshotMatchesAlias` and `TestMatchCategoryInSnapshotNoMatch` to use the new 2-parameter signature of `matchCategoryInSnapshot`.

- [ ] **Step 4: Run the full test suite for the package**

```bash
cd ChenWeb && go test ./server/api/doc-processing/... -count=1 -timeout 120s 2>&1 | tail -30
```

Expected: all new tests PASS; pre-existing unrelated failures (`*SummaryLog`, `*InsertsMSUsed`) unchanged.

- [ ] **Step 5: Commit**

```bash
git add server/api/doc-processing/artifact_category_resolver.go \
        server/api/doc-processing/artifact_category_registry.go \
        server/api/doc-processing/artifact_category_wiring.go \
        server/api/doc-processing/artifact_category_resolver_test.go \
        server/api/doc-processing/artifact_category_batch_test.go \
        server/api/doc-processing/artifact_category_registry_test.go
git commit -m "feat: category resolution via process-wide hashmap; drop cosine/embedding path"
```

---

## Final verification

- [ ] **Full package build**

```bash
cd ChenWeb && go build ./... 2>&1 | head -20
```

Expected: no errors.

- [ ] **Full package tests**

```bash
cd ChenWeb && go test ./server/api/doc-processing/... -count=1 -timeout 120s 2>&1 | grep -E "FAIL|PASS|ok" | tail -10
```

Expected: package passes (pre-existing unrelated failures are noted in implementation notes).

- [ ] **Update spec doc** — mark the hybrid-search section in `category-mgmt-spec.md` as superseded by the redesign doc.

```bash
# Add a note at top of section 3 in category-mgmt-spec.md:
# "NOTE: Steps 1-4 below are superseded by category-resolution-redesign-2026-06-07.md"
```
