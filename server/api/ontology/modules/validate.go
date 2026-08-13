package modules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// snapshot is the immutable, serializable representation of a module release:
// the module's approved terms (latest version per term_id) plus their approved
// labels, axioms, and mappings. It is what content_checksum hashes and what
// kb.ontology_module_releases.payload stores, so a release is reproducible
// from its own row.
type snapshot struct {
	ModuleID     string                 `json:"module_id"`
	Version      string                 `json:"version"`
	Terms        []terms.Term           `json:"terms"`
	Labels       []terms.TermLabel      `json:"labels"`
	Axioms       []terms.Axiom          `json:"axioms"`
	Mappings     []terms.Mapping        `json:"mappings"`
	Profiles     []profiles.Profile     `json:"profiles"`
	ProfileRules []profiles.ProfileRule `json:"profile_rules"`
}

// buildSnapshot collects the module's approved content. It runs against a
// queryer (connection or transaction) so the release flow can validate and
// snapshot inside the same tx that tags the rows.
func buildSnapshot(ctx context.Context, db terms.DBX, moduleID, version string) (snapshot, error) {
	ts := terms.TermStore{DB: db}
	approvedTerms, err := ts.ListTerms(ctx, moduleID, "approved")
	if err != nil {
		return snapshot{}, err
	}
	latest := make([]terms.Term, 0, len(approvedTerms))
	seen := make(map[string]bool, len(approvedTerms))
	for _, t := range approvedTerms {
		if seen[t.TermID] {
			continue // ListTerms returns newest-first; first occurrence is latest
		}
		seen[t.TermID] = true
		latest = append(latest, t)
	}

	ls := terms.LabelStore{DB: db}
	labels := make([]terms.TermLabel, 0)
	for _, t := range latest {
		all, err := ls.ListLabels(ctx, t.TermID)
		if err != nil {
			return snapshot{}, err
		}
		for _, l := range all {
			if l.Status == "approved" {
				labels = append(labels, l)
			}
		}
	}

	as := terms.AxiomStore{DB: db}
	axiomRows, err := as.ListAxioms(ctx, moduleID)
	if err != nil {
		return snapshot{}, err
	}
	axioms := make([]terms.Axiom, 0, len(axiomRows))
	for _, a := range axiomRows {
		if a.Status == "approved" {
			axioms = append(axioms, a)
		}
	}

	ms := terms.MappingStore{DB: db}
	mappingRows, err := ms.ListMappings(ctx, moduleID)
	if err != nil {
		return snapshot{}, err
	}
	mappings := make([]terms.Mapping, 0, len(mappingRows))
	for _, m := range mappingRows {
		if m.Status == "approved" {
			mappings = append(mappings, m)
		}
	}

	ps := profiles.ProfileStore{DB: db}
	approvedProfiles, err := ps.ListApprovedProfiles(ctx, moduleID)
	if err != nil {
		return snapshot{}, err
	}
	rs := profiles.ProfileRuleStore{DB: db}
	approvedRules, err := rs.ListApprovedProfileRules(ctx, moduleID)
	if err != nil {
		return snapshot{}, err
	}

	return snapshot{
		ModuleID:     moduleID,
		Version:      version,
		Terms:        latest,
		Labels:       labels,
		Axioms:       axioms,
		Mappings:     mappings,
		Profiles:     approvedProfiles,
		ProfileRules: approvedRules,
	}, nil
}

// validateDepsForModule validates only the target module and its dependency
// closure. Unrelated registered modules may be temporarily incomplete while
// their own external importer is being staged; they must not prevent a
// release of this module.
func validateDepsForModule(allModules []Module, moduleID string) error {
	byID := make(map[string]Module, len(allModules))
	for _, m := range allModules {
		byID[m.ModuleID] = m
	}
	var visit func(id string, visiting, visited map[string]bool) error
	visit = func(id string, visiting, visited map[string]bool) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("dependency cycle detected at module %q", id)
		}
		m, ok := byID[id]
		if !ok {
			return fmt.Errorf("declared dependency module %q does not exist", id)
		}
		visiting[id] = true
		defer delete(visiting, id)
		for _, dep := range m.DependsOn {
			if _, exists := byID[dep]; !exists {
				return fmt.Errorf("module %q depends on unknown module %q", id, dep)
			}
			if err := visit(dep, visiting, visited); err != nil {
				return err
			}
		}
		visited[id] = true
		return nil
	}
	return visit(moduleID, map[string]bool{}, map[string]bool{})
}

// pinDependencies resolves each declared dependency to a pinnable release:
// the active release version if one exists, else the latest release version.
// A dependency with no release cannot be pinned.
func (s ReleaseStore) pinDependencies(ctx context.Context, db terms.DBX, module Module) (map[string]string, error) {
	pins := make(map[string]string, len(module.DependsOn))
	for _, dep := range module.DependsOn {
		active, err := s.activeReleaseVersion(ctx, db, dep)
		if err == nil && active != "" {
			pins[dep] = active
			continue
		}
		latest, err := s.latestReleaseVersion(ctx, db, dep)
		if err != nil {
			return nil, fmt.Errorf("dependency %q has no release to pin: %w", dep, err)
		}
		pins[dep] = latest
	}
	return pins, nil
}

// validateAndBuildSnapshot is the release-time gate: the module must exist,
// hold at least one approved term, have acyclic registered dependencies with
// pinnable releases, and every axiom/mapping reference must resolve to an
// existing governed term (or an external IRI). It returns the snapshot and
// the dependency pins.
func (s ReleaseStore) validateAndBuildSnapshot(ctx context.Context, db terms.DBX, moduleID, version string) (snapshot, map[string]string, error) {
	ms := ModuleStore{DB: s.DB}
	module, err := ms.GetModule(ctx, moduleID)
	if err != nil {
		return snapshot{}, nil, fmt.Errorf("module %q not found", moduleID)
	}

	snap, err := buildSnapshot(ctx, db, moduleID, version)
	if err != nil {
		return snapshot{}, nil, err
	}
	if len(snap.Terms) == 0 {
		return snapshot{}, nil, fmt.Errorf("module %q has no approved terms to release", moduleID)
	}

	allModules, err := ms.ListModules(ctx)
	if err != nil {
		return snapshot{}, nil, err
	}
	if err := validateDepsForModule(allModules, moduleID); err != nil {
		return snapshot{}, nil, err
	}

	// Dangling-reference guard: every term reference must resolve somewhere
	// in the governed term space (any module/status); external IRIs stand
	// alone.
	known := make(map[string]bool, len(snap.Terms))
	for _, t := range snap.Terms {
		known[t.TermID] = true
	}
	refs := make([]string, 0, len(snap.Axioms)*3+len(snap.Mappings)*2)
	for _, a := range snap.Axioms {
		refs = append(refs, a.SubjectTermID)
		if a.PredicateTermID != "" {
			refs = append(refs, a.PredicateTermID)
		}
		if a.ObjectTermID != "" {
			refs = append(refs, a.ObjectTermID)
		}
	}
	for _, m := range snap.Mappings {
		refs = append(refs, m.FromTermID)
		if m.ToTermID != "" {
			refs = append(refs, m.ToTermID)
		}
	}
	if len(refs) > 0 || len(snap.ProfileRules) > 0 {
		known = unionTermIDs(ctx, db, known)
		for _, ref := range refs {
			if !known[ref] {
				return snapshot{}, nil, fmt.Errorf("dangling reference to term %q in module %q", ref, moduleID)
			}
		}
	}
	if err := validateProfileRuleReferences(snap.ProfileRules, known); err != nil {
		return snapshot{}, nil, err
	}

	pins, err := s.pinDependencies(ctx, db, module)
	if err != nil {
		return snapshot{}, nil, err
	}
	return snap, pins, nil
}

// validateProfileRuleReferences is the rule-content counterpart of the
// axiom/mapping dangling-reference guard above. A rule's config is opaque
// JSONB whose meaning is owned by its registered rule_kind (extension seam
// 6), so reference extraction is delegated to that kind's own
// ReferencedTermIDs (optional; a rule kind with no term references leaves it
// nil). A rule whose kind isn't registered at all cannot be evaluated once
// released, so that is rejected here too, not just a dangling term.
func validateProfileRuleReferences(rules []profiles.ProfileRule, known map[string]bool) error {
	for _, r := range rules {
		kind, ok := profiles.LookupRuleKind(r.RuleKind)
		if !ok {
			return fmt.Errorf("profile rule %q references unregistered rule_kind %q", r.RuleID, r.RuleKind)
		}
		if kind.ReferencedTermIDs == nil {
			continue
		}
		refs, err := kind.ReferencedTermIDs(r)
		if err != nil {
			return fmt.Errorf("profile rule %q: %w", r.RuleID, err)
		}
		for _, ref := range refs {
			if !known[ref] {
				return fmt.Errorf("dangling reference to term %q in profile rule %q", ref, r.RuleID)
			}
		}
	}
	return nil
}

// unionTermIDs loads the full governed term_id space so refs that point at
// dependency modules' terms (e.g. an axiom in measurement referencing a
// quantity term) are not flagged as dangling.
func unionTermIDs(ctx context.Context, db terms.DBX, seed map[string]bool) map[string]bool {
	rows, err := db.QueryContext(ctx, "SELECT DISTINCT term_id FROM kb.ontology_terms")
	if err != nil {
		return seed
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			seed[strings.TrimSpace(id)] = true
		}
	}
	return seed
}

// Checksum returns the deterministic content checksum of a snapshot payload.
func Checksum(payload []byte) (string, error) {
	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
