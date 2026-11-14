package searchengine

import (
	"context"
	"database/sql"
)

// Load returns the display fields of the given artifacts in the order given, as Search fills them
// for its results, but without ranking or diagnostics (Rank and Score are 0, Diag is empty). A
// reference with no kb.search_artifacts row is left out, so callers compare lengths when every
// reference must exist. It is for callers that already know which artifacts they want, such as an
// evaluation that judges passages another run reported.
func Load(ctx context.Context, db *sql.DB, cfg Config, refs []ArtifactRef) ([]Result, error) {
	kept := make([]*fusedCand, 0, len(refs))
	for _, r := range refs {
		kept = append(kept, &fusedCand{candKey: candKey{r.Type, r.ID}})
	}
	results, err := loadResults(ctx, db, cfg, "", kept, 0)
	if err != nil {
		return nil, err
	}
	for i := range results {
		results[i].Rank = 0
	}
	return results, nil
}
