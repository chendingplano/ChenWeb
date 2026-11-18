package docprocessing

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/attention"
)

func TestMetricReferenceAttentionReport(t *testing.T) {
	m := RefMention{InputRecordID: 377, RefID: "377_ref_177", MentionKey: "mk-1", CitedRaw: "GB/T 17758"}
	out := ResolveOutcome{State: metricRefNeedsReview, TargetInputID: 338, Candidates: []ArtifactCandidate{{Score: 0.5}}}

	a := metricReferenceAttention(m, out)
	if a.ItemType != attention.TypeUnresolvedReference || a.InputRecordID != 377 || a.Title != "GB/T 17758" {
		t.Fatalf("report = %+v", a)
	}
	if !attention.KnownType(a.ItemType) {
		t.Fatalf("item type %q is not declared", a.ItemType)
	}
	m.RefID = "377_ref_999"
	if b := metricReferenceAttention(m, out); b.DedupeKey != a.DedupeKey {
		t.Fatalf("key changed with ref_id: %q vs %q", b.DedupeKey, a.DedupeKey)
	}
	out.TargetInputID = 339
	out.Candidates = nil
	out.Notes = []string{"no_located_set"}
	if c := metricReferenceAttention(m, out); c.DedupeKey != a.DedupeKey || c.ItemType != attention.TypeReferenceMetricNoMatch || c.Details["reason"] != "no_located_set" || c.Details["target_record_id"] != int64(339) {
		t.Fatalf("retargeted report = %+v", c)
	}
}
