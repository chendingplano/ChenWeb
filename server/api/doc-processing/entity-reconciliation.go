package docprocessing

import (
	"context"
	"sort"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// This file sketches the corpus-level entity reconciliation job from
// ADR 2026061701 (resolving ADR 2026061302 Open Question #3). It is the
// downstream identity layer that runs AFTER per-document extraction/linking.
//
// Per-document consolidation (consolidateEntities, ADR 2026061302 D6) assigns a
// canonical entity_id WITHIN one document. This job operates across the whole
// corpus: it merges duplicate entities from different documents and promotes
// provisional entities once a real match appears.
//
// Loop:  block -> adjudicate -> apply -> enrich
//
// Cheap, SQL-driven blocking (no LLM) produces candidate pairs; rule thresholds
// auto-decide the tails; only the ambiguous middle band reaches the LLM; humans
// review only the residual `needs_human` queue. Driven incrementally by a
// watermark over kb.entities.modify_time.
//
// The pure scoring helpers below are unit-testable (no DB / no LLM); the DB and
// LLM seams are behind ReconcileStore / MergeAdjudicator interfaces.

// ---- configuration ----

// ReconcileConfig holds the tuning thresholds (wired from env per ADR R6).
type ReconcileConfig struct {
	// BlockTrgmMin is the minimum name similarity for a pair to be blocked.
	BlockTrgmMin float64
	// AutoMergeMin: score >= this is auto-merged by rule (no LLM).
	AutoMergeMin float64
	// AutoRejectMax: score <= this is auto-rejected by rule (no LLM).
	AutoRejectMax float64
	// BatchSize bounds entities scanned per run.
	BatchSize int
	// MaxLLMGroupsPerRun bounds LLM cost per run.
	MaxLLMGroupsPerRun int
}

// Candidate decision values (mirror kb.entity_merge_candidates.decision).
const (
	decisionPending    = "pending"
	decisionAutoMerge  = "auto_merge"
	decisionAutoReject = "auto_reject"
	decisionNeedsHuman = "needs_human"
	decisionApplied    = "applied"
	decisionDismissed  = "dismissed"
)

// Merge methods (mirror kb.entity_merges.method).
const (
	mergeMethodRule  = "rule"
	mergeMethodLLM   = "llm"
	mergeMethodHuman = "human"
)

// ---- seams ----

// ReconcileStore is the DB seam. Implemented by the SQL store (kb schema).
// Blocking lives in SQL (pg_trgm on entity_en, name_key equality, alias/type
// overlap, search_vector rank) because it must scale to the whole corpus.
type ReconcileStore interface {
	// BeginRun inserts a kb.reconcile_runs row (status='running') and returns
	// its run_id plus the watermark_from to scan since (previous run's
	// watermark_to).
	BeginRun(ctx context.Context, phase string) (runID int64, watermarkFrom time.Time, err error)
	// FinishRun sets status/stats/error and, only on ok, advances watermark_to.
	FinishRun(ctx context.Context, runID int64, ok bool, watermarkTo time.Time, stats ReconcileStats, runErr error) error

	// BlockCandidates runs the SQL blocking pass over entities modified since
	// watermarkFrom, upserting into kb.entity_merge_candidates
	// (ON CONFLICT (lo,hi) DO UPDATE signals) and returning the new pending set.
	BlockCandidates(ctx context.Context, runID int64, watermarkFrom time.Time, cfg ReconcileConfig) ([]CandidatePair, error)

	// LoadEntity hydrates the identity-bearing fields used by pure scoring.
	LoadEntity(ctx context.Context, entityID string) (map[string]any, error)

	// RecordDecision persists a candidate's decision/method/by.
	RecordDecision(ctx context.Context, candidateID int64, decision, method, by string) error

	// ApplyMerge folds from->into atomically: writes kb.entity_merges, sets the
	// loser's canonical_entity_id + reconcile_status='merged', unions
	// aliases/keywords/line_spans onto the head, re-points kb.relations
	// subject/object entity ids, promotes a provisional loser, and syncs the
	// survivor into kb.entity_names. Returns relations re-pointed.
	ApplyMerge(ctx context.Context, m MergeDecision) (relationsRepointed int64, err error)

	// MarkClustered sets reconcile_status='clustered'/'held' + reconciled_at for
	// heads that survived a run without merging.
	MarkClustered(ctx context.Context, entityIDs []string, status string) error
}

// MergeAdjudicator is the LLM seam: given a cluster of blocked entities it
// returns the merge groups it is confident about and flags the rest for humans.
type MergeAdjudicator interface {
	Adjudicate(ctx context.Context, cluster []map[string]any) (AdjudicationResult, error)
}

// ---- data carried between phases ----

type CandidatePair struct {
	CandidateID int64
	LoEntityID  string
	HiEntityID  string
	Signals     map[string]any
	Score       float64
}

type MergeDecision struct {
	CandidateID  int64
	FromEntityID string // absorbed
	IntoEntityID string // survivor / canonical head
	Method       string // rule | llm | human
	Confidence   float64
	Reason       string
	Evidence     map[string]any
	RunID        int64
	DecidedBy    string
}

type AdjudicationResult struct {
	Merges     []MergeDecision
	NeedsHuman []int64 // candidate_ids the LLM declined to auto-decide
}

type ReconcileStats struct {
	Scanned     int
	Candidates  int
	AutoMerged  int
	QueuedHuman int
	Promoted    int
	Enriched    int
}

// ---- orchestration ----

type Reconciler struct {
	store ReconcileStore
	llm   MergeAdjudicator
	cfg   ReconcileConfig
	log   ApiTypes.JimoLogger // existing package logger seam
}

// Run executes one block -> adjudicate -> apply cycle over entities changed
// since the last successful watermark. Enrich is a follow-on pass (TODO).
func (r *Reconciler) Run(ctx context.Context) (stats ReconcileStats, err error) {
	runID, watermarkFrom, err := r.store.BeginRun(ctx, "full")
	if err != nil {
		return stats, err
	}
	// Watermark advances to "now" only if the run completes ok; FinishRun
	// enforces that so a failure re-processes the same slice next time (R6).
	watermarkTo := time.Now()
	defer func() {
		_ = r.store.FinishRun(ctx, runID, err == nil, watermarkTo, stats, err)
	}()

	// 1. BLOCK — cheap SQL, no LLM. Produces candidate pairs.
	pairs, err := r.store.BlockCandidates(ctx, runID, watermarkFrom, r.cfg)
	if err != nil {
		return stats, err
	}
	stats.Candidates = len(pairs)

	// 2. ADJUDICATE — rule tails decide cheaply; ambiguous middle -> LLM.
	var llmCluster []CandidatePair
	var ruleMerges []MergeDecision
	for _, p := range pairs {
		switch {
		case p.Score >= r.cfg.AutoMergeMin:
			from, into := electSurvivor(ctx, r.store, p)
			ruleMerges = append(ruleMerges, MergeDecision{
				CandidateID: p.CandidateID, FromEntityID: from, IntoEntityID: into,
				Method: mergeMethodRule, Confidence: p.Score, RunID: runID,
				Reason: "score>=AutoMergeMin", Evidence: p.Signals, DecidedBy: "reconciler",
			})
		case p.Score <= r.cfg.AutoRejectMax:
			_ = r.store.RecordDecision(ctx, p.CandidateID, decisionAutoReject, mergeMethodRule, "reconciler")
		default:
			llmCluster = append(llmCluster, p)
		}
	}

	// 3a. APPLY rule merges.
	for _, m := range ruleMerges {
		if err = r.applyOne(ctx, m, &stats); err != nil {
			return stats, err
		}
	}

	// 2b/3b. LLM adjudication on the bounded middle band, then apply.
	if len(llmCluster) > 0 && r.llm != nil {
		if len(llmCluster) > r.cfg.MaxLLMGroupsPerRun {
			llmCluster = llmCluster[:r.cfg.MaxLLMGroupsPerRun]
		}
		res, lerr := r.llm.Adjudicate(ctx, r.hydrate(ctx, llmCluster))
		if lerr != nil {
			return stats, lerr
		}
		for _, m := range res.Merges {
			m.RunID = runID
			if err = r.applyOne(ctx, m, &stats); err != nil {
				return stats, err
			}
		}
		for _, cid := range res.NeedsHuman {
			_ = r.store.RecordDecision(ctx, cid, decisionNeedsHuman, mergeMethodLLM, "reconciler")
			stats.QueuedHuman++
		}
	}

	// TODO(R-enrich): backfill missing fields/_en on surviving heads.
	return stats, nil
}

func (r *Reconciler) applyOne(ctx context.Context, m MergeDecision, stats *ReconcileStats) error {
	repointed, err := r.store.ApplyMerge(ctx, m)
	if err != nil {
		return err
	}
	_ = r.store.RecordDecision(ctx, m.CandidateID, decisionApplied, m.Method, m.DecidedBy)
	stats.AutoMerged++
	_ = repointed // counted in kb.entity_merges.relations_repointed
	return nil
}

func (r *Reconciler) hydrate(ctx context.Context, pairs []CandidatePair) []map[string]any {
	seen := map[string]bool{}
	var out []map[string]any
	for _, p := range pairs {
		for _, id := range []string{p.LoEntityID, p.HiEntityID} {
			if seen[id] {
				continue
			}
			seen[id] = true
			if e, err := r.store.LoadEntity(ctx, id); err == nil && e != nil {
				out = append(out, e)
			}
		}
	}
	return out
}

// electSurvivor picks the canonical head: prefer an 'extracted' entity over a
// 'provisional' one, then the one with more surface forms (richer), else the
// lexically smaller entity_id for determinism. Returns (from=absorbed, into=head).
func electSurvivor(ctx context.Context, store ReconcileStore, p CandidatePair) (from, into string) {
	lo, _ := store.LoadEntity(ctx, p.LoEntityID)
	hi, _ := store.LoadEntity(ctx, p.HiEntityID)
	if survivorScore(hi) > survivorScore(lo) {
		return p.LoEntityID, p.HiEntityID
	}
	return p.HiEntityID, p.LoEntityID
}

func survivorScore(e map[string]any) int {
	if e == nil {
		return -1
	}
	score := len(entitySurfaceForms(e))
	if asString(e["entity_status"]) == entityStatusExtracted {
		score += 1000 // extracted always beats provisional
	}
	return score
}

// ---- pure scoring (unit-testable, no DB / no LLM) ----

// computeSignals derives the blocking signals for a pair from their loaded
// fields. SQL blocking computes the same shape via pg_trgm; this mirror lets
// the rule layer re-score and the tests assert on it.
func computeSignals(a, b map[string]any) map[string]any {
	fa, fb := entitySurfaceForms(a), entitySurfaceForms(b)
	return map[string]any{
		"name_exact":      hasCommon(fa, fb),
		"surface_jaccard": jaccard(fa, fb),
		"type_match":      normalizeSurfaceForm(asString(a["entity_type_en"])) == normalizeSurfaceForm(asString(b["entity_type_en"])),
		"category_overlap": jaccard(
			toStringSlice(a["entity_categories"]), toStringSlice(b["entity_categories"]),
		),
	}
}

// scoreCandidate collapses signals into [0,1]. Conservative: name identity
// dominates; type/category agreement nudges; a type CONFLICT caps the score so
// two same-named things of different kinds are not auto-merged.
func scoreCandidate(sig map[string]any) float64 {
	score := 0.0
	if b, _ := sig["name_exact"].(bool); b {
		score += 0.7
	}
	if j, ok := sig["surface_jaccard"].(float64); ok {
		score += 0.2 * j
	}
	if b, _ := sig["type_match"].(bool); b {
		score += 0.1
	} else {
		if score > 0.6 {
			score = 0.6 // type conflict -> never auto-merge on name alone
		}
	}
	if score > 1 {
		score = 1
	}
	return score
}

func hasCommon(a, b []string) bool {
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, y := range b {
		if set[y] {
			return true
		}
	}
	return false
}

func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	inter := 0
	union := map[string]bool{}
	for _, x := range a {
		union[x] = true
	}
	for _, y := range b {
		if set[y] {
			inter++
		}
		union[y] = true
	}
	return float64(inter) / float64(len(union))
}

// sortPair returns the (lo, hi) ordering used by the UNIQUE(lo,hi) constraint.
func sortPair(a, b string) (lo, hi string) {
	ids := []string{a, b}
	sort.Strings(ids)
	return ids[0], ids[1]
}
