package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// EmbeddingClient is reconciliation's tier-6 seam (spec §13.1, §22 Q2): text
// in, one vector per text out, in the same order. Kept out of the online
// resolve path by design -- only keywords.Reconciler calls this. A concrete
// implementation wraps shared/go/api/llm's OpenAIJSONClient.EmbedBatch; see
// cmd/keyword-reconcile.
type EmbeddingClient interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
}

const (
	// tier6EmbeddingMinScore is the cosine-similarity floor for an automatic
	// tier-6 merge. Conservative default per spec §22 Q1 ("not derivable
	// from first principles -- measure against the gold set, ship
	// conservative, tune"); D10 biases toward under-merging.
	tier6EmbeddingMinScore = 0.90
	// reconcileLexicalBlockMin is the pg_trgm similarity() floor used only
	// to shortlist candidates before the real edit-distance gate
	// (fuzzyCandidateScore) decides -- deliberately permissive.
	reconcileLexicalBlockMin = 0.30
	// reconcileTopK bounds both the lexical and semantic candidate lists.
	reconcileTopK = 10
)

// ReconcileStats summarizes one Reconciler.Run call.
type ReconcileStats struct {
	Scanned            int
	Merged             int
	SkippedVetoed      int
	SkippedNoCandidate int
}

// Reconciler is the offline batch job that unifies D11 auto-created
// provisional concepts with their true match (spec §13, the "minimum
// reconciliation loop" build-order step 11 names) -- the mechanism
// Appendix A Stage 5 describes. It merges through the already-guardrailed
// ConceptStore.MergeConcept; it never writes kb.keyword_concepts/surfaces
// directly.
type Reconciler struct {
	DB           *sql.DB
	ConceptStore ConceptStore
	SurfaceStore SurfaceStore
	DecisionLog  semid.DecisionLogStore
	Embeddings   EmbeddingClient
	Scope        string
}

// Run scans every D11 auto-created provisional concept in scope and, for
// each, looks for a merge target via lexical (pg_trgm) and semantic
// (embedding cosine) blocking. Eligibility is structurally guaranteed by
// scanning only the auto-created-provisional population (spec §14.3: "an
// auto-created provisional concept... into an established one" is
// automatic because the provisional side always has no curated content to
// lose -- the forbidden case, two established concepts, cannot arise here
// since one side is always drawn from this population). A merged concept's
// status flips to 'merged' and drops out of the population on the next
// Run, so no watermark/run-tracking table is needed at this scale.
func (r *Reconciler) Run(ctx context.Context) (ReconcileStats, error) {
	var stats ReconcileStats
	candidates, err := r.ConceptStore.ListAutoCreatedProvisional(ctx, r.Scope, 0)
	if err != nil {
		return stats, err
	}
	live, err := r.ConceptStore.ListConcepts(ctx, r.Scope)
	if err != nil {
		return stats, err
	}
	liveEmbeds, err := r.embedConcepts(ctx, live)
	if err != nil {
		return stats, err
	}

	merged := map[string]bool{}
	for _, cand := range candidates {
		stats.Scanned++
		if merged[cand.ConceptID] {
			continue
		}
		target, method, score, ok, err := r.findMergeTarget(ctx, cand, live, liveEmbeds, merged)
		if err != nil {
			return stats, err
		}
		if !ok {
			stats.SkippedNoCandidate++
			continue
		}

		absorbCount, survCount, err := r.surfaceCounts(ctx, cand.ConceptID, target.ConceptID)
		if err != nil {
			return stats, err
		}
		absorbedID, survivorID := electMergeDirection(cand, target, absorbCount, survCount)

		// ConceptStore.MergeConcept already checks kb.semid_never_merge
		// internally (mergeGuards -> isNeverMerge) -- no separate check here.
		if _, err := r.ConceptStore.MergeConcept(ctx, absorbedID, survivorID); err != nil {
			if errors.Is(err, ErrMergeRejected) {
				stats.SkippedVetoed++
				continue
			}
			return stats, err
		}
		merged[absorbedID] = true

		input, err := json.Marshal(map[string]any{"absorbed": absorbedID, "survivor": survivorID, "method": method, "score": score})
		if err != nil {
			return stats, err
		}
		// Known limitation (documented, not fixed): MergeConcept — its own
		// transaction — and this decision-log append are NOT atomic. If the
		// append fails after the merge committed, the merge persists with no
		// audit row, and a retry will not re-merge the tombstoned concept (it
		// left the auto-created-provisional population when its status flipped
		// to 'merged'). Inherent to the approved propagate-errors design;
		// atomic merge+audit is out of scope for the minimum reconciliation
		// loop.
		if _, err := r.DecisionLog.Append(ctx, semid.DecisionLogEntry{
			Family:  "keyword_reconcile",
			Scope:   r.Scope,
			Input:   input,
			Output:  input,
			Verdict: "auto_merged",
			Actor:   "keyword_reconciler",
		}); err != nil {
			// A tier-6 auto-merge exists precisely to produce an auditable trail
			// (ADR DR15): a failed append must surface as a run error, not as a
			// silent success with no record of the merge.
			return stats, err
		}
		stats.Merged++
	}
	return stats, nil
}

// findMergeTarget runs lexical and semantic blocking for one candidate and
// returns the better-scoring eligible target, if any.
func (r *Reconciler) findMergeTarget(ctx context.Context, cand Concept, live []Concept, liveEmbeds map[string][]float64, merged map[string]bool) (Concept, string, float64, bool, error) {
	var (
		bestTarget Concept
		bestScore  float64
		bestMethod string
		found      bool
	)

	lexRows, err := r.ConceptStore.SearchSimilarPrefLabel(ctx, cand.PrefLabel, r.Scope, cand.ConceptID, reconcileLexicalBlockMin, reconcileTopK)
	if err != nil {
		return Concept{}, "", 0, false, err
	}
	// fuzzyCandidateScore's guardrails (digit veto, negation/affix veto, the
	// length-5-to-8 first-rune rule, and the 0.8-at-length-5 score floor)
	// all assume normalized input, matching tier5FuzzyMatch's own usage --
	// comparing raw, differently-cased pref labels would distort edit
	// distances on casing differences that carry no spelling signal.
	n := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	queryNorm := n.Normalize(cand.PrefLabel).Norm
	queryRuneLen := len([]rune(queryNorm))
	for _, row := range lexRows {
		if merged[row.ConceptID] {
			continue
		}
		candNorm := n.Normalize(row.PrefLabel).Norm
		if score, ok := fuzzyCandidateScore(queryNorm, candNorm, queryRuneLen); ok && score > bestScore {
			bestTarget, bestScore, bestMethod, found = row, score, "tier5_fuzzy", true
		}
	}

	candEmbed, ok := liveEmbeds[cand.ConceptID]
	if ok {
		for _, target := range live {
			if target.ConceptID == cand.ConceptID || merged[target.ConceptID] {
				continue
			}
			// The digit veto here runs on the raw pref labels, not normalized
			// forms. The normalizer's NFKC step (semid §6.3) folds full-width
			// digits (U+FF10–U+FF19) to ASCII, but digitsOnly extracts only
			// ASCII 0-9, so a full-width-only digit difference (e.g. 版２ vs
			// 版３) is invisible to the veto in this semantic branch. The
			// lexical path above normalizes both sides first and so sees
			// full-width digits already folded.
			targetEmbed, ok := liveEmbeds[target.ConceptID]
			if !ok || digitsDiffer(cand.PrefLabel, target.PrefLabel) {
				continue
			}
			score := cosineSimilarity(candEmbed, targetEmbed)
			if score >= tier6EmbeddingMinScore && score > bestScore {
				bestTarget, bestScore, bestMethod, found = target, score, "tier6_embedding", true
			}
		}
	}

	return bestTarget, bestMethod, bestScore, found, nil
}

// embedConcepts batches one embedding call for every live concept's
// pref_label, so a run with N auto-created candidates against M live
// concepts pays for M embeddings once, not once per candidate pair.
func (r *Reconciler) embedConcepts(ctx context.Context, live []Concept) (map[string][]float64, error) {
	if r.Embeddings == nil || len(live) == 0 {
		return map[string][]float64{}, nil
	}
	texts := make([]string, len(live))
	for i, c := range live {
		texts[i] = c.PrefLabel
	}
	vecs, err := r.Embeddings.EmbedBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]float64, len(live))
	for i, c := range live {
		if i < len(vecs) && vecs[i] != nil {
			out[c.ConceptID] = vecs[i]
		}
	}
	return out, nil
}

// surfaceCounts loads surface counts for both concepts, propagating the first
// query error so a transient read failure can't silently flip the
// absorbed/survivor direction electMergeDirection picks.
func (r *Reconciler) surfaceCounts(ctx context.Context, a, b string) (int, int, error) {
	as, err := r.SurfaceStore.ListSurfacesByConcept(ctx, a)
	if err != nil {
		return 0, 0, err
	}
	bs, err := r.SurfaceStore.ListSurfacesByConcept(ctx, b)
	if err != nil {
		return 0, 0, err
	}
	return len(as), len(bs), nil
}

// electMergeDirection picks the survivor: prefer the concept with more
// surface forms (richer -- mirrors doc-processing's entity-reconciliation
// electSurvivor), then the earlier CreateTime, then the lexically smaller
// concept_id for determinism. Both directions are always safe here because
// Run only ever calls this with at least one side drawn from the
// auto-created-provisional population (spec §14.3).
func electMergeDirection(cand, target Concept, candSurfaces, targetSurfaces int) (absorbedID, survivorID string) {
	if candSurfaces != targetSurfaces {
		if candSurfaces > targetSurfaces {
			return target.ConceptID, cand.ConceptID
		}
		return cand.ConceptID, target.ConceptID
	}
	if !cand.CreateTime.Equal(target.CreateTime) {
		if cand.CreateTime.Before(target.CreateTime) {
			return target.ConceptID, cand.ConceptID
		}
		return cand.ConceptID, target.ConceptID
	}
	ids := []string{cand.ConceptID, target.ConceptID}
	sort.Strings(ids)
	if ids[0] == cand.ConceptID {
		return target.ConceptID, cand.ConceptID
	}
	return cand.ConceptID, target.ConceptID
}

// cosineSimilarity is tier 6's score. Returns 0 for a zero-norm vector
// (e.g. a missing embedding) rather than NaN.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
