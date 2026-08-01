// Package docprocessing: normalize_assertions is the first of the three DR8
// Phase D stages (spec §10.1). It runs after Phase C, dispatches to every
// registered assertions.Normalizer (DR11 seam 5), and only ever writes
// kb.semantic_decision_candidates rows -- never kb.semantic_assertions
// directly. Gated by SEMANTIC_ASSOCIATION_ENABLED (default false, matching
// the ADR config table) so Phase D stays inert until explicitly turned on.
package docprocessing

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// SemanticAssociationEnabledFromEnv resolves the SEMANTIC_ASSOCIATION_ENABLED
// setting. Unset (or any value that does not parse as a boolean true)
// resolves to disabled, matching the ADR's stated default of 'false'.
func SemanticAssociationEnabledFromEnv() bool {
	raw := strings.TrimSpace(os.Getenv("SEMANTIC_ASSOCIATION_ENABLED"))
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	return err == nil && enabled
}

// runNormalizeAssertions executes Phase D stage 1 for one input record: for
// every registered artifact-family normalizer, read that family's artifacts
// for this record and propose candidate assertions. Errors from one
// normalizer are logged and do not block sibling families, mirroring Phase
// C's runPostProcessIndexing tolerance.
func (s *ControlService) runNormalizeAssertions(ctx context.Context, recordID int64) {
	if !SemanticAssociationEnabledFromEnv() {
		return
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return
	}

	for _, family := range assertions.RegisteredFamilies() {
		normalizer, ok := assertions.LookupNormalizer(family)
		if !ok {
			continue
		}
		report, err := normalizer.Normalize(ctx, db, recordID)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Error("normalize_assertions family failed",
					"record_id", recordID, "family", family, "error", err)
			}
			continue
		}
		if s.Logger != nil && report.Examined > 0 {
			s.Logger.Info("normalize_assertions family complete",
				"record_id", recordID, "family", family,
				"examined", report.Examined, "proposed", report.Proposed,
				"reused", report.Reused, "skipped", report.Skipped)
		}
	}
}
