package docreviews

import (
	"context"
	"database/sql"
	"fmt"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

// findSimilarMatches asks the hybrid search engine for artifacts of one type similar to one of the
// record's own. The record's own artifacts are excluded before ranking, so they cannot take the
// slots of other documents. A missing index row is a readiness error, not a completed empty search.
func findSimilarMatches(ctx context.Context, db *sql.DB, artifactType, artifactID string, recordID int64, maxMatches int) ([]docprocessing.OnTheFlySemanticMatch, error) {
	if db == nil {
		return nil, fmt.Errorf("review similarity search: nil db handle")
	}
	if maxMatches <= 0 {
		maxMatches = searchengine.ConnectMaxLinks()
	}
	response, err := searchengine.Search(ctx, db, searchengine.LoadConfig(), searchengine.Request{
		Query:    searchengine.Query{Ref: &searchengine.ArtifactRef{Type: artifactType, ID: artifactID}},
		Filters:  searchengine.Filters{ArtifactTypes: []string{artifactType}, ExcludeRecordIDs: []int64{recordID}},
		Criteria: searchengine.Criteria{MinCosine: searchengine.ConnectMinCosine()},
		Options:  searchengine.Options{Limit: maxMatches},
	})
	if err != nil {
		return nil, err
	}
	hits := make([]docprocessing.OnTheFlySemanticMatch, 0, len(response.Results))
	for _, res := range response.Results {
		hits = append(hits, docprocessing.OnTheFlySemanticMatch{
			ArtifactType: res.ArtifactType, ArtifactID: res.ArtifactID, RecordID: res.InputRecordID, RRFScore: res.Score,
		})
	}
	return hits, nil
}
