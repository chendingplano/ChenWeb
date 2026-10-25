package kbhandler

import (
	"database/sql"
	"time"
)

// Hybrid search traces (openspec hybrid-search-benchmark-trace): the candidate lists a hybrid query
// builds, read from the same CTEs in the same snapshot as the search. Only benchmark runs set a
// sink; live search never runs the trace statement.
const (
	searchTraceStageLexicalPartition = "lexical_partition" // each partition's BM25 list (ParadeDB)
	searchTraceStageLexical          = "lexical"           // the merged lexical list RRF used
	searchTraceStageSemantic         = "semantic"
	searchTraceStageFused            = "fused"
	searchTraceErrMismatch           = "trace differs from the search results"
)

// searchTraceSink collects one entry per traced hybrid search, in search order. Each benchmark
// question has its own sinks; a sink is not safe for concurrent searches.
type searchTraceSink struct {
	entries []searchTrace
}

// traceMs is the time of all trace statements so far, which benchmark timings exclude.
func (s *searchTraceSink) traceMs() int {
	ms := 0
	for _, e := range s.entries {
		ms += e.Ms
	}
	return ms
}

// searchTrace is one hybrid search's candidate lists.
type searchTrace struct {
	Query      string
	Keywords   []string // the BM25 keywords this query used; empty when off or failed
	Candidates []searchTraceCandidate
	Ms         int
	Error      string // the trace statement failed, or its fused order differs from the results
}

type searchTraceCandidate struct {
	Stage         string
	PartitionName string
	ArtifactType  string
	ArtifactID    string
	Rank          int // within the stage (within the partition for lexical_partition)
	Score         float64
	PartitionRank *int // lexical rows: rank within the partition (ParadeDB only)
	LexicalRank   *int // fused rows
	SemanticRank  *int // fused rows
}

// hybridTraceQuery is the trace statement of one hybrid search: its CTEs plus hybridTraceSelect,
// with the search's arguments minus page size and offset.
type hybridTraceQuery struct {
	sink     *searchTraceSink
	sql      string
	args     []any
	offset   int
	query    string
	keywords []string
}

// hybridTraceSelect follows a hybrid query's CTEs and returns one row per candidate per stage. With
// partitions (ParadeDB) it ranks lexical_partitions within each partition and gives each merged
// lexical row its partition rank; tsvector has no per-partition lists.
func hybridTraceSelect(partitions bool) string {
	if !partitions {
		return `
SELECT 'lexical' AS stage, l.artifact_type, l.artifact_id, l.rnk AS stage_rank, l.score::float8 AS score,
	NULL::bigint AS partition_rank, NULL::bigint AS lexical_rank, NULL::bigint AS semantic_rank
FROM lexical l` + hybridTraceSemanticFused
	}
	return `,
lexical_partition_ranks AS (
	SELECT lp.artifact_type, lp.artifact_id, lp.score,
		ROW_NUMBER() OVER (PARTITION BY lp.artifact_type ORDER BY lp.score DESC, lp.artifact_id ASC) AS rnk
	FROM lexical_partitions lp
)
SELECT 'lexical_partition' AS stage, p.artifact_type, p.artifact_id, p.rnk AS stage_rank, p.score::float8 AS score,
	NULL::bigint AS partition_rank, NULL::bigint AS lexical_rank, NULL::bigint AS semantic_rank
FROM lexical_partition_ranks p
UNION ALL
SELECT 'lexical', l.artifact_type, l.artifact_id, l.rnk, l.score::float8, p.rnk, NULL, NULL
FROM lexical l
JOIN lexical_partition_ranks p ON p.artifact_type = l.artifact_type AND p.artifact_id = l.artifact_id` + hybridTraceSemanticFused
}

// The fused rank uses the production final ORDER BY (score, then artifact ID).
const hybridTraceSemanticFused = `
UNION ALL
SELECT 'semantic', s.artifact_type, s.artifact_id, s.rnk, (1 - s.distance)::float8, NULL, NULL, NULL
FROM semantic s
UNION ALL
SELECT 'fused', f.artifact_type, f.artifact_id, ROW_NUMBER() OVER (ORDER BY f.score DESC, f.artifact_id ASC), f.score, NULL, f.lexical_rnk, f.semantic_rnk
FROM fused f`

// run executes the trace statement in the search's transaction and appends its entry. It returns
// false when the statement failed, which leaves the transaction unusable.
func (q *hybridTraceQuery) run(tx *sql.Tx, results []artifactSearchResult) bool {
	entry := searchTrace{Query: q.query, Keywords: q.keywords, Candidates: []searchTraceCandidate{}}
	if entry.Keywords == nil {
		entry.Keywords = []string{}
	}
	start := time.Now()
	candidates, err := scanSearchTrace(tx, q.sql, q.args)
	entry.Ms = int(time.Since(start).Milliseconds())
	ok := err == nil
	switch {
	case !ok:
		entry.Error = "trace failed: " + err.Error()
	case !traceMatchesResults(candidates, results, q.offset):
		entry.Candidates, entry.Error = candidates, searchTraceErrMismatch
	default:
		entry.Candidates = candidates
	}
	q.sink.entries = append(q.sink.entries, entry)
	return ok
}

func scanSearchTrace(tx *sql.Tx, sqlText string, args []any) ([]searchTraceCandidate, error) {
	rows, err := tx.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := []searchTraceCandidate{}
	for rows.Next() {
		var c searchTraceCandidate
		var partitionRank, lexicalRank, semanticRank sql.NullInt64
		if err := rows.Scan(&c.Stage, &c.ArtifactType, &c.ArtifactID, &c.Rank, &c.Score, &partitionRank, &lexicalRank, &semanticRank); err != nil {
			return nil, err
		}
		c.PartitionName = searchArtifactPartition(c.ArtifactType)
		c.PartitionRank, c.LexicalRank, c.SemanticRank = nullRank(partitionRank), nullRank(lexicalRank), nullRank(semanticRank)
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// traceMatchesResults reports whether the results are the fused list from rank offset+1 on.
func traceMatchesResults(candidates []searchTraceCandidate, results []artifactSearchResult, offset int) bool {
	fused := map[int]string{}
	for _, c := range candidates {
		if c.Stage == searchTraceStageFused {
			fused[c.Rank] = c.ArtifactType + "/" + c.ArtifactID
		}
	}
	for i, r := range results {
		if fused[offset+i+1] != searchEffortKey(r) {
			return false
		}
	}
	return true
}
