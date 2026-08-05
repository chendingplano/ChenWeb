package keywords

import (
	"context"
	"database/sql"
	"encoding/json"
	"unicode/utf8"

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
	if kf.NormalizerVersion <= 0 {
		kf.NormalizerVersion = semid.CurrentNormalizerVersion
	}
	if kf.ConceptStore.DB == nil && kf.DB != nil {
		kf.ConceptStore = ConceptStore{DB: kf.DB}
		kf.SurfaceStore = SurfaceStore{DB: kf.DB, NormalizerVersion: kf.NormalizerVersion}
		kf.SurfaceKeyStore = SurfaceKeyStore{DB: kf.DB}
		kf.MentionStore = MentionStore{DB: kf.DB}
		kf.UnresolvedStore = UnresolvedStore{DB: kf.DB}
		kf.RewriteRuleStore = RewriteRuleStore{DB: kf.DB}
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

// AutoAcceptPolicy implements semid.FamilyAdapter. Keywords are ungoverned:
// tiers 0-4 auto-accept on a single unambiguous match at score ≥ 0.8.
func (kf *KeywordFamily) AutoAcceptPolicy() semid.AutoAcceptPolicy {
	return semid.AutoAcceptPolicy{Enabled: true, MinScore: 0.8, MaxCandidates: 1}
}

// normalizer returns the one shared normalizer (D3) at the family's version.
func (kf *KeywordFamily) normalizer() semid.Normalizer {
	kf.ensureDefaults()
	return semid.Normalizer{Version: kf.NormalizerVersion}
}

// kernel assembles the shared kernel for this family.
func (kf *KeywordFamily) kernel() semid.Kernel {
	return semid.Kernel{Family: kf, Normalizer: kf.normalizer()}
}

// CandidateNodes implements semid.FamilyAdapter. Multi-tier candidate
// generation with early exit at the first tier that produces candidates
// (§9.1). Tiers 5-6 (fuzzy/ANN) are deferred and return empty.
func (kf *KeywordFamily) CandidateNodes(ctx context.Context, surface, scope string) ([]semid.NodeCandidate, error) {
	kf.ensureDefaults()
	// Fail closed: any mode other than observe/on is inert. An empty or
	// unrecognized ResolverMode must never leave resolution running.
	if !kf.modeActive() || kf.DB == nil {
		return nil, nil
	}

	ks := kf.normalizer().Normalize(surface)

	// Tier 0: exact surface match (version-independent).
	if matches, ok := kf.tier0ExactMatch(ctx, ks, scope); ok {
		return matches, nil
	}

	// Tier 1: norm key match at the current normalizer version (N4).
	if matches, ok := kf.tier1NormKeyMatch(ctx, ks.Norm, scope); ok {
		return matches, nil
	}

	// Tier 2: alnum/sorted/singular alternate key match.
	if matches, ok := kf.tier2AlternateKeyMatch(ctx, ks, scope); ok {
		return matches, nil
	}

	// Tier 3: rewrite rules + retry tiers 0-1.
	if matches, ok := kf.tier3RewriteMatch(ctx, surface, scope); ok {
		return matches, nil
	}

	// Tier 4: initials bridge (N3) — the query's normalized form is looked
	// up against stored initials keys.
	if matches, ok := kf.tier4InitialsMatch(ctx, ks, scope); ok {
		return matches, nil
	}

	// Tiers 5-6: fuzzy/ANN — deferred. Tier 7: miss — the caller writes the
	// backlog (or auto-creates on targeted names, D11).
	return nil, nil
}

// tier0ExactMatch queries kb.keyword_surfaces for the exact verbatim surface.
// No norm_version filter: surface identity is version-independent.
func (kf *KeywordFamily) tier0ExactMatch(ctx context.Context, ks semid.KeySet, scope string) ([]semid.NodeCandidate, bool) {
	rows, err := kf.DB.QueryContext(ctx, `
		SELECT s.concept_id
		FROM kb.keyword_surfaces s
		WHERE s.surface = $1 AND s.scope = $2
		LIMIT 10`, ks.Exact, scope)
	if err != nil || rows == nil {
		return nil, false
	}
	defer rows.Close()

	// The match evidence is surface identity; the candidate carries the
	// query's own bundle so scoring stays independent of any norm_key stored
	// under an older normalizer version.
	bundle := ks.Bundle()
	seen := map[string]bool{}
	var candidates []semid.NodeCandidate
	for rows.Next() {
		var conceptID string
		if err := rows.Scan(&conceptID); err != nil {
			continue
		}
		if seen[conceptID] {
			continue
		}
		seen[conceptID] = true
		candidates = append(candidates, semid.NodeCandidate{
			NodeID:    conceptID,
			KeyBundle: bundle,
			Method:    "tier0_exact",
		})
	}
	return candidates, len(candidates) > 0
}

// tier1NormKeyMatch queries for surfaces matching the normalized key at the
// current normalizer version (N4: keys only mean something together with the
// version that produced them).
func (kf *KeywordFamily) tier1NormKeyMatch(ctx context.Context, normKey, scope string) ([]semid.NodeCandidate, bool) {
	rows, err := kf.DB.QueryContext(ctx, `
		SELECT s.concept_id, s.norm_key
		FROM kb.keyword_surfaces s
		WHERE s.norm_key = $1 AND s.scope = $2 AND s.norm_version = $3
		ORDER BY s.confidence DESC
		LIMIT 10`, normKey, scope, kf.NormalizerVersion)
	if err != nil || rows == nil {
		return nil, false
	}
	defer rows.Close()

	seen := map[string]bool{}
	var candidates []semid.NodeCandidate
	for rows.Next() {
		var conceptID, nk string
		if err := rows.Scan(&conceptID, &nk); err != nil {
			continue
		}
		if seen[conceptID] {
			continue
		}
		seen[conceptID] = true
		candidates = append(candidates, semid.NodeCandidate{
			NodeID:    conceptID,
			KeyBundle: semid.KeyBundle{CanonicalKey: nk},
			Method:    "tier1_norm",
		})
	}
	return candidates, len(candidates) > 0
}

// tier2AlternateKeyMatch queries surface_keys for alnum/sorted/singular
// matches, in that order.
func (kf *KeywordFamily) tier2AlternateKeyMatch(ctx context.Context, ks semid.KeySet, scope string) ([]semid.NodeCandidate, bool) {
	for _, pair := range []struct{ kind, value string }{
		{"alnum", ks.Alnum},
		{"sorted", ks.Sorted},
		{"singular", ks.Singular},
	} {
		if pair.value == "" {
			continue
		}
		if candidates, ok := kf.lookupByKeyKind(ctx, pair.kind, pair.value, scope, "tier2_altkey"); ok {
			return candidates, true
		}
	}
	return nil, false
}

// tier3RewriteMatch applies at most one enabled rewrite rule (byte equality
// on the raw surface) and retries tiers 0-1 with the rewritten surface.
func (kf *KeywordFamily) tier3RewriteMatch(ctx context.Context, surface, scope string) ([]semid.NodeCandidate, bool) {
	rules, err := kf.RewriteRuleStore.ListEnabledRules(ctx, scope)
	if err != nil || len(rules) == 0 {
		return nil, false
	}
	rewritten := surface
	for _, r := range rules {
		if r.Pattern != "" && rewritten == r.Pattern {
			rewritten = r.Replacement
			break
		}
	}
	if rewritten == surface {
		return nil, false
	}

	rb := kf.normalizer().Normalize(rewritten)
	if matches, ok := kf.tier0ExactMatch(ctx, rb, scope); ok {
		return retagMethod(matches, "tier3_rewrite"), true
	}
	if matches, ok := kf.tier1NormKeyMatch(ctx, rb.Norm, scope); ok {
		return retagMethod(matches, "tier3_rewrite"), true
	}
	return nil, false
}

// tier4InitialsMatch implements the initials bridge (N3): the query's
// normalized form is looked up against stored initials keys — stored initials
// are lower-cased like every other key. The rune-length gate keeps ordinary
// words out of the initials index.
func (kf *KeywordFamily) tier4InitialsMatch(ctx context.Context, ks semid.KeySet, scope string) ([]semid.NodeCandidate, bool) {
	if r := utf8.RuneCountInString(ks.Norm); r < 2 || r > 8 {
		return nil, false
	}
	return kf.lookupByKeyKind(ctx, "initials", ks.Norm, scope, "tier4_initials")
}

func retagMethod(matches []semid.NodeCandidate, method string) []semid.NodeCandidate {
	for i := range matches {
		matches[i].Method = method
	}
	return matches
}

// lookupByKeyKind joins kb.keyword_surface_keys against kb.keyword_surfaces
// for one key kind/value within scope. Candidates carry honest bundles: the
// concept's stored norm key as canonical, the matched key value as an
// alternate so Score can see the bridge. Candidates are deduplicated by
// concept — two surfaces of one concept are not two candidates.
func (kf *KeywordFamily) lookupByKeyKind(ctx context.Context, keyKind, keyValue, scope, method string) ([]semid.NodeCandidate, bool) {
	rows, err := kf.DB.QueryContext(ctx, `
		SELECT s.concept_id, s.norm_key
		FROM kb.keyword_surface_keys sk
		JOIN kb.keyword_surfaces s ON s.surface_id = sk.surface_id
		WHERE sk.key_kind = $1 AND sk.key_value = $2 AND s.scope = $3
		LIMIT 10`, keyKind, keyValue, scope)
	if err != nil || rows == nil {
		return nil, false
	}
	defer rows.Close()

	seen := map[string]bool{}
	var candidates []semid.NodeCandidate
	for rows.Next() {
		var conceptID, normKey string
		if err := rows.Scan(&conceptID, &normKey); err != nil {
			continue
		}
		if seen[conceptID] {
			continue
		}
		seen[conceptID] = true
		candidates = append(candidates, semid.NodeCandidate{
			NodeID: conceptID,
			KeyBundle: semid.KeyBundle{
				CanonicalKey:  normKey,
				AlternateKeys: []string{keyValue},
			},
			Method: method,
		})
	}
	return candidates, len(candidates) > 0
}

// -- Resolution methods -------------------------------------------------------

// ResolveSurface runs the kernel over a surface in the caller's scope and
// returns the resolution. Pure: it writes nothing — no mention row, no
// decision log, no backlog (read/write split, §9.3; K2: scope is the
// caller's input, never a family constant). An inert family (mode off, or no
// DB) returns a zero Resolution and no error.
func (kf *KeywordFamily) ResolveSurface(ctx context.Context, surface, scope string) (semid.Resolution, error) {
	kf.ensureDefaults()
	if !kf.modeActive() || kf.DB == nil {
		return semid.Resolution{}, nil
	}
	res, err := kf.kernel().Resolve(ctx, surface, scope)
	if err != nil {
		return semid.Resolution{}, err
	}
	// §14.1 item 3: resolution chases merged_into so a caller that stored a
	// tombstone id still sees the survivor. Surfaces are re-pointed at merge
	// time, so this normally costs one primary-key lookup of a live concept.
	if res.ResolvedNodeID != "" {
		if survivor, err := kf.ConceptStore.FollowMerge(ctx, res.ResolvedNodeID); err == nil {
			res.ResolvedNodeID = survivor
		}
	}
	return res, nil
}

// ObserveSurface is the observing entry point (§9.3): it writes the mention
// row, runs the pure resolution, appends the decision log, and applies the
// verdict side effects — surface persistence on auto_accepted, and the
// unresolved backlog keyed on the derived norm key (K5) on
// deferred/ambiguous. An inert family returns nil, nil.
func (kf *KeywordFamily) ObserveSurface(ctx context.Context, surface, scope, artifactRef, contextText string) (*semid.Resolution, error) {
	kf.ensureDefaults()
	if !kf.modeActive() || kf.DB == nil {
		return nil, nil
	}

	if _, err := kf.MentionStore.InsertMention(ctx, Mention{
		ArtifactRef: &artifactRef,
		ContextText: &contextText,
	}); err != nil {
		return nil, err
	}

	res, err := kf.ResolveSurface(ctx, surface, scope)
	if err != nil {
		return nil, err
	}

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

	ks := kf.normalizer().Normalize(surface)
	switch res.Verdict {
	case semid.VerdictAutoAccept:
		// Ensure the accepted surface row exists. The human_review arm
		// deliberately does not persist surfaces: below-threshold decisions
		// are recorded in the decision log and sampled (D11), not written
		// into the identity graph.
		if res.ResolvedNodeID != "" {
			surfaces, _ := kf.SurfaceStore.ListSurfacesByNormKey(ctx, ks.Norm, scope)
			exists := false
			for _, s := range surfaces {
				if s.ConceptID == res.ResolvedNodeID && s.Surface == surface {
					exists = true
					break
				}
			}
			if !exists {
				_, _ = kf.SurfaceStore.CreateSurface(ctx, Surface{
					ConceptID:   res.ResolvedNodeID,
					Surface:     surface,
					NormKey:     ks.Norm,
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
		// K5: the backlog PK is (norm_key, scope) — pass the derived norm
		// key, never the raw surface.
		if err := kf.UnresolvedStore.UpsertUnresolved(ctx, ks.Norm, scope, surface, contextText); err != nil {
			return nil, err
		}
	}

	return &res, nil
}
