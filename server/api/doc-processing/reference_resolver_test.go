package docprocessing

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/attention"
)

// An unresolved citation becomes an attention item whose key is stable across re-extraction
// (the mention's ref_id may change, its mention_key does not) and differs per target.
func TestUnresolvedReferenceReport(t *testing.T) {
	m := RefMention{InputRecordID: 377, RefID: "377_ref_177", MentionKey: "mk-1", CitedRaw: "GB/T 17758"}
	out := ResolveOutcome{State: "needs_review", Notes: []string{"no locator"}}

	a := unresolvedReferenceReport("metric", m, 338, out)
	if a.ItemType != attention.TypeUnresolvedReference || a.InputRecordID != 377 || a.Title != "GB/T 17758" {
		t.Fatalf("report = %+v", a)
	}
	if !attention.KnownType(a.ItemType) {
		t.Fatalf("item type %q is not declared", a.ItemType)
	}
	m.RefID = "377_ref_999"
	if b := unresolvedReferenceReport("metric", m, 338, out); b.DedupeKey != a.DedupeKey {
		t.Fatalf("key changed with ref_id: %q vs %q", b.DedupeKey, a.DedupeKey)
	}
	if c := unresolvedReferenceReport("metric", m, 339, out); c.DedupeKey == a.DedupeKey {
		t.Fatalf("different target shares key %q", c.DedupeKey)
	}
}
