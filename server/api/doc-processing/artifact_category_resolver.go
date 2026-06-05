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

// categoryEmbedder embeds category text for the semantic match channel. ok is
// false when embedding is unavailable (semantic search disabled, no model, etc.).
type categoryEmbedder interface {
	EmbedCategory(ctx context.Context, text string) ([]float64, bool)
}

// categoryResolver implements the Identify Artifact Categories procedure: it
// resolves a raw category key to a kb.artifact_categories.category_id, matching an
// existing category (exact/alias, then semantic) before creating a new one. The
// active-category snapshot is loaded once per type and grows as new categories are
// minted within the pass.
type categoryResolver struct {
	reg             artifactCategoryRegistry
	creator         categoryCreator
	embedder        categoryEmbedder
	cosineThreshold float64

	// mu guards loaded/snapshot/byKey, which are mutated by addToSnapshot from
	// concurrent create goroutines during ResolveBatch.
	mu       sync.Mutex
	loaded   map[string]bool
	snapshot map[string][]artifactCategoryRecord
	byKey    map[string]map[string]artifactCategoryRecord
}

func newCategoryResolver(reg artifactCategoryRegistry, creator categoryCreator, embedder categoryEmbedder, cosineThreshold float64) *categoryResolver {
	return &categoryResolver{
		reg:             reg,
		creator:         creator,
		embedder:        embedder,
		cosineThreshold: cosineThreshold,
		loaded:          map[string]bool{},
		snapshot:        map[string][]artifactCategoryRecord{},
		byKey:           map[string]map[string]artifactCategoryRecord{},
	}
}

// Resolve returns the category_id for rawKey under categoryType, creating the
// category via the LLM when no existing one matches. evidence is optional context
// (the triggering artifact) passed to the create LLM for disambiguation.
func (cr *categoryResolver) Resolve(ctx context.Context, rawKey, categoryType string, evidence map[string]any) (int64, error) {
	normKey := normalizeCategoryKey(rawKey)
	if normKey == "" {
		return 0, fmt.Errorf("(MID_26060420) empty category key")
	}
	if err := cr.ensureLoaded(ctx, categoryType); err != nil {
		return 0, err
	}

	var queryVec []float64
	if cr.embedder != nil {
		if v, ok := cr.embedder.EmbedCategory(ctx, normKey); ok {
			queryVec = v
		}
	}

	if matched, ok := matchCategoryInSnapshot(normKey, queryVec, cr.snapshot[categoryType], cr.cosineThreshold); ok {
		canonical := resolveCanonicalCategory(matched, cr.byKey[categoryType])
		// Absorb the surface form so the next lookup hits the deterministic layer.
		if err := cr.reg.absorbAlias(ctx, canonical.CategoryID, normKey); err != nil {
			return 0, err
		}
		return canonical.CategoryID, nil
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

// ResolveBatch resolves many raw keys of one categoryType in a single pass and is
// the concurrent replacement for looping Resolve. It (1) normalizes and dedups the
// keys, (2) matches each against the existing snapshot (exact/alias, then semantic),
// (3) clusters the still-novel keys so intra-batch synonyms collapse to a single
// create, then (4) creates one category per cluster concurrently — bounded by
// maxConcurrency and coalesced process-wide by categoryCreateGroup. It returns a map
// from normalized key to category_id; keys that could not be resolved are absent from
// ids and present in errs (callers log and continue). Cluster members that are aliases
// of a created representative are absorbed and mapped to the same id.
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

	if err := cr.ensureLoaded(ctx, categoryType); err != nil {
		for _, d := range distinct {
			errs[d.norm] = err
		}
		return ids, errs
	}

	// Phase 1/2 — embed every distinct key (bounded concurrent), then match each
	// against the existing snapshot (exact/alias + semantic). Misses fall through.
	keys := make([]string, len(distinct))
	for i, d := range distinct {
		keys[i] = d.norm
	}
	vecs := cr.embedKeys(ctx, keys, maxConcurrency)

	type missEntry struct {
		norm     string
		evidence map[string]any
		vec      []float64
	}
	var misses []missEntry
	for _, d := range distinct {
		vec := vecs[d.norm]
		if matched, ok := matchCategoryInSnapshot(d.norm, vec, cr.snapshot[categoryType], cr.cosineThreshold); ok {
			canonical := resolveCanonicalCategory(matched, cr.byKey[categoryType])
			if err := cr.reg.absorbAlias(ctx, canonical.CategoryID, d.norm); err != nil {
				errs[d.norm] = err
				continue
			}
			ids[d.norm] = canonical.CategoryID
			continue
		}
		misses = append(misses, missEntry{norm: d.norm, evidence: d.evidence, vec: vec})
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

	// Phase 3a — cluster misses so two novel keys that are aliases of each other
	// collapse to one create; the rest of the cluster is absorbed as aliases.
	missVecs := make([][]float64, len(misses))
	for i := range misses {
		missVecs[i] = misses[i].vec
	}
	clusters := clusterCategoryMisses(missVecs, cr.cosineThreshold)

	// Phase 3b — create one category per cluster representative, concurrently.
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, maxConcurrency)
	)
	for _, members := range clusters {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			rep := misses[members[0]]
			id, err := cr.createAndMint(ctx, categoryType, rep.norm, rep.evidence)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				for _, mi := range members {
					errs[misses[mi].norm] = err
				}
				return
			}
			ids[rep.norm] = id
			for _, mi := range members[1:] {
				alias := misses[mi].norm
				if aerr := cr.reg.absorbAlias(ctx, id, alias); aerr != nil {
					errs[alias] = aerr
					continue
				}
				ids[alias] = id
			}
		}()
	}
	wg.Wait()
	return ids, errs
}

// createAndMint creates a category via the LLM and persists it, coalescing concurrent
// creates of the same (categoryType, normKey) across the process via categoryCreateGroup
// so only one LLM call is made per novel key. The shared work runs on a detached context
// (context.WithoutCancel): one caller giving up must not cancel a create another pipeline
// is awaiting. mintCategory's ON CONFLICT remains the cross-process safety net.
func (cr *categoryResolver) createAndMint(ctx context.Context, categoryType, normKey string, evidence map[string]any) (int64, error) {
	ch := categoryCreateGroup.DoChan(categoryType+"\x00"+normKey, func() (any, error) {
		bg := context.WithoutCancel(ctx)
		created, err := cr.creator.CreateCategory(bg, normKey, categoryType, evidence)
		if err != nil {
			return nil, fmt.Errorf("(MID_26060421) create category %q: %w", normKey, err)
		}
		var newVec []float64
		if cr.embedder != nil {
			text := created.SearchDocument
			if text == "" {
				text = normKey
			}
			if v, ok := cr.embedder.EmbedCategory(bg, text); ok {
				newVec = v
			}
		}
		id, err := cr.reg.mintCategory(bg, created, categoryType, newVec)
		if err != nil {
			return nil, err
		}
		cr.addToSnapshot(categoryType, created, id, newVec)
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

// embedKeys embeds each key for the semantic match channel, bounded by maxConcurrency.
// Keys the embedder cannot embed are simply absent from the result (lexical-only).
func (cr *categoryResolver) embedKeys(ctx context.Context, keys []string, maxConcurrency int) map[string][]float64 {
	out := make(map[string][]float64, len(keys))
	if cr.embedder == nil {
		return out
	}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, maxConcurrency)
	)
	for _, k := range keys {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if v, ok := cr.embedder.EmbedCategory(ctx, k); ok {
				mu.Lock()
				out[k] = v
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return out
}

// clusterCategoryMisses groups novel keys whose embeddings are within cosineThreshold
// so intra-batch synonyms collapse to one create. It is a greedy single-pass grouping
// against each cluster's representative (first member). Keys without a usable embedding
// each form their own cluster (they fall through to the cross-batch merge path). Returns
// clusters as index slices into the input; the first index of each is the representative.
func clusterCategoryMisses(vecs [][]float64, cosineThreshold float64) [][]int {
	var clusters [][]int
	var repVecs [][]float64
	for i, v := range vecs {
		placed := false
		if len(v) >= categoryEmbeddingMinDims {
			for ci, rv := range repVecs {
				if len(rv) == len(v) && categoryCosine(v, rv) >= cosineThreshold {
					clusters[ci] = append(clusters[ci], i)
					placed = true
					break
				}
			}
		}
		if !placed {
			clusters = append(clusters, []int{i})
			repVecs = append(repVecs, v)
		}
	}
	return clusters
}

func (cr *categoryResolver) ensureLoaded(ctx context.Context, categoryType string) error {
	if cr.loaded[categoryType] {
		return nil
	}
	recs, err := cr.reg.loadActiveCategories(ctx, categoryType)
	if err != nil {
		return err
	}
	cr.snapshot[categoryType] = recs
	byKey := make(map[string]artifactCategoryRecord, len(recs))
	for _, rec := range recs {
		byKey[rec.CategoryKey] = rec
	}
	cr.byKey[categoryType] = byKey
	cr.loaded[categoryType] = true
	return nil
}

// addToSnapshot makes a freshly minted category resolvable by later keys in the
// same pass without reloading from the DB.
func (cr *categoryResolver) addToSnapshot(categoryType string, c createdCategory, id int64, vec []float64) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	key := normalizeCategoryKey(c.CategoryKey)
	rec := artifactCategoryRecord{
		CategoryID:   id,
		CategoryKey:  key,
		CategoryType: categoryType,
		Status:       "pending_review",
		MatchKeys:    buildArtifactMatchKeys(key, c.DisplayNames, c.Aliases, c.Acronyms),
		Embedding:    vec,
	}
	cr.snapshot[categoryType] = append(cr.snapshot[categoryType], rec)
	if cr.byKey[categoryType] == nil {
		cr.byKey[categoryType] = map[string]artifactCategoryRecord{}
	}
	cr.byKey[categoryType][key] = rec
}
