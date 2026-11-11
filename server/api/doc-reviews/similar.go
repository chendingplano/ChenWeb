package docreviews

import (
	"context"
	"database/sql"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

// findSimilarMatches asks the hybrid search engine for artifacts of one type similar to one of the
// record's own. The record's own artifacts are excluded before ranking, so they cannot take the
// slots of other documents. An artifact with no index row has no matches.
func findSimilarMatches(ctx context.Context, db *sql.DB, artifactType, artifactID string, recordID int64, maxMatches int) ([]docprocessing.OnTheFlySemanticMatch, error) {
	results, err := searchengine.Similar(ctx, db, searchengine.ArtifactRef{Type: artifactType, ID: artifactID}, searchengine.SimilarOptions{
		Filters: searchengine.Filters{
			ArtifactTypes:    []string{artifactType},
			ExcludeRecordIDs: []int64{recordID},
		},
		Limit: maxMatches,
	})
	if err != nil {
		return nil, err
	}
	hits := make([]docprocessing.OnTheFlySemanticMatch, 0, len(results))
	for _, res := range results {
		hits = append(hits, docprocessing.OnTheFlySemanticMatch{
			ArtifactType: res.ArtifactType, ArtifactID: res.ArtifactID, RecordID: res.InputRecordID, RRFScore: res.Score,
		})
	}
	return hits, nil
}
