package docprocessing

import (
	"context"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// defaultInventoryCategoryFuzzyThreshold is the cosine-similarity cutoff above
// which a newly observed category surface form is folded into an existing
// category instead of minting a new pending_review entry.
const defaultInventoryCategoryFuzzyThreshold = 0.86

// InventoryCategoryCurator runs the post-extraction pass that admits novel
// item_category values into the registry. It is deliberately decoupled from the
// extraction hot path (per the chosen design): extraction only records raw
// categories plus the unknown_category flag; this curator canonicalizes and
// admits them, optionally using embeddings to merge synonyms.
type InventoryCategoryCurator struct {
	Registry       InventoryCategoryRegistry
	Embedder       Embedder // optional; nil → string-only matching
	EmbedModelName string
	FuzzyThreshold float64
	Logger         ApiTypes.JimoLogger
}

func (c InventoryCategoryCurator) threshold() float64 {
	if c.FuzzyThreshold > 0 {
		return c.FuzzyThreshold
	}
	return defaultInventoryCategoryFuzzyThreshold
}

// CurateObservedCategories admits the given surface forms (typically the
// item_category values produced for a single document) into the registry. It
// loads the active-category snapshot once, then matches-before-minting each
// surface form. Best-effort: a failure on one category is logged and does not
// abort the rest.
func (c InventoryCategoryCurator) CurateObservedCategories(ctx context.Context, surfaceForms []string) error {
	if c.Registry.DB == nil {
		return nil
	}
	deduped := dedupeNonEmptyNormalized(surfaceForms)
	if len(deduped) == 0 {
		return nil
	}
	existing, err := c.Registry.LoadActiveCategories(ctx)
	if err != nil {
		return err
	}
	for _, surface := range deduped {
		key := normalizeInventoryToken(surface)
		// Already-known categories only need a sighting bump; skip embedding cost.
		if _, ok := existing[key]; ok {
			if err := c.Registry.touchCategory(ctx, key, surface); err != nil && c.Logger != nil {
				c.Logger.Warn("inventory category touch failed", "category", key, "error", err)
			}
			continue
		}
		embedding := c.embed(ctx, surface)
		resolved, obsErr := c.Registry.ObserveCategory(ctx, surface, embedding, existing, c.threshold())
		if obsErr != nil {
			if c.Logger != nil {
				c.Logger.Warn("inventory category observe failed", "category", surface, "error", obsErr)
			}
			continue
		}
		// Reflect the new/merged category into the local snapshot so later
		// surface forms in the same batch can match it (intra-batch convergence).
		if _, ok := existing[resolved]; !ok {
			existing[resolved] = InventoryCategoryRecord{
				CategoryKey:  resolved,
				Status:       InventoryCategoryStatusPending,
				DisplayNames: []string{strings.TrimSpace(surface)},
				Embedding:    embedding,
			}
		}
	}
	return nil
}

// embed returns the embedding for a category surface form, or nil if no embedder
// is configured or the call fails (the matcher degrades to string tiers).
func (c InventoryCategoryCurator) embed(ctx context.Context, surface string) []float64 {
	surface = strings.TrimSpace(surface)
	if c.Embedder == nil || surface == "" {
		return nil
	}
	vec, err := c.Embedder.Embed(ctx, llmclients.EmbedInput{
		ModelName: c.EmbedModelName,
		InputText: surface,
	})
	if err != nil {
		if c.Logger != nil {
			c.Logger.Warn("inventory category embed failed; using string match", "category", surface, "error", err)
		}
		return nil
	}
	return vec
}

// collectInventoryItemCategories extracts the distinct, normalized item_category
// surface forms from a batch of extracted item rows for the curation post-pass.
func collectInventoryItemCategories(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		for _, cat := range toStringSlice(item["item_categories"]) {
			if cat = strings.TrimSpace(cat); cat != "" {
				out = append(out, cat)
			}
		}
	}
	return out
}

// refreshCategoryVocabulary seeds the curated dictionary into the registry once
// (idempotent), then merges every active category (approved ∪ pending) from the
// registry into the in-memory dictionary. After this, a category that has been
// admitted — even if still pending review — is treated as known vocabulary and
// no longer trips the unknown_category flag; its review state is surfaced
// separately at read time.
/* Chen Ding, 2026/06/06. Seed approved categories is turned off for now
func (p *InventoryItemsProcessor) refreshCategoryVocabulary(ctx context.Context) {
	if p.CategoryRegistry.DB == nil {
		return
	}
	if !p.categorySeedDone {
		if err := p.CategoryRegistry.SeedApprovedCategories(ctx, p.Dictionary); err != nil {
			if p.Logger != nil {
				p.Logger.Warn("seed inventory categories failed", "error", err)
			}
		} else {
			p.categorySeedDone = true
		}
	}
	active, err := p.CategoryRegistry.LoadActiveCategories(ctx)
	if err != nil {
		if p.Logger != nil {
			p.Logger.Warn("load active inventory categories failed", "error", err)
		}
		return
	}
	if p.Dictionary.Categories == nil {
		p.Dictionary.Categories = map[string]inventoryCategorySchema{}
	}
	for key, rec := range active {
		if _, ok := p.Dictionary.Categories[key]; ok {
			continue
		}
		p.Dictionary.Categories[key] = inventoryCategorySchemaFromRecord(rec)
	}
}
*/

func dedupeNonEmptyNormalized(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		key := normalizeInventoryToken(v)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, strings.TrimSpace(v))
	}
	return out
}
