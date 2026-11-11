package kbhandler

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

func intp(v int) *int { return &v }

// The trace of a target 2nd in BM25 (1st in its partition) and 1st in semantic search.
func engineTrace(fusedFirst string) *searchengine.Response {
	return &searchengine.Response{
		Keywords: []string{"battery"},
		Trace: &searchengine.Trace{
			LexicalPartitions: []searchengine.TraceHit{
				{ArtifactType: "metric", ArtifactID: "9_mtc_2", Score: 12.5},
				{ArtifactType: "metric", ArtifactID: "9_mtc_3", Score: 4},
			},
			Lexical: []searchengine.TraceHit{
				{ArtifactType: "metric", ArtifactID: "9_mtc_1", Score: 13},
				{ArtifactType: "metric", ArtifactID: "9_mtc_2", Score: 12.5},
			},
			Vector: []searchengine.TraceHit{{ArtifactType: "metric", ArtifactID: "9_mtc_1", Score: 0.91}},
			Fused: []searchengine.TraceFused{
				{ArtifactType: "metric", ArtifactID: fusedFirst, Score: 0.032, LexicalRank: intp(2), VectorRank: intp(1)},
			},
		},
	}
}

func TestSearchTraceSinkAddBuildsStages(t *testing.T) {
	sink := &searchTraceSink{}
	results := []artifactSearchResult{{ArtifactType: "metric", ArtifactID: "9_mtc_1"}}
	sink.add("battery", engineTrace("9_mtc_1"), results, 0)
	if len(sink.entries) != 1 {
		t.Fatalf("entries=%#v", sink.entries)
	}
	e := sink.entries[0]
	if e.Query != "battery" || e.Error != "" || len(e.Keywords) != 1 || len(e.Candidates) != 6 {
		t.Fatalf("entry=%#v", e)
	}
	byStage := map[string][]searchTraceCandidate{}
	for _, c := range e.Candidates {
		byStage[c.Stage] = append(byStage[c.Stage], c)
	}
	// The partition list is ranked by score within the partition.
	part := byStage[searchTraceStageLexicalPartition]
	if len(part) != 2 || part[0].ArtifactID != "9_mtc_2" || part[0].Rank != 1 || part[1].Rank != 2 || part[0].PartitionName != "kb.search_artifacts_metric" {
		t.Fatalf("partition rows=%#v", part)
	}
	// A merged lexical row carries its rank within the partition; a row not in a partition list has none.
	lex := byStage[searchTraceStageLexical]
	if lex[1].ArtifactID != "9_mtc_2" || lex[1].Rank != 2 || lex[1].PartitionRank == nil || *lex[1].PartitionRank != 1 || lex[0].PartitionRank != nil {
		t.Fatalf("lexical rows=%#v", lex)
	}
	if sem := byStage[searchTraceStageSemantic]; len(sem) != 1 || sem[0].Score != 0.91 || sem[0].Rank != 1 {
		t.Fatalf("semantic rows=%#v", sem)
	}
	fused := byStage[searchTraceStageFused][0]
	if fused.Rank != 1 || fused.LexicalRank == nil || *fused.LexicalRank != 2 || fused.SemanticRank == nil || *fused.SemanticRank != 1 || fused.PartitionRank != nil {
		t.Fatalf("fused=%#v", fused)
	}
}

func TestSearchTraceSinkFlagsMismatchWithResults(t *testing.T) {
	sink := &searchTraceSink{}
	sink.add("battery", engineTrace("9_mtc_9"), []artifactSearchResult{{ArtifactType: "metric", ArtifactID: "9_mtc_1"}}, 0)
	if sink.entries[0].Error != searchTraceErrMismatch {
		t.Fatalf("entry=%#v", sink.entries[0])
	}
}

func TestSearchTraceSinkWithoutPartitionsHasNoPartitionStage(t *testing.T) {
	resp := engineTrace("9_mtc_1")
	resp.Trace.LexicalPartitions = nil // the Postgres backend has no per-partition lists
	sink := &searchTraceSink{}
	sink.add("battery", resp, []artifactSearchResult{{ArtifactType: "metric", ArtifactID: "9_mtc_1"}}, 0)
	for _, c := range sink.entries[0].Candidates {
		if c.Stage == searchTraceStageLexicalPartition || c.PartitionRank != nil {
			t.Fatalf("unexpected partition data: %#v", c)
		}
	}
}

func TestTraceMatchesResultsUsesOffset(t *testing.T) {
	candidates := []searchTraceCandidate{
		{Stage: searchTraceStageFused, ArtifactType: "metric", ArtifactID: "a", Rank: 1},
		{Stage: searchTraceStageFused, ArtifactType: "metric", ArtifactID: "b", Rank: 2},
		{Stage: searchTraceStageLexical, ArtifactType: "metric", ArtifactID: "b", Rank: 1},
	}
	second := []artifactSearchResult{{ArtifactType: "metric", ArtifactID: "b"}}
	if !traceMatchesResults(candidates, second, 1) || traceMatchesResults(candidates, second, 0) {
		t.Fatal("offset not applied")
	}
}
