package keywords

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// KeywordFamily adapts the semid kernel to the keyword identity family (DR16).
// Keywords are ungoverned — auto-accept is enabled for tiers 0-4.
// Shipped behind KEYWORD_RESOLVER_MODE=observe: resolution runs but no
// downstream consumer is connected.
type KeywordFamily struct {
	DB                *sql.DB
	NormalizerVersion int
	ResolverMode      string // "off", "observe", "on"

	ConceptStore     ConceptStore
	SurfaceStore     SurfaceStore
	SurfaceKeyStore  SurfaceKeyStore
	MentionStore     MentionStore
	UnresolvedStore  UnresolvedStore
	RewriteRuleStore RewriteRuleStore
}

// ensureDefaults sets up store instances from DB when not explicitly provided.
func (kf *KeywordFamily) ensureDefaults() {
	if kf.ConceptStore.DB == nil && kf.DB != nil {
		kf.ConceptStore = ConceptStore{DB: kf.DB}
		kf.SurfaceStore = SurfaceStore{DB: kf.DB}
		kf.SurfaceKeyStore = SurfaceKeyStore{DB: kf.DB}
		kf.MentionStore = MentionStore{DB: kf.DB}
		kf.UnresolvedStore = UnresolvedStore{DB: kf.DB}
		kf.RewriteRuleStore = RewriteRuleStore{DB: kf.DB}
	}
	if kf.NormalizerVersion <= 0 {
		kf.NormalizerVersion = 1
	}
}

// modeActive reports whether the family's resolver mode permits resolution.
// Only "observe" and "on" are active; every other value (including "") is
// treated as off so the gate fails closed.
func (kf *KeywordFamily) modeActive() bool {
	return kf.ResolverMode == "observe" || kf.ResolverMode == "on"
}

// FamilyName implements semid.FamilyAdapter.
func (kf *KeywordFamily) FamilyName() string { return "keyword" }

// Normalizer implements semid.FamilyAdapter. Returns a Normalizer whose
// NormFunc delegates to the keyword normalizer pipeline.
func (kf *KeywordFamily) Normalizer() semid.Normalizer {
	kf.ensureDefaults()
	return semid.Normalizer{
		Name:    "keyword",
		Version: kf.NormalizerVersion,
		NormFunc: func(surface string) semid.KeyBundle {
			kb := (KeywordNormalizer{Version: kf.NormalizerVersion}).Normalize(surface)
			canonical, alternates := kb.ToSemidKeyBundle()
			return semid.KeyBundle{CanonicalKey: canonical, AlternateKeys: alternates}
		},
	}
}

// AutoAcceptPolicy implements semid.FamilyAdapter. Keywords are ungoverned:
// tiers 0-4 auto-accept on a single unambiguous match at score ≥ 0.8.
func (kf *KeywordFamily) AutoAcceptPolicy() semid.AutoAcceptPolicy {
	return semid.AutoAcceptPolicy{Enabled: true, MinScore: 0.8, MaxCandidates: 1}
}

// Scope implements semid.FamilyAdapter. Scopes by knowledge store; uses "_"
// for system scope.
func (kf *KeywordFamily) Scope(surface string) string { return "_" }

// CandidateNodes implements semid.FamilyAdapter. Multi-tier candidate
// generation with early exit at the first tier that produces candidates.
// Tiers 5-6 (fuzzy/ANN) are deferred and return empty.
func (kf *KeywordFamily) CandidateNodes(ctx context.Context, surface, scope string) ([]semid.NodeCandidate, error) {
	kf.ensureDefaults()
	// Fail closed: any mode other than observe/on is inert. An empty or
	// unrecognized ResolverMode must never leave resolution running.
	if !kf.modeActive() || kf.DB == nil {
		return nil, nil
	}

	n := KeywordNormalizer{Version: kf.NormalizerVersion}
	kb := n.Normalize(surface)

	// Tier 0: exact surface match.
	if matches, ok := kf.tier0ExactMatch(ctx, surface, scope); ok {
		return matches, nil
	}

	// Tier 1: norm key match.
	if matches, ok := kf.tier1NormKeyMatch(ctx, kb.Norm, scope); ok {
		return matches, nil
	}

	// Tier 2: alnum/sorted key match.
	if matches, ok := kf.tier2AlternateKeyMatch(ctx, kb, scope); ok {
		return matches, nil
	}

	// Tier 3: rewrite rules + retry tiers 0-2.
	rules, err := kf.RewriteRuleStore.ListEnabledRules(ctx, scope)
	if err == nil && len(rules) > 0 {
		rewritten := surface
		for _, r := range rules {
			if r.Pattern != "" && rewritten == r.Pattern {
				rewritten = r.Replacement
				break
			}
		}
		if rewritten != surface {
			// Retry tiers 0-1 with rewritten surface.
			if matches, ok := kf.tier0ExactMatch(ctx, rewritten, scope); ok {
				return matches, nil
			}
			rb := n.Normalize(rewritten)
			if matches, ok := kf.tier1NormKeyMatch(ctx, rb.Norm, scope); ok {
				return matches, nil
			}
		}
	}

	// Tier 4: initials key match within scope.
	if matches, ok := kf.tier4InitialsMatch(ctx, kb.Initials, scope); ok {
		return matches, nil
	}

	// Tiers 5-6: fuzzy/ANN — deferred, return empty.
	// Tier 7: miss — return empty; caller writes unresolved.

	return nil, nil // no candidates found
}

// tier0ExactMatch queries kb.keyword_surfaces for the exact verbatim surface.
func (kf *KeywordFamily) tier0ExactMatch(ctx context.Context, surface, scope string) ([]semid.NodeCandidate, bool) {
	rows, err := kf.DB.QueryContext(ctx, `
		SELECT s.concept_id, s.norm_key
		FROM kb.keyword_surfaces s
		WHERE s.surface = $1 AND s.scope = $2
		LIMIT 10`, surface, scope)
	if err != nil || rows == nil {
		return nil, false
	}
	defer rows.Close()

	var candidates []semid.NodeCandidate
	for rows.Next() {
		var conceptID, normKey string
		if err := rows.Scan(&conceptID, &normKey); err != nil {
			continue
		}
		candidates = append(candidates, semid.NodeCandidate{
			NodeID:    conceptID,
			KeyBundle: semid.KeyBundle{CanonicalKey: normKey},
		})
	}
	return candidates, len(candidates) > 0
}

// tier1NormKeyMatch queries for surfaces matching the normalized key.
func (kf *KeywordFamily) tier1NormKeyMatch(ctx context.Context, normKey, scope string) ([]semid.NodeCandidate, bool) {
	rows, err := kf.DB.QueryContext(ctx, `
		SELECT s.concept_id, s.norm_key
		FROM kb.keyword_surfaces s
		WHERE s.norm_key = $1 AND s.scope = $2
		ORDER BY s.confidence DESC
		LIMIT 10`, normKey, scope)
	if err != nil || rows == nil {
		return nil, false
	}
	defer rows.Close()

	var candidates []semid.NodeCandidate
	for rows.Next() {
		var conceptID, nk string
		if err := rows.Scan(&conceptID, &nk); err != nil {
			continue
		}
		candidates = append(candidates, semid.NodeCandidate{
			NodeID:    conceptID,
			KeyBundle: semid.KeyBundle{CanonicalKey: nk},
		})
	}
	return candidates, len(candidates) > 0
}

// tier2AlternateKeyMatch queries surface_keys for alnum/sorted matches.
// The candidate's CanonicalKey is set to the matching alternate key value,
// so the kernel's Score() produces 0.8 (alternate key match).
func (kf *KeywordFamily) tier2AlternateKeyMatch(ctx context.Context, kb KeywordKeyBundle, scope string) ([]semid.NodeCandidate, bool) {
	for _, pair := range []struct{ kind, value string }{
		{"alnum", kb.Alnum},
		{"sorted", kb.Sorted},
	} {
		if pair.value == "" {
			continue
		}
		candidates, ok := kf.lookupByKeyKind(ctx, pair.kind, pair.value, scope)
		if ok {
			return candidates, true
		}
	}
	return nil, false
}

// tier4InitialsMatch queries surface_keys for initials key matches.
func (kf *KeywordFamily) tier4InitialsMatch(ctx context.Context, initials, scope string) ([]semid.NodeCandidate, bool) {
	if initials == "" {
		return nil, false
	}
	return kf.lookupByKeyKind(ctx, "initials", initials, scope)
}

func (kf *KeywordFamily) lookupByKeyKind(ctx context.Context, keyKind, keyValue, scope string) ([]semid.NodeCandidate, bool) {
	rows, err := kf.DB.QueryContext(ctx, `
		SELECT s.surface_id, s.concept_id, s.norm_key
		FROM kb.keyword_surface_keys sk
		JOIN kb.keyword_surfaces s ON s.surface_id = sk.surface_id
		WHERE sk.key_kind = $1 AND sk.key_value = $2 AND s.scope = $3
		LIMIT 10`, keyKind, keyValue, scope)
	if err != nil || rows == nil {
		return nil, false
	}
	defer rows.Close()

	var candidates []semid.NodeCandidate
	for rows.Next() {
		var surfaceID, conceptID, normKey string
		if err := rows.Scan(&surfaceID, &conceptID, &normKey); err != nil {
			continue
		}
		_ = surfaceID
		// Set CanonicalKey to the matching key value so Score() returns 0.8
		// (surface's AlternateKeys contains this value).
		candidates = append(candidates, semid.NodeCandidate{
			NodeID:    conceptID,
			KeyBundle: semid.KeyBundle{CanonicalKey: keyValue},
		})
	}
	return candidates, len(candidates) > 0
}

// -- Resolution methods -------------------------------------------------------

// ResolveSurface runs the kernel over a surface and records the result.
// In observe mode: writes mentions, surfaces/keys for auto-accepted matches,
// and unresolved backlog for deferred/ambiguous results. In off mode: no-op.
func (kf *KeywordFamily) ResolveSurface(ctx context.Context, surface, scope, artifactRef, contextText string) (*semid.Resolution, error) {
	kf.ensureDefaults()

	if !kf.modeActive() || kf.DB == nil {
		return nil, nil
	}

	// Write a mention row (observe-only).
	kf.MentionStore.InsertMention(ctx, Mention{
		ArtifactRef: &artifactRef,
		ContextText: &contextText,
	})

	res, err := (semid.Kernel{Family: kf}).Resolve(ctx, surface)
	if err != nil {
		return nil, err
	}
	res.Scope = scope

	// Write decision to shared decision log.
	input, _ := json.Marshal(map[string]any{"surface": surface, "scope": scope})
	output, _ := json.Marshal(res)
	_ = (semid.DecisionLogStore{DB: kf.DB}).Append(ctx, semid.DecisionLogEntry{
		Family:  kf.FamilyName(),
		Scope:   scope,
		Input:   input,
		Output:  output,
		Verdict: string(res.Verdict),
		Actor:   "keyword_family",
	})

	// In observe mode: record outcomes.
	switch res.Verdict {
	case semid.VerdictAutoAccept, semid.VerdictHumanReview:
		// For auto-accepted matches, ensure surface/keys exist.
		if res.ResolvedNodeID != "" {
			n := KeywordNormalizer{Version: kf.NormalizerVersion}
			kb := n.Normalize(surface)
			// Only write if not already present (best-effort).
			surfaces, _ := kf.SurfaceStore.ListSurfacesByNormKey(ctx, kb.Norm, scope)
			exists := false
			for _, s := range surfaces {
				if s.ConceptID == res.ResolvedNodeID && s.Surface == surface {
					exists = true
					break
				}
			}
			if !exists {
				kf.SurfaceStore.CreateSurface(ctx, Surface{
					ConceptID:   res.ResolvedNodeID,
					Surface:     surface,
					NormKey:     kb.Norm,
					NormVersion: kf.NormalizerVersion,
					LabelRole:   "alt",
					AliasType:   "synonym",
					Provenance:  "llm:observe",
					Scope:       scope,
					Confidence:  0.8,
				})
			}
		}
	case semid.VerdictDeferred, semid.VerdictAmbiguous:
		// Record in unresolved backlog.
		kf.UnresolvedStore.UpsertUnresolved(ctx, surface, scope, surface, contextText)
	}

	return &res, nil
}
