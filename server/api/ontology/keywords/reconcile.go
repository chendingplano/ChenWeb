package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
	"github.com/lib/pq"
)

// EmbeddingClient is a model-neutral proposal seam. Embeddings rank a bounded
// diagnostic candidate list; their values never authorize identity writes.
type EmbeddingClient interface {
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
}

const (
	reconcileLexicalBlockMin = 0.30
	reconcileLexicalTopK     = 10
	reconcileEmbeddingTopK   = 10
	tier5TieEpsilon          = 1e-9

	methodTier5     = "tier5_fuzzy"
	methodEmbedding = "tier6_embedding"
	methodIdentity  = "tier6_identity_claim"

	outcomeValidate    = "validate"
	outcomeAutoMerge   = "auto_merge"
	outcomeDeferred    = "deferred_unvalidated"
	outcomeReject      = "reject"
	outcomeNoCandidate = "no_candidate"

	// defaultAmbiguousMargin is ReconcileAmbiguous's default auto-apply
	// margin (spec 2026080403 §13): the winning candidate's embedding
	// similarity to the query must exceed the runner-up's by at least this
	// much. Coarser than tier5TieEpsilon on purpose -- this ranks a handful
	// of already-plausible candidates, not detecting an exact float tie.
	defaultAmbiguousMargin = 0.15

	ambiguousOutcomeApplied     = "auto_applied"
	ambiguousOutcomeCollapsed   = "collapsed_by_merge"
	ambiguousOutcomeDeferred    = "deferred"
	ambiguousOutcomeNoCandidate = "no_active_candidate"

	methodAmbiguousMargin    = "ambiguous_embedding_margin"
	methodAmbiguousLexical   = "ambiguous_lexical_corroboration"
	methodAmbiguousCollapsed = "ambiguous_collapsed_by_merge"
)

// ReconcileStats distinguishes attempted candidates from committed decisions.
type ReconcileStats struct {
	Scanned             int
	Decided             int
	Failed              int
	Merged              int
	DeferredUnvalidated int
	Rejected            int
	NoCandidate         int
}

func (s ReconcileStats) Validate() error {
	if s.Failed < 0 || s.Failed > 1 || s.Scanned < 0 || s.Decided < 0 || s.Merged < 0 || s.DeferredUnvalidated < 0 || s.Rejected < 0 || s.NoCandidate < 0 {
		return errors.New("invalid reconciliation counters")
	}
	if s.Merged+s.DeferredUnvalidated+s.Rejected+s.NoCandidate != s.Decided {
		return errors.New("reconciliation outcomes do not sum to decided")
	}
	if s.Failed == 0 && s.Scanned != s.Decided {
		return errors.New("successful reconciliation must decide every scanned candidate")
	}
	if s.Failed == 1 && s.Scanned != s.Decided+1 {
		return errors.New("failed reconciliation must have exactly one uncommitted attempt")
	}
	return nil
}

// Reconciler scans D11 provisional concepts. Every candidate is decided and
// audited in one caller-owned transaction under the keyword identity lock.
type Reconciler struct {
	DB                *sql.DB
	ConceptStore      ConceptStore
	EvidenceProviders []IdentityEvidenceProvider
	Embeddings        EmbeddingClient
	EmbeddingModelID  string
	Scope             string

	// AmbiguousMarginThreshold is ReconcileAmbiguous's auto-apply margin.
	// Defaults to defaultAmbiguousMargin when <= 0.
	AmbiguousMarginThreshold float64
}

// AmbiguousReconcileStats distinguishes attempted backlog rows from
// committed decisions, mirroring ReconcileStats' shape (§13).
type AmbiguousReconcileStats struct {
	Scanned     int
	Decided     int
	Failed      int
	AutoApplied int
	Collapsed   int
	Deferred    int
	NoCandidate int
}

func (s AmbiguousReconcileStats) Validate() error {
	if s.Failed < 0 || s.Failed > 1 || s.Scanned < 0 || s.Decided < 0 ||
		s.AutoApplied < 0 || s.Collapsed < 0 || s.Deferred < 0 || s.NoCandidate < 0 {
		return errors.New("invalid ambiguous reconciliation counters")
	}
	if s.AutoApplied+s.Collapsed+s.Deferred+s.NoCandidate != s.Decided {
		return errors.New("ambiguous reconciliation outcomes do not sum to decided")
	}
	if s.Failed == 0 && s.Scanned != s.Decided {
		return errors.New("successful ambiguous reconciliation must decide every scanned row")
	}
	if s.Failed == 1 && s.Scanned != s.Decided+1 {
		return errors.New("failed ambiguous reconciliation must have exactly one uncommitted attempt")
	}
	return nil
}

type ReconcileProposal struct {
	TargetConceptID string                    `json:"target_concept_id"`
	Methods         []ReconcileProposalMethod `json:"methods"`
	Claims          []IdentityClaim           `json:"claims"`
}

type ReconcileProposalMethod struct {
	Method            string   `json:"method"`
	Rank              int      `json:"rank"`
	Score             *float64 `json:"score"`
	ModelID           string   `json:"model_id"`
	CandidateSetSize  int      `json:"candidate_set_size"`
	CandidateSetLimit int      `json:"candidate_set_limit"`
}

type reconcileAudit struct {
	CandidateConceptID string              `json:"candidate_concept_id"`
	Scope              string              `json:"scope"`
	Outcome            string              `json:"outcome"`
	Reason             string              `json:"reason"`
	Method             string              `json:"method"`
	AbsorbedConceptID  string              `json:"absorbed_concept_id"`
	SurvivorConceptID  string              `json:"survivor_concept_id"`
	EmbeddingModelID   string              `json:"embedding_model_id"`
	Proposals          []ReconcileProposal `json:"proposals"`
	Claims             []IdentityClaim     `json:"claims"`
	Vetoes             []string            `json:"vetoes"`
}

type candidateDecision struct {
	Outcome         string
	TargetConceptID string
	Method          string
	Reason          string
}

type tier5State int

const (
	tier5None tier5State = iota
	tier5Unique
	tier5Tie
)

type tier5Result struct {
	State           tier5State
	TargetConceptID string
	Score           float64
	Scored          []tier5ScoredTarget
}

type tier5ScoredTarget struct {
	ConceptID string
	Score     float64
}

func (r *Reconciler) validateConfiguration() error {
	if err := ValidateIdentityEvidenceProviders(r.EvidenceProviders); err != nil {
		return err
	}
	if r.Embeddings != nil && strings.TrimSpace(r.EmbeddingModelID) == "" {
		return errors.New("EmbeddingModelID is required when embeddings are configured")
	}
	if r.Embeddings == nil && strings.TrimSpace(r.EmbeddingModelID) != "" {
		return errors.New("EmbeddingModelID requires an embedding client")
	}
	if r.ConceptStore.Alignments.Assertions.DB == nil {
		return errors.New("alignment store is required for reconciliation")
	}
	if r.DB == nil {
		return errors.New("reconciler DB is nil")
	}
	if strings.TrimSpace(r.Scope) == "" {
		return errors.New("reconciliation scope is blank")
	}
	return nil
}

func (r *Reconciler) Run(ctx context.Context) (ReconcileStats, error) {
	var stats ReconcileStats
	if err := r.validateConfiguration(); err != nil {
		return stats, err
	}
	scanStore := r.ConceptStore
	scanStore.DB = r.DB
	candidates, err := scanStore.ListAutoCreatedProvisional(ctx, r.Scope, 0)
	if err != nil {
		return stats, err
	}
	live, err := scanStore.ListConcepts(ctx, r.Scope)
	if err != nil {
		return stats, err
	}
	embeddings, err := r.embedConcepts(ctx, live)
	if err != nil {
		return stats, err
	}

	for _, candidate := range candidates {
		embeddingProposals := embeddingProposals(candidate, live, embeddings, r.EmbeddingModelID)
		stats.Scanned++
		outcome, err := r.decideCandidate(ctx, candidate, embeddingProposals)
		if err != nil {
			stats.Failed = 1
			return stats, err
		}
		stats.Decided++
		switch outcome {
		case outcomeAutoMerge:
			stats.Merged++
		case outcomeDeferred:
			stats.DeferredUnvalidated++
		case outcomeReject:
			stats.Rejected++
		case outcomeNoCandidate:
			stats.NoCandidate++
		default:
			return stats, fmt.Errorf("unknown committed reconciliation outcome %q", outcome)
		}
	}
	return stats, stats.Validate()
}

func (r *Reconciler) decideCandidate(ctx context.Context, scanned Concept, embedding []ReconcileProposal) (string, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
		return "", err
	}

	candidate, err := getConceptForUpdate(ctx, tx, scanned.ConceptID)
	if err != nil {
		return "", fmt.Errorf("lock candidate %s: %w", scanned.ConceptID, err)
	}
	eligible := candidate.Status == "provisional" && candidate.GlossSource == "auto:d11" && candidate.Scope == r.Scope
	candidateSurfaces, err := listSurfacesForUpdate(ctx, tx, candidate.ConceptID)
	if err != nil {
		return "", err
	}
	if !eligible {
		decision := candidateDecision{Outcome: outcomeReject, Reason: "candidate_no_longer_eligible"}
		proposals := mergeAuditProposals(embedding, tier5Result{}, nil)
		if err := appendReconcileAudit(ctx, tx, r.Scope, candidate.ConceptID, r.EmbeddingModelID, proposals, []IdentityClaim{}, decision, []string{"candidate_no_longer_eligible"}); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return outcomeReject, nil
	}

	tier5Shortlist, err := listTier5Shortlist(ctx, tx, candidate.PrefLabel, r.Scope, candidate.ConceptID)
	if err != nil {
		return "", err
	}
	tier5 := recomputeTier5(candidate.PrefLabel, tier5Shortlist, semid.Normalizer{Version: semid.CurrentNormalizerVersion})
	claims, err := r.loadClaims(ctx, tx, candidate, candidateSurfaces)
	if err != nil {
		return "", err
	}
	claimTargets, err := loadClaimTargets(ctx, tx, claims)
	if err != nil {
		return "", err
	}
	proposals := mergeAuditProposals(embedding, tier5, claims)

	decision := chooseCandidateDecision(tier5, claims, embedding, claimTargets)
	if decision.Outcome != outcomeValidate {
		if err := appendReconcileAudit(ctx, tx, r.Scope, candidate.ConceptID, r.EmbeddingModelID, proposals, claims, decision, decisionVetoes(decision)); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return decision.Outcome, nil
	}

	target, err := getConceptForUpdate(ctx, tx, decision.TargetConceptID)
	if err != nil {
		return "", fmt.Errorf("lock reconciliation target %s: %w", decision.TargetConceptID, err)
	}
	targetSurfaces, err := listSurfacesForUpdate(ctx, tx, target.ConceptID)
	if err != nil {
		return "", err
	}
	vetoes, err := r.pairVetoes(ctx, tx, candidate, target, candidateSurfaces, targetSurfaces, decision, claims, claimTargets)
	if err != nil {
		return "", err
	}
	if len(vetoes) > 0 {
		rejected := candidateDecision{Outcome: outcomeReject, Method: decision.Method, TargetConceptID: target.ConceptID, Reason: "pair_veto"}
		if err := appendReconcileAudit(ctx, tx, r.Scope, candidate.ConceptID, r.EmbeddingModelID, proposals, claims, rejected, vetoes); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return outcomeReject, nil
	}

	txStore := r.ConceptStore
	txStore.DB = tx
	if err := txStore.MergeConceptTx(ctx, tx, candidate.ConceptID, target.ConceptID); err != nil {
		if errors.Is(err, ErrMergeRejected) {
			rejected := candidateDecision{Outcome: outcomeReject, Method: decision.Method, TargetConceptID: target.ConceptID, Reason: "merge_guard_veto"}
			if auditErr := appendReconcileAudit(ctx, tx, r.Scope, candidate.ConceptID, r.EmbeddingModelID, proposals, claims, rejected, []string{"merge_guard_veto"}); auditErr != nil {
				return "", auditErr
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return "", commitErr
			}
			return outcomeReject, nil
		}
		return "", err
	}
	merged := candidateDecision{Outcome: outcomeAutoMerge, Method: decision.Method, TargetConceptID: target.ConceptID, Reason: decision.Reason}
	if err := appendReconcileAudit(ctx, tx, r.Scope, candidate.ConceptID, r.EmbeddingModelID, proposals, claims, merged, []string{}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return outcomeAutoMerge, nil
}

func (r *Reconciler) loadClaims(ctx context.Context, tx *sql.Tx, candidate Concept, surfaces []Surface) ([]IdentityClaim, error) {
	ids := make([]string, len(surfaces))
	for i := range surfaces {
		ids[i] = surfaces[i].SurfaceID
	}
	request := CanonicalCandidateIdentityContext(CandidateIdentityContext{CandidateConceptID: candidate.ConceptID, Scope: candidate.Scope, SurfaceIDs: ids})
	providers := append([]IdentityEvidenceProvider(nil), r.EvidenceProviders...)
	sort.Slice(providers, func(i, j int) bool { return providers[i].ProviderID() < providers[j].ProviderID() })
	batches := make([]IdentityClaimBatch, 0, len(providers))
	for _, provider := range providers {
		claims, err := provider.LoadClaims(ctx, tx, request)
		if err != nil {
			return nil, fmt.Errorf("identity evidence provider %q: %w", provider.ProviderID(), err)
		}
		batches = append(batches, IdentityClaimBatch{ProviderID: provider.ProviderID(), Claims: claims})
	}
	return NormalizeIdentityClaimBatches(candidate.ConceptID, batches)
}

func chooseCandidateDecision(tier5 tier5Result, claims []IdentityClaim, embeddings []ReconcileProposal, active map[string]Concept) candidateDecision {
	authoritative := authoritativeActiveTargets(claims, active)
	if tier5.State == tier5Unique {
		if len(authoritative) > 1 || (len(authoritative) == 1 && authoritative[0] != tier5.TargetConceptID) {
			return candidateDecision{Outcome: outcomeReject, Reason: "tier5_authority_conflict"}
		}
		return candidateDecision{Outcome: outcomeValidate, TargetConceptID: tier5.TargetConceptID, Method: methodTier5, Reason: "unique_tier5"}
	}
	if len(authoritative) > 1 {
		return candidateDecision{Outcome: outcomeReject, Reason: "conflicting_authoritative_targets"}
	}
	if len(authoritative) == 1 {
		return candidateDecision{Outcome: outcomeValidate, TargetConceptID: authoritative[0], Method: methodIdentity, Reason: "unique_authoritative_identity"}
	}
	if tier5.State == tier5Tie || len(claims) > 0 || len(embeddings) > 0 {
		return candidateDecision{Outcome: outcomeDeferred, Reason: "insufficient_authoritative_identity"}
	}
	return candidateDecision{Outcome: outcomeNoCandidate, Reason: "no_proposal_or_evidence"}
}

func authoritativeActiveTargets(claims []IdentityClaim, active map[string]Concept) []string {
	set := map[string]struct{}{}
	for _, claim := range claims {
		if claim.Relation == IdentityRelationExactEquivalent && claim.Authority == IdentityAuthorityAuthoritative {
			if target, ok := active[claim.TargetConceptID]; ok && target.Status == "active" {
				set[target.ConceptID] = struct{}{}
			}
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func recomputeTier5(label string, active []Concept, normalizer semid.Normalizer) tier5Result {
	norm := normalizer.Normalize(label).Norm
	runeLen := utf8.RuneCountInString(norm)
	if runeLen <= 4 {
		return tier5Result{State: tier5None, Scored: []tier5ScoredTarget{}}
	}
	scored := make([]tier5ScoredTarget, 0)
	for _, target := range active {
		targetNorm := normalizer.Normalize(target.PrefLabel).Norm
		if score, ok := fuzzyCandidateScore(norm, targetNorm, runeLen); ok {
			scored = append(scored, tier5ScoredTarget{ConceptID: target.ConceptID, Score: score})
		}
	}
	return selectTier5(scored)
}

func selectTier5(scored []tier5ScoredTarget) tier5Result {
	scored = append([]tier5ScoredTarget(nil), scored...)
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].ConceptID < scored[j].ConceptID
	})
	if len(scored) == 0 {
		return tier5Result{State: tier5None, Scored: scored}
	}
	if len(scored) > 1 && math.Abs(scored[0].Score-scored[1].Score) <= tier5TieEpsilon {
		return tier5Result{State: tier5Tie, Score: scored[0].Score, Scored: scored}
	}
	return tier5Result{State: tier5Unique, TargetConceptID: scored[0].ConceptID, Score: scored[0].Score, Scored: scored}
}

func (r *Reconciler) pairVetoes(ctx context.Context, tx *sql.Tx, candidate, target Concept, candidateSurfaces, targetSurfaces []Surface, decision candidateDecision, claims []IdentityClaim, active map[string]Concept) ([]string, error) {
	var vetoes []string
	if candidate.Status != "provisional" || candidate.GlossSource != "auto:d11" {
		vetoes = append(vetoes, "candidate_not_d11_provisional")
	}
	if target.Status != "active" {
		vetoes = append(vetoes, "target_not_active")
	}
	if candidate.Scope != r.Scope || target.Scope != r.Scope || candidate.Scope != target.Scope {
		vetoes = append(vetoes, "scope_conflict")
	}
	for _, surface := range candidateSurfaces {
		if surface.Locked {
			vetoes = append(vetoes, "absorbed_surface_locked")
			break
		}
	}
	if !equalDigitSignatureSets(candidateSurfaces, targetSurfaces) {
		vetoes = append(vetoes, "digit_conflict")
	}
	authoritative := authoritativeActiveTargets(claims, active)
	if decision.Method == methodIdentity && (len(authoritative) != 1 || authoritative[0] != target.ConceptID) {
		vetoes = append(vetoes, "identity_authority_changed")
	}
	if decision.Method == methodTier5 && (len(authoritative) > 1 || (len(authoritative) == 1 && authoritative[0] != target.ConceptID)) {
		vetoes = append(vetoes, "tier5_authority_conflict")
	}
	blocked, err := isNeverMerge(ctx, tx, candidate.ConceptID, target.ConceptID)
	if err != nil {
		return nil, err
	}
	if blocked {
		vetoes = append(vetoes, "never_merge")
	}
	conflict, err := r.ConceptStore.Alignments.MergeConflict(ctx, tx, candidate.ConceptID, target.ConceptID)
	if err != nil {
		return nil, err
	}
	if conflict {
		vetoes = append(vetoes, "alignment_conflict")
	}
	sort.Strings(vetoes)
	return dedupeStrings(vetoes), nil
}

func equalDigitSignatureSets(left, right []Surface) bool {
	set := func(surfaces []Surface) map[string]struct{} {
		out := map[string]struct{}{}
		n := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
		for _, surface := range surfaces {
			if digits := digitsOnly(n.Normalize(surface.Surface).Norm); digits != "" {
				out[digits] = struct{}{}
			}
		}
		return out
	}
	a, b := set(left), set(right)
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return false
		}
	}
	return true
}

func listTier5Shortlist(ctx context.Context, tx *sql.Tx, label, scope, exclude string) ([]Concept, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+conceptColumns+` `+conceptFrom+`
WHERE scope = $1 AND status = 'active' AND concept_id != $4
  AND similarity(pref_label, $2) > $3
ORDER BY similarity(pref_label, $2) DESC, concept_id
LIMIT $5`, scope, label, reconcileLexicalBlockMin, exclude, reconcileLexicalTopK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var concepts []Concept
	for rows.Next() {
		concept, err := scanConcept(rows.Scan)
		if err != nil {
			return nil, err
		}
		concepts = append(concepts, concept)
	}
	return concepts, rows.Err()
}

// loadClaimTargets resolves target status globally. Cross-scope authority is
// still candidate-global; scope is a pair veto only after target selection.
func loadClaimTargets(ctx context.Context, tx *sql.Tx, claims []IdentityClaim) (map[string]Concept, error) {
	set := map[string]struct{}{}
	for _, claim := range claims {
		if claim.TargetConceptID != "" {
			set[claim.TargetConceptID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return map[string]Concept{}, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+conceptColumns+` `+conceptFrom+` WHERE concept_id = ANY($1) ORDER BY concept_id`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]Concept, len(ids))
	for rows.Next() {
		concept, err := scanConcept(rows.Scan)
		if err != nil {
			return nil, err
		}
		result[concept.ConceptID] = concept
	}
	return result, rows.Err()
}

func listSurfacesForUpdate(ctx context.Context, tx *sql.Tx, conceptID string) ([]Surface, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+surfaceColumns+` `+surfaceFrom+` WHERE concept_id = $1 ORDER BY surface_id FOR UPDATE`, conceptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var surfaces []Surface
	for rows.Next() {
		surface, err := scanSurface(rows.Scan)
		if err != nil {
			return nil, err
		}
		surfaces = append(surfaces, surface)
	}
	return surfaces, rows.Err()
}

func appendReconcileAudit(ctx context.Context, tx *sql.Tx, scope, candidateID, modelID string, proposals []ReconcileProposal, claims []IdentityClaim, decision candidateDecision, vetoes []string) error {
	audit := reconcileAudit{CandidateConceptID: candidateID, Scope: scope, Outcome: decision.Outcome, Reason: decision.Reason, Method: decision.Method, EmbeddingModelID: modelID, Proposals: proposals, Claims: claims, Vetoes: vetoes}
	if audit.Proposals == nil {
		audit.Proposals = []ReconcileProposal{}
	}
	if audit.Claims == nil {
		audit.Claims = []IdentityClaim{}
	}
	if audit.Vetoes == nil {
		audit.Vetoes = []string{}
	}
	if decision.Outcome == outcomeAutoMerge {
		audit.AbsorbedConceptID, audit.SurvivorConceptID = candidateID, decision.TargetConceptID
	}
	payload, err := json.Marshal(audit)
	if err != nil {
		return err
	}
	_, err = (semid.DecisionLogStore{DB: tx}).Append(ctx, semid.DecisionLogEntry{Family: "keyword_reconcile", Scope: scope, Input: payload, Output: payload, Verdict: decision.Outcome, Model: modelID, Actor: "keyword_reconciler"})
	return err
}

func decisionVetoes(decision candidateDecision) []string {
	if decision.Outcome == outcomeReject {
		return []string{decision.Reason}
	}
	return []string{}
}

func mergeAuditProposals(embedding []ReconcileProposal, tier5 tier5Result, claims []IdentityClaim) []ReconcileProposal {
	byTarget := map[string]*ReconcileProposal{}
	ensure := func(id string) *ReconcileProposal {
		if byTarget[id] == nil {
			byTarget[id] = &ReconcileProposal{TargetConceptID: id, Methods: []ReconcileProposalMethod{}, Claims: []IdentityClaim{}}
		}
		return byTarget[id]
	}
	for _, proposal := range embedding {
		target := ensure(proposal.TargetConceptID)
		target.Methods = append(target.Methods, proposal.Methods...)
	}
	for rank, target := range tier5.Scored {
		score := target.Score
		proposal := ensure(target.ConceptID)
		proposal.Methods = append(proposal.Methods, ReconcileProposalMethod{Method: methodTier5, Rank: rank + 1, Score: &score})
	}
	for _, claim := range claims {
		if claim.TargetConceptID != "" {
			proposal := ensure(claim.TargetConceptID)
			if len(proposal.Claims) == 0 {
				proposal.Methods = append(proposal.Methods, ReconcileProposalMethod{Method: methodIdentity})
			}
			proposal.Claims = append(proposal.Claims, claim)
		}
	}
	ids := make([]string, 0, len(byTarget))
	for id := range byTarget {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ReconcileProposal, 0, len(ids))
	for _, id := range ids {
		proposal := byTarget[id]
		sort.Slice(proposal.Methods, func(i, j int) bool {
			a, b := proposal.Methods[i], proposal.Methods[j]
			if a.Method != b.Method {
				return a.Method < b.Method
			}
			if a.Rank != b.Rank {
				return a.Rank < b.Rank
			}
			if a.ModelID != b.ModelID {
				return a.ModelID < b.ModelID
			}
			return scoreValue(a.Score) < scoreValue(b.Score)
		})
		sort.Slice(proposal.Claims, func(i, j int) bool { return identityClaimLess(proposal.Claims[i], proposal.Claims[j]) })
		out = append(out, *proposal)
	}
	return out
}

func scoreValue(score *float64) float64 {
	if score == nil {
		return math.Inf(-1)
	}
	return *score
}

func embeddingProposals(candidate Concept, concepts []Concept, vectors map[string][]float64, modelID string) []ReconcileProposal {
	candidateVector, ok := vectors[candidate.ConceptID]
	if !ok {
		return []ReconcileProposal{}
	}
	proposals := make([]ReconcileProposal, 0, len(concepts))
	for _, target := range concepts {
		if target.ConceptID == candidate.ConceptID || target.Status != "active" || target.Scope != candidate.Scope {
			continue
		}
		targetVector, ok := vectors[target.ConceptID]
		if !ok {
			continue
		}
		score := cosineSimilarity(candidateVector, targetVector)
		proposals = append(proposals, ReconcileProposal{TargetConceptID: target.ConceptID, Methods: []ReconcileProposalMethod{{Method: methodEmbedding, Score: &score, ModelID: modelID}}, Claims: []IdentityClaim{}})
	}
	sort.Slice(proposals, func(i, j int) bool {
		scoreI, scoreJ := *proposals[i].Methods[0].Score, *proposals[j].Methods[0].Score
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return proposals[i].TargetConceptID < proposals[j].TargetConceptID
	})
	fullEligiblePool := len(proposals)
	if len(proposals) > reconcileEmbeddingTopK {
		proposals = proposals[:reconcileEmbeddingTopK]
	}
	for index := range proposals {
		proposals[index].Methods[0].Rank = index + 1
		proposals[index].Methods[0].CandidateSetSize = fullEligiblePool
		proposals[index].Methods[0].CandidateSetLimit = reconcileEmbeddingTopK
	}
	if proposals == nil {
		proposals = []ReconcileProposal{}
	}
	return proposals
}

func (r *Reconciler) embedConcepts(ctx context.Context, concepts []Concept) (map[string][]float64, error) {
	if r.Embeddings == nil {
		return map[string][]float64{}, nil
	}
	texts := make([]string, len(concepts))
	for i := range concepts {
		texts[i] = concepts[i].PrefLabel
	}
	vectors, err := r.Embeddings.EmbedBatch(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(concepts) {
		return nil, fmt.Errorf("embedding client returned %d vectors for %d texts", len(vectors), len(concepts))
	}
	dimension := 0
	out := make(map[string][]float64, len(concepts))
	for i, vector := range vectors {
		if len(vector) == 0 {
			return nil, fmt.Errorf("embedding vector %d is empty", i)
		}
		if dimension == 0 {
			dimension = len(vector)
		} else if len(vector) != dimension {
			return nil, fmt.Errorf("embedding vector %d has dimension %d, want %d", i, len(vector), dimension)
		}
		for component, value := range vector {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("embedding vector %d component %d is not finite", i, component)
			}
		}
		out[concepts[i].ConceptID] = append([]float64(nil), vector...)
	}
	return out, nil
}

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

func dedupeStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

// ambiguousDecisionAudit is the decision-log payload for one
// ReconcileAmbiguous row (spec 2026080403 §13, following the ambiguity
// reconciliation design agreed for the tied-candidate backlog).
type ambiguousDecisionAudit struct {
	NormKey          string                       `json:"norm_key"`
	Scope            string                       `json:"scope"`
	Query            string                       `json:"query"`
	Outcome          string                       `json:"outcome"`
	Method           string                       `json:"method"`
	WinnerConceptID  string                       `json:"winner_concept_id,omitempty"`
	EmbeddingModelID string                       `json:"embedding_model_id,omitempty"`
	Candidates       []ambiguousCandidateEvidence `json:"candidates"`
}

type ambiguousCandidateEvidence struct {
	ConceptID      string   `json:"concept_id"`
	OriginalScore  float64  `json:"original_score"`
	OriginalMethod string   `json:"original_method"`
	StillLive      bool     `json:"still_live"`
	EmbeddingScore *float64 `json:"embedding_score,omitempty"`
	LexicalMatch   bool     `json:"lexical_match"`
}

func appendAmbiguousAudit(ctx context.Context, tx *sql.Tx, scope string, audit ambiguousDecisionAudit) error {
	if audit.Candidates == nil {
		audit.Candidates = []ambiguousCandidateEvidence{}
	}
	payload, err := json.Marshal(audit)
	if err != nil {
		return err
	}
	_, err = (semid.DecisionLogStore{DB: tx}).Append(ctx, semid.DecisionLogEntry{
		Family: "keyword_reconcile_ambiguous", Scope: scope, Input: payload, Output: payload,
		Verdict: audit.Outcome, Model: audit.EmbeddingModelID, Actor: "keyword_reconciler_ambiguous",
	})
	return err
}

// ReconcileAmbiguous re-ranks backlog rows an `ambiguous` verdict logged
// (spec 2026080403 §13): tiers 0-5 tied across more concepts than
// KeywordFamily's AutoAcceptPolicy.MaxCandidates allows, so the online path
// picked the lowest concept_id as an arbitrary tiebreak (§8.3) and queued
// the tie here (§9.3, VerdictAmbiguous) rather than guessing confidently.
//
// This never merges concepts (that stays tier 6's job, D10-conservative,
// via ReconcileAmbiguous's sibling Run) and never rewrites kb.keyword_
// occurrences (append-only by design). It only: re-verifies the tied
// concepts are still live (chasing any merge that happened since), and,
// for a genuine remaining tie, embeds the original query and ranks it
// against each live candidate's pref_label -- auto-applying (a new `alt`
// surface on the winner) only when the top candidate clears a configured
// margin over the runner-up or is independently corroborated by the same
// edit-distance check tier 5 uses online. A bare top-1 embedding pick with
// neither signal is left in the backlog, not silently applied -- embeddings
// rank here exactly as they do in Run's tier-6 path, they do not decide
// alone.
func (r *Reconciler) ReconcileAmbiguous(ctx context.Context) (AmbiguousReconcileStats, error) {
	var stats AmbiguousReconcileStats
	if err := r.validateConfiguration(); err != nil {
		return stats, err
	}
	rows, err := (UnresolvedStore{DB: r.DB}).ListAmbiguous(ctx, r.Scope, 0)
	if err != nil {
		return stats, err
	}
	for _, row := range rows {
		stats.Scanned++
		outcome, err := r.decideAmbiguous(ctx, row)
		if err != nil {
			stats.Failed = 1
			return stats, err
		}
		stats.Decided++
		switch outcome {
		case ambiguousOutcomeApplied:
			stats.AutoApplied++
		case ambiguousOutcomeCollapsed:
			stats.Collapsed++
		case ambiguousOutcomeDeferred:
			stats.Deferred++
		case ambiguousOutcomeNoCandidate:
			stats.NoCandidate++
		default:
			return stats, fmt.Errorf("unknown ambiguous reconciliation outcome %q", outcome)
		}
	}
	return stats, stats.Validate()
}

// liveAmbiguousCandidate is one tied candidate re-verified against current
// concept state.
type liveAmbiguousCandidate struct {
	Original TiedCandidate
	Concept  Concept
}

func (r *Reconciler) decideAmbiguous(ctx context.Context, row Unresolved) (string, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if err := semid.AcquireKeywordIdentityMutationLock(ctx, tx); err != nil {
		return "", err
	}

	txConcepts := r.ConceptStore
	txConcepts.DB = tx

	// Re-verify: chase merged_into for every originally-tied concept, then
	// keep only distinct, currently-active survivors. A concept that merged
	// away, was deprecated, or is still provisional is not a safe target to
	// attach new evidence to with the confidence this pass requires (the
	// same active-only bar Run's tier-6 targets already use).
	seen := map[string]bool{}
	var live []liveAmbiguousCandidate
	for _, tc := range row.Candidates {
		survivorID, err := txConcepts.FollowMerge(ctx, tc.ConceptID)
		if err != nil {
			continue // tombstoned/unreachable id: drop it, not a hard failure
		}
		if seen[survivorID] {
			continue
		}
		seen[survivorID] = true
		c, err := txConcepts.GetConcept(ctx, survivorID)
		if err != nil || c.Status != "active" {
			continue
		}
		live = append(live, liveAmbiguousCandidate{Original: tc, Concept: c})
	}

	audit := ambiguousDecisionAudit{NormKey: row.NormKey, Scope: r.Scope, EmbeddingModelID: r.EmbeddingModelID}
	if len(row.Surfaces) > 0 {
		audit.Query = row.Surfaces[0]
	} else {
		audit.Query = row.NormKey
	}
	for _, c := range live {
		audit.Candidates = append(audit.Candidates, ambiguousCandidateEvidence{
			ConceptID: c.Concept.ConceptID, OriginalScore: c.Original.Score, OriginalMethod: c.Original.Method, StillLive: true,
		})
	}

	var outcome string
	switch {
	case len(live) == 0:
		outcome = ambiguousOutcomeNoCandidate
		audit.Outcome, audit.Method = outcome, ""
		if err := appendAmbiguousAudit(ctx, tx, r.Scope, audit); err != nil {
			return "", err
		}
		if err := (UnresolvedStore{DB: tx}).UpdateUnresolvedStatus(ctx, row.NormKey, r.Scope, "pending", "keyword_reconciler_ambiguous"); err != nil {
			return "", err
		}
	case len(live) == 1:
		outcome = ambiguousOutcomeCollapsed
		audit.Outcome, audit.Method, audit.WinnerConceptID = outcome, methodAmbiguousCollapsed, live[0].Concept.ConceptID
		if err := r.applyAmbiguousWinner(ctx, tx, live[0].Concept.ConceptID, audit.Query, 0.9); err != nil {
			return "", err
		}
		if err := appendAmbiguousAudit(ctx, tx, r.Scope, audit); err != nil {
			return "", err
		}
		if err := (UnresolvedStore{DB: tx}).UpdateUnresolvedStatus(ctx, row.NormKey, r.Scope, "resolved", "keyword_reconciler_ambiguous"); err != nil {
			return "", err
		}
	default:
		winnerID, method, err := r.rankAmbiguousCandidates(ctx, audit.Query, live, &audit)
		if err != nil {
			return "", err
		}
		if winnerID != "" {
			outcome = ambiguousOutcomeApplied
			audit.Outcome, audit.Method, audit.WinnerConceptID = outcome, method, winnerID
			if err := r.applyAmbiguousWinner(ctx, tx, winnerID, audit.Query, 0.7); err != nil {
				return "", err
			}
			if err := appendAmbiguousAudit(ctx, tx, r.Scope, audit); err != nil {
				return "", err
			}
			if err := (UnresolvedStore{DB: tx}).UpdateUnresolvedStatus(ctx, row.NormKey, r.Scope, "resolved", "keyword_reconciler_ambiguous"); err != nil {
				return "", err
			}
		} else {
			outcome = ambiguousOutcomeDeferred
			audit.Outcome = outcome
			if err := appendAmbiguousAudit(ctx, tx, r.Scope, audit); err != nil {
				return "", err
			}
			if err := (UnresolvedStore{DB: tx}).UpdateUnresolvedStatus(ctx, row.NormKey, r.Scope, "pending", "keyword_reconciler_ambiguous"); err != nil {
				return "", err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return outcome, nil
}

// applyAmbiguousWinner writes a new `alt` surface for the query text on the
// winning concept. CreateSurface is idempotent on the uniqueness key (F3),
// so re-running a resolved row is safe. This never touches kb.keyword_
// occurrences (append-only) -- past occurrences keep the answer the system
// actually gave at the time; future resolves of this literal now hit tier 1
// against the winner instead of re-tying, unless the losing candidate also
// already carries an exact surface for the same literal (a duplicate-
// concept case reconciliation's merge path, not this one, addresses).
func (r *Reconciler) applyAmbiguousWinner(ctx context.Context, tx *sql.Tx, conceptID, query string, confidence float64) error {
	surfaces := SurfaceStore{DB: tx, NormalizerVersion: semid.CurrentNormalizerVersion}
	_, err := surfaces.CreateSurface(ctx, Surface{
		ConceptID:  conceptID,
		Surface:    query,
		LabelRole:  "alt",
		AliasType:  "synonym",
		Provenance: "reconcile:ambiguous",
		Scope:      r.Scope,
		Confidence: confidence,
	})
	return err
}

// rankAmbiguousCandidates embeds the query and each live candidate's
// pref_label, ranks by cosine similarity, and auto-applies the top pick
// only when it clears AmbiguousMarginThreshold over the runner-up or is
// independently corroborated by tier 5's own edit-distance check (the same
// two-signal discipline Run's chooseCandidateDecision already applies to
// merges -- embeddings rank, they do not decide alone). Returns ("", "")
// when neither bar is cleared.
func (r *Reconciler) rankAmbiguousCandidates(ctx context.Context, query string, live []liveAmbiguousCandidate, audit *ambiguousDecisionAudit) (string, string, error) {
	texts := make([]string, 0, len(live)+1)
	texts = append(texts, query)
	for _, c := range live {
		texts = append(texts, c.Concept.PrefLabel)
	}
	vectors, err := r.Embeddings.EmbedBatch(ctx, texts)
	if err != nil {
		return "", "", fmt.Errorf("embed ambiguous candidates: %w", err)
	}
	if len(vectors) != len(texts) {
		return "", "", fmt.Errorf("embedding client returned %d vectors for %d texts", len(vectors), len(texts))
	}
	queryVector := vectors[0]

	type scored struct {
		conceptID string
		score     float64
	}
	ranked := make([]scored, len(live))
	for i, c := range live {
		s := cosineSimilarity(queryVector, vectors[i+1])
		ranked[i] = scored{conceptID: c.Concept.ConceptID, score: s}
		for j := range audit.Candidates {
			if audit.Candidates[j].ConceptID == c.Concept.ConceptID {
				sv := s
				audit.Candidates[j].EmbeddingScore = &sv
			}
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].conceptID < ranked[j].conceptID
	})

	// Lexical corroboration: tier 5's own edit-distance check, run against
	// the query and each live candidate's label. If exactly one candidate
	// clears it, that is independent evidence -- agreement with the top
	// embedding pick corroborates it regardless of margin.
	normalizer := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	queryNorm := normalizer.Normalize(query).Norm
	runeLen := utf8.RuneCountInString(queryNorm)
	lexicalMatches := 0
	lexicalWinner := ""
	for i := range audit.Candidates {
		candNorm := normalizer.Normalize(live[i].Concept.PrefLabel).Norm
		if _, ok := fuzzyCandidateScore(queryNorm, candNorm, runeLen); ok {
			audit.Candidates[i].LexicalMatch = true
			lexicalMatches++
			lexicalWinner = audit.Candidates[i].ConceptID
		}
	}
	lexicalCorroborates := lexicalMatches == 1 && lexicalWinner == ranked[0].conceptID

	margin := r.AmbiguousMarginThreshold
	if margin <= 0 {
		margin = defaultAmbiguousMargin
	}
	clearsMargin := len(ranked) >= 2 && (ranked[0].score-ranked[1].score) >= margin

	switch {
	case clearsMargin && lexicalCorroborates:
		return ranked[0].conceptID, methodAmbiguousLexical, nil
	case clearsMargin:
		return ranked[0].conceptID, methodAmbiguousMargin, nil
	case lexicalCorroborates:
		return ranked[0].conceptID, methodAmbiguousLexical, nil
	default:
		return "", "", nil
	}
}
