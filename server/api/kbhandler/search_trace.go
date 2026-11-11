package kbhandler

import (
	"sort"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

// Hybrid search traces (openspec hybrid-search-benchmark-trace): the candidate lists a hybrid search
// builds, taken from the engine's own lists. Only benchmark runs set a
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

// add records one search's candidate lists from the engine's trace: each partition's own list (ParadeDB
// only), the merged keyword list the fusion used, the vector list and the fused list. The fused order is
// checked against the returned page.
func (s *searchTraceSink) add(query string, resp *searchengine.Response, results []artifactSearchResult, offset int) {
	entry := searchTrace{Query: query, Keywords: resp.Keywords, Candidates: searchTraceCandidates(resp.Trace)}
	if entry.Keywords == nil {
		entry.Keywords = []string{}
	}
	if !traceMatchesResults(entry.Candidates, results, offset) {
		entry.Error = searchTraceErrMismatch
	}
	s.entries = append(s.entries, entry)
}

func searchTraceCandidates(t *searchengine.Trace) []searchTraceCandidate {
	out := []searchTraceCandidate{}
	// Rank within each partition: score descending, then artifact ID, as the partition's own list.
	byPartition := map[string][]searchengine.TraceHit{}
	for _, h := range t.LexicalPartitions {
		byPartition[h.ArtifactType] = append(byPartition[h.ArtifactType], h)
	}
	partitionRank := map[string]int{}
	for typ, hits := range byPartition {
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].Score != hits[j].Score {
				return hits[i].Score > hits[j].Score
			}
			return hits[i].ArtifactID < hits[j].ArtifactID
		})
		for i, h := range hits {
			rank := i + 1
			partitionRank[typ+"/"+h.ArtifactID] = rank
			out = append(out, searchTraceCandidate{
				Stage: searchTraceStageLexicalPartition, PartitionName: searchArtifactPartition(typ),
				ArtifactType: typ, ArtifactID: h.ArtifactID, Rank: rank, Score: h.Score,
			})
		}
	}
	for i, h := range t.Lexical {
		c := searchTraceCandidate{
			Stage: searchTraceStageLexical, PartitionName: searchArtifactPartition(h.ArtifactType),
			ArtifactType: h.ArtifactType, ArtifactID: h.ArtifactID, Rank: i + 1, Score: h.Score,
		}
		if r, ok := partitionRank[h.ArtifactType+"/"+h.ArtifactID]; ok {
			c.PartitionRank = &r
		}
		out = append(out, c)
	}
	for i, h := range t.Vector {
		out = append(out, searchTraceCandidate{
			Stage: searchTraceStageSemantic, PartitionName: searchArtifactPartition(h.ArtifactType),
			ArtifactType: h.ArtifactType, ArtifactID: h.ArtifactID, Rank: i + 1, Score: h.Score,
		})
	}
	for i, f := range t.Fused {
		out = append(out, searchTraceCandidate{
			Stage: searchTraceStageFused, PartitionName: searchArtifactPartition(f.ArtifactType),
			ArtifactType: f.ArtifactType, ArtifactID: f.ArtifactID, Rank: i + 1, Score: f.Score,
			LexicalRank: f.LexicalRank, SemanticRank: f.VectorRank,
		})
	}
	return out
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
