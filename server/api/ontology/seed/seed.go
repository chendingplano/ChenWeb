// Package seed installs the curated, platform-owned ontology modules required
// by the document-processing and ontology API startup paths.
package seed

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

type moduleContent struct {
	ModuleID  string
	Version   string // base semantic version for derived curated releases
	Title     string
	Owner     string
	DependsOn []string
	Terms     []seedTerm
}

var curatedModules = map[string]moduleContent{
	"core":               coreModule,
	"document-authority": documentAuthorityModule,
	"measurement":        measurementModule,
}

// BootstrapWarning reports a nonfatal bootstrap condition. Curated content is
// still safe to consume when a dependency is deferred or an operator-selected
// release is preserved, but the condition must be visible to operators.
type BootstrapWarning struct {
	Kind               string
	ModuleID           string
	DependencyModuleID string
}

const (
	WarningDeferredDependency = "deferred_dependency"
	WarningPendingActivation  = "pending_activation"
)

func (w BootstrapWarning) String() string {
	if w.Kind == WarningPendingActivation {
		return fmt.Sprintf("curated module %s has a newer release pending activation", w.ModuleID)
	}
	return fmt.Sprintf("deferred curated module %s: no active release for dependency %s", w.ModuleID, w.DependencyModuleID)
}

// EnsureCuratedModules authors, releases, and activates the service-critical
// curated modules. measurement is deferred nonfatally until quantity has an
// active release, which the QUDT import owns.
func EnsureCuratedModules(ctx context.Context, db *sql.DB) ([]BootstrapWarning, error) {
	return ensureCuratedModules(ctx, db, SeedCuratedModules)
}

func ensureCuratedModules(ctx context.Context, db *sql.DB, seedModules func(context.Context, *sql.DB, []string, bool) ([]BootstrapWarning, error)) ([]BootstrapWarning, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	warnings, err := seedModules(ctx, db, []string{"core", "document-authority"}, false)
	if err != nil {
		return nil, err
	}
	if _, err := (modules.ReleaseStore{DB: db}).GetActiveRelease(ctx, "quantity"); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return append(warnings, BootstrapWarning{Kind: WarningDeferredDependency, ModuleID: "measurement", DependencyModuleID: "quantity"}), nil
		}
		return nil, fmt.Errorf("get active release for quantity: %w", err)
	}
	measurementWarnings, err := seedModules(ctx, db, []string{"measurement"}, false)
	if err != nil {
		return nil, err
	}
	return append(warnings, measurementWarnings...), nil
}

// SeedCuratedModules installs selected curated modules. authorOnly is kept for
// the one-off CLI authoring workflow; production callers use
// EnsureCuratedModules.
func SeedCuratedModules(ctx context.Context, db *sql.DB, moduleIDs []string, authorOnly bool) ([]BootstrapWarning, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	var warnings []BootstrapWarning
	for _, id := range moduleIDs {
		mc, ok := curatedModules[id]
		if !ok {
			return nil, fmt.Errorf("unknown curated module %q", id)
		}
		if err := authorModule(ctx, db, mc); err != nil {
			return nil, err
		}
		if !authorOnly {
			moduleWarnings, err := releaseAndActivate(ctx, db, mc)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, moduleWarnings...)
		}
	}
	return warnings, nil
}

func authorModule(ctx context.Context, db *sql.DB, mc moduleContent) error {
	ms := modules.ModuleStore{DB: db}
	module, err := ms.GetModule(ctx, mc.ModuleID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("get module %s: %w", mc.ModuleID, err)
		}
		if _, err := ms.CreateModule(ctx, modules.Module{
			ModuleID: mc.ModuleID, Title: mc.Title, Owner: mc.Owner,
			DependsOn: mc.DependsOn, Status: "active", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
		}); err != nil {
			return fmt.Errorf("register module %s: %w", mc.ModuleID, err)
		}
	} else if module.Title != mc.Title || module.Owner != mc.Owner || !sameStringSlice(module.DependsOn, mc.DependsOn) {
		if _, err := ms.UpdateModuleMetadata(ctx, mc.ModuleID, mc.Title, mc.Owner, mc.DependsOn, "ontology-seed"); err != nil {
			return fmt.Errorf("update module %s metadata: %w", mc.ModuleID, err)
		}
	}

	ts := terms.TermStore{DB: db}
	ls := terms.LabelStore{DB: db}
	for _, t := range mc.Terms {
		termExists := true
		latest, err := ts.GetTermLatest(ctx, t.ID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get term %s: %w", t.ID, err)
			}
			termExists = false
			if _, err := ts.CreateTerm(ctx, terms.Term{
				TermID: t.ID, TermKind: t.Kind, ModuleID: mc.ModuleID,
				Definition: t.Def, Status: "approved", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
			}); err != nil {
				return fmt.Errorf("author term %s: %w", t.ID, err)
			}
		} else if latest.TermKind != t.Kind || latest.ModuleID != mc.ModuleID || latest.Definition != t.Def || !curatedTermStatusUsable(latest.Status) {
			if _, err := ts.CreateTermVersion(ctx, terms.Term{
				TermID: t.ID, TermKind: t.Kind, ModuleID: mc.ModuleID,
				Definition: t.Def, Status: "approved", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
			}); err != nil {
				return fmt.Errorf("update curated term %s: %w", t.ID, err)
			}
		}
		labels, err := ls.ListLabels(ctx, t.ID)
		if err != nil {
			return fmt.Errorf("list labels for %s: %w", t.ID, err)
		}
		for _, l := range t.Labels {
			if hasLabel(labels, l) {
				continue
			}
			if l.Role == "prefLabel" && hasCurrentPrefLabel(labels, l.Lang) {
				if err := ls.SupersedePrefLabels(ctx, t.ID, l.Lang, "ontology-seed"); err != nil {
					return fmt.Errorf("supersede preferred label for %s: %w", t.ID, err)
				}
			}
			label := terms.TermLabel{
				TermID: t.ID, Label: l.Label, Lang: l.Lang, LabelRole: l.Role,
				Status: "approved", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
			}
			var createErr error
			if termExists {
				_, createErr = ls.CreateLabelVersion(ctx, label)
			} else {
				_, createErr = ls.CreateLabel(ctx, label)
			}
			if createErr != nil {
				return fmt.Errorf("author label %s (%s): %w", t.ID, l.Label, createErr)
			}
		}
	}
	return nil
}

// curatedTermStatusUsable reports whether the latest version of a curated term
// can stand as the current content without re-authoring: it is either already
// approved (staged for the next release) or included in a release. Any other
// status -- draft, in_review, superseded, rejected, auto-promoted -- is a
// governance state the seed must repair with a fresh approved version,
// mirroring how the label path re-authors superseded or rejected labels.
// Without this, a superseded curated term is never repaired, so the module
// cuts a new release on every start, or cannot be released at all when it is
// the module's last curated term.
func curatedTermStatusUsable(status string) bool {
	return status == "approved" || status == "included_in_release"
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasLabel(labels []terms.TermLabel, want seedLabel) bool {
	for _, label := range labels {
		if label.Label == want.Label && label.Lang == want.Lang && label.LabelRole == want.Role && label.Status != "rejected" && label.Status != "superseded" {
			return true
		}
	}
	return false
}

func hasCurrentPrefLabel(labels []terms.TermLabel, lang string) bool {
	for _, label := range labels {
		if label.Lang == lang && label.LabelRole == "prefLabel" && label.Status != "rejected" && label.Status != "superseded" {
			return true
		}
	}
	return false
}

// stageContentForNewCuratedRelease creates approved versions of content that
// a prior release marked included_in_release. CreateRelease snapshots only
// approved rows, so carrying that content forward is required before a new
// content-derived release can be constructed. An already-approved latest
// version is a prior successful staging attempt and must not be duplicated.
func stageContentForNewCuratedRelease(ctx context.Context, db *sql.DB, mc moduleContent) error {
	ts := terms.TermStore{DB: db}
	ls := terms.LabelStore{DB: db}
	for _, wantTerm := range mc.Terms {
		latest, err := ts.GetTermLatest(ctx, wantTerm.ID)
		if err != nil {
			return fmt.Errorf("get term %s for release staging: %w", wantTerm.ID, err)
		}
		if latest.Status == "included_in_release" {
			latest.Status = "approved"
			latest.CreateBy = "ontology-seed"
			latest.ModifyBy = "ontology-seed"
			if _, err := ts.CreateTermVersion(ctx, latest); err != nil {
				return fmt.Errorf("stage term %s for release: %w", wantTerm.ID, err)
			}
		}

		labels, err := ls.ListLabels(ctx, wantTerm.ID)
		if err != nil {
			return fmt.Errorf("list labels for release staging %s: %w", wantTerm.ID, err)
		}
		for _, wantLabel := range wantTerm.Labels {
			if hasApprovedLabel(labels, wantLabel) || !hasIncludedLabel(labels, wantLabel) {
				continue
			}
			if _, err := ls.CreateLabelVersion(ctx, terms.TermLabel{
				TermID: wantTerm.ID, Label: wantLabel.Label, Lang: wantLabel.Lang, LabelRole: wantLabel.Role,
				Status: "approved", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
			}); err != nil {
				return fmt.Errorf("stage label %s (%s) for release: %w", wantTerm.ID, wantLabel.Label, err)
			}
		}
	}
	return nil
}

func hasApprovedLabel(labels []terms.TermLabel, want seedLabel) bool {
	for _, label := range labels {
		if label.Label == want.Label && label.Lang == want.Lang && label.LabelRole == want.Role && label.Status == "approved" {
			return true
		}
	}
	return false
}

func hasIncludedLabel(labels []terms.TermLabel, want seedLabel) bool {
	for _, label := range labels {
		if label.Label == want.Label && label.Lang == want.Lang && label.LabelRole == want.Role && label.Status == "included_in_release" {
			return true
		}
	}
	return false
}

// curatedReleaseVersion derives a stable release version from the exact
// Go-defined module content. A content edit therefore cannot silently reuse
// an existing curated release version.
func curatedReleaseVersion(mc moduleContent) string {
	payload, err := json.Marshal(struct {
		ModuleID  string
		Title     string
		Owner     string
		DependsOn []string
		Terms     []seedTerm
	}{
		ModuleID: mc.ModuleID, Title: mc.Title, Owner: mc.Owner,
		DependsOn: mc.DependsOn, Terms: mc.Terms,
	})
	if err != nil {
		panic(fmt.Sprintf("marshal curated module %s: %v", mc.ModuleID, err))
	}
	sum := sha256.Sum256(payload)
	baseVersion := mc.Version
	if baseVersion == "" {
		baseVersion = "1.0.0"
	}
	return baseVersion + "+seed." + hex.EncodeToString(sum[:])[:12]
}

func nextCuratedReleaseVersion(base string, existing map[string]bool) string {
	for revision := 2; ; revision++ {
		candidate := fmt.Sprintf("%s.r%d", base, revision)
		if !existing[candidate] {
			return candidate
		}
	}
}

func seedReleaseStore(db *sql.DB) modules.ReleaseStore {
	// Seed updates may create a newer inactive release while an operator keeps
	// an older release active. Preserve that active release during supersession.
	return modules.ReleaseStore{DB: db, PreserveActive: true}
}

func releaseAndActivate(ctx context.Context, db *sql.DB, mc moduleContent) ([]BootstrapWarning, error) {
	rs := seedReleaseStore(db)
	version := curatedReleaseVersion(mc)
	rel, err := rs.GetRelease(ctx, mc.ModuleID, version)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get release %s@%s: %w", mc.ModuleID, version, err)
		}
		if err := stageContentForNewCuratedRelease(ctx, db, mc); err != nil {
			return nil, err
		}
		rel, err = rs.CreateRelease(ctx, mc.ModuleID, version, "ontology-seed")
		if err != nil {
			return nil, fmt.Errorf("release %s@%s: %w", mc.ModuleID, version, err)
		}
	} else {
		released, newest, err := curatedContentReleased(ctx, db, mc)
		if err != nil {
			return nil, err
		}
		if !released {
			existing := map[string]bool{version: true}
			for {
				version = nextCuratedReleaseVersion(curatedReleaseVersion(mc), existing)
				candidate, lookupErr := rs.GetRelease(ctx, mc.ModuleID, version)
				if errors.Is(lookupErr, sql.ErrNoRows) {
					break
				}
				if lookupErr != nil {
					return nil, fmt.Errorf("get release %s@%s: %w", mc.ModuleID, version, lookupErr)
				}
				existing[candidate.Version] = true
			}
			if err := stageContentForNewCuratedRelease(ctx, db, mc); err != nil {
				return nil, err
			}
			rel, err = rs.CreateRelease(ctx, mc.ModuleID, version, "ontology-seed")
			if err != nil {
				return nil, fmt.Errorf("release %s@%s: %w", mc.ModuleID, version, err)
			}
		} else {
			// The release at the derived version exists, but a later .rN
			// descendant may actually carry the current content (the revert
			// flow in the branch above). Activation must target the newest
			// release, never the stale derived-version release.
			rel = newest
		}
	}
	active, err := rs.GetActiveRelease(ctx, mc.ModuleID)
	if err == nil {
		if active.ReleaseID == rel.ID {
			return nil, nil
		}
		// Advance activation when the current pointer was set by the seed
		// itself, so consumers that read the activation pointer (profile
		// loaders, classify_document's governed vocabulary) serve the current
		// curated content. Operator-selected activations (any other actor) are
		// preserved and surfaced as pending.
		if active.ActivatedBy == "ontology-seed" {
			if _, err := rs.Activate(ctx, mc.ModuleID, rel.ID, "ontology-seed"); err != nil {
				return nil, fmt.Errorf("activate %s@%s: %w", mc.ModuleID, rel.Version, err)
			}
			return nil, nil
		}
		return []BootstrapWarning{{Kind: WarningPendingActivation, ModuleID: mc.ModuleID}}, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get active release for %s: %w", mc.ModuleID, err)
	}
	if _, err := rs.Activate(ctx, mc.ModuleID, rel.ID, "ontology-seed"); err != nil {
		return nil, fmt.Errorf("activate %s@%s: %w", mc.ModuleID, rel.Version, err)
	}
	return nil, nil
}

func curatedContentReleased(ctx context.Context, db *sql.DB, mc moduleContent) (bool, modules.Release, error) {
	ts := terms.TermStore{DB: db}
	ls := terms.LabelStore{DB: db}
	for _, wantTerm := range mc.Terms {
		latest, err := ts.GetTermLatest(ctx, wantTerm.ID)
		if err != nil {
			return false, modules.Release{}, fmt.Errorf("get latest curated term %s: %w", wantTerm.ID, err)
		}
		if latest.Status != "included_in_release" {
			return false, modules.Release{}, nil
		}
		labels, err := ls.ListLabels(ctx, wantTerm.ID)
		if err != nil {
			return false, modules.Release{}, fmt.Errorf("list latest curated labels for %s: %w", wantTerm.ID, err)
		}
		for _, wantLabel := range wantTerm.Labels {
			if !hasIncludedLabel(labels, wantLabel) {
				return false, modules.Release{}, nil
			}
		}
	}
	// The module's newest release must pin exactly the curated dependency set.
	// A metadata-only revert (Title, Owner, or DependsOn) resolves the derived
	// version back to an older release while a newer release with stale pins
	// stays the newest; without this check that revert is silently not
	// re-released and the newest release's dependency pins stay stale.
	rs := seedReleaseStore(db)
	newest, err := rs.LatestRelease(ctx, mc.ModuleID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, modules.Release{}, nil
		}
		return false, modules.Release{}, fmt.Errorf("get latest release for %s: %w", mc.ModuleID, err)
	}
	match, err := dependencyPinsMatch(newest.DependencyReleases, mc.DependsOn)
	if err != nil {
		return false, modules.Release{}, err
	}
	if !match {
		return false, modules.Release{}, nil
	}
	return true, newest, nil
}

// dependencyPinsMatch reports whether a release's dependency pins reference
// exactly the curated dependency module set. Pin versions are ignored; only
// the module set matters for re-release decisions.
func dependencyPinsMatch(raw json.RawMessage, want []string) (bool, error) {
	var pins map[string]string
	if err := json.Unmarshal(raw, &pins); err != nil {
		return false, fmt.Errorf("parse dependency pins: %w", err)
	}
	if len(pins) != len(want) {
		return false, nil
	}
	for _, dep := range want {
		if _, ok := pins[dep]; !ok {
			return false, nil
		}
	}
	return true, nil
}
