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
	Version   string
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

// DeferredModuleWarning reports a curated module that was intentionally not
// installed during service bootstrap because its external dependency is not
// ready yet. It is informational: callers should log it and continue startup.
type DeferredModuleWarning struct {
	ModuleID           string
	DependencyModuleID string
}

func (w DeferredModuleWarning) String() string {
	return fmt.Sprintf("deferred curated module %s: no active release for dependency %s", w.ModuleID, w.DependencyModuleID)
}

// EnsureCuratedModules authors, releases, and activates the service-critical
// curated modules. measurement is deferred nonfatally until quantity has an
// active release, which the QUDT import owns.
func EnsureCuratedModules(ctx context.Context, db *sql.DB) ([]DeferredModuleWarning, error) {
	return ensureCuratedModules(ctx, db, SeedCuratedModules)
}

func ensureCuratedModules(ctx context.Context, db *sql.DB, seedModules func(context.Context, *sql.DB, []string, bool) error) ([]DeferredModuleWarning, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	if err := seedModules(ctx, db, []string{"core", "document-authority"}, false); err != nil {
		return nil, err
	}
	if _, err := (modules.ReleaseStore{DB: db}).GetActiveRelease(ctx, "quantity"); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []DeferredModuleWarning{{ModuleID: "measurement", DependencyModuleID: "quantity"}}, nil
		}
		return nil, fmt.Errorf("get active release for quantity: %w", err)
	}
	if err := seedModules(ctx, db, []string{"measurement"}, false); err != nil {
		return nil, err
	}
	return nil, nil
}

// SeedCuratedModules installs selected curated modules. authorOnly is kept for
// the one-off CLI authoring workflow; production callers use
// EnsureCuratedModules.
func SeedCuratedModules(ctx context.Context, db *sql.DB, moduleIDs []string, authorOnly bool) error {
	if db == nil {
		return errors.New("db is nil")
	}
	for _, id := range moduleIDs {
		mc, ok := curatedModules[id]
		if !ok {
			return fmt.Errorf("unknown curated module %q", id)
		}
		if err := authorModule(ctx, db, mc); err != nil {
			return err
		}
		if !authorOnly {
			if err := releaseAndActivate(ctx, db, mc); err != nil {
				return err
			}
		}
	}
	return nil
}

func authorModule(ctx context.Context, db *sql.DB, mc moduleContent) error {
	ms := modules.ModuleStore{DB: db}
	if _, err := ms.GetModule(ctx, mc.ModuleID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("get module %s: %w", mc.ModuleID, err)
		}
		if _, err := ms.CreateModule(ctx, modules.Module{
			ModuleID: mc.ModuleID, Title: mc.Title, Owner: mc.Owner,
			DependsOn: mc.DependsOn, Status: "active", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
		}); err != nil {
			return fmt.Errorf("register module %s: %w", mc.ModuleID, err)
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
		} else if latest.TermKind != t.Kind || latest.ModuleID != mc.ModuleID || latest.Definition != t.Def {
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
	return "1.0.0+seed." + hex.EncodeToString(sum[:])[:12]
}

func seedReleaseStore(db *sql.DB) modules.ReleaseStore {
	// Seed updates may create a newer inactive release while an operator keeps
	// an older release active. Preserve that active release during supersession.
	return modules.ReleaseStore{DB: db, PreserveActive: true}
}

func releaseAndActivate(ctx context.Context, db *sql.DB, mc moduleContent) error {
	rs := seedReleaseStore(db)
	version := curatedReleaseVersion(mc)
	rel, err := rs.GetRelease(ctx, mc.ModuleID, version)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("get release %s@%s: %w", mc.ModuleID, version, err)
		}
		if err := stageContentForNewCuratedRelease(ctx, db, mc); err != nil {
			return err
		}
		rel, err = rs.CreateRelease(ctx, mc.ModuleID, version, "ontology-seed")
		if err != nil {
			return fmt.Errorf("release %s@%s: %w", mc.ModuleID, version, err)
		}
	}
	_, err = rs.GetActiveRelease(ctx, mc.ModuleID)
	if err == nil {
		// Startup must never replace an operator-selected active release.
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get active release for %s: %w", mc.ModuleID, err)
	}
	if _, err := rs.Activate(ctx, mc.ModuleID, rel.ID, "ontology-seed"); err != nil {
		return fmt.Errorf("activate %s@%s: %w", mc.ModuleID, version, err)
	}
	return nil
}
