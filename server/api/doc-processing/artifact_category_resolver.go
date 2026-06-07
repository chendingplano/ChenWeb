package docprocessing

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/singleflight"
)

// categoryCreateGroup coalesces concurrent creates of the same (category_type, k)
// across the whole process — across the metrics and inventory processors and across
// concurrently-running pipelines — so a novel category triggers exactly one LLM call
// even when many goroutines miss it at once. It is the in-process complement to the
// DB upsert (ON CONFLICT), which remains the cross-process/replica safety net.
var categoryCreateGroup singleflight.Group

// categoryCreator mints a brand-new category via the CREATE_ARTIFACT_CATEGORY LLM.
type categoryCreator interface {
	CreateCategory(ctx context.Context, rawKey, categoryType string, evidence map[string]any) (createdCategory, error)
}

// categoryResolver implements the Identify Artifact Categories procedure: it resolves a
// raw category key to a kb.artifact_categories.category_id. It looks up the key in the
// process-wide category index (exact/alias match) before creating a new category via the
// LLM. The index field is globalCategoryIndex in production; tests inject a fresh
// categoryIndex per test for isolation.
type categoryResolver struct {
	reg     artifactCategoryRegistry
	creator categoryCreator
	index   *categoryIndex
}

func newCategoryResolver(reg artifactCategoryRegistry, creator categoryCreator) *categoryResolver {
	return &categoryResolver{reg: reg, creator: creator, index: globalCategoryIndex}
}

// Resolve returns the category_id for rawKey under categoryType, creating the
// category via the LLM when no existing one matches. evidence is optional context
// (the triggering artifact) passed to the create LLM for disambiguation.
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

// categoryRequest is one raw key to resolve within a batch, with optional evidence
// (the triggering artifact) passed to the create LLM for disambiguation on a miss.
type categoryRequest struct {
	RawKey   string
	Evidence map[string]any
}

// ResolveBatch resolves many raw keys of one categoryType in a single pass. It
// normalizes and dedups, looks each up in the process-wide index, and concurrently
// creates any still-missing categories — bounded by maxConcurrency and coalesced
// process-wide by categoryCreateGroup. Returns a map from normalized key to
// category_id; keys that failed are absent from ids and present in errs.
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

// createAndMint creates a category via the LLM and persists it, coalescing concurrent
// creates of the same (categoryType, normKey) via categoryCreateGroup so only one LLM
// call is made per novel key. The shared work runs on a detached context so one caller
// cancelling does not abort a create another pipeline is awaiting. After a successful
// create, the canonical key and the original normKey (the "translation cache") are both
// written to the process-wide index.
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
		// Populate the index with the canonical key and all LLM-returned surface forms.
		cr.index.put(categoryType, normalizeCategoryKey(created.CategoryKey), id, 1)
		cr.index.putAll(categoryType, normalizeCategoryKeys(created.DisplayNames), id)
		cr.index.putAll(categoryType, normalizeCategoryKeys(created.Aliases), id)
		cr.index.putAll(categoryType, normalizeCategoryKeys(created.Acronyms), id)
		// Also index the original normKey — this is the translation cache: next time
		// the same non-English (or alternate-form) key appears it hits the index directly.
		cr.index.put(categoryType, normKey, id, 1)
		if aerr := cr.reg.absorbAlias(bg, id, normKey); aerr != nil {
			// best-effort: keeps the DB match_keys in sync for cross-process loads
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

// ensureIndexLoaded populates the process-wide index for categoryType from the DB if it
// has not been loaded yet. Alias conflicts detected during load are written to
// kb.category_alias_conflicts (best-effort; logged but not fatal).
func (cr *categoryResolver) ensureIndexLoaded(ctx context.Context, categoryType string) error {
	if cr.index.isLoaded(categoryType) {
		return nil
	}
	conflicts, err := cr.reg.loadIntoIndex(ctx, categoryType, cr.index)
	if err != nil {
		return err
	}
	for _, c := range conflicts {
		_ = cr.reg.logAliasConflict(ctx, categoryType, c.Alias, c.IDs)
	}
	return nil
}
