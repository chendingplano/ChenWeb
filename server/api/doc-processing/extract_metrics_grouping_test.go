package docprocessing

import "testing"

func groupingTestCandidate(id string, chunk int, quote string, spans ...string) metricCandidate {
	return metricCandidate{
		CandidateID: id,
		ChunkIndex:  chunk,
		SupportingMentions: []map[string]any{{
			"evidence_quote":    quote,
			"source_line_spans": spans,
		}},
	}
}

// Record 371 run 221: candidates 8_40 (计时 0~100 s), 8_41 (分度值 0.1 s) and 8_42 (允许误差 ±0.1 s)
// quote the same Table 1 row. A size-10 cut between 8_40 and 8_41 sent them to two enrich calls,
// and the call holding only 8_40 re-emitted the row's other cells, duplicating 8_41/8_42.
func TestGroupCandidatesByChunk_KeepsSameEvidenceSiblingsTogether(t *testing.T) {
	var cands []metricCandidate
	for i := 1; i <= 9; i++ {
		cands = append(cands, groupingTestCandidate(string(rune('a'+i)), 8, "other row", "119"))
		cands[len(cands)-1].SupportingMentions[0]["evidence_quote"] = "row " + string(rune('a'+i))
	}
	row := "计时:0 s~100 s | 0.1 s | ±0.1 s"
	cands = append(cands,
		groupingTestCandidate("8_40", 8, row, "119"),
		groupingTestCandidate("8_41", 8, row, "119"),
		groupingTestCandidate("8_42", 8, row, "119"),
		groupingTestCandidate("8_43", 8, "next row", "119"),
	)

	batches := groupCandidatesByChunk(cands, 10)
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want 2", len(batches))
	}
	if got := len(batches[0].candidates); got != 12 {
		t.Fatalf("first batch size = %d, want 12 (row siblings 8_40..8_42 kept together)", got)
	}
	if got := batches[1].candidates[0].CandidateID; got != "8_43" {
		t.Fatalf("second batch starts with %s, want 8_43", got)
	}
}

func TestGroupCandidatesByChunk_EmptyEvidenceDoesNotChain(t *testing.T) {
	var cands []metricCandidate
	for i := 0; i < 4; i++ {
		cands = append(cands, groupingTestCandidate("c", 1, "", "5"))
	}
	batches := groupCandidatesByChunk(cands, 2)
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want 2 (no evidence quote → plain size cut)", len(batches))
	}
}
