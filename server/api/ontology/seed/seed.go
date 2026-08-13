// Package seed installs the curated, platform-owned ontology modules required
// by the document-processing and ontology API startup paths.
package seed

import (
	"context"
	"database/sql"
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

var allModuleIDs = []string{"core", "document-authority", "measurement"}

// EnsureCuratedModules authors, releases, and activates every curated
// platform module. It is safe to call on every service start.
func EnsureCuratedModules(ctx context.Context, db *sql.DB) error {
	return SeedCuratedModules(ctx, db, allModuleIDs, false)
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
		if _, err := ts.GetTermLatest(ctx, t.ID); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get term %s: %w", t.ID, err)
			}
			if _, err := ts.CreateTerm(ctx, terms.Term{
				TermID: t.ID, TermKind: t.Kind, ModuleID: mc.ModuleID,
				Definition: t.Def, Status: "approved", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
			}); err != nil {
				return fmt.Errorf("author term %s: %w", t.ID, err)
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
			if _, err := ls.CreateLabel(ctx, terms.TermLabel{
				TermID: t.ID, Label: l.Label, Lang: l.Lang, LabelRole: l.Role,
				Status: "approved", CreateBy: "ontology-seed", ModifyBy: "ontology-seed",
			}); err != nil {
				return fmt.Errorf("author label %s (%s): %w", t.ID, l.Label, err)
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

func releaseAndActivate(ctx context.Context, db *sql.DB, mc moduleContent) error {
	rs := modules.ReleaseStore{DB: db}
	rel, err := rs.GetRelease(ctx, mc.ModuleID, mc.Version)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("get release %s@%s: %w", mc.ModuleID, mc.Version, err)
		}
		rel, err = rs.CreateRelease(ctx, mc.ModuleID, mc.Version, "ontology-seed")
		if err != nil {
			return fmt.Errorf("release %s@%s: %w", mc.ModuleID, mc.Version, err)
		}
	}
	active, err := rs.GetActiveRelease(ctx, mc.ModuleID)
	if err == nil && active.ReleaseID == rel.ID {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get active release for %s: %w", mc.ModuleID, err)
	}
	if _, err := rs.Activate(ctx, mc.ModuleID, rel.ID, "ontology-seed"); err != nil {
		return fmt.Errorf("activate %s@%s: %w", mc.ModuleID, mc.Version, err)
	}
	return nil
}
