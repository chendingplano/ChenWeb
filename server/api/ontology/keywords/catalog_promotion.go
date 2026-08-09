package keywords

import (
	"context"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// PromotionCounts reports what one PromoteCatalogEntries pass did.
type PromotionCounts struct {
	EntriesScanned    int
	ConceptsCreated   int
	ConceptsConverged int
	Errors            int
}

// catalogLabel is one row read back from kb.keyword_catalog_labels.
type catalogLabel struct {
	Language  string
	LabelRole string
	Label     string
}

// PromoteCatalogEntries auto-promotes every non-deprecated staged catalog
// entry of (source, release) into a flagged, provisional kb.keyword_concepts
// row, without requiring any human action (keyword-catalog-auto-promotion
// openspec change). It mirrors KeywordFamily.autoCreateConcept's
// create-or-converge pattern: the concept id is a content hash of the
// entry's preferred label (normalized) and scope, so promoting the same
// entry more than once (e.g. a replayed Approve) converges on the same
// concept instead of duplicating it.
func PromoteCatalogEntries(ctx context.Context, db DBX, source, release, scope string) (PromotionCounts, error) {
	var counts PromotionCounts
	normalizer := semid.Normalizer{Version: semid.CurrentNormalizerVersion}
	glossSource := "auto:import:" + source
	conceptStore := ConceptStore{DB: db}
	surfaceStore := SurfaceStore{DB: db}

	rows, err := db.QueryContext(ctx, `
SELECT external_id FROM kb.keyword_catalog_entries
WHERE source = $1 AND release = $2 AND entry_status <> 'deprecated'`,
		source, release)
	if err != nil {
		return counts, fmt.Errorf("list catalog entries: %w", err)
	}
	externalIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return counts, fmt.Errorf("scan catalog entry: %w", err)
		}
		externalIDs = append(externalIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return counts, fmt.Errorf("iterate catalog entries: %w", err)
	}
	rows.Close()

	for _, externalID := range externalIDs {
		counts.EntriesScanned++
		label, err := preferredCatalogLabel(ctx, db, source, release, externalID)
		if err != nil {
			counts.Errors++
			continue
		}
		if label == "" {
			counts.Errors++
			continue
		}
		converged, err := promoteOneEntry(ctx, conceptStore, surfaceStore, normalizer, label, scope, glossSource)
		if err != nil {
			counts.Errors++
			continue
		}
		if converged {
			counts.ConceptsConverged++
		} else {
			counts.ConceptsCreated++
		}
	}
	return counts, nil
}

// promoteOneEntry creates (or converges on) one concept for label, following
// the same collision-handling KeywordFamily.autoCreateConcept uses: a
// concurrent or repeated promotion of the same normalized label converges on
// the existing concept instead of erroring. Returns true when it converged
// on an existing concept rather than creating a new one.
func promoteOneEntry(ctx context.Context, concepts ConceptStore, surfaces SurfaceStore, normalizer semid.Normalizer, label, scope, glossSource string) (bool, error) {
	ks := normalizer.Normalize(label)
	conceptID := autoConceptID(ks.Norm, scope)

	created, err := concepts.CreateConcept(ctx, Concept{
		ConceptID:   conceptID,
		PrefLabel:   label,
		Scope:       scope,
		Status:      "provisional",
		GlossSource: glossSource,
	})
	converged := false
	if err != nil {
		existing, gerr := concepts.GetConcept(ctx, conceptID)
		if gerr != nil || existing.Scope != scope {
			return false, fmt.Errorf("create or converge concept %s: %w", conceptID, err)
		}
		switch existing.Status {
		case "active", "provisional":
			created = existing
			converged = true
		case "merged":
			survivorID, ferr := concepts.FollowMerge(ctx, existing.ConceptID)
			if ferr != nil {
				return false, fmt.Errorf("follow merge for %s: %w", existing.ConceptID, ferr)
			}
			created = Concept{ConceptID: survivorID}
			converged = true
		default:
			return false, fmt.Errorf("concept %s has unusable status %q", conceptID, existing.Status)
		}
	}

	if _, err := surfaces.CreateSurface(ctx, Surface{
		ConceptID:  created.ConceptID,
		Surface:    label,
		LabelRole:  "pref",
		AliasType:  "pref",
		Provenance: glossSource,
		Scope:      scope,
		Confidence: 1.0,
	}); err != nil {
		return converged, fmt.Errorf("create pref surface for %s: %w", created.ConceptID, err)
	}
	return converged, nil
}

// preferredCatalogLabel picks the English preferred label for one catalog
// entry, falling back to any preferred label and then the first available
// label -- the same fallback chain used elsewhere in this codebase for QUDT
// term import (terminology.pickQUDTLabel).
func preferredCatalogLabel(ctx context.Context, db DBX, source, release, externalID string) (string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT language, label_role, label FROM kb.keyword_catalog_labels
WHERE source = $1 AND release = $2 AND external_id = $3`,
		source, release, externalID)
	if err != nil {
		return "", fmt.Errorf("list catalog labels: %w", err)
	}
	defer rows.Close()

	labels := make([]catalogLabel, 0)
	for rows.Next() {
		var l catalogLabel
		if err := rows.Scan(&l.Language, &l.LabelRole, &l.Label); err != nil {
			return "", fmt.Errorf("scan catalog label: %w", err)
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate catalog labels: %w", err)
	}

	for _, l := range labels {
		if l.LabelRole == "preferred" && l.Language == "en" {
			return l.Label, nil
		}
	}
	for _, l := range labels {
		if l.LabelRole == "preferred" {
			return l.Label, nil
		}
	}
	if len(labels) > 0 {
		return labels[0].Label, nil
	}
	return "", nil
}
